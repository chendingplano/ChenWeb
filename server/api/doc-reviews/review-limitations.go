package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// limitationsReviewer identifies undocumented or insufficiently described
// limitations of the system, method, or product described in the document:
// scope boundaries that are unstated, known constraints not disclosed, conditions
// under which the described approach fails, untested or unvalidated claims, and
// inapplicable use cases that an uninformed reader might attempt. P5 (Technical &
// Compliance), StrategyChunk; one-shot by default, tool-use when max_tool_turns > 0.
type limitationsReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
}

func (r *limitationsReviewer) Name() string             { return "limitations" }
func (r *limitationsReviewer) Group() string            { return "P5" }
func (r *limitationsReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *limitationsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062801) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062802) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062803) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062804) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("limitations review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	docCtx := buildDocContextLine(rec)

	const windowSize = 200
	windows := buildLimitationsWindows(lines, docCtx, windowSize)
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

type limitationsWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

func buildLimitationsWindows(lines []Line, docCtx string, size int) []limitationsWindow {
	var windows []limitationsWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)
		windows = append(windows, limitationsWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *limitationsReviewer) processBlock(ctx context.Context, recordID int64, index, total int, cfg ReviewerConfig, b pageBlock) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, limitationsWindow{inputJSON: b.inputJSON, startLine: b.lineStart, endLine: b.lineEnd})
}

func (r *limitationsReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w limitationsWindow,
) []ReviewFinding {
	r.logger.Info("limitations review window start",
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
			userCtx, tools, recordID, r.logger, "review_limitations", callInfo, "MID-20260706-009",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("limitations tool-use loop failed; no findings for window",
				"record_id", recordID, "window", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_limitations", "MID-CWB-REVIEW-LIMITATIONS"))
		if err != nil {
			r.logger.Warn("limitations review window failed; skipping",
				"record_id", recordID, "window", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "limitations"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "undocumented_limitation"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("limitations review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
