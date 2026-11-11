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

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// This file implements the source passage and scheduling of the artifact
// reviewers (metrics, provisions, inventory_items, metrics_completeness):
// ADR 2026070201 AR2/AR3 as amended on 2026-11-10 (openspec change
// artifact-review-source-chunk).
//
//   - Source passage: every artifact review call carries the extraction chunk
//     that contains the artifact — the chunk the extractors read, from the
//     record's saved .chunks file — rendered compactly (RenderLinesCompact) and
//     placed FIRST in the user message.
//   - Scheduling: units are grouped by chunk and run seed → stagger →
//     remainder: one unit per chunk fires first, its siblings follow, so calls
//     sharing a chunk share a cacheable prefix. Sibling bursts are bounded by
//     the run-level semaphore (maxTasks) plus LLM_CALL_STAGGER.

// artifactSourceChunk is one extraction chunk prepared as an artifact review
// passage: its compact text and its line ranges. A chunk's lines are its
// overlap lines (the previous chunk's tail) followed by its regular lines;
// every document line is regular in exactly one chunk.
type artifactSourceChunk struct {
	text         string
	startLine    int // first line, overlap or regular
	endLine      int // last line
	regularStart int // first regular line; 0 when the chunk has none
	regularEnd   int // last regular line
}

// loadArtifactReviewChunks loads the document's lines and its saved extraction
// chunks and renders each chunk as a compact passage. When the record has no
// saved .chunks file the chunks are rebuilt with the configured chunk size and
// a warning is logged, because they may differ from what the extractors read.
func loadArtifactReviewChunks(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) ([]artifactSourceChunk, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070201) load record %d for artifact review chunks: %w", recordID, err)
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
	chunks, rebuilt, err := docprocessing.LoadRecordChunks(rec, lines)
	if err != nil {
		return nil, fmt.Errorf("(MID_26111001) load extraction chunks for record %d: %w", recordID, err)
	}
	if rebuilt && logger != nil {
		logger.Warn("artifact review: no saved .chunks file; rebuilt extraction chunks with the current chunk size",
			"record_id", recordID, "chunks", len(chunks))
	}
	return buildArtifactSourceChunks(chunks, buildDocContextLine(rec)), nil
}

// buildArtifactSourceChunks renders chunks as compact passages with their line
// ranges. Chunks without lines are kept (empty) so indexes match the chunk list.
func buildArtifactSourceChunks(chunks []docprocessing.Chunk, docCtx string) []artifactSourceChunk {
	out := make([]artifactSourceChunk, len(chunks))
	for i, c := range chunks {
		sc := artifactSourceChunk{text: docprocessing.RenderLinesCompact(c.Lines, docCtx)}
		for _, ml := range c.Lines {
			n := ml.Line.LineNo
			if sc.startLine == 0 || n < sc.startLine {
				sc.startLine = n
			}
			if n > sc.endLine {
				sc.endLine = n
			}
			if strings.EqualFold(strings.TrimSpace(ml.Mark), "o") {
				continue
			}
			if sc.regularStart == 0 || n < sc.regularStart {
				sc.regularStart = n
			}
			if n > sc.regularEnd {
				sc.regularEnd = n
			}
		}
		out[i] = sc
	}
	return out
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

// chunkForSpans picks the extraction chunk for an artifact whose spans run from
// line s (earliest start) to line e (latest end):
//
//  1. the chunk whose regular lines contain s and whose range contains e;
//  2. else a chunk whose range contains s..e (the artifact sits in the overlap
//     of the next chunk, the chunk that could have produced it);
//  3. else the chunk whose regular lines contain s, with truncated = true (the
//     spans run past it; the payload flags this so the model does not misread
//     the cut-off as an extraction error).
//
// Returns -1 when no span parses or no chunk contains s.
func chunkForSpans(spans []string, chunks []artifactSourceChunk) (idx int, truncated bool) {
	s, e := 0, 0
	for _, sp := range spans {
		start, end := parseArtifactSpan(sp)
		if start == 0 {
			continue
		}
		if s == 0 || start < s {
			s = start
		}
		if end > e {
			e = end
		}
	}
	if s == 0 {
		return -1, false
	}
	regularHas := func(c artifactSourceChunk, n int) bool {
		return c.regularStart > 0 && n >= c.regularStart && n <= c.regularEnd
	}
	for i, c := range chunks {
		if regularHas(c, s) && e <= c.endLine {
			return i, false
		}
	}
	for i, c := range chunks {
		if c.startLine > 0 && s >= c.startLine && e <= c.endLine {
			return i, false
		}
	}
	for i, c := range chunks {
		if regularHas(c, s) {
			return i, true
		}
	}
	return -1, false
}

// sourceChunkFor resolves an artifact's spans to its chunk index, compact
// passage and truncation flag; passage is "" when no chunk resolves.
func sourceChunkFor(spans []string, chunks []artifactSourceChunk) (idx int, passage string, truncated bool) {
	idx, truncated = chunkForSpans(spans, chunks)
	if idx < 0 {
		return -1, "", false
	}
	return idx, chunks[idx].text, truncated
}

// artifactReviewTaskText appends the unique per-artifact payload after the
// stable reviewer rubric. With the source chunk as the document input, the
// composed user message is: chunk (shared by the artifacts in it) → rubric →
// payload (the only tokens unique to the call).
func artifactReviewTaskText(rubric, payloadJSON string) string {
	return rubric + "\n\n# ARTIFACT REVIEW INPUT\n\n" + payloadJSON
}

// artifactReviewToolUserContext composes the tool-use loop's user message:
// the source passage in <DOCUMENT_INPUT>, then the task. When no source chunk
// resolved the task text is sent alone.
func artifactReviewToolUserContext(passage, taskText string) string {
	if passage == "" {
		return "<REVIEW_TASK>\n" + taskText + "\n</REVIEW_TASK>"
	}
	return "<DOCUMENT_INPUT>\n" + passage + "\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n" + taskText + "\n</REVIEW_TASK>"
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

// artifactReviewUnit is one artifact review LLM call, tagged with the source
// chunk whose prefix it shares. chunkIdx -1 means no chunk resolved; such
// units have unique prefixes and run in the remainder phase only.
type artifactReviewUnit struct {
	chunkIdx int
	run      func(ctx context.Context) ([]ReviewFinding, error)
}

// runArtifactUnitsChunkGrouped executes artifact review units with the
// chunk-grouped seed → stagger → remainder pattern:
//
//  1. For each chunk, the first unit fires concurrently to plant (or confirm)
//     the chunk's prefix in DeepSeek's cache. Seeds keep running.
//  2. Wait LLM_CALL_STAGGER for the prefixes to persist.
//  3. All remaining units fire, bounded by maxTasks; there is deliberately no
//     per-chunk group cap (the cache is keyed by prefix, not by group).
//
// Failed units return their error so an incomplete assessment cannot appear successful.
func runArtifactUnitsChunkGrouped(
	ctx context.Context,
	maxTasks int,
	units []artifactReviewUnit,
	cfg ReviewerConfig,
	reviewer string,
	logger ApiTypes.JimoLogger,
	recordID int64,
	onProgress ReviewerProgressFunc,
) ([]ReviewFinding, error) {
	if ctx.Err() != nil {
		if logger != nil {
			logger.Error("reviewer aborted before units started", "reviewer", reviewer, "record_id", recordID, "error", context.Cause(ctx))
		}
		return nil, context.Cause(ctx)
	}
	if len(units) == 0 {
		return nil, nil
	}
	tracker := newReviewerProgressTracker(len(units), onProgress)

	// Partition into seeds (first unit per chunk) and remainder.
	seen := make(map[int]bool)
	var seedIdxs, remainIdxs []int
	for i, u := range units {
		if u.chunkIdx >= 0 && !seen[u.chunkIdx] {
			seen[u.chunkIdx] = true
			seedIdxs = append(seedIdxs, i)
		} else {
			remainIdxs = append(remainIdxs, i)
		}
	}
	gate := newReviewWorkGate(cfg.MaxFindings, cfg.MaxAnalyses, seedIdxs)

	results := make([][]ReviewFinding, len(units))
	var wg sync.WaitGroup
	workerCtx, stopWorkers := context.WithCancelCause(ctx)
	defer stopWorkers(nil)

	var (
		errMu    sync.Mutex
		firstErr error
	)
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr != nil {
			return
		}
		firstErr = err
		if logger != nil {
			logger.Error("reviewer aborted after unit failure", "reviewer", reviewer, "record_id", recordID, "error", err)
		}
		stopWorkers(err)
	}

	runUnit := func(idx int) bool {
		if workerCtx.Err() != nil {
			return false
		}
		findings, err := units[idx].run(workerCtx)
		results[idx] = findings
		if err != nil {
			recordErr(fmt.Errorf("%s unit %d: %w", reviewer, idx, err))
			return false
		}
		if workerCtx.Err() != nil {
			recordErr(context.Cause(workerCtx))
			return false
		}
		gate.complete(results[idx])
		if err := tracker.add(len(results[idx])); err != nil {
			recordErr(fmt.Errorf("persist %s progress: %w", reviewer, err))
			return false
		}
		return true
	}

	claimedSeeds := make([]int, 0, len(seedIdxs))
	for range seedIdxs {
		idx, ok := gate.claimNext()
		if !ok {
			break
		}
		claimedSeeds = append(claimedSeeds, idx)
	}

	// Phase 1: fire all chunk seeds concurrently; do NOT wait for completion.
	for _, i := range claimedSeeds {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			runUnit(idx)
		}(i)
	}

	if len(remainIdxs) > 0 {
		// Phase 2: wait LLM_CALL_STAGGER for the cached prefixes to persist.
		if stagger := reviewCallStagger(); stagger > 0 && len(claimedSeeds) > 0 {
			select {
			case <-time.After(stagger):
			case <-workerCtx.Done():
			}
		}

		// Phase 3: fire the remaining units with at most maxTasks claimers.
		gate.replaceQueue(remainIdxs)
		workerCount := min(maxTasks, len(remainIdxs))
		if workerCount < 1 {
			workerCount = 1
		}
		for range workerCount {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					if workerCtx.Err() != nil {
						return
					}
					idx, ok := gate.claimNext()
					if !ok {
						return
					}
					if !runUnit(idx) {
						return
					}
				}
			}()
		}
	}

	wg.Wait()

	if ctx.Err() != nil {
		recordErr(context.Cause(ctx))
		return nil, context.Cause(ctx)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	skipped := gate.unclaimedIndexes(len(units))
	logOutputLimitWarning(logger, reviewer, cfg, recordID, gate.snapshot(), skipped)
	if len(skipped) > 0 {
		recordErr(fmt.Errorf("%w: %s has %d unassessed units", ErrReviewOutputLimit, reviewer, len(skipped)))
		return nil, firstErr
	}

	var all []ReviewFinding
	for _, fs := range results {
		all = append(all, fs...)
	}
	return all, nil
}

// marshalArtifactPayload serializes an artifact review payload map to JSON,
// returning "" on failure (callers log and abort the reviewer).
func marshalArtifactPayload(payload map[string]any) string {
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}
