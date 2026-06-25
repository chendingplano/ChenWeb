package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// internalContradictionsReviewer checks for contradictory statements,
// conflicting claims, and inconsistencies across a document. P4
// (Consistency), StrategyDocument, one-shot by default; when
// max_tool_turns > 0 in doc-review.local.toml and a tool client resolves,
// runs the Phase II tool-use conversation loop (DR10b) with the document-
// intrinsic core tools.
//
// Unlike the P1/P3 chunk reviewers, internal_contradictions reasons across
// the whole document to find passages that disagree with one another. Because
// a document may be too large for one LLM call, the reviewer splits it into
// page-aligned blocks of up to DefaultInputBlockSize pages and reviews each
// block concurrently. Cross-block contradictions may be missed; the prompt
// instructs the model to lower confidence when the apparent contradiction
// may involve content outside the block.
type internalContradictionsReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient // non-nil when tool-use path is active
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
	blockSize    int // pages per block; 0 = DefaultInputBlockSize
}

func (r *internalContradictionsReviewer) Name() string             { return "internal_contradictions" }
func (r *internalContradictionsReviewer) Group() string            { return "P4" }
func (r *internalContradictionsReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *internalContradictionsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062601) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062602) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062603) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062604) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("internal_contradictions review skipped: no lines", "record_id", recordID)
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

func (r *internalContradictionsReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("internal_contradictions review block start",
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
		// Tool-use path (DR10b). The reviewer can investigate by querying entities,
		// metrics, provisions, and chunk summaries to verify whether an apparent
		// contradiction is real or resolvable.
		tools := selectTools(r.toolRegistry, cfg.Tools)
		r.logger.Info("internal_contradictions tool-use path active",
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
			r.logger.Warn("internal_contradictions tool-use loop failed; no findings for block",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", loopErr,
			)
		}
		findings = loopFindings
	} else {
		// One-shot path.
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_internal_contradictions", "MID-CWB-REVIEW-INTERNAL-CONTRADICTIONS"))
		if err != nil {
			r.logger.Warn("internal_contradictions review block failed; skipping",
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
		findings[i].Aspect = "internal_contradictions"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "contradiction"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("internal_contradictions review block end",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
