package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// readabilityReviewer evaluates how easily a reader can follow the prose:
// sentence complexity, paragraph length, passive-voice / nominalization
// overuse, and undefined jargon. P1, StrategyChunk, one-shot (no tool use).
// Uses a cheap model.
type readabilityReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks
	maxTasks   int
}

func (r *readabilityReviewer) Name() string             { return "readability" }
func (r *readabilityReviewer) Group() string            { return "P1" }
func (r *readabilityReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *readabilityReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062309) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062310) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062311) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062312) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("readability review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope.
	docCtx := buildDocContextLine(rec)

	// Split lines into windows for the LLM. Use a window of 200 lines per call —
	// readability assessment benefits from a wider context window so the reviewer
	// can judge paragraph length and sentence flow across a passage, while staying
	// small enough for one-shot processing.
	const windowSize = 200
	windows := buildReadabilityWindows(lines, docCtx, windowSize)

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

// readabilityWindow holds the lines JSON and line range for one LLM call.
type readabilityWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildReadabilityWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope. A larger window (200 lines) helps the LLM judge
// paragraph length and sentence-level complexity across a passage.
func buildReadabilityWindows(lines []Line, docCtx string, size int) []readabilityWindow {
	var windows []readabilityWindow
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

		windows = append(windows, readabilityWindow{
			inputJSON: jsonText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return windows
}

func (r *readabilityReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, readabilityWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *readabilityReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w readabilityWindow,
) []ReviewFinding {
	r.logger.Info("readability review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_readability", "MID-CWB-REVIEW-READABILITY"))
	if err != nil {
		r.logger.Warn("readability review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P1"
		findings[i].Aspect = "readability"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "readability"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		// Enrich location with line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("readability review window end ",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
