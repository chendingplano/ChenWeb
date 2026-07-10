package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// testableClaimsReviewer checks whether the document's claims are stated in a
// way that can be verified or tested: requirements with measurable acceptance
// criteria, assertions with unambiguous pass/fail conditions, and performance
// or compliance claims backed by quantifiable thresholds. P3 (Content Quality),
// StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Testability is judged relative to the
// document type and audience inferred from doc_context. This reviewer is
// complementary to the P3 correctness reviewer (whether what is stated is right)
// and the P3 completeness reviewer (whether required content is present):
// testable_claims flags claims that are present but cannot be objectively
// verified because they are vague, subjective, or lack measurable criteria.
type testableClaimsReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *testableClaimsReviewer) Name() string             { return "testable_claims" }
func (r *testableClaimsReviewer) Group() string            { return "P3" }
func (r *testableClaimsReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *testableClaimsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062581) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062582) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062583) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062584) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("testable_claims review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document
	// type and audience, which determines what level of testability is expected
	// (strict acceptance criteria for specs, verifiable steps for SOPs, etc.).
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to judge whether a requirement or claim in
	// the passage is supported by a measurable threshold or acceptance criterion,
	// while remaining tractable for one-shot processing.
	const windowSize = 200
	windows := buildTestableClaimsWindows(lines, docCtx, windowSize)

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

// testableClaimsWindow holds the lines JSON and line range for one LLM call.
type testableClaimsWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildTestableClaimsWindows splits lines into fixed-size windows, wrapping
// each in the doc_context envelope.
func buildTestableClaimsWindows(lines []Line, docCtx string, size int) []testableClaimsWindow {
	var windows []testableClaimsWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, testableClaimsWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *testableClaimsReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, testableClaimsWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *testableClaimsReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w testableClaimsWindow,
) []ReviewFinding {
	r.logger.Info("testable_claims review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_testable_claims", "MID-CWB-REVIEW-TESTABLE-CLAIMS"))
	if err != nil {
		r.logger.Warn("testable_claims review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "testable_claims"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "untestable_claim"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("testable_claims review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
