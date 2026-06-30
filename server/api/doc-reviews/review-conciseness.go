package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// concisenessReviewer checks whether the document's content is free of
// unnecessary verbosity: redundant phrases, tautologies, padding expressions,
// excessive hedging where the document type demands directness, avoidable
// nominalizations, and content that is restated without adding new information.
// P3 (Content Quality), StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Conciseness is judged from the chunk
// text plus the document type inferred from doc_context. This reviewer is
// complementary to the P1 readability reviewer (which flags sentence length and
// passive voice) and the P3 clarity reviewer (which flags ambiguity of meaning).
// Conciseness flags whether *words or phrases can be removed or compressed*
// without losing meaning, not whether the text is readable or unambiguous.
type concisenessReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *concisenessReviewer) Name() string             { return "conciseness" }
func (r *concisenessReviewer) Group() string            { return "P3" }
func (r *concisenessReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *concisenessReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062531) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062532) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062533) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062534) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("conciseness review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer when
	// verbosity is genuinely inappropriate for the document type and audience.
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to detect repetition across a passage
	// (restated content, repeated caveats) while staying tractable for
	// one-shot processing.
	const windowSize = 200
	windows := buildConcisenessWindows(lines, docCtx, windowSize)

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

// concisenessWindow holds the lines JSON and line range for one LLM call.
type concisenessWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildConcisenessWindows splits lines into fixed-size windows, wrapping each
// in the doc_context envelope.
func buildConcisenessWindows(lines []Line, docCtx string, size int) []concisenessWindow {
	var windows []concisenessWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, concisenessWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *concisenessReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, concisenessWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *concisenessReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w concisenessWindow,
) []ReviewFinding {
	r.logger.Info("conciseness review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_conciseness", "MID-CWB-REVIEW-CONCISENESS"))
	if err != nil {
		r.logger.Warn("conciseness review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "conciseness"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "verbosity"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("conciseness review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
