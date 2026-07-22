package docbenchmarkadmin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

type Service struct {
	Store Store
	DB    *sql.DB
}

var (
	jobRunnerMu sync.Mutex
	jobRunners  = map[int64]struct{}{}
)

func NewService(db *sql.DB) Service {
	return Service{Store: Store{DB: db}, DB: db}
}

func (s Service) GetConfig(ctx context.Context) (Config, error) {
	return s.Store.LoadConfig(ctx, DefaultScope)
}

func (s Service) SaveConfig(ctx context.Context, cfg Config) (Config, error) {
	cfg.Scope = DefaultScope
	return s.Store.SaveConfig(ctx, normalizeConfig(cfg))
}

func (s Service) SetupState(ctx context.Context) (SetupState, error) {
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return SetupState{}, err
	}
	jobs, err := s.Store.ListJobs(ctx, DefaultScope, 20)
	if err != nil {
		return SetupState{}, err
	}
	active := make([]Job, 0, len(jobs))
	lastByStep := map[string]Job{}
	lastExperimentID := ""
	for _, job := range jobs {
		if job.StepID != "" {
			if _, ok := lastByStep[job.StepID]; !ok {
				lastByStep[job.StepID] = job
			}
		}
		if !terminalJobStatus(job.Status) {
			active = append(active, job)
		}
		if lastExperimentID == "" && job.Result["experiment_id"] != nil {
			lastExperimentID = fmt.Sprint(job.Result["experiment_id"])
		}
	}
	steps := s.inspectSteps(ctx, cfg, lastByStep)
	return SetupState{
		Config:           cfg,
		Steps:            steps,
		ActiveJobs:       active,
		RecentJobs:       jobs,
		LastExperimentID: lastExperimentID,
	}, nil
}

func (s Service) RunNext(ctx context.Context, createdBy string) (Job, error) {
	state, err := s.SetupState(ctx)
	if err != nil {
		return Job{}, err
	}
	for _, step := range state.Steps {
		if step.Status == StatusCompleted {
			continue
		}
		if step.Status == StatusRunning {
			return Job{}, fmt.Errorf("step %s is already running", step.ID)
		}
		if step.Status == StatusBlocked {
			return Job{}, fmt.Errorf("step %s is blocked: %s", step.ID, step.Message)
		}
		return s.RunStep(ctx, step.ID, createdBy)
	}
	return Job{}, fmt.Errorf("all benchmark steps are already completed")
}

func (s Service) RunStep(ctx context.Context, stepID, createdBy string) (Job, error) {
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return Job{}, err
	}
	if stepID == StepRuntimeConfig || stepID == StepRoots {
		result, err := s.runImmediateStep(ctx, stepID, cfg)
		status := JobSucceeded
		errText := ""
		message := "step completed"
		if err != nil {
			status = JobFailed
			errText = err.Error()
			message = err.Error()
		}
		job, insertErr := s.Store.InsertJob(ctx, DefaultScope, stepID, stepID, map[string]any{"scope": DefaultScope}, createdBy)
		if insertErr != nil {
			return Job{}, insertErr
		}
		if updateErr := s.Store.UpdateJob(ctx, job.ID, status, message, result, errText, true, true); updateErr != nil {
			return Job{}, updateErr
		}
		return s.Store.GetJob(ctx, job.ID)
	}
	job, err := s.Store.InsertJob(ctx, DefaultScope, stepID, stepID, map[string]any{"scope": DefaultScope}, createdBy)
	if err != nil {
		return Job{}, err
	}
	s.startJobRunner(job.ID, stepID, cfg)
	return job, nil
}

func (s Service) startJobRunner(jobID int64, stepID string, cfg Config) {
	jobRunnerMu.Lock()
	if _, exists := jobRunners[jobID]; exists {
		jobRunnerMu.Unlock()
		return
	}
	jobRunners[jobID] = struct{}{}
	jobRunnerMu.Unlock()

	go func() {
		defer func() {
			jobRunnerMu.Lock()
			delete(jobRunners, jobID)
			jobRunnerMu.Unlock()
		}()
		ctx := context.Background()
		_ = s.Store.UpdateJob(ctx, jobID, JobRunning, "running", map[string]any{}, "", true, false)
		result, err := s.runAsyncStep(ctx, stepID, cfg)
		if err != nil {
			_ = s.Store.UpdateJob(ctx, jobID, JobFailed, err.Error(), result, err.Error(), true, true)
			return
		}
		_ = s.Store.UpdateJob(ctx, jobID, JobSucceeded, "completed", result, "", true, true)
	}()
}

func (s Service) runImmediateStep(ctx context.Context, stepID string, cfg Config) (map[string]any, error) {
	switch stepID {
	case StepRuntimeConfig:
		return s.validateRuntimeConfig(ctx, cfg)
	case StepRoots:
		return s.ensureRoots(cfg)
	default:
		return map[string]any{}, fmt.Errorf("step %s is not an immediate step", stepID)
	}
}

func (s Service) runAsyncStep(ctx context.Context, stepID string, cfg Config) (map[string]any, error) {
	switch stepID {
	case StepWorkingCopy:
		return s.checkWorkingCopy(ctx, cfg)
	case StepValidate:
		return s.validateExperiment(cfg)
	case StepRun:
		return s.runBenchmark(ctx, cfg)
	case StepReport:
		return s.generateReport(ctx, cfg)
	case StepCompare:
		return s.generateCompare(ctx, cfg)
	default:
		return map[string]any{}, fmt.Errorf("unknown benchmark step %q", stepID)
	}
}

func (s Service) validateRuntimeConfig(_ context.Context, cfg Config) (map[string]any, error) {
	detected := map[string]any{
		"experiment_path":   cfg.ExperimentPath,
		"artifact_root":     cfg.ArtifactRoot,
		"metrics_model_name": cfg.MetricsModelName,
	}
	if strings.TrimSpace(cfg.ExperimentPath) == "" {
		return detected, errors.New("experiment path is required")
	}
	if strings.TrimSpace(cfg.ArtifactRoot) == "" {
		return detected, errors.New("artifact root is required")
	}
	if strings.TrimSpace(cfg.MetricsModelName) == "" {
		return detected, errors.New("metrics model name is required")
	}
	expPath := resolvePath(cfg.ExperimentPath)
	if _, err := os.Stat(expPath); err != nil {
		return detected, fmt.Errorf("experiment path %s is not readable", expPath)
	}
	return map[string]any{"experiment_path": expPath}, nil
}

func (s Service) ensureRoots(cfg Config) (map[string]any, error) {
	paths := []string{cfg.ArtifactRoot, cfg.WorkRoot, cfg.EvidenceRoot}
	out := map[string]any{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		resolved := resolvePath(path)
		if err := os.MkdirAll(resolved, 0o755); err != nil {
			return out, fmt.Errorf("create %s: %w", resolved, err)
		}
		out[pathKey(path)] = resolved
	}
	return out, nil
}

func (s Service) checkWorkingCopy(ctx context.Context, cfg Config) (map[string]any, error) {
	dirty, err := workingCopyDirty(ctx)
	if err != nil {
		return map[string]any{}, err
	}
	if dirty && !cfg.AllowDirty {
		return map[string]any{"dirty": true}, errors.New("working copy is dirty; enable allow dirty or commit changes")
	}
	return map[string]any{"dirty": dirty, "allow_dirty": cfg.AllowDirty}, nil
}

func (s Service) validateExperiment(cfg Config) (map[string]any, error) {
	if _, err := s.validateRuntimeConfig(context.Background(), cfg); err != nil {
		return map[string]any{}, err
	}
	restore := setMetricsEnv(cfg.MetricsModelName)
	defer restore()
	raw, err := os.ReadFile(resolvePath(cfg.ExperimentPath))
	if err != nil {
		return map[string]any{}, err
	}
	experiment, err := docbenchmark.LoadExperiment(raw, resolvePath(cfg.DatasetRoot))
	if err != nil {
		return map[string]any{}, err
	}
	return map[string]any{
		"dataset_id":                  experiment.DatasetID,
		"dataset_version":             experiment.DatasetVersion,
		"dataset_hash":                experiment.DatasetHash,
		"request_hash":                experiment.RequestHash,
		"processor_case_set_hashes":   experiment.ProcessorCaseSetHashes,
		"validated_experiment_path":   resolvePath(cfg.ExperimentPath),
		"validated_dataset_root_path": resolvePath(cfg.DatasetRoot),
	}, nil
}

func (s Service) runBenchmark(ctx context.Context, cfg Config) (map[string]any, error) {
	if _, err := s.checkWorkingCopy(ctx, cfg); err != nil {
		return map[string]any{}, err
	}
	if _, err := s.ensureRoots(cfg); err != nil {
		return map[string]any{}, err
	}
	restore := setMetricsEnv(cfg.MetricsModelName)
	defer restore()
	if err := os.Setenv("ARTIFACT_DIR", resolvePath(cfg.ArtifactRoot)); err != nil {
		return map[string]any{}, err
	}
	raw, err := os.ReadFile(resolvePath(cfg.ExperimentPath))
	if err != nil {
		return map[string]any{}, err
	}
	app := docbenchmark.Application{Config: docbenchmark.ApplicationConfig{
		DB:           s.DB,
		DatasetRoot:  resolvePath(cfg.DatasetRoot),
		WorkRoot:     resolvePath(cfg.WorkRoot),
		EvidenceRoot: resolvePath(cfg.EvidenceRoot),
		ArtifactRoot: resolvePath(cfg.ArtifactRoot),
		Owner:        strings.TrimSpace(cfg.Owner),
		TenantID:     strings.TrimSpace(cfg.TenantID),
		StoreID:      cfg.StoreID,
	}}
	prepared, err := app.Prepare(ctx, raw)
	if err != nil {
		return map[string]any{}, err
	}
	store := docbenchmark.SQLStore{DB: s.DB}
	executable, _ := os.Executable()
	executableHash, _ := docbenchmark.ExecutableSHA256(executable)
	gitCommit := commandOutput(ctx, "git", "-C", repoRoot(), "rev-parse", "HEAD")
	jjChange := commandOutput(ctx, "jj", "--repository", repoRoot(), "log", "-r", "@", "--no-graph", "-T", "change_id")
	dirty, _ := workingCopyDirty(ctx)
	for _, name := range prepared.VariantOrder {
		stored, err := store.GetRun(ctx, prepared.Runs[name].RunID)
		if err != nil {
			return map[string]any{}, err
		}
		if stored.Lifecycle == "succeeded" || stored.Lifecycle == "failed" || stored.Lifecycle == "canceled" {
			continue
		}
		if err := store.AttachRunProvenance(ctx, prepared.Runs[name].RunID, gitCommit, jjChange, executable, executableHash, dirty, prepared.Experiment.MaxParallelCases); err != nil {
			return map[string]any{}, err
		}
	}
	for _, name := range prepared.VariantOrder {
		worker := docbenchmark.VariantWorker{Application: app, Experiment: prepared.Experiment, Run: prepared.Runs[name]}
		if err := worker.RunCases(ctx); err != nil {
			return map[string]any{}, fmt.Errorf("variant %s: %w", name, err)
		}
	}
	return map[string]any{"experiment_id": prepared.ExperimentID, "variants": prepared.VariantOrder, "dirty": dirty}, nil
}

func (s Service) generateReport(ctx context.Context, cfg Config) (map[string]any, error) {
	experimentID, err := s.lastExperimentID(ctx)
	if err != nil {
		return map[string]any{}, err
	}
	store := docbenchmark.SQLStore{DB: s.DB}
	report, err := store.BuildExperimentReport(ctx, experimentID, "", "", false)
	if err != nil {
		return map[string]any{}, err
	}
	format := strings.ToLower(strings.TrimSpace(cfg.ReportFormat))
	if format == "" {
		format = "markdown"
	}
	var raw []byte
	outputPath := strings.TrimSpace(cfg.ReportOutputPath)
	if outputPath == "" {
		outputPath = filepath.Join(resolvePath(cfg.ArtifactRoot), "doc-benchmark", fmt.Sprintf("report-%s.%s", experimentID, extForFormat(format)))
	} else {
		outputPath = resolvePath(outputPath)
	}
	switch format {
	case "json":
		raw, err = docbenchmark.RenderJSON(report)
	default:
		raw = []byte(docbenchmark.RenderMarkdown(report))
	}
	if err != nil {
		return map[string]any{}, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return map[string]any{}, err
	}
	if err := os.WriteFile(outputPath, raw, 0o644); err != nil {
		return map[string]any{}, err
	}
	return map[string]any{"experiment_id": experimentID, "format": format, "output_path": outputPath}, nil
}

func (s Service) generateCompare(ctx context.Context, cfg Config) (map[string]any, error) {
	experimentID, err := s.lastExperimentID(ctx)
	if err != nil {
		return map[string]any{}, err
	}
	store := docbenchmark.SQLStore{DB: s.DB}
	var outputs []map[string]any
	pairs := []struct {
		kind      string
		baseline  string
		candidate string
	}{
		{"metrics", cfg.MetricsBaseline, cfg.MetricsCandidate},
		{"chunk", cfg.ChunkBaseline, cfg.ChunkCandidate},
	}
	for _, pair := range pairs {
		if strings.TrimSpace(pair.baseline) == "" || strings.TrimSpace(pair.candidate) == "" {
			continue
		}
		report, err := store.BuildExperimentReport(ctx, experimentID, pair.baseline, pair.candidate, false)
		if err != nil {
			return map[string]any{}, err
		}
		outputPath := filepath.Join(resolvePath(cfg.ArtifactRoot), "doc-benchmark", fmt.Sprintf("compare-%s-%s.%s", pair.kind, experimentID, extForFormat(cfg.ReportFormat)))
		raw := []byte(docbenchmark.RenderMarkdown(report))
		if strings.EqualFold(cfg.ReportFormat, "json") {
			raw, err = docbenchmark.RenderJSON(report)
			if err != nil {
				return map[string]any{}, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return map[string]any{}, err
		}
		if err := os.WriteFile(outputPath, raw, 0o644); err != nil {
			return map[string]any{}, err
		}
		outputs = append(outputs, map[string]any{
			"kind":       pair.kind,
			"baseline":   pair.baseline,
			"candidate":  pair.candidate,
			"output_path": outputPath,
		})
	}
	return map[string]any{"experiment_id": experimentID, "outputs": outputs}, nil
}

func (s Service) lastExperimentID(ctx context.Context) (string, error) {
	jobs, err := s.Store.ListJobs(ctx, DefaultScope, 50)
	if err != nil {
		return "", err
	}
	for _, job := range jobs {
		if job.Status != JobSucceeded {
			continue
		}
		if experimentID, ok := job.Result["experiment_id"].(string); ok && strings.TrimSpace(experimentID) != "" {
			return experimentID, nil
		}
	}
	return "", errors.New("no successful benchmark run found")
}

func (s Service) inspectSteps(ctx context.Context, cfg Config, lastByStep map[string]Job) []StepState {
	steps := []StepState{
		{ID: StepRuntimeConfig, Title: "Prepare runtime configuration", Description: "Confirm experiment path, model ref, and required runtime inputs."},
		{ID: StepRoots, Title: "Create benchmark roots", Description: "Ensure artifact, work, and evidence roots exist and are writable."},
		{ID: StepWorkingCopy, Title: "Check working copy", Description: "Require a clean working copy unless allow dirty is enabled."},
		{ID: StepValidate, Title: "Validate experiment", Description: "Validate the experiment and dataset before spending model tokens."},
		{ID: StepRun, Title: "Run benchmark", Description: "Execute the benchmark variants against the configured dataset."},
		{ID: StepReport, Title: "Generate report", Description: "Render the stored benchmark report to the configured output path."},
		{ID: StepCompare, Title: "Generate compares", Description: "Render the stored compare outputs for metrics and chunk variants."},
	}
	for i := range steps {
		step := &steps[i]
		if job, ok := lastByStep[step.ID]; ok {
			if !terminalJobStatus(job.Status) {
				step.Status = StatusRunning
				step.Message = job.Message
				step.RunningJobID = job.ID
				step.Detected = job.Result
				continue
			}
			if job.Status == JobSucceeded {
				step.Status = StatusCompleted
				step.CompletedAt = job.FinishedAt
				step.Message = coalesce(job.Message, "completed")
				step.Detected = job.Result
				continue
			}
			step.Status = StatusFailed
			step.FailedAt = job.FinishedAt
			step.Message = coalesce(job.ErrorText, job.Message)
			step.Detected = job.Result
			continue
		}
		status, msg, detected := s.defaultStepState(ctx, cfg, step.ID)
		step.Status, step.Message, step.Detected = status, msg, detected
	}
	return steps
}

func (s Service) defaultStepState(ctx context.Context, cfg Config, stepID string) (string, string, map[string]any) {
	switch stepID {
	case StepRuntimeConfig:
		detected := map[string]any{"experiment_path": cfg.ExperimentPath, "artifact_root": cfg.ArtifactRoot, "metrics_model_name": cfg.MetricsModelName}
		if _, err := s.validateRuntimeConfig(ctx, cfg); err != nil {
			return StatusBlocked, err.Error(), detected
		}
		return StatusReady, "configuration looks ready", detected
	case StepRoots:
		detected := map[string]any{"artifact_root": cfg.ArtifactRoot, "work_root": cfg.WorkRoot, "evidence_root": cfg.EvidenceRoot}
		if strings.TrimSpace(cfg.ArtifactRoot) == "" || strings.TrimSpace(cfg.WorkRoot) == "" || strings.TrimSpace(cfg.EvidenceRoot) == "" {
			return StatusBlocked, "root paths are not fully configured", detected
		}
		return StatusReady, "roots can be created from the browser", detected
	case StepWorkingCopy:
		dirty, err := workingCopyDirty(ctx)
		if err != nil {
			return StatusUnknown, err.Error(), map[string]any{}
		}
		if dirty && !cfg.AllowDirty {
			return StatusBlocked, "working copy is dirty", map[string]any{"dirty": true}
		}
		return StatusReady, "working copy check is ready", map[string]any{"dirty": dirty, "allow_dirty": cfg.AllowDirty}
	case StepValidate, StepRun:
		if _, err := s.validateRuntimeConfig(ctx, cfg); err != nil {
			return StatusBlocked, err.Error(), map[string]any{}
		}
		return StatusReady, "ready to run", map[string]any{}
	case StepReport, StepCompare:
		if _, err := s.lastExperimentID(ctx); err != nil {
			return StatusBlocked, "a successful benchmark run is required first", map[string]any{}
		}
		return StatusReady, "ready to render stored results", map[string]any{}
	default:
		return StatusUnknown, "", map[string]any{}
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.Scope == "" {
		cfg.Scope = DefaultScope
	}
	if strings.TrimSpace(cfg.DatasetRoot) == "" {
		cfg.DatasetRoot = "benchmark/doc-processors/datasets"
	}
	if strings.TrimSpace(cfg.WorkRoot) == "" {
		cfg.WorkRoot = ".benchmark/work"
	}
	if strings.TrimSpace(cfg.EvidenceRoot) == "" {
		cfg.EvidenceRoot = ".benchmark/evidence"
	}
	if cfg.StoreID == 0 {
		cfg.StoreID = 1
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		cfg.TenantID = "benchmark"
	}
	if strings.TrimSpace(cfg.ReportFormat) == "" {
		cfg.ReportFormat = "markdown"
	}
	return cfg
}

func resolvePath(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(repoRoot(), path)
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if filepath.Base(wd) == "ChenWeb" {
		return wd
	}
	return filepath.Join(wd, "ChenWeb")
}

func setMetricsEnv(value string) func() {
	old, had := os.LookupEnv("DOC_BENCHMARK_METRICS_MODEL_NAME")
	_ = os.Setenv("DOC_BENCHMARK_METRICS_MODEL_NAME", value)
	return func() {
		if had {
			_ = os.Setenv("DOC_BENCHMARK_METRICS_MODEL_NAME", old)
			return
		}
		_ = os.Unsetenv("DOC_BENCHMARK_METRICS_MODEL_NAME")
	}
}

func workingCopyDirty(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "jj", "--repository", repoRoot(), "status")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("jj status: %w", err)
	}
	return !strings.Contains(string(raw), "working copy has no changes"), nil
}

func commandOutput(ctx context.Context, name string, args ...string) string {
	raw, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func extForFormat(format string) string {
	if strings.EqualFold(format, "json") {
		return "json"
	}
	return "md"
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pathKey(path string) string {
	switch path {
	case "":
		return "path"
	default:
		base := filepath.Base(path)
		if base == "." || base == string(filepath.Separator) {
			return "path"
		}
		return strings.ReplaceAll(base, "-", "_") + "_path"
	}
}
