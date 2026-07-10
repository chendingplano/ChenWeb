package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// clarityReviewer checks whether the document's content is unambiguous and
// easy to understand in meaning: vague or imprecise language, ambiguous
// statements that can be read two ways, unclear pronoun antecedents, undefined
// jargon or acronyms, and implicit assumptions left unstated. P3 (Content
// Quality), StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Clarity is judged from the chunk text
// plus the document type inferred from doc_context. This reviewer is
// complementary to the P1 readability reviewer (which flags surface-level
// writing quality — sentence length, passive voice, paragraph density) and to
// the P3 correctness reviewer (which flags whether stated content is right).
// Clarity flags whether the *meaning* is unambiguous, not whether the text
// reads smoothly.
type clarityReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *clarityReviewer) Name() string             { return "clarity" }
func (r *clarityReviewer) Group() string            { return "P3" }
func (r *clarityReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *clarityReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062521) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062522) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062523) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062524) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("clarity review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer what "clear"
	// means for the intended audience and document type.
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to catch an undefined acronym used across
	// a section while remaining tractable for one-shot processing.
	const windowSize = 200
	windows := buildClarityWindows(lines, docCtx, windowSize)

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

// clarityWindow holds the lines JSON and line range for one LLM call.
type clarityWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildClarityWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildClarityWindows(lines []Line, docCtx string, size int) []clarityWindow {
	var windows []clarityWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, clarityWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *clarityReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, clarityWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *clarityReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w clarityWindow,
) []ReviewFinding {
	r.logger.Info("clarity review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_clarity", "MID-CWB-REVIEW-CLARITY"))
	if err != nil {
		r.logger.Warn("clarity review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "clarity"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "ambiguity"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("clarity review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
