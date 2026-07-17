package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// correctnessReviewer checks whether the content the document *does* state is
// factually and internally accurate: wrong values/units/calculations, claims
// that contradict each other within the passage, misstated definitions,
// mislabeled references, and assertions that are internally inconsistent or
// self-contradictory. P3 (Content Quality), StrategyChunk, one-shot (no tool
// use).
//
// Phase I runs one-shot per window: correctness is judged from the chunk text
// plus the document type inferred from doc_context. The cross-document
// verification against reference standards (DR4/DR6) and the tool-use
// investigation loop (DR10) — where the LLM chases a suspect claim into related
// entities/metrics/provisions — are Phase II+ and out of scope here. This pass
// flags what is *checkable from the passage itself*; claims that require
// external ground truth are reported at low confidence.
type correctnessReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks
	maxTasks   int
}

func (r *correctnessReviewer) Name() string             { return "correctness" }
func (r *correctnessReviewer) Group() string            { return "P3" }
func (r *correctnessReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *correctnessReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062511) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062512) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062513) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062514) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("correctness review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope. doc_context lets the reviewer
	// infer the document type and what "correct" means for content of this kind.
	docCtx := buildDocContextLine(rec)

	// Split lines into windows for the LLM. Use a window of 200 lines per call —
	// correctness assessment benefits from a wider context window so the reviewer
	// can catch a value or claim that contradicts another statement made earlier
	// in the same passage, while staying small enough for one-shot processing.
	const windowSize = 200
	windows := buildCorrectnessWindows(lines, docCtx, windowSize)

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

// correctnessWindow holds the lines JSON and line range for one LLM call.
type correctnessWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildCorrectnessWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope. A larger window (200 lines) helps the LLM catch a
// value or claim that contradicts another statement made within the passage.
func buildCorrectnessWindows(lines []Line, docCtx string, size int) []correctnessWindow {
	var windows []correctnessWindow
	for i := 0; i < len(lines); i += size {
		end := i + size
		if end > len(lines) {
			end = len(lines)
		}
		slice := lines[i:end]
		objs := rawLinesToJSON(slice) // from input_lines.go
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		startLine := slice[0].LineNo
		endLine := slice[len(slice)-1].LineNo

		windows = append(windows, correctnessWindow{
			inputJSON: jsonText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return windows
}

func (r *correctnessReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, correctnessWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *correctnessReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w correctnessWindow,
) []ReviewFinding {
	r.logger.Info("correctness review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_correctness", "MID-CWB-REVIEW-CORRECTNESS"))
	if err != nil {
		r.logger.Warn("correctness review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload, cfg.ModelName)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "correctness"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "incorrectness"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		// Enrich location with line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("correctness review window end ",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
