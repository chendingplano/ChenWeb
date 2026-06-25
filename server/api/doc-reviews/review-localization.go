package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// localizationReviewer checks whether content is adapted to and consistent with
// the document's target locale: date/number/currency formats, measurement units,
// untranslated fragments, untranslatable idioms, culture-specific references,
// hard-coded locale assumptions, and encoding/typography issues. P1,
// StrategyChunk, one-shot (no tool use). Uses a cheap model.
type localizationReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks
	maxTasks   int
}

func (r *localizationReviewer) Name() string             { return "localization" }
func (r *localizationReviewer) Group() string            { return "P1" }
func (r *localizationReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *localizationReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062313) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062314) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062315) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062316) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("localization review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope. The reviewer infers the target
	// locale (language, region, audience) from this context.
	docCtx := buildDocContextLine(rec)

	// Split lines into windows for the LLM. Use a window of 200 lines per call —
	// localization assessment benefits from a wider context window so the reviewer
	// can spot inconsistent date/number/unit conventions used for the same
	// construct across a passage, while staying small enough for one-shot.
	const windowSize = 200
	windows := buildLocalizationWindows(lines, docCtx, windowSize)

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

// localizationWindow holds the lines JSON and line range for one LLM call.
type localizationWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildLocalizationWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope. A larger window (200 lines) helps the LLM detect
// inconsistent locale conventions (dates, numbers, units) across a passage.
func buildLocalizationWindows(lines []Line, docCtx string, size int) []localizationWindow {
	var windows []localizationWindow
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

		windows = append(windows, localizationWindow{
			inputJSON: jsonText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return windows
}

func (r *localizationReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w localizationWindow,
) []ReviewFinding {
	r.logger.Info("localization review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_localization", "MID-CWB-REVIEW-LOCALIZATION"))
	if err != nil {
		r.logger.Warn("localization review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P1"
		findings[i].Aspect = "localization"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "localization"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		// Enrich location with line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("localization review window end ",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
	)
	return findings
}
