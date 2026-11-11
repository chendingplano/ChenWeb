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
// Tasks must already be sorted by orderReviewTasksForPromptCache. The seed
// reviewer is the one with the lowest reviewerIndex across ALL batches (both
// per-chunk batchID=0 and per-block batchID=1). The three phases:
//
//  1. All seed reviewer tasks (from both batches) fire concurrently, planting
//     every input's prefix into DeepSeek's cache. Seeds keep running.
//  2. Wait LLM_CALL_STAGGER seconds for DeepSeek to persist the prefixes.
//     Seeds continue running concurrently during this wait.
//  3. All remaining reviewer tasks (both batches) fire concurrently (bounded
//     by maxTasks), hitting the now-warm cache. Seeds may still be running.
func runReviewTasksForPromptCache(ctx context.Context, maxTasks int, tasks []reviewTask) ([]ReviewFinding, []error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	// Cancellation belongs to one aspect, so a failed reviewer cannot stop its
	// independent siblings or publish results from only part of its document.
	type aspectRun struct {
		ctx    context.Context
		cancel context.CancelCauseFunc
		mu     sync.Mutex
		err    error
	}
	aspects := make(map[string]*aspectRun)
	for _, task := range tasks {
		if aspects[task.aspect] == nil {
			child, cancel := context.WithCancelCause(ctx)
			aspects[task.aspect] = &aspectRun{ctx: child, cancel: cancel}
			defer cancel(nil)
		}
	}
	results := make([]reviewTaskResult, len(tasks))
	finish := func() ([]ReviewFinding, []error) {
		var findings []ReviewFinding
		var errs []error
		reported := make(map[string]bool)
		for i, task := range tasks {
			state := aspects[task.aspect]
			if state.err != nil {
				if !reported[task.aspect] {
					errs = append(errs, state.err)
					reported[task.aspect] = true
				}
				continue
			}
			if results[i].err != nil {
				errs = append(errs, results[i].err)
				continue
			}
			findings = append(findings, results[i].findings...)
		}
		return findings, errs
	}
	run := func(i int) {
		state := aspects[tasks[i].aspect]
		result := runPromptCacheReviewTask(state.ctx, tasks[i])
		if result.err != nil {
			state.mu.Lock()
			if state.err == nil {
				state.err = result.err
				state.cancel(result.err)
				logger.Error("reviewer aborted", "aspect", tasks[i].aspect, "error", result.err)
			}
			state.mu.Unlock()
		}
		results[i] = result
	}
	stagger := reviewCallStagger()
	sem := make(chan struct{}, max(maxTasks, 1))
	minPerBatch := make(map[int]int)
	for _, task := range tasks {
		if cur, ok := minPerBatch[task.batchID]; !ok || task.reviewerIndex < cur {
			minPerBatch[task.batchID] = task.reviewerIndex
		}
	}
	var seeds, remaining []int
	for i, task := range tasks {
		if task.reviewerIndex == minPerBatch[task.batchID] {
			seeds = append(seeds, i)
		} else {
			remaining = append(remaining, i)
		}
	}
	var wg sync.WaitGroup
	for _, i := range seeds {
		wg.Add(1)
		go func() { defer wg.Done(); run(i) }()
	}
	if len(remaining) > 0 && stagger > 0 {
		select {
		case <-time.After(stagger):
		case <-ctx.Done():
		}
	}
	for _, i := range remaining {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state := aspects[tasks[i].aspect]
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				run(i)
			case <-state.ctx.Done():
				results[i] = reviewTaskResult{err: context.Cause(state.ctx)}
			}
		}()
	}
	wg.Wait()
	return finish()
}

func collectPromptCacheResults(results []reviewTaskResult) ([]ReviewFinding, []error) {
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
	if ctx.Err() != nil {
		return reviewTaskResult{err: context.Cause(ctx)}
	}
	if task.run == nil {
		return reviewTaskResult{err: fmt.Errorf("%s: missing review task", task.aspect)}
	}
	findings, err := task.run(ctx)
	if err == nil && ctx.Err() != nil {
		err = context.Cause(ctx)
	}
	if err != nil {
		return reviewTaskResult{err: &reviewerExecutionError{Aspect: task.aspect, Err: err}}
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
		chunks      []chunkInput // built once, shared across all per-chunk reviewers
		blocks      []pageBlock  // built once, shared across all per-block reviewers
	)

	addTask := func(aspect, inputKey string, batchID, reviewerIndex, inputOrder int, tracker *reviewerProgressTracker, run func(context.Context) ([]ReviewFinding, error)) {
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
				if taskCtx.Err() != nil {
					return nil, context.Cause(taskCtx)
				}
				findings, err := run(taskCtx)
				if err == nil {
					err = tracker.add(len(findings))
				}
				return findings, err
			},
		})
	}

	tomlCfg, _ := GetDocReviewConfig()

	for runnerIdx, runner := range runners {
		// Resolve input type from ReviewerConfig.Input, falling back to TOML config.
		inputType := strings.TrimSpace(runner.cfg.Input)
		if inputType == "" && tomlCfg != nil {
			if rev, ok := tomlCfg.Reviewers[runner.reviewer.Name()]; ok {
				inputType = strings.TrimSpace(rev.Input)
			}
		}

		name := runner.reviewer.Name()

		switch inputType {
		case "per-chunk":
			cr, ok := runner.reviewer.(chunkReviewer)
			if !ok {
				tasks = append(tasks, reviewTask{aspect: name, run: func(context.Context) ([]ReviewFinding, error) {
					return nil, fmt.Errorf("%s: configured %s input is unsupported; no fallback", name, inputType)
				}})
				continue
			}
			if chunks == nil {
				chunks = buildChunkInputs(lines, docCtx, DefaultChunkInputSize)
			}
			tracker := newPromptCacheReviewTracker(progressFor, name, len(chunks))
			for i, inp := range chunks {
				addTask(name, inp.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) ([]ReviewFinding, error) {
					return cr.processChunk(taskCtx, recordID, i, runner.cfg, inp)
				})
			}

		case "per-block":
			br, ok := runner.reviewer.(blockReviewer)
			if !ok {
				tasks = append(tasks, reviewTask{aspect: name, run: func(context.Context) ([]ReviewFinding, error) {
					return nil, fmt.Errorf("%s: configured %s input is unsupported; no fallback", name, inputType)
				}})
				continue
			}
			if blocks == nil {
				blocks = buildBlocksForPromptCache(0, lines, docCtx)
			}
			total := len(blocks)
			tracker := newPromptCacheReviewTracker(progressFor, name, total)
			for i, b := range blocks {
				addTask(name, b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) ([]ReviewFinding, error) {
					return br.processBlock(taskCtx, recordID, i, total, runner.cfg, b)
				})
			}

		case "artifact", "document", "document-level", "":
			if inputType == "" && runner.reviewer.Strategy() == StrategyChunk {
				tasks = append(tasks, reviewTask{aspect: name, run: func(context.Context) ([]ReviewFinding, error) {
					return nil, fmt.Errorf("%s: per-chunk input configuration is required; no fallback", name)
				}})
			} else {
				unsupported = append(unsupported, runner)
			}
		default:
			tasks = append(tasks, reviewTask{aspect: name, run: func(context.Context) ([]ReviewFinding, error) {
				return nil, fmt.Errorf("%s: unknown input strategy %q", name, inputType)
			}})
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
		p.Logger.Error("review input failed", "record_id", recordID, "error", err)
		return nil, []error{err}
	}
	if len(lines) == 0 {
		err := fmt.Errorf("record %d: review input contains no lines", recordID)
		p.Logger.Error("review input failed", "record_id", recordID, "error", err)
		return nil, []error{err}
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
		documentFindings, documentErrs := p.runReviewersLegacy(ctx, recordID, unsupported, runID)
		allFindings = append(allFindings, documentFindings...)
		errs = append(errs, documentErrs...)
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
			if err == nil && ctx.Err() != nil {
				err = context.Cause(ctx)
			}
			if err != nil {
				p.Logger.Error("reviewer failed",
					"record_id", recordID,
					"reviewer", reviewer.Name(),
					"error", err,
				)
				mu.Lock()
				errs = append(errs, &reviewerExecutionError{Aspect: reviewer.Name(), Err: err})
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
