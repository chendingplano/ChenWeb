package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// currencyReviewer checks whether the document's content is up-to-date:
// deprecated APIs or libraries, superseded or withdrawn standards cited as
// current, obsolete product versions or model numbers, stale date references
// (past deadlines presented as future, expired certifications), out-of-date
// regulatory requirements, and replaced technologies. P3 (Content Quality),
// StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Currency is judged relative to the
// document type and domain inferred from doc_context. This reviewer is
// complementary to the P3 correctness reviewer (wrong facts) and the P3
// completeness reviewer (missing content): currency flags content that was
// once correct but is now stale or superseded, not content that is logically
// incorrect or absent.
type currencyReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *currencyReviewer) Name() string             { return "currency" }
func (r *currencyReviewer) Group() string            { return "P3" }
func (r *currencyReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *currencyReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062551) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062552) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062553) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062554) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("currency review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document
	// type and domain, which is the baseline for judging whether cited standards,
	// versions, or dates are still current.
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to catch inconsistent version references
	// across a passage while remaining tractable for one-shot processing.
	const windowSize = 200
	windows := buildCurrencyWindows(lines, docCtx, windowSize)

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

// currencyWindow holds the lines JSON and line range for one LLM call.
type currencyWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildCurrencyWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildCurrencyWindows(lines []Line, docCtx string, size int) []currencyWindow {
	var windows []currencyWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, currencyWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *currencyReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w currencyWindow,
) []ReviewFinding {
	r.logger.Info("currency review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_currency", "MID-CWB-REVIEW-CURRENCY"))
	if err != nil {
		r.logger.Warn("currency review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "currency"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "outdated"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("currency review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
