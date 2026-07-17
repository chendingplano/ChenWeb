package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

type fakeAttemptWorkspace struct {
	path  string
	order *[]string
}

func (w fakeAttemptWorkspace) Path() string { return w.path }
func (w fakeAttemptWorkspace) Capture(raw []byte, _ string) (Artifact, error) {
	*w.order = append(*w.order, "workspace.capture")
	return Artifact{Path: "/evidence/bundle.json", SHA256: sha256Hex(raw), SizeBytes: int64(len(raw)), Verified: true}, nil
}
func (w fakeAttemptWorkspace) Cleanup(CleanupOptions) error {
	*w.order = append(*w.order, "workspace.cleanup")
	return nil
}
func (w fakeAttemptWorkspace) Close() error { return nil }

type fakeExecutionAdapter struct{ order *[]string }

func (a fakeExecutionAdapter) Processor() Processor             { return ProcessorChunking }
func (a fakeExecutionAdapter) AllowedOverrides() map[string]any { return nil }
func (a fakeExecutionAdapter) Applicable(ExpectedOutput) bool   { return true }
func (a fakeExecutionAdapter) Capture(context.Context, int64) (any, error) {
	*a.order = append(*a.order, "adapter.capture")
	return ChunkCapture{Rows: []ChunkDBRow{{NormalLines: [][]int{{1}}, OverlapLines: [][]int{{}}, ChunkLines: []string{"line"}}}, File: []byte("overlap: []\nlines: [1]\n"), SourceMaxLine: 1}, nil
}
func (a fakeExecutionAdapter) Reconcile(v any) (any, error) {
	*a.order = append(*a.order, "adapter.reconcile")
	return ChunkAdapter{}.Reconcile(v)
}
func (a fakeExecutionAdapter) Cleanup(context.Context, int64) error {
	*a.order = append(*a.order, "adapter.cleanup")
	return nil
}

func executionFixture() (PreparedRun, CaseUnit, *Experiment) {
	unit := CaseUnit{CaseID: "case-1", Repetition: 1}
	caseData := DatasetCase{ManifestCase: ManifestCase{CaseID: "case-1", Processors: []Processor{ProcessorChunking}}, ExpectedOutput: ExpectedOutput{SchemaVersion: 1, Chunking: &ExpectedChunking{Chunks: []ExpectedChunk{{Sequence: 1, NormalLines: []int{1}}}}}, InputBytes: []byte("1\t1\tparagraph\tArial\t12\t[0,0,1,1]\tline"), ExpectedBytes: []byte(`{"schema_version":1,"chunking":{"protected_groups":[],"chunks":[{"sequence":1,"overlap_lines":[],"normal_lines":[1]}]}}`)}
	return PreparedRun{RunID: "run", CaseRuns: map[CaseUnit]string{unit: "case-run"}, Cases: map[CaseUnit]DatasetCase{unit: caseData}, Processors: map[CaseUnit][]Processor{unit: {ProcessorChunking}}}, unit, &Experiment{Timeout: time.Minute, AttemptLease: 2 * time.Minute, MaxAttempts: 2}
}

func TestApplicationExecuteCaseWiresCallbacksAndCleansAfterTerminal(t *testing.T) {
	var order []string
	runtime := &fakeApplicationRuntime{allowed: map[string][]string{"chunking": nil}, snapshot: resolvedSnapshotForTest()}
	run, unit, experiment := executionFixture()
	app := Application{Config: ApplicationConfig{
		DB: &sql.DB{}, Owner: "worker", WorkRoot: "/work", EvidenceRoot: "/evidence", StoreID: 1, AllowUnverifiedFixtureFiles: true,
		AttemptRunner: func(ctx context.Context, caseRunID string, cfg RunnerConfig, work AttemptWork) error {
			attempt := AttemptRecord{ID: "attempt", CaseRunID: caseRunID, Kind: "execution", StartedAt: sql.NullTime{Time: time.Now(), Valid: true}}
			if err := work.Execute(ctx, attempt); err != nil {
				return err
			}
			captured, err := work.Capture(ctx, attempt)
			if err != nil {
				return err
			}
			reconciled, err := work.Reconcile(captured)
			if err != nil {
				return err
			}
			if err := work.Score(ctx, attempt, reconciled); err != nil {
				return err
			}
			order = append(order, "terminal")
			return work.Cleanup(ctx, attempt)
		},
		AllocateWorkspace: func(WorkspaceConfig) (AttemptWorkspace, error) {
			order = append(order, "allocate")
			return fakeAttemptWorkspace{path: t.TempDir(), order: &order}, nil
		},
		Seed: func(context.Context, *sql.DB, SeedInputRequest) (SeededInput, error) {
			order = append(order, "seed")
			return SeededInput{ID: 42, ResultFilename: "/tmp/input.lines.txt", ParserName: "benchmark"}, nil
		},
		AdapterFactory: func(Processor, DatasetCase, SeededInput) Adapter { return fakeExecutionAdapter{order: &order} },
		ControllerExecutor: func(ctx context.Context, got ApplicationRuntime, payload []byte) error {
			order = append(order, "runtime")
			return got.RunEvent(ctx, payload)
		},
		InsertArtifact: func(context.Context, ArtifactRecord) error { order = append(order, "artifact.insert"); return nil },
		InsertScore: func(context.Context, ScoreRecord) error {
			if len(order) == 0 || order[len(order)-1] == "terminal" {
				t.Fatal("score after terminal")
			}
			order = append(order, "score.insert")
			return nil
		},
	}}
	configJSON := json.RawMessage(`{"chunk_size":100}`)
	session := VariantSession{Runtime: runtime, ConfigJSON: configJSON, ConfigHash: sha256Hex(configJSON)}
	if err := app.ExecuteCase(context.Background(), experiment, run, unit, session); err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{"allocate", "seed", "runtime", "adapter.capture", "workspace.capture", "artifact.insert", "adapter.reconcile"}
	if !reflect.DeepEqual(order[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("order=%v", order)
	}
	if order[len(order)-2] != "terminal" || order[len(order)-1] != "workspace.cleanup" {
		t.Fatalf("cleanup ordering=%v", order)
	}
}

func TestApplicationExecuteCaseRescoreLoadsVerifiedEvidenceWithoutRuntimeCall(t *testing.T) {
	var order []string
	runtime := &fakeApplicationRuntime{}
	run, unit, experiment := executionFixture()
	capture := ChunkCapture{Rows: []ChunkDBRow{{NormalLines: [][]int{{1}}, OverlapLines: [][]int{{}}, ChunkLines: []string{"line"}}}, File: []byte("overlap: []\nlines: [1]\n"), SourceMaxLine: 1}
	captureJSON, _ := canonicalJSON(capture)
	configJSON := json.RawMessage(`{"chunk_size":100}`)
	scorerJSON, scorerHash := scorerSnapshot([]Processor{ProcessorChunking})
	bundle := EvidenceBundle{SchemaVersion: 1, AttemptID: "source", InputSHA256: sha256Hex(run.Cases[unit].InputBytes), InputBytes: run.Cases[unit].InputBytes, ExpectedJSON: run.Cases[unit].ExpectedBytes, ConfigJSON: configJSON, ConfigHash: sha256Hex(configJSON), ScorerJSON: scorerJSON, ScorerHash: scorerHash, Processors: map[string]EvidenceProcessor{"chunking": {Capture: captureJSON, Actual: json.RawMessage(`{"chunks":[]}`)}}}
	app := Application{Config: ApplicationConfig{
		DB: &sql.DB{}, Owner: "worker", AllowUnverifiedFixtureFiles: true,
		AttemptRunner: func(ctx context.Context, _ string, _ RunnerConfig, work AttemptWork) error {
			attempt := AttemptRecord{ID: "rescore", Kind: "rescore", SourceExecutionAttemptID: sql.NullString{String: "source", Valid: true}, InputRecordID: sql.NullInt64{Int64: 42, Valid: true}}
			captured, err := work.Capture(ctx, attempt)
			if err != nil {
				return err
			}
			reconciled, err := work.Reconcile(captured)
			if err != nil {
				return err
			}
			return work.Score(ctx, attempt, reconciled)
		},
		LoadEvidence: func(context.Context, string) (EvidenceBundle, error) {
			order = append(order, "evidence.load")
			return bundle, nil
		},
		AdapterFactory: func(Processor, DatasetCase, SeededInput) Adapter { return fakeExecutionAdapter{order: &order} },
		InsertScore:    func(context.Context, ScoreRecord) error { order = append(order, "score.insert"); return nil },
	}}
	if err := app.ExecuteCase(context.Background(), experiment, run, unit, VariantSession{Runtime: runtime, ConfigJSON: configJSON, ConfigHash: sha256Hex(configJSON)}); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 0 || len(order) == 0 || order[0] != "evidence.load" {
		t.Fatalf("runtime calls=%d order=%v", runtime.calls, order)
	}
}

func TestVariantWorkerInitializesRuntimeOnceAndAttachesSnapshot(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	runtime := &fakeApplicationRuntime{allowed: map[string][]string{"chunking": nil}, snapshot: resolvedSnapshotForTest()}
	factoryCalls := 0
	app := Application{Config: ApplicationConfig{DB: db, Owner: "worker", AllowUnverifiedRuntimeHash: true, RuntimeFactory: func(context.Context, ExperimentVariant, []Processor) (ApplicationRuntime, error) {
		factoryCalls++
		return runtime, nil
	}}}
	mock.ExpectQuery("SELECT lifecycle FROM kb.benchmark_runs").WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{"lifecycle"}).AddRow("queued"))
	mock.ExpectExec("UPDATE kb.benchmark_runs SET resolved_json").WithArgs("run", []byte(`{"chunk_size":100}`), "cfg").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.benchmark_runs SET lifecycle='running'").WithArgs("run", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	worker := VariantWorker{Application: app, Experiment: &Experiment{Processors: []Processor{ProcessorChunking}}, Run: PreparedRun{RunID: "run", Variant: ExperimentVariant{Name: "base"}}}
	if err := worker.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := worker.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || worker.Session.ConfigHash != "cfg" {
		t.Fatalf("calls=%d session=%#v", factoryCalls, worker.Session)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func resolvedSnapshotForTest() docprocessing.ResolvedConfigSnapshot {
	return docprocessing.ResolvedConfigSnapshot{CanonicalJSON: []byte(`{"chunk_size":100}`), Hash: "cfg"}
}

func TestBenchmarkHeartbeatHonorsRunnerMaximum(t *testing.T) {
	if got := benchmarkHeartbeat(25 * time.Minute); got != time.Minute {
		t.Fatalf("heartbeat=%s", got)
	}
}
