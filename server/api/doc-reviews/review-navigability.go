package docreviews

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// navigabilityReviewer evaluates how easily a reader can find and reach the
// relevant parts of a document: whether internal cross-references ("see section
// 4.2", "as described above", "Table 3") resolve to a real target, whether the
// document provides the navigational aids a document of its type is expected to
// have (table of contents, section overviews, signposting, an index for long
// references), and whether section titles are descriptive enough to be found by
// scanning. P2, StrategyDocument, one-shot (no tool use). It is the P2
// (Structure & Organization) reviewer that follows logical_flow and
// heading_hierarchy.
//
// Like the other P2 document-level reviewers, navigability reasons across the
// whole document rather than a single passage. Because a document may be too
// large for one LLM call, the reviewer splits it into page-aligned blocks of up
// to DefaultInputBlockSize pages (ADR DR2) and makes one document-level call per
// block, concurrently. The prompt instructs the model to lower confidence at
// block boundaries where the target of a cross-reference may fall in an unseen
// block.
type navigabilityReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks (unused today; kept for symmetry)
	maxTasks   int
	blockSize  int // pages per block; 0 = DefaultInputBlockSize
}

func (r *navigabilityReviewer) Name() string             { return "navigability" }
func (r *navigabilityReviewer) Group() string            { return "P2" }
func (r *navigabilityReviewer) Strategy() ReviewStrategy { return StrategyDocument }

func (r *navigabilityReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062421) load record %d: %w", recordID, err)
	}

	// Resolve line file path.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062422) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062423) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062424) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("navigability review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope. The reviewer infers the document
	// type (and thus the navigational aids it is expected to provide) from this
	// context.
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

	results, runErr := runConcurrent(ctx, r.maxTasks, len(blocks),
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

func (r *navigabilityReviewer) processBlock(
	ctx context.Context,
	recordID int64,
	index int,
	total int,
	cfg ReviewerConfig,
	b pageBlock,
) []ReviewFinding {
	r.logger.Info("navigability review block start",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"pages", fmt.Sprintf("%d-%d", b.pageStart, b.pageEnd),
		"lines", fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd),
	)

	startTime := time.Now()

	payload, err := r.client.ExtractJSON(ctx, newLLMJSONInput(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, b.inputJSON, "review_navigability", "MID-CWB-REVIEW-NAVIGABILITY"))
	if err != nil {
		r.logger.Warn("navigability review block failed; skipping",
			"record_id", recordID,
			"block", fmt.Sprintf("%d/%d", index+1, total),
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P2"
		findings[i].Aspect = "navigability"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "navigability"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		// Enrich location with the block's line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", b.lineStart, b.lineEnd)
		}
	}

	r.logger.Info("navigability review block end ",
		"record_id", recordID,
		"block", fmt.Sprintf("%d/%d", index+1, total),
		"findings", len(findings),
		"ms_used", time.Since(startTime).Milliseconds(),
	)
	return findings
}
