package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// completenessReviewer checks whether the document covers everything it should:
// missing topics/sections expected for the document type, sections that are
// started but left without required detail, placeholders (TODO/TBD), dangling
// references to content that is never provided, and truncated/unfinished
// passages. P3 (Content Quality), StrategyChunk, one-shot (no tool use).
//
// Phase I runs one-shot per window: completeness is judged from the chunk text
// plus the document type inferred from doc_context (DR6c — implicit expectation
// injection at the prompt level). The cross-document roster comparison against
// reference standards (DR6a/DR6b) and the tool-use investigation loop (DR10)
// are Phase II+ and out of scope here.
type completenessReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks
	maxTasks   int
}

func (r *completenessReviewer) Name() string             { return "completeness" }
func (r *completenessReviewer) Group() string            { return "P3" }
func (r *completenessReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *completenessReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062501) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062502) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062503) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062504) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("completeness review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope. doc_context lets the reviewer
	// infer the document type and the topics/sections such a document is
	// expected to cover (DR6c).
	docCtx := buildDocContextLine(rec)

	// Split lines into windows for the LLM. Use a window of 200 lines per call —
	// completeness assessment benefits from a wider context window so the
	// reviewer can tell whether a section that promises content actually
	// delivers it within the same passage, while staying small enough for
	// one-shot processing.
	const windowSize = 200
	windows := buildCompletenessWindows(lines, docCtx, windowSize)

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

// completenessWindow holds the lines JSON and line range for one LLM call.
type completenessWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildCompletenessWindows splits lines into fixed-size windows, wrapping each
// in the doc_context envelope. A larger window (200 lines) helps the LLM judge
// whether a section's promised content is actually delivered within the passage.
func buildCompletenessWindows(lines []Line, docCtx string, size int) []completenessWindow {
	var windows []completenessWindow
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

		windows = append(windows, completenessWindow{
			inputJSON: jsonText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return windows
}

func (r *completenessReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w completenessWindow,
) []ReviewFinding {
	r.logger.Info("completeness review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_completeness", "MID-CWB-REVIEW-COMPLETENESS"))
	if err != nil {
		r.logger.Warn("completeness review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "completeness"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "incompleteness"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		// Enrich location with line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("completeness review window end ",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
	)
	return findings
}
