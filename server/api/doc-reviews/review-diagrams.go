package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// diagramsReviewer checks whether the document uses diagrams effectively:
// whether expected diagrams are present, whether they match the surrounding
// text, and whether they are readable, labelled, and informative. P3 (Content
// Quality), StrategyChunk, one-shot (no tool use).
//
// Scope: Phase I — one-shot per window. Adequacy is judged relative to the
// document type and audience inferred from doc_context. This reviewer is
// complementary to the P3 examples reviewer (code / worked-scenario coverage)
// and the P3 completeness reviewer (required sections absent): diagrams flags
// passages where diagrams are missing, unclear, unlabelled, or mismatched, not
// where entire sections are absent or prose is unclear.
type diagramsReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore
	maxTasks   int
}

func (r *diagramsReviewer) Name() string             { return "diagrams" }
func (r *diagramsReviewer) Group() string            { return "P3" }
func (r *diagramsReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *diagramsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062571) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062572) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062573) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062574) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("diagrams review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document
	// type and audience, which determines what kinds of diagrams are expected
	// (architecture diagrams for design docs, flow charts for SOPs, etc.).
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to spot that a concept or process in the
	// passage lacks a supporting diagram, while remaining tractable for one-shot
	// processing.
	const windowSize = 200
	windows := buildDiagramsWindows(lines, docCtx, windowSize)

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

// diagramsWindow holds the lines JSON and line range for one LLM call.
type diagramsWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildDiagramsWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildDiagramsWindows(lines []Line, docCtx string, size int) []diagramsWindow {
	var windows []diagramsWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, diagramsWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *diagramsReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, diagramsWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *diagramsReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w diagramsWindow,
) []ReviewFinding {
	r.logger.Info("diagrams review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_diagrams", "MID-CWB-REVIEW-DIAGRAMS"))
	if err != nil {
		r.logger.Warn("diagrams review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "diagrams"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "diagram_issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("diagrams review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
