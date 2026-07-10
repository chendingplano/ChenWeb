package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// evidenceRationaleReviewer checks whether the document provides evidence and
// rationale for design decisions, claims, and recommendations — benchmarks,
// experiments, references, risk assessments, or reasoned justifications that
// allow a reviewer to evaluate *why* a decision was made, not only *what* was
// decided. P3 (Content Quality), StrategyChunk, one-shot by default;
// when max_tool_turns > 0 in doc-review.local.toml and a tool client resolves,
// runs the Phase II tool-use conversation loop (DR10b) with the document-
// intrinsic core tools.
//
// Scope: Phase II — tool-use path lets the reviewer chase down design
// decisions by querying the document's entities, metrics, provisions, and
// chunk summaries before producing findings, rather than relying solely on
// what is visible within a 200-line window. One-shot path unchanged.
type evidenceRationaleReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient // non-nil when tool-use path is active
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
}

func (r *evidenceRationaleReviewer) Name() string             { return "evidence_rationale" }
func (r *evidenceRationaleReviewer) Group() string            { return "P3" }
func (r *evidenceRationaleReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *evidenceRationaleReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062591) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062592) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062593) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062594) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("evidence_rationale review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context. doc_context lets the reviewer infer the document
	// type and audience, which determines what level of rationale is expected
	// (explicit design trade-off justification for ADRs, risk-based rationale
	// for regulatory submissions, benchmark results for performance specs, etc.).
	docCtx := buildDocContextLine(rec)

	// 200-line windows: wide enough to judge whether a decision or recommendation
	// in the passage is supported by evidence or reasoned justification within
	// the same section, while remaining tractable for one-shot processing.
	const windowSize = 200
	windows := buildEvidenceRationaleWindows(lines, docCtx, windowSize)

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

// evidenceRationaleWindow holds the lines JSON and line range for one LLM call.
type evidenceRationaleWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildEvidenceRationaleWindows splits lines into fixed-size windows, wrapping
// each in the doc_context envelope.
func buildEvidenceRationaleWindows(lines []Line, docCtx string, size int) []evidenceRationaleWindow {
	var windows []evidenceRationaleWindow
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		windows = append(windows, evidenceRationaleWindow{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return windows
}

func (r *evidenceRationaleReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	return r.processWindow(ctx, recordID, index, cfg, evidenceRationaleWindow{inputJSON: input.inputJSON, startLine: input.startLine, endLine: input.endLine})
}

func (r *evidenceRationaleReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w evidenceRationaleWindow,
) []ReviewFinding {
	r.logger.Info("evidence_rationale review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
		"max_tool_turns", cfg.MaxToolTurns,
	)

	startTime := time.Now()
	var findings []ReviewFinding
	var cacheHitTokens int
	var cacheMissTokens int

	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		// Tool-use path (Phase II, DR10b). The reviewer can investigate
		// by querying the document's entities, metrics, provisions, and
		// chunk summaries before producing findings.
		tools := selectTools(r.toolRegistry, cfg.Tools)
		r.logger.Info("evidence_rationale tool-use path active",
			"record_id", recordID,
			"window", index,
			"tools", len(tools),
		)
		// DR8a layout: canonical doc input before reviewer task.
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", w.inputJSON, cfg.PromptText)
		callInfo := docReviewCallInfo(ctx, map[string]any{
			"window": index,
			"lines":  fmt.Sprintf("%d-%d", w.startLine, w.endLine),
		})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_evidence_rationale", callInfo, "MID-20260706-004",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("evidence_rationale tool-use loop failed; no findings for window",
				"record_id", recordID,
				"window", index,
				"error", loopErr,
			)
		}
		findings = loopFindings
	} else {
		// One-shot path (unchanged Phase I behavior).
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, w.inputJSON, "review_evidence_rationale", "MID-CWB-REVIEW-EVIDENCE-RATIONALE"))
		if err != nil {
			r.logger.Warn("evidence_rationale review window failed; skipping",
				"record_id", recordID,
				"window", index,
				"error", err,
			)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	// Both paths funnel through the same finding-defaulting.
	for i := range findings {
		findings[i].Pass = "P3"
		findings[i].Aspect = "evidence_rationale"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "missing_evidence"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("evidence_rationale review window end",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
