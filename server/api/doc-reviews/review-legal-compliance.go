package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// legalComplianceReviewer checks whether the document addresses applicable legal
// requirements: missing contractual obligations, absent liability disclaimers,
// unaddressed intellectual property considerations, missing warranty/indemnification
// clauses, privacy regulation gaps, and legal references that are outdated or
// incorrectly cited. Document-level reasoning is used because legal obligations
// typically apply to the document as a whole. P5 (Technical & Compliance),
// StrategyDocument; one-shot by default, tool-use when max_tool_turns > 0.
type legalComplianceReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	chunkStore   SQLStore
	maxTasks     int
	blockSize    int
}

func (r *legalComplianceReviewer) Name() string             { return "legal_compliance" }
func (r *legalComplianceReviewer) Group() string            { return "P5" }
func (r *legalComplianceReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *legalComplianceReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062741) load record %d: %w", recordID, err)
	}

	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062742) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062743) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062744) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("legal_compliance review skipped: no lines", "record_id", recordID)
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

func (r *legalComplianceReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding {
	r.logger.Info("legal_compliance review chunk start",
		"record_id", recordID,
		"chunk", index,
		"lines", fmt.Sprintf("%d-%d", input.startLine, input.endLine),
		"max_tool_turns", cfg.MaxToolTurns,
	)

	startTime := time.Now()
	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int

	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", input.inputJSON, cfg.PromptText)
		callInfo := docReviewCallInfo(ctx, map[string]any{
			"chunk": index,
			"lines": fmt.Sprintf("%d-%d", input.startLine, input.endLine),
		})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_legal_compliance", callInfo, "MID-20260706-008",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("legal_compliance tool-use loop failed; no findings for chunk",
				"record_id", recordID, "chunk", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, input.inputJSON, "review_legal_compliance", "MID-CWB-REVIEW-LEGAL-COMPLIANCE"))
		if err != nil {
			r.logger.Warn("legal_compliance review chunk failed; skipping",
				"record_id", recordID, "chunk", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "legal_compliance"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "legal_compliance_issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", input.startLine, input.endLine)
		}
	}

	r.logger.Info("legal_compliance review chunk end",
		"record_id", recordID,
		"chunk", index,
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}

func (r *legalComplianceReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("legal_compliance review block start",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"pages", fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
		"lines", fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
		"max_tool_turns", cfg.MaxToolTurns,
	)

	startTime := time.Now()
	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int

	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := fmt.Sprintf("<DOCUMENT_INPUT>\n%s\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\n%s\n</REVIEW_TASK>", b.inputJSON, cfg.PromptText)
		callInfo := docReviewCallInfo(ctx, map[string]any{
			"block": fmt.Sprintf("%d/%d", index+1, total),
			"pages": fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
			"lines": fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
		})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_legal_compliance", callInfo, "MID-20260706-008",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("legal_compliance tool-use loop failed; no findings for block",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", loopErr)
		}
		findings = loopFindings
	} else {
		payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_legal_compliance", "MID-CWB-REVIEW-LEGAL-COMPLIANCE"))
		if err != nil {
			r.logger.Warn("legal_compliance review block failed; skipping",
				"record_id", recordID,
				"block", fmt.Sprintf("%d/%d", index+1, total),
				"error", err)
			return nil
		}
		findings = normalizeFindingsJSON(payload)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "legal_compliance"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "legal_compliance_issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "high"
		}
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("legal_compliance review block end",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}
