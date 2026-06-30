package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// examplesReviewer checks whether the document provides adequate, relevant, and
// well-formed examples where they would be expected: practical code snippets, use
// cases, worked scenarios, sample inputs/outputs, and illustrative cases that
// help the reader apply or verify the document's content. P3 (Content Quality),
// StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Adequacy is judged relative to the
// document type and audience inferred from doc_context. This reviewer is
// complementary to the P3 clarity reviewer (concepts stated but hard to follow)
// and the P3 completeness reviewer (required sections absent): examples flags
// passages where examples are missing, inadequate, misleading, or poorly formed,
// not where entire sections are absent or prose is unclear.
type examplesReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *examplesReviewer) Name() string             { return "examples" }
func (r *examplesReviewer) Group() string            { return "P3" }
func (r *examplesReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *examplesReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062561) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062562) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062563) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062564) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("examples review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document
	// type and audience, which determines what kinds of examples are expected
	// (code snippets for API references, worked scenarios for SOPs, etc.).
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to judge whether a concept or procedure
	// within the passage is supported by an example, while remaining tractable
	// for one-shot processing.
	const windowSize = 200
	windows := buildExamplesWindows(lines, docCtx, windowSize)

	if len(windows) == 0 {
		return nil, nil
	}

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(windows), cfg.OnProgress,
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

// examplesWindow holds the lines JSON and line range for one LLM call.
type examplesWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildExamplesWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildExamplesWindows(lines []Line, docCtx string, size int) []examplesWindow {
	var windows []examplesWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, examplesWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *examplesReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, examplesWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *examplesReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w examplesWindow,
) []ReviewFinding {
	r.logger.Info("examples review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_examples", "MID-CWB-REVIEW-EXAMPLES"))
	if err != nil {
		r.logger.Warn("examples review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "examples"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "insufficient_examples"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("examples review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
