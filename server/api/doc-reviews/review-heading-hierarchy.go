package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// headingHierarchyReviewer evaluates whether the document's headings form a
// correct, consistent nesting structure: whether section levels are used in
// order (no skipped levels), whether numbering is coherent and in sequence,
// whether sibling headings are styled and phrased consistently, and whether the
// hierarchy has empty or orphaned sections. P2, StrategyDocument, one-shot (no
// tool use). It is the second P2 (Structure & Organization) reviewer, after
// logical_flow.
//
// Like logical_flow, heading_hierarchy reasons across the whole document rather
// than a single passage. Because a document may be too large for one LLM call,
// the reviewer splits it into page-aligned blocks of up to DefaultInputBlockSize
// pages (ADR DR2) and makes one document-level call per block, concurrently. The
// prompt instructs the model to lower confidence at block boundaries where a
// parent or sibling heading may fall in an unseen block.
type headingHierarchyReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks (unused today; kept for symmetry)
	maxTasks   int
	blockSize  int // pages per block; 0 = DefaultInputBlockSize
}

func (r *headingHierarchyReviewer) Name() string             { return "heading_hierarchy" }
func (r *headingHierarchyReviewer) Group() string            { return "P2" }
func (r *headingHierarchyReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *headingHierarchyReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062411) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062412) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062413) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062414) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("heading_hierarchy review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope. The reviewer infers the document
	// type (and thus the expected heading/numbering convention) from this context.
	docCtx := buildDocContextLine(rec)

	// StrategyDocument: assemble the whole document and make one document-level
	// call. When the document is too large for a single call, split it into
	// page-aligned blocks of up to blockSize pages (ADR DR2) and review each
	// block concurrently.
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

func (r *headingHierarchyReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("heading_hierarchy review block start",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"pages", fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
		"lines", fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_heading_hierarchy", "MID-CWB-REVIEW-HEADING-HIERARCHY"))
	if err != nil {
		r.logger.Warn("heading_hierarchy review block failed; skipping",
			"record_id", recordID,
			"block", fmt.Sprintf("%d/%d", index+1, total),
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P2"
		findings[i].Aspect = "heading_hierarchy"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "heading_hierarchy"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		// Enrich location with the block's line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("heading_hierarchy review block end ",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}
