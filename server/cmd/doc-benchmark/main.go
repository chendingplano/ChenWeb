package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	code := execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if errors.Is(ctx.Err(), context.Canceled) && code != 0 {
		code = 130
	}
	os.Exit(code)
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		emitError(stderr, "usage_error", errors.New("a command is required"))
		return 2
	}
	var err error
	switch args[0] {
	case "validate":
		err = validate(args[1:], stdout, stderr)
	case "run":
		err = runBenchmark(ctx, args[1:], stdout, stderr)
	case "compare":
		err = renderStored(ctx, args[1:], stdout, stderr, true)
	case "report":
		err = renderStored(ctx, args[1:], stdout, stderr, false)
	case "clean":
		err = clean(ctx, args[1:], stdout, stderr)
	default:
		emitError(stderr, "usage_error", fmt.Errorf("unknown command %q", args[0]))
		return 2
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, flag.ErrHelp) || errors.Is(err, errUsage) {
		emitError(stderr, "validation_error", err)
		return 2
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		emitError(stderr, "canceled", ctx.Err())
		return 130
	}
	emitError(stderr, "infrastructure_error", err)
	return 3
}

var errUsage = errors.New("invalid command arguments")

func validate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	experiment := fs.String("experiment", "", "experiment TOML path")
	datasets := fs.String("datasets-root", "benchmark/doc-processors/datasets", "dataset root")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if *experiment == "" {
		return fmt.Errorf("%w: --experiment is required", errUsage)
	}
	b, err := os.ReadFile(*experiment)
	if err != nil {
		return err
	}
	e, err := docbenchmark.LoadExperiment(b, *datasets)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	processors := map[string]string{}
	for p, h := range e.ProcessorCaseSetHashes {
		processors[string(p)] = h
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"dataset_id": e.DatasetID, "dataset_version": e.DatasetVersion, "dataset_hash": e.DatasetHash, "request_hash": e.RequestHash, "processor_case_set_hashes": processors})
}

type commonFlags struct{ config string }

func addCommon(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.config, "config", "config.toml", "ChenWeb config TOML")
	return c
}

func runBenchmark(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	experimentPath := fs.String("experiment", "", "experiment TOML path")
	datasets := fs.String("datasets-root", "benchmark/doc-processors/datasets", "dataset root")
	workRoot := fs.String("work-root", envOr("BENCHMARK_WORK_ROOT", ".benchmark/work"), "disposable work root")
	evidenceRoot := fs.String("evidence-root", envOr("BENCHMARK_EVIDENCE_ROOT", ".benchmark/evidence"), "immutable evidence root")
	artifactRoot := fs.String("artifact-root", os.Getenv("ARTIFACT_DIR"), "production artifact root")
	owner := fs.String("owner", hostname(), "lease owner")
	tenant := fs.String("tenant-id", "benchmark", "benchmark tenant ID")
	storeID := fs.Int64("store-id", envInt64("BENCHMARK_STORE_ID", 1), "kb store ID")
	allowDirty := fs.Bool("allow-dirty", false, "allow a non-reproducible dirty working copy")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if *experimentPath == "" || *artifactRoot == "" {
		return fmt.Errorf("%w: --experiment and --artifact-root (or ARTIFACT_DIR) are required", errUsage)
	}
	// Production processors read ARTIFACT_DIR when their dependency graph is
	// initialized. Keep that process-local value identical to the adapter root;
	// otherwise the controller can write artifacts that the benchmark cannot
	// reconcile.
	if err := os.Setenv("ARTIFACT_DIR", filepath.Clean(*artifactRoot)); err != nil {
		return fmt.Errorf("set ARTIFACT_DIR: %w", err)
	}
	dirty, err := workingCopyDirty(ctx)
	if err != nil {
		return err
	}
	if dirty && !*allowDirty {
		return fmt.Errorf("%w: working copy is dirty; commit it or pass --allow-dirty", errUsage)
	}
	raw, err := os.ReadFile(*experimentPath)
	if err != nil {
		return err
	}
	db, err := bootstrap(ctx, common.config)
	if err != nil {
		return err
	}
	app := docbenchmark.Application{Config: docbenchmark.ApplicationConfig{DB: db, DatasetRoot: *datasets, WorkRoot: *workRoot, EvidenceRoot: *evidenceRoot, ArtifactRoot: *artifactRoot, Owner: *owner, TenantID: *tenant, StoreID: *storeID}}
	prepared, err := app.Prepare(ctx, raw)
	if err != nil {
		return err
	}
	executable, _ := os.Executable()
	executableHash, _ := docbenchmark.ExecutableSHA256(executable)
	gitCommit := commandOutput(ctx, "git", "rev-parse", "HEAD")
	jjChange := commandOutput(ctx, "jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
	store := docbenchmark.SQLStore{DB: db}
	for _, name := range prepared.VariantOrder {
		stored, err := store.GetRun(ctx, prepared.Runs[name].RunID)
		if err != nil {
			return err
		}
		if stored.Lifecycle == "succeeded" || stored.Lifecycle == "failed" || stored.Lifecycle == "canceled" {
			continue
		}
		if err := store.AttachRunProvenance(ctx, prepared.Runs[name].RunID, gitCommit, jjChange, executable, executableHash, dirty, prepared.Experiment.MaxParallelCases); err != nil {
			return err
		}
	}
	for _, name := range prepared.VariantOrder {
		run := prepared.Runs[name]
		worker := docbenchmark.VariantWorker{Application: app, Experiment: prepared.Experiment, Run: run}
		if err := worker.RunCases(ctx); err != nil {
			return fmt.Errorf("variant %s: %w", name, err)
		}
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"experiment_id": prepared.ExperimentID, "variants": prepared.VariantOrder, "dirty": dirty})
}

func renderStored(ctx context.Context, args []string, stdout, stderr io.Writer, comparison bool) error {
	name := "report"
	if comparison {
		name = "compare"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	experimentID := fs.String("experiment-id", "", "stored experiment UUID")
	baseline := fs.String("baseline", "", "baseline variant")
	candidate := fs.String("candidate", "", "candidate variant")
	format := fs.String("format", "json", "json or markdown")
	output := fs.String("output", "", "output file (stdout when omitted)")
	allow := fs.Bool("allow-incompatible", false, "render an explicitly incompatible comparison")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if *experimentID == "" || comparison && (*baseline == "" || *candidate == "") {
		return fmt.Errorf("%w: --experiment-id is required; compare also requires --baseline and --candidate", errUsage)
	}
	if !comparison {
		*baseline, *candidate = "", ""
	}
	db, err := bootstrap(ctx, common.config)
	if err != nil {
		return err
	}
	report, err := (docbenchmark.SQLStore{DB: db}).BuildExperimentReport(ctx, *experimentID, *baseline, *candidate, *allow)
	if err != nil {
		return err
	}
	var raw []byte
	switch *format {
	case "json":
		raw, err = docbenchmark.RenderJSON(report)
	case "markdown", "md":
		raw = []byte(docbenchmark.RenderMarkdown(report))
	default:
		return fmt.Errorf("%w: --format must be json or markdown", errUsage)
	}
	if err != nil {
		return err
	}
	if *output == "" {
		_, err = stdout.Write(raw)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil && filepath.Dir(*output) != "." {
		return err
	}
	return os.WriteFile(*output, raw, 0o644)
}

func clean(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := addCommon(fs)
	experimentID := fs.String("experiment-id", "", "clean all retained workspaces for an experiment")
	attemptID := fs.String("attempt-id", "", "clean one attempt")
	discard := fs.Bool("discard-unverified", false, "explicitly discard an unverified attempt")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if (*experimentID == "") == (*attemptID == "") || *discard && *attemptID == "" {
		return fmt.Errorf("%w: choose exactly one of --experiment-id or --attempt-id; --discard-unverified requires --attempt-id", errUsage)
	}
	db, err := bootstrap(ctx, common.config)
	if err != nil {
		return err
	}
	query := `SELECT a.id,c.case_id,c.run_id,w.work_root,w.evidence_root,w.nonce,w.cleanup_state FROM kb.benchmark_case_attempts a JOIN kb.benchmark_case_runs c ON c.id=a.case_run_id JOIN kb.benchmark_runs r ON r.id=c.run_id JOIN kb.benchmark_workspaces w ON w.execution_attempt_id=a.id WHERE (NULLIF($1,'')::uuid IS NOT NULL AND r.experiment_id=NULLIF($1,'')::uuid) OR (NULLIF($2,'')::uuid IS NOT NULL AND a.id=NULLIF($2,'')::uuid) ORDER BY a.id`
	rows, err := db.QueryContext(ctx, query, *experimentID, *attemptID)
	if err != nil {
		return err
	}
	defer rows.Close()
	store := docbenchmark.SQLStore{DB: db}
	count := 0
	for rows.Next() {
		var attempt, caseID, runID, workRoot, evidenceRoot, nonce, state string
		if err := rows.Scan(&attempt, &caseID, &runID, &workRoot, &evidenceRoot, &nonce, &state); err != nil {
			return err
		}
		if state == "cleaned" {
			continue
		}
		allocation, err := docbenchmark.AllocateWorkspace(docbenchmark.WorkspaceConfig{WorkRoot: workRoot, EvidenceRoot: evidenceRoot, AttemptID: attempt, CaseID: caseID, RunID: runID, Nonce: nonce, Store: store})
		if err != nil {
			return err
		}
		err = allocation.Cleanup(docbenchmark.CleanupOptions{DiscardUnverified: *discard, Cleanup: func(tx docbenchmark.CleanupTx) error {
			if err := tx.DeleteProductionRows(); err != nil {
				return err
			}
			if err := tx.DeleteInput(); err != nil {
				return err
			}
			return tx.MarkState("files_pending", nil)
		}})
		_ = allocation.Close()
		if err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(map[string]int{"cleaned_attempts": count})
}

func bootstrap(ctx context.Context, configPath string) (*sql.DB, error) {
	_ = godotenv.Load()
	ApiUtils.LoadLibConfig("CWB_DOC_BENCHMARK")
	logger := loggerutil.CreateDefaultLogger("CWB_DOC_BENCHMARK")
	ctx = context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_DOC_BENCHMARK")
	if err := appconfig.LoadConfig(ctx, logger, configPath); err != nil {
		return nil, err
	}
	appconfig.NormalizeMigrationPaths(logger, configPath)
	if err := databaseutil.InitDB(ctx, ApiTypes.CommonConfig); err != nil {
		return nil, err
	}
	if err := appconfig.RunMigrations(ctx, logger); err != nil {
		return nil, err
	}
	if ApiTypes.ProjectDBHandle == nil {
		return nil, errors.New("project database handle is nil")
	}
	return ApiTypes.ProjectDBHandle, nil
}

func workingCopyDirty(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "jj", "status")
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

func emitError(w io.Writer, code string, err error) {
	var e errorEnvelope
	e.Error.Code, e.Error.Message = code, err.Error()
	_ = json.NewEncoder(w).Encode(e)
}
func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}
func envInt64(k string, fallback int64) int64 {
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(k)), 10, 64); err == nil {
		return v
	}
	return fallback
}
func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "doc-benchmark"
	}
	return h
}
