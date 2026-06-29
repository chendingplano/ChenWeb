package docreviews

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type reviewTask struct {
	aspect        string
	inputKey      string
	batchID       int // 0 = per-chunk (window), 1 = per-block
	reviewerIndex int // position in the runners slice; 0 = seed reviewer for this batch
	inputOrder    int
	taskOrder     int
	run           func(context.Context) ([]ReviewFinding, error)
}

func orderReviewTasksForPromptCache(tasks []reviewTask) []reviewTask {
	ordered := append([]reviewTask(nil), tasks...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].batchID != ordered[j].batchID {
			return ordered[i].batchID < ordered[j].batchID
		}
		if ordered[i].inputOrder != ordered[j].inputOrder {
			return ordered[i].inputOrder < ordered[j].inputOrder
		}
		if ordered[i].inputKey != ordered[j].inputKey {
			return ordered[i].inputKey < ordered[j].inputKey
		}
		return ordered[i].taskOrder < ordered[j].taskOrder
	})
	return ordered
}

type reviewTaskResult struct {
	findings []ReviewFinding
	err      error
}

// runReviewTasksForPromptCache executes ordered review tasks using a two-phase
// batch algorithm that maximises DeepSeek prompt-cache hit rate.
//
// Tasks must already be sorted by orderReviewTasksForPromptCache. Within each
// batch (batchID 0 = per-chunk, batchID 1 = per-block) the split is determined
// by reviewerIndex: the reviewer with the lowest reviewerIndex in the batch is
// the seed reviewer; all others are remaining. The two phases:
//
//  1. All seed reviewer tasks for the batch fire concurrently, planting every
//     input's prefix into DeepSeek's cache.
//  2. Wait LLM_CALL_STAGGER seconds for DeepSeek to persist the prefixes.
//  3. All remaining reviewer tasks for the batch fire concurrently (bounded by
//     maxTasks), hitting the now-warm cache.
//
// The per-block batch starts only after the per-chunk batch completes.
func runReviewTasksForPromptCache(ctx context.Context, maxTasks int, tasks []reviewTask) ([]ReviewFinding, []error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	stagger := reviewCallStagger()
	results := make([]reviewTaskResult, len(tasks))
	concurrency := max(maxTasks, 1)
	sem := make(chan struct{}, concurrency)

	for _, targetBatchID := range []int{0, 1} {
		// Collect task indices for this batch.
		var batchIdxs []int
		for i, t := range tasks {
			if t.batchID == targetBatchID {
				batchIdxs = append(batchIdxs, i)
			}
		}
		if len(batchIdxs) == 0 {
			continue
		}

		// The reviewer with the lowest reviewerIndex in this batch is the seed.
		minReviewerIdx := tasks[batchIdxs[0]].reviewerIndex
		for _, i := range batchIdxs {
			if tasks[i].reviewerIndex < minReviewerIdx {
				minReviewerIdx = tasks[i].reviewerIndex
			}
		}
		var seedIdxs, remainIdxs []int
		for _, i := range batchIdxs {
			if tasks[i].reviewerIndex == minReviewerIdx {
				seedIdxs = append(seedIdxs, i)
			} else {
				remainIdxs = append(remainIdxs, i)
			}
		}

		// Phase 1: all seed reviewer tasks for this batch fire concurrently.
		var seedWg sync.WaitGroup
		for _, i := range seedIdxs {
			seedWg.Go(func() {
				results[i] = runPromptCacheReviewTask(ctx, tasks[i])
			})
		}
		seedWg.Wait()

		if len(remainIdxs) == 0 {
			continue
		}

		if isCtxStopped(ctx) {
			for _, i := range remainIdxs {
				results[i] = reviewTaskResult{err: ErrPipelineStopped}
			}
			continue
		}

		// Phase 2: wait for DeepSeek to persist the cached prefixes.
		if stagger > 0 {
			select {
			case <-time.After(stagger):
			case <-ctx.Done():
				for _, i := range remainIdxs {
					results[i] = reviewTaskResult{err: ErrPipelineStopped}
				}
				continue
			}
		}

		// Phase 3: all remaining reviewer tasks for this batch fire concurrently.
		var remainWg sync.WaitGroup
		for _, i := range remainIdxs {
			if isCtxStopped(ctx) {
				results[i] = reviewTaskResult{err: ErrPipelineStopped}
				continue
			}
			remainWg.Go(func() {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					results[i] = reviewTaskResult{err: ErrPipelineStopped}
					return
				}
				defer func() { <-sem }()
				results[i] = runPromptCacheReviewTask(ctx, tasks[i])
			})
		}
		remainWg.Wait()
	}

	var allFindings []ReviewFinding
	var errs []error
	for _, result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		allFindings = append(allFindings, result.findings...)
	}
	return allFindings, errs
}

// reviewCallStagger returns the duration to wait between a chunk's seed call
// and its remaining concurrent reviewers. Controlled by LLM_CALL_STAGGER
// (seconds); default 1s.
func reviewCallStagger() time.Duration {
	v := strings.TrimSpace(os.Getenv("LLM_CALL_STAGGER"))
	if v == "" {
		return time.Second
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec < 0 {
		return time.Second
	}
	return time.Duration(sec) * time.Second
}

func runPromptCacheReviewTask(ctx context.Context, task reviewTask) reviewTaskResult {
	if isCtxStopped(ctx) {
		return reviewTaskResult{err: ErrPipelineStopped}
	}
	if task.run == nil {
		return reviewTaskResult{}
	}
	findings, err := task.run(ctx)
	if err != nil {
		return reviewTaskResult{err: fmt.Errorf("%s: %w", task.aspect, err)}
	}
	return reviewTaskResult{findings: findings}
}

func maxDocReviewerTasks(fallback int) int {
	if fallback <= 0 {
		fallback = 1
	}
	return envInt("MAX_DOC_REVIEWER_TASKS", fallback, 1)
}

func buildPromptCacheReviewTasks(
	recordID int64,
	rec DocMetadataInputRecord,
	lines []Line,
	runners []reviewRunner,
	progressFor func(aspect string, total int) ReviewerProgressFunc,
) ([]reviewTask, []reviewRunner) {
	if len(lines) == 0 {
		return nil, nil
	}
	docCtx := buildDocContextLine(rec)
	var (
		tasks       []reviewTask
		unsupported []reviewRunner
		taskOrder   int
	)

	addTask := func(aspect, inputKey string, batchID, reviewerIndex, inputOrder int, tracker *reviewerProgressTracker, run func(context.Context) []ReviewFinding) {
		currentOrder := taskOrder
		taskOrder++
		tasks = append(tasks, reviewTask{
			aspect:        aspect,
			inputKey:      inputKey,
			batchID:       batchID,
			reviewerIndex: reviewerIndex,
			inputOrder:    inputOrder,
			taskOrder:     currentOrder,
			run: func(taskCtx context.Context) ([]ReviewFinding, error) {
				if isCtxStopped(taskCtx) {
					return nil, ErrPipelineStopped
				}
				findings := run(taskCtx)
				tracker.add(len(findings))
				return findings, nil
			},
		})
	}

	for runnerIdx, runner := range runners {
		switch reviewer := runner.reviewer.(type) {
		case *grammarSpellingReviewer:
			windows := buildGrammarWindows(lines, docCtx, 100)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *toneVoiceReviewer:
			windows := buildToneVoiceWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *formattingConsistencyReviewer:
			windows := buildFormattingConsistencyWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *readabilityReviewer:
			windows := buildReadabilityWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *localizationReviewer:
			windows := buildLocalizationWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *completenessReviewer:
			windows := buildCompletenessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *correctnessReviewer:
			windows := buildCorrectnessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *clarityReviewer:
			windows := buildClarityWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *concisenessReviewer:
			windows := buildConcisenessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *relevanceReviewer:
			windows := buildRelevanceWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *currencyReviewer:
			windows := buildCurrencyWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *examplesReviewer:
			windows := buildExamplesWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *diagramsReviewer:
			windows := buildDiagramsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *testableClaimsReviewer:
			windows := buildTestableClaimsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *evidenceRationaleReviewer:
			windows := buildEvidenceRationaleWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *logicalFlowReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *headingHierarchyReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *navigabilityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *sectionBalanceReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *modularityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *internalContradictionsReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *terminologyConsistencyReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *crossReferenceCorrectnessReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *requirementTraceabilityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}

		// ── P5 — Technical & Compliance (chunk) ──────────────────────────────
		case *technicalAccuracyReviewer:
			windows := buildTechnicalAccuracyWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *assumptionsReviewer:
			windows := buildAssumptionsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *prerequisitesReviewer:
			windows := buildPrerequisitesWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *securityReviewer:
			windows := buildSecurityWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *performanceReviewer:
			windows := buildPerformanceWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *errorHandlingReviewer:
			windows := buildErrorHandlingWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *limitationsReviewer:
			windows := buildLimitationsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				addTask(reviewer.Name(), w.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}

		// ── P5 — Technical & Compliance (document) ───────────────────────────
		case *standardsComplianceReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *legalComplianceReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *regulatoryComplianceReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *internalPolicyReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				addTask(reviewer.Name(), b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}

		default:
			unsupported = append(unsupported, runner)
		}
	}
	return tasks, unsupported
}

func newPromptCacheReviewTracker(progressFor func(aspect string, total int) ReviewerProgressFunc, aspect string, total int) *reviewerProgressTracker {
	if progressFor == nil {
		return nil
	}
	return newReviewerProgressTracker(total, progressFor(aspect, total))
}

func buildBlocksForPromptCache(blockSize int, lines []Line, docCtx string) []pageBlock {
	if blockSize <= 0 {
		blockSize = DefaultInputBlockSize
	}
	return buildPageBlocks(lines, docCtx, blockSize)
}

func (p *ReviewProcessor) runReviewersPromptCacheOptimized(
	ctx context.Context,
	recordID int64,
	rec DocMetadataInputRecord,
	reviewers []reviewRunner,
	runID int64,
) ([]ReviewFinding, []error) {
	lines, err := loadPromptCacheReviewLines(recordID, rec)
	if err != nil {
		return nil, []error{err}
	}
	if len(lines) == 0 {
		return nil, nil
	}

	tasks, unsupported := buildPromptCacheReviewTasks(recordID, rec, lines, reviewers,
		func(aspect string, _ int) ReviewerProgressFunc {
			return p.makeProgressReporter(runID, aspect)
		},
	)
	orderedTasks := orderReviewTasksForPromptCache(tasks)
	maxTasks := maxDocReviewerTasks(p.MaxConcurrent)
	p.Logger.Info("document review prompt-cache scheduler",
		"record_id", recordID,
		"tasks", len(orderedTasks),
		"max_tasks", maxTasks,
		"unsupported_reviewers", len(unsupported),
	)

	allFindings, errs := runReviewTasksForPromptCache(ctx, maxTasks, orderedTasks)
	if len(unsupported) > 0 {
		fallbackFindings, fallbackErrs := p.runReviewersLegacy(ctx, recordID, unsupported, runID)
		allFindings = append(allFindings, fallbackFindings...)
		errs = append(errs, fallbackErrs...)
	}
	return allFindings, errs
}

func loadPromptCacheReviewLines(recordID int64, rec DocMetadataInputRecord) ([]Line, error) {
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062591) resolve line file for record %d: %w", recordID, err)
	}
	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062592) read line file %s: %w", lineFilePath, err)
	}
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26062593) parse line file for record %d: %w", recordID, err)
	}
	return lines, nil
}

func (p *ReviewProcessor) runReviewersLegacy(
	ctx context.Context,
	recordID int64,
	reviewers []reviewRunner,
	runID int64,
) ([]ReviewFinding, []error) {
	var (
		mu          sync.Mutex
		allFindings []ReviewFinding
		wg          sync.WaitGroup
		errs        []error
	)

	for _, r := range reviewers {
		wg.Add(1)
		go func(reviewer Reviewer, cfg ReviewerConfig) {
			defer wg.Done()

			cfg.OnProgress = p.makeProgressReporter(runID, reviewer.Name())

			p.Logger.Info("reviewer start",
				"record_id", recordID,
				"reviewer", reviewer.Name(),
				"group", reviewer.Group(),
				"model", cfg.ModelName,
			)

			findings, err := reviewer.ReviewDocument(ctx, recordID, cfg)
			if err != nil {
				p.Logger.Error("reviewer failed",
					"record_id", recordID,
					"reviewer", reviewer.Name(),
					"error", err,
				)
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", reviewer.Name(), err))
				mu.Unlock()
				return
			}

			p.Logger.Info("reviewer complete",
				"record_id", recordID,
				"reviewer", reviewer.Name(),
				"findings", len(findings),
			)

			mu.Lock()
			allFindings = append(allFindings, findings...)
			mu.Unlock()
		}(r.reviewer, r.cfg)
	}
	wg.Wait()
	return allFindings, errs
}
