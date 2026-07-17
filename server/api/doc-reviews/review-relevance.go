package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// relevanceReviewer checks whether all content in the document is relevant to
// its stated purpose and scope: off-topic tangents, diverging subsections,
// content that belongs in a different document, and filler passages that do not
// advance the document's purpose. P3 (Content Quality), StrategyChunk,
// one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Relevance is judged relative to the
// document type and purpose inferred from doc_context. This reviewer is
// complementary to the P3 completeness reviewer (missing content) and the P3
// conciseness reviewer (wordy content): relevance flags content that should not
// be present at all, not content that is incomplete or verbose.
type relevanceReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *relevanceReviewer) Name() string             { return "relevance" }
func (r *relevanceReviewer) Group() string            { return "P3" }
func (r *relevanceReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *relevanceReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062541) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062542) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062543) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062544) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("relevance review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document's
	// stated purpose and scope, which is the baseline for judging relevance.
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to judge whether a subsection is drifting
	// off-topic across a passage while remaining tractable for one-shot processing.
	const windowSize = 200
	windows := buildRelevanceWindows(lines, docCtx, windowSize)

	if len(windows) == 0 {
		return nil, nil
	}

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(windows), cfg, r.Name(), r.logger, recordID, cfg.OnProgress,
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.processWindow(workerCtx, recordID, i, cfg, windows[i]), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	var allFindings []ReviewFinding
	for _, wf := range results {
		allFindings = append(allFindings, wf...)
	}
	return allFindings, nil
}

// relevanceWindow holds the lines JSON and line range for one LLM call.
type relevanceWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildRelevanceWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildRelevanceWindows(lines []Line, docCtx string, size int) []relevanceWindow {
	var windows []relevanceWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, relevanceWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *relevanceReviewer) processBlock(ctx context.Context, recordID int64, index, total int, cfg ReviewerConfig, b pageBlock) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, relevanceWindow{inputJSON: b.inputJSON, startLine: b.lineStart, endLine: b.lineEnd})
}

func (r *relevanceReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w relevanceWindow,
) []ReviewFinding {
	r.logger.Info("relevance review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_relevance", "MID-CWB-REVIEW-RELEVANCE"))
	if err != nil {
		r.logger.Warn("relevance review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload, cfg.ModelName)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "relevance"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "irrelevance"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("relevance review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
