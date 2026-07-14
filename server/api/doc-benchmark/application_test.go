package docbenchmark

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

type fakeApplicationRuntime struct {
	allowed  map[string][]string
	snapshot docprocessing.ResolvedConfigSnapshot
	calls    int
}

func (f *fakeApplicationRuntime) AllowedOverrides() map[string][]string { return f.allowed }
func (f *fakeApplicationRuntime) ResolvedConfig() docprocessing.ResolvedConfigSnapshot {
	return f.snapshot
}
func (f *fakeApplicationRuntime) RunEvent(context.Context, []byte) error { f.calls++; return nil }

func TestApplicationInitializeVariantValidatesRuntimeAllowlistAndReturnsRedactedSnapshot(t *testing.T) {
	runtime := &fakeApplicationRuntime{
		allowed:  map[string][]string{"chunking": {"CHUNK_SIZE"}},
		snapshot: docprocessing.ResolvedConfigSnapshot{Values: map[string]any{"chunk_size": 80}, CanonicalJSON: []byte(`{"chunk_size":80}`), Hash: "cfg-hash"},
	}
	factoryCalls := 0
	app := Application{Config: ApplicationConfig{RuntimeFactory: func(context.Context, ExperimentVariant, []Processor) (ApplicationRuntime, error) {
		factoryCalls++
		return runtime, nil
	}}}

	session, err := app.InitializeVariant(context.Background(), ExperimentVariant{Name: "base", Overrides: map[string]string{"CHUNK_SIZE": "80"}}, []Processor{ProcessorChunking})
	if err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || session.ConfigHash != "cfg-hash" || string(session.ConfigJSON) != `{"chunk_size":80}` {
		t.Fatalf("session=%#v factory calls=%d", session, factoryCalls)
	}
	encoded, _ := json.Marshal(session)
	if strings.Contains(strings.ToLower(string(encoded)), "secret") || strings.Contains(string(encoded), "CHUNK_SIZE") {
		t.Fatalf("session serialized requested values or secrets: %s", encoded)
	}

	_, err = app.InitializeVariant(context.Background(), ExperimentVariant{Name: "bad", Overrides: map[string]string{"NOT_ALLOWED": "x"}}, []Processor{ProcessorChunking})
	if err == nil || !strings.Contains(err.Error(), "NOT_ALLOWED") {
		t.Fatalf("allowlist error=%v", err)
	}
}

func TestApplicationInitializeVariantRejectsSecretShapedResolvedSnapshot(t *testing.T) {
	runtime := &fakeApplicationRuntime{
		allowed:  map[string][]string{"chunking": nil},
		snapshot: docprocessing.ResolvedConfigSnapshot{Values: map[string]any{"api_token": "do-not-store"}, CanonicalJSON: []byte(`{"api_token":"do-not-store"}`), Hash: "bad"},
	}
	app := Application{Config: ApplicationConfig{RuntimeFactory: func(context.Context, ExperimentVariant, []Processor) (ApplicationRuntime, error) { return runtime, nil }}}
	if _, err := app.InitializeVariant(context.Background(), ExperimentVariant{Name: "bad"}, []Processor{ProcessorChunking}); err == nil {
		t.Fatal("secret-shaped snapshot accepted")
	}
}

func TestMetricConversionsPreservePresenceEmptyStableJSONAndSourceLines(t *testing.T) {
	empty := ""
	explicit := false
	gold := GoldMetric{GoldID: "g1", MetricName: &empty, IsExplicitMetric: &explicit, MetricCategories: json.RawMessage(`[]`), SourceLines: []int{4, 2, 4}}
	record, err := goldMetricRecord(gold)
	if err != nil {
		t.Fatal(err)
	}
	if record.Name == nil || *record.Name != "" || record.Subject != nil || record.IsExplicitMetric == nil || *record.IsExplicitMetric {
		t.Fatalf("presence lost: %#v", record)
	}
	if string(record.StableFields["metric_categories"]) != "[]" || !reflect.DeepEqual(record.SourceLines, []int{4, 2, 4}) {
		t.Fatalf("stable fields/source lines lost: %#v", record)
	}

	pred, err := metricActualRecords(MetricActual{Rows: []map[string]any{{
		"metric_id": "p1", "metric_name": "", "metric_subject": nil,
		"source_line_spans": []any{map[string]any{"start": float64(7), "end": float64(8)}},
		"objects":           []any{},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pred) != 1 || pred[0].PredictionInputIndex != 0 || pred[0].Name == nil || *pred[0].Name != "" || pred[0].Subject != nil || !reflect.DeepEqual(pred[0].SourceLines, []int{7, 8}) || string(pred[0].StableFields["objects"]) != "[]" {
		t.Fatalf("prediction conversion lost fields: %#v", pred)
	}
}

func TestScoreRecordConversionPersistsEveryMetricRowAndAdditiveComponent(t *testing.T) {
	v := .75
	rows := []ScoreRow{{Metric: "detection_f1", Direction: "higher", AggregationKind: "count_derived_micro", Value: &v, Numerator: 3, Denominator: 4}, {Metric: "detection", Component: "tp", Direction: "higher", AggregationKind: "additive", Value: &v, Numerator: 3, Denominator: 1, ConditionalAttribution: true}}
	records, err := metricScoreRecords("attempt-1", rows, json.RawMessage(`{"diagnostic":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].AttemptID.String != "attempt-1" || records[0].Processor != "extract_metrics" || records[0].ScorerVersion != MetricScorerVersion || !records[0].Value.Valid || records[0].Numerator.Float64 != 3 || !records[0].Applicable {
		t.Fatalf("primary record=%#v", records[0])
	}
	if records[1].Metric != "detection" || records[1].Slice != "tp" || !records[1].AdditiveComponent.Valid || records[1].AdditiveComponent.Float64 != .75 || !records[1].Applicable {
		t.Fatalf("additive record=%#v", records[1])
	}
}

func TestApplicationRuntimeFactoryErrorIsReturned(t *testing.T) {
	want := errors.New("factory")
	app := Application{Config: ApplicationConfig{RuntimeFactory: func(context.Context, ExperimentVariant, []Processor) (ApplicationRuntime, error) { return nil, want }}}
	if _, err := app.InitializeVariant(context.Background(), ExperimentVariant{Name: "v"}, []Processor{ProcessorChunking}); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestLineFileGeneratedPayloadUsesExactSeededInputAndApplicableOperations(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "case.lines.txt")
	payload, err := lineFileGeneratedPayload(91, filename, []Processor{ProcessorChunking, ProcessorExtractMetrics})
	if err != nil {
		t.Fatal(err)
	}
	event, err := docprocessing.ParseLineFileGeneratedEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.RecordID != 91 || event.Filename != filename || !event.Force || !event.ForceClear || event.Type != "pdf" || event.Status != "success" || !reflect.DeepEqual(event.Operations, []string{"chunking", "extract_metrics"}) {
		t.Fatalf("event=%#v payload=%s", event, payload)
	}
}

func TestEvidenceBundleIsDeterministicSecretFreeAndReverifiedBeforeRescore(t *testing.T) {
	bundle := EvidenceBundle{SchemaVersion: 1, AttemptID: "a", InputSHA256: strings.Repeat("a", 64), InputBytes: []byte("input"), ExpectedJSON: json.RawMessage(`{"schema_version":1}`), ConfigHash: "cfg", ScorerHash: "score", Processors: map[string]EvidenceProcessor{"chunking": {Capture: json.RawMessage(`{"rows":[]}`)}}}
	one, err := bundle.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	two, _ := bundle.CanonicalJSON()
	if !reflect.DeepEqual(one, two) || strings.Contains(strings.ToLower(string(one)), "secret") {
		t.Fatalf("non-deterministic/secret bundle: %s", one)
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, one, 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha256Hex(one)
	loaded, err := loadVerifiedEvidence(path, hash, int64(len(one)))
	if err != nil || loaded.AttemptID != "a" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	if err := os.WriteFile(path, append(one, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadVerifiedEvidence(path, hash, int64(len(one))); err == nil {
		t.Fatal("tampered source evidence accepted")
	}
}

func TestChunkScoreRecordConversionPersistsAllScalarAndRuleRows(t *testing.T) {
	score := ScoreChunks(ChunkScoreInput{})
	records, err := chunkScoreRecords("attempt", score)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 14 {
		t.Fatalf("only %d chunk score rows", len(records))
	}
	for _, record := range records {
		if record.AttemptID.String != "attempt" || record.Processor != "chunking" || record.Scorer == "" || record.Direction == "" || record.AggregationKind == "" || !record.Applicable {
			t.Fatalf("invalid row: %#v", record)
		}
	}
}

func TestApplicationPrepareCreatesLexicalVariantRunsAndExactApplicableCaseUnits(t *testing.T) {
	datasetRoot := t.TempDir()
	versionRoot := filepath.Join(datasetRoot, "doc-processors-synthetic-core", "1.0.0")
	mustWrite(t, filepath.Join(versionRoot, "manifest.json"), validManifest())
	mustWrite(t, filepath.Join(versionRoot, "cases/metric-001/input.lines.txt"), validInput())
	mustWrite(t, filepath.Join(versionRoot, "cases/metric-001/expected.json"), validExpected())
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("INSERT INTO kb.benchmark_experiments").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("experiment-id"))
	// Input order is zeta, alpha; persisted order must be lexical.
	mock.ExpectQuery("INSERT INTO kb.benchmark_runs").WithArgs("experiment-id", "alpha", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("run-alpha"))
	mock.ExpectQuery("INSERT INTO kb.benchmark_case_runs").WithArgs("run-alpha", "metric-001", 1, "applicable", sqlmock.AnyArg(), (*string)(nil)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("case-alpha"))
	mock.ExpectQuery("INSERT INTO kb.benchmark_runs").WithArgs("experiment-id", "zeta", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("run-zeta"))
	mock.ExpectQuery("INSERT INTO kb.benchmark_case_runs").WithArgs("run-zeta", "metric-001", 1, "applicable", sqlmock.AnyArg(), (*string)(nil)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("case-zeta"))

	raw := []byte(`name="application"
dataset="doc-processors-synthetic-core@1.0.0"
processors=["chunking"]
repetitions=1
[[variants]]
name="zeta"
[[variants]]
name="alpha"
`)
	prepared, err := (Application{Config: ApplicationConfig{DB: db, DatasetRoot: datasetRoot}}).Prepare(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.ExperimentID != "experiment-id" || !reflect.DeepEqual(prepared.VariantOrder, []string{"alpha", "zeta"}) || prepared.Runs["alpha"].CaseRuns[CaseUnit{CaseID: "metric-001", Repetition: 1}] != "case-alpha" {
		t.Fatalf("prepared=%#v", prepared)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
