package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// crossReferenceCorrectnessReviewer checks that cross-references within a
// document are accurate and resolvable — that every "see §X", "as described
// in Chapter Y", "refer to Appendix Z", or "per section N" points to a real,
// reachable target with the referenced topic or content. P4 (Consistency),
// StrategyDocument, one-shot by default; when max_tool_turns > 0 in
// doc-review.local.toml and a tool client resolves, runs the tool-use
// conversation loop (DR10b) with the document-intrinsic core tools.
//
// Unlike the P1/P3 chunk reviewers, cross_reference_correctness requires
// reasoning across the whole document to resolve forward and backward
// references. Because a document may be too large for one LLM call, the
// reviewer splits it into page-aligned blocks of up to DefaultInputBlockSize
// pages and reviews each block concurrently. References whose target falls
// in an unseen block are reported at lower confidence.
type crossReferenceCorrectnessReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient // non-nil when tool-use path is active
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
	blockSize    int // pages per block; 0 = DefaultInputBlockSize
}

func (r *crossReferenceCorrectnessReviewer) Name() string             { return "cross_reference_correctness" }
func (r *crossReferenceCorrectnessReviewer) Group() string            { return "P4" }
func (r *crossReferenceCorrectnessReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *crossReferenceCorrectnessReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062621) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062622) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062623) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062624) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("cross_reference_correctness review skipped: no lines", "record_id", recordID)
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

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(blocks), cfg, r.Name(), r.logger, recordID, cfg.OnProgress,
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

func (r *crossReferenceCorrectnessReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("cross_reference_correctness review block start",
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
		// Tool-use path (DR10b). The reviewer can investigate by querying chunk
		// summaries and lines from other parts of the document to resolve whether
		// a cross-reference has a valid target.
		tools := selectTools(r.toolRegistry, cfg.Tools)
		r.logger.Info("cross_reference_correctness tool-use path active",
			"record_id", recordID,
			"block", fmt.Sprintf("%d/%d", index+1, total),
			"tools", len(tools),
		)
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", b.inputJSON, cfg.PromptText)
		callInfo := docReviewCallInfo(ctx, map[string]any{
			"block": fmt.Sprintf("%d/%d", index+1, total),
			"pages": fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
			"lines": fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
		})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_cross_reference_correctness", callInfo, "MID-20260706-002",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("cross_reference_correctness tool-use loop failed; no findings for block",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", loopErr,
			)
		}
		findings = loopFindings
	} else {
		// One-shot path.
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_cross_reference_correctness", "MID-CWB-REVIEW-CROSS-REF-CORRECTNESS"))
		if err != nil {
			r.logger.Warn("cross_reference_correctness review block failed; skipping",
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
		findings[i].Aspect = "cross_reference_correctness"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "broken_reference"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("cross_reference_correctness review block end",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
