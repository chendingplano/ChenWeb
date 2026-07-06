package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// technicalAccuracyReviewer checks whether the technical content in the document
// is accurate: code snippets, formulas, API signatures, configuration values, CLI
// commands, algorithmic descriptions, and numeric/unit claims that can be verified
// from the passage itself or against domain knowledge. P5 (Technical & Compliance),
// StrategyChunk; one-shot by default, tool-use when max_tool_turns > 0.
type technicalAccuracyReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
}

func (r *technicalAccuracyReviewer) Name() string             { return "technical_accuracy" }
func (r *technicalAccuracyReviewer) Group() string            { return "P5" }
func (r *technicalAccuracyReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *technicalAccuracyReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062701) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062702) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062703) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062704) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("technical_accuracy review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to catch a value or command that contradicts
	// a parameter defined a few lines earlier in the same passage.
	const windowSize = 200
	windows := buildTechnicalAccuracyWindows(lines, docCtx, windowSize)
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

type technicalAccuracyWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

func buildTechnicalAccuracyWindows(lines []Line, docCtx string, size int) []technicalAccuracyWindow {
	var windows []technicalAccuracyWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)
		windows = append(windows, technicalAccuracyWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *technicalAccuracyReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, technicalAccuracyWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *technicalAccuracyReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w technicalAccuracyWindow,
) []ReviewFinding {
	r.logger.Info("technical_accuracy review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
		"max_tool_turns", cfg.MaxToolTurns,
	)

	startTime := time.Now()
	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int

	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", w.inputJSON, cfg.PromptText)
		callInfo := docReviewCallInfo(ctx, map[string]any{
			"window": index,
			"lines":  fmt.Sprintf("%d-%d", w.startLine, w.endLine),
		})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_technical_accuracy", callInfo, "MID-20260706-021",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("technical_accuracy tool-use loop failed; no findings for window",
				"record_id", recordID, "window", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_technical_accuracy", "MID-CWB-REVIEW-TECHNICAL-ACCURACY"))
		if err != nil {
			r.logger.Warn("technical_accuracy review window failed; skipping",
				"record_id", recordID, "window", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "technical_accuracy"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "technical_error"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("technical_accuracy review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
