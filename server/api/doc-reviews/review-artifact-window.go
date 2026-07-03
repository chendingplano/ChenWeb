package docreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// This file implements ADR 2026070201 AR2/AR3 for the artifact reviewers
// (metrics, provisions, inventory_items):
//
//   - AR2: every artifact review call includes the canonical 200-line scheduler
//     window containing the artifact's span start, placed FIRST in the user
//     message so calls sharing a window share a DeepSeek-cacheable prefix. The
//     window is byte-identical to buildChunkInputs output, so artifact calls
//     ride on prefixes the per-chunk reviewers already warmed in the same run.
//   - AR3: artifact review units are grouped by window and executed with the
//     scheduler's seed → stagger → remainder pattern: one unit per window fires
//     first to plant (or confirm) the prefix, the window's siblings follow.
//     There is no per-window group cap; sibling bursts are bounded by the
//     run-level semaphore (maxTasks) plus LLM_CALL_STAGGER.

// loadArtifactReviewWindows loads the document's lines and builds the canonical
// scheduler windows for it. It uses the exact code path the prompt-cache
// scheduler uses (same lines, same doc_context envelope, same serialization),
// so the returned windows are byte-identical to the per-chunk reviewers'
// inputs for the same document.
func loadArtifactReviewWindows(ctx context.Context, recordID int64) ([]chunkInput, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070201) load record %d for artifact windows: %w", recordID, err)
	}
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename, rec.ParserName, rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070202) resolve line file for record %d: %w", recordID, err)
	}
	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070203) read line file %s: %w", lineFilePath, err)
	}
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070204) parse line file for record %d: %w", recordID, err)
	}
	docCtx := buildDocContextLine(rec)
	return buildChunkInputs(lines, docCtx, DefaultChunkInputSize), nil
}

// parseArtifactSpan parses one line-span string ("14", "14-16", or "14:16")
// into (start, end). Returns (0, 0) on parse failure.
func parseArtifactSpan(s string) (start, end int) {
	s = strings.TrimSpace(s)
	sep := strings.IndexAny(s, "-:")
	if sep > 0 {
		a, err1 := strconv.Atoi(strings.TrimSpace(s[:sep]))
		b, err2 := strconv.Atoi(strings.TrimSpace(s[sep+1:]))
		if err1 == nil && err2 == nil && a > 0 && b >= a {
			return a, b
		}
		return 0, 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, 0
	}
	return n, n
}

// windowIndexForSpans maps an artifact's line spans to the window containing
// the earliest span start (AR2). Spans crossing a window boundary map to the
// window containing the span start; windows are never concatenated (a
// concatenation is a new, uncacheable prefix). Returns -1 when no span parses
// or no window contains the start line.
func windowIndexForSpans(spans []string, windows []chunkInput) int {
	if len(windows) == 0 {
		return -1
	}
	minStart := 0
	for _, s := range spans {
		start, _ := parseArtifactSpan(s)
		if start > 0 && (minStart == 0 || start < minStart) {
			minStart = start
		}
	}
	if minStart == 0 {
		return -1
	}
	for i, w := range windows {
		if minStart >= w.startLine && minStart <= w.endLine {
			return i
		}
	}
	return -1
}

// spansTruncatedByWindow reports whether any of the artifact's spans extend
// outside the window (AR2 multi-window mitigation: the payload flags the
// truncation so the model does not misread it as an extraction error).
func spansTruncatedByWindow(spans []string, w chunkInput) bool {
	for _, s := range spans {
		start, end := parseArtifactSpan(s)
		if start == 0 {
			continue
		}
		if start < w.startLine || end > w.endLine {
			return true
		}
	}
	return false
}

// artifactReviewTaskText appends the unique per-artifact payload after the
// stable reviewer rubric. With the window as the document input, the composed
// user message is: window (cached across reviewers) → rubric (cached within
// reviewer+window) → payload (the only regularly-paid tokens).
func artifactReviewTaskText(rubric, payloadJSON string) string {
	return rubric + "\n\n# ARTIFACT REVIEW INPUT\n\n" + payloadJSON
}

// artifactReviewToolUserContext composes the tool-use loop's user message with
// the AR2 window-first layout, mirroring the per-chunk tool-use reviewers'
// envelope so the <DOCUMENT_INPUT> prefix is byte-identical and cacheable.
// When no window is resolvable the task text is sent alone.
func artifactReviewToolUserContext(windowJSON, taskText string) string {
	if windowJSON == "" {
		return "<REVIEW_TASK>\n" + taskText + "\n</REVIEW_TASK>"
	}
	return "<DOCUMENT_INPUT>\n" + windowJSON + "\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n" + taskText + "\n</REVIEW_TASK>"
}

// docAuthorityRegex matches document numbers of governing standards bodies
// (GB/T 12237, ISO 9001, IEC 60079, ASTM A106, DIN EN 10204, ...).
var docAuthorityRegex = regexp.MustCompile(`(?i)\b(GB/?T?|ISO|IEC|ANSI|ASTM|ASME|DIN|JIS|BS|EN|API|NFPA|UL)[\s_/-]*\d`)

// docAuthorityClass classifies a matched document for severity weighting
// (AR5 §3): "standard" (governing national/international standard),
// "regulation" (law/regulation), or "peer_document". Heuristic over
// doc_no/title/filename until a curated per-document authority rank exists.
func docAuthorityClass(docNo, title, filename string) string {
	s := docNo + " " + title + " " + filename
	if docAuthorityRegex.MatchString(s) || strings.Contains(s, "标准") {
		return "standard"
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "regulation") || strings.Contains(lower, "directive") ||
		strings.Contains(s, "条例") || strings.Contains(s, "法规") || strings.Contains(s, "办法") ||
		strings.Contains(s, "规章") || strings.HasSuffix(strings.TrimSpace(title), "法") {
		return "regulation"
	}
	return "peer_document"
}

// artifactReviewUnit is one artifact review LLM call, tagged with the AR2
// window whose prefix it shares. windowIdx -1 means no window was resolvable;
// such units have unique prefixes and run in the remainder phase only.
type artifactReviewUnit struct {
	windowIdx int
	run       func(ctx context.Context) []ReviewFinding
}

// runArtifactUnitsWindowGrouped executes artifact review units with the AR3
// window-grouped seed → stagger → remainder pattern:
//
//  1. For each window, the first unit fires concurrently to plant (or confirm)
//     the window's prefix in DeepSeek's cache. Seeds keep running.
//  2. Wait LLM_CALL_STAGGER for the prefixes to persist.
//  3. All remaining units fire, bounded by maxTasks; there is deliberately no
//     per-window group cap (the cache is keyed by prefix, not by group).
//
// Unit run functions handle their own LLM errors (returning nil findings), so
// the only error returned here is ErrPipelineStopped.
func runArtifactUnitsWindowGrouped(
	ctx context.Context,
	maxTasks int,
	units []artifactReviewUnit,
	onProgress ReviewerProgressFunc,
) ([]ReviewFinding, error) {
	if len(units) == 0 {
		return nil, nil
	}
	tracker := newReviewerProgressTracker(len(units), onProgress)

	// Partition into seeds (first unit per window) and remainder.
	seen := make(map[int]bool)
	var seedIdxs, remainIdxs []int
	for i, u := range units {
		if u.windowIdx >= 0 && !seen[u.windowIdx] {
			seen[u.windowIdx] = true
			seedIdxs = append(seedIdxs, i)
		} else {
			remainIdxs = append(remainIdxs, i)
		}
	}

	results := make([][]ReviewFinding, len(units))
	stopped := make([]bool, len(units))
	var wg sync.WaitGroup

	runUnit := func(idx int) {
		if isCtxStopped(ctx) {
			stopped[idx] = true
			return
		}
		results[idx] = units[idx].run(ctx)
		tracker.add(len(results[idx]))
	}

	// Phase 1: fire all window seeds concurrently; do NOT wait for completion.
	for _, i := range seedIdxs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			runUnit(idx)
		}(i)
	}

	if len(remainIdxs) > 0 {
		// Phase 2: wait LLM_CALL_STAGGER for the cached prefixes to persist.
		if stagger := reviewCallStagger(); stagger > 0 && len(seedIdxs) > 0 {
			select {
			case <-time.After(stagger):
			case <-ctx.Done():
			}
		}

		// Phase 3: fire the remaining units bounded by maxTasks.
		sem := make(chan struct{}, max(maxTasks, 1))
		for _, i := range remainIdxs {
			if isCtxStopped(ctx) {
				stopped[i] = true
				continue
			}
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					stopped[idx] = true
					return
				}
				defer func() { <-sem }()
				runUnit(idx)
			}(i)
		}
	}

	wg.Wait()

	var all []ReviewFinding
	for _, fs := range results {
		all = append(all, fs...)
	}
	for _, s := range stopped {
		if s {
			return all, ErrPipelineStopped
		}
	}
	return all, nil
}

// marshalArtifactPayload serializes an artifact review payload map to JSON,
// returning "" on failure (callers log and skip the unit).
func marshalArtifactPayload(payload map[string]any) string {
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}
