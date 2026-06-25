package docreviews

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
)

type reviewTask struct {
	aspect     string
	inputKey   string
	inputOrder int
	taskOrder  int
	run        func(context.Context) ([]ReviewFinding, error)
}

func orderReviewTasksForPromptCache(tasks []reviewTask) []reviewTask {
	ordered := append([]reviewTask(nil), tasks...)
	sort.SliceStable(ordered, func(i, j int) bool {
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

func runReviewTasksForPromptCache(ctx context.Context, maxTasks int, tasks []reviewTask) ([]ReviewFinding, []error) {
	var (
		allFindings []ReviewFinding
		errs        []error
	)
	if len(tasks) == 0 {
		return nil, nil
	}

	workers := maxTasks
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	results := make([]reviewTaskResult, len(tasks))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = runPromptCacheReviewTask(ctx, tasks[i])
			}
		}()
	}
	for i := range tasks {
		if isCtxStopped(ctx) {
			results[i] = reviewTaskResult{err: ErrPipelineStopped}
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for _, result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		if len(result.findings) == 0 {
			continue
		}
		allFindings = append(allFindings, result.findings...)
	}
	return allFindings, errs
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

	addTask := func(aspect, inputKey string, inputOrder int, tracker *reviewerProgressTracker, run func(context.Context) []ReviewFinding) {
		currentOrder := taskOrder
		taskOrder++
		tasks = append(tasks, reviewTask{
			aspect:     aspect,
			inputKey:   inputKey,
			inputOrder: inputOrder,
			taskOrder:  currentOrder,
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

	for _, runner := range runners {
		switch reviewer := runner.reviewer.(type) {
		case *grammarSpellingReviewer:
			windows := buildGrammarWindows(lines, docCtx, 100)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *toneVoiceReviewer:
			windows := buildToneVoiceWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *formattingConsistencyReviewer:
			windows := buildFormattingConsistencyWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *readabilityReviewer:
			windows := buildReadabilityWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *localizationReviewer:
			windows := buildLocalizationWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *completenessReviewer:
			windows := buildCompletenessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *correctnessReviewer:
			windows := buildCorrectnessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *clarityReviewer:
			windows := buildClarityWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *concisenessReviewer:
			windows := buildConcisenessWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *relevanceReviewer:
			windows := buildRelevanceWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *currencyReviewer:
			windows := buildCurrencyWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *examplesReviewer:
			windows := buildExamplesWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *diagramsReviewer:
			windows := buildDiagramsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *testableClaimsReviewer:
			windows := buildTestableClaimsWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *evidenceRationaleReviewer:
			windows := buildEvidenceRationaleWindows(lines, docCtx, 200)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(windows))
			for i, w := range windows {
				i, w := i, w
				addTask(reviewer.Name(), w.inputJSON, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processWindow(taskCtx, recordID, i, runner.cfg, w)
				})
			}
		case *logicalFlowReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *headingHierarchyReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *navigabilityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *sectionBalanceReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *modularityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *internalContradictionsReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *terminologyConsistencyReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *crossReferenceCorrectnessReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return reviewer.processBlock(taskCtx, recordID, i, len(blocks), runner.cfg, b)
				})
			}
		case *requirementTraceabilityReviewer:
			blocks := buildBlocksForPromptCache(reviewer.blockSize, lines, docCtx)
			tracker := newPromptCacheReviewTracker(progressFor, reviewer.Name(), len(blocks))
			for i, b := range blocks {
				i, b := i, b
				addTask(reviewer.Name(), b.inputJSON, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
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
	reviewRunID string,
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
			return p.makeProgressReporter(reviewRunID, aspect)
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
		fallbackFindings, fallbackErrs := p.runReviewersLegacy(ctx, recordID, unsupported, reviewRunID)
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
	reviewRunID string,
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

			cfg.OnProgress = p.makeProgressReporter(reviewRunID, reviewer.Name())

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
