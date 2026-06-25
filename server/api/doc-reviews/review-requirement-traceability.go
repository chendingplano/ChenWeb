package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// requirementTraceabilityReviewer checks that requirements, provisions, and
// obligations stated in the document are traceable — each has a clear source,
// owner, or rationale, and can be linked to a corresponding implementation,
// test, or verification criterion. P4 (Consistency), StrategyDocument,
// one-shot by default; when max_tool_turns > 0 in doc-review.local.toml
// and a tool client resolves, runs the tool-use conversation loop (DR10b)
// with the document-intrinsic core tools.
//
// Unlike the P1/P3 chunk reviewers, requirement_traceability reasons across
// the whole document to trace each requirement to its verification. Because
// a document may be too large for one LLM call, the reviewer splits it into
// page-aligned blocks of up to DefaultInputBlockSize pages and reviews each
// block concurrently. Cross-block traceability gaps may be missed; the prompt
// instructs the model to lower confidence when a requirement's downstream
// verification may fall in an unseen block.
type requirementTraceabilityReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient // non-nil when tool-use path is active
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
	blockSize    int // pages per block; 0 = DefaultInputBlockSize
}

func (r *requirementTraceabilityReviewer) Name() string             { return "requirement_traceability" }
func (r *requirementTraceabilityReviewer) Group() string            { return "P4" }
func (r *requirementTraceabilityReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *requirementTraceabilityReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062631) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062632) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062633) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062634) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("requirement_traceability review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	docCtx := buildDocContextLine(rec)

	blockSize := r.blockSize
	if blockSize <= 0 {
		blockSize = DefaultInputBlockSize
	}
	blocks := buildPageBlocks(lines, docCtx, blockSize)
	if len(blocks) == 0 {
		return nil, nil
	}

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(blocks), cfg.OnProgress,
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.processBlock(workerCtx, recordID, i, len(blocks), cfg, blocks[i]), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	var allFindings []ReviewFinding
	for _, bf := range results {
		allFindings = append(allFindings, bf...)
	}
	return allFindings, nil
}

func (r *requirementTraceabilityReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("requirement_traceability review block start",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"pages", fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
		"lines", fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
		"max_tool_turns", cfg.MaxToolTurns,
	)

	startTime := time.Now()
	var findings []ReviewFinding
	var cacheHitTokens int
	var cacheMissTokens int

	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		// Tool-use path (DR10b). The reviewer can investigate by querying
		// provisions, entities, and chunk summaries to determine whether a
		// stated requirement has a corresponding verification or test criterion
		// elsewhere in the document.
		tools := selectTools(r.toolRegistry, cfg.Tools)
		r.logger.Info("requirement_traceability tool-use path active",
			"record_id", recordID,
			"block", fmt.Sprintf("%d/%d", index+1, total),
			"tools", len(tools),
		)
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", b.inputJSON, cfg.PromptText)
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger,
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("requirement_traceability tool-use loop failed; no findings for block",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", loopErr,
			)
		}
		findings = loopFindings
	} else {
		// One-shot path.
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_requirement_traceability", "MID-CWB-REVIEW-REQ-TRACEABILITY"))
		if err != nil {
			r.logger.Warn("requirement_traceability review block failed; skipping",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", err,
			)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P4"
		findings[i].Aspect = "requirement_traceability"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "untraceable"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("requirement_traceability review block end",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
