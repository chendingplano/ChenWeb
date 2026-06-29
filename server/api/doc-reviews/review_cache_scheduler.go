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

	stagger := reviewCallStagger()
	results := make([]reviewTaskResult, len(tasks))
	concurrency := max(maxTasks, 1)
	sem := make(chan struct{}, concurrency)

	// Find the minimum reviewerIndex per batchID.
	minPerBatch := make(map[int]int)
	for _, t := range tasks {
		if cur, ok := minPerBatch[t.batchID]; !ok || t.reviewerIndex < cur {
			minPerBatch[t.batchID] = t.reviewerIndex
		}
	}

	// Partition into seeds (min reviewerIndex for their batch) and remaining.
	var seedIdxs, remainIdxs []int
	for i, t := range tasks {
		if minIdx, ok := minPerBatch[t.batchID]; ok && t.reviewerIndex == minIdx {
			seedIdxs = append(seedIdxs, i)
		} else {
			remainIdxs = append(remainIdxs, i)
		}
	}

	var allWg sync.WaitGroup

	// Phase 1: fire all seeds concurrently; do NOT wait for completion.
	// Seeds keep running while the stagger timer counts down.
	for _, i := range seedIdxs {
		allWg.Add(1)
		go func(idx int) {
			defer allWg.Done()
			results[idx] = runPromptCacheReviewTask(ctx, tasks[idx])
		}(i)
	}

	if len(remainIdxs) == 0 {
		allWg.Wait()
		return collectPromptCacheResults(results)
	}

	// Phase 2: wait LLM_CALL_STAGGER for DeepSeek to persist the cached prefixes.
	// Seeds continue running concurrently during this sleep.
	if stagger > 0 {
		select {
		case <-time.After(stagger):
		case <-ctx.Done():
			for _, i := range remainIdxs {
				results[i] = reviewTaskResult{err: ErrPipelineStopped}
			}
			allWg.Wait()
			return collectPromptCacheResults(results)
		}
	}

	if isCtxStopped(ctx) {
		for _, i := range remainIdxs {
			results[i] = reviewTaskResult{err: ErrPipelineStopped}
		}
		allWg.Wait()
		return collectPromptCacheResults(results)
	}

	// Phase 3: fire all remaining (chunk + block) concurrently.
	// Seeds may still be running; all goroutines complete before we return.
	for _, i := range remainIdxs {
		if isCtxStopped(ctx) {
			results[i] = reviewTaskResult{err: ErrPipelineStopped}
			continue
		}
		allWg.Add(1)
		go func(idx int) {
			defer allWg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[idx] = reviewTaskResult{err: ErrPipelineStopped}
				return
			}
			defer func() { <-sem }()
			results[idx] = runPromptCacheReviewTask(ctx, tasks[idx])
		}(i)
	}

	allWg.Wait()
	return collectPromptCacheResults(results)
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
		chunks      []chunkInput // built once, shared across all per-chunk reviewers
		blocks      []pageBlock  // built once, shared across all per-block reviewers
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
				unsupported = append(unsupported, runner)
				continue
			}
			if chunks == nil {
				chunks = buildChunkInputs(lines, docCtx, DefaultChunkInputSize)
			}
			tracker := newPromptCacheReviewTracker(progressFor, name, len(chunks))
			for i, inp := range chunks {
				addTask(name, inp.inputJSON, 0, runnerIdx, i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return cr.processChunk(taskCtx, recordID, i, runner.cfg, inp)
				})
			}

		case "per-block":
			br, ok := runner.reviewer.(blockReviewer)
			if !ok {
				unsupported = append(unsupported, runner)
				continue
			}
			if blocks == nil {
				blocks = buildBlocksForPromptCache(0, lines, docCtx)
			}
			total := len(blocks)
			tracker := newPromptCacheReviewTracker(progressFor, name, total)
			for i, b := range blocks {
				addTask(name, b.inputJSON, 1, runnerIdx, len(lines)+i, tracker, func(taskCtx context.Context) []ReviewFinding {
					return br.processBlock(taskCtx, recordID, i, total, runner.cfg, b)
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
