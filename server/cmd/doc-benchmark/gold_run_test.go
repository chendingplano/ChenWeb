package main

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

func TestGoldRunEnvelopeIncludesProvenance(t *testing.T) {
	ds := &docbenchmark.CorpusDataset{
		Manifest: docbenchmark.CorpusManifest{
			DatasetID:      "dataset-a",
			DatasetVersion: "1.2.3",
		},
	}
	corpusCase := &docbenchmark.CorpusCase{CaseID: "case-7", ContentHash: "sha256:abc123"}
	results := []docbenchmark.GoldRunDocumentResult{{Document: "doc-a", RecordID: 42}}

	env := goldRunEnvelope(ds, corpusCase, false, []string{"generate_topics", "extract_metrics"}, results)
	got, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	want := `{"schema_version":2,"dataset":{"id":"dataset-a","version":"1.2.3","content_hash":"sha256:abc123"},"case_id":"case-7","selected_processors":["generate_topics","extract_metrics"],"dry_run":false,"results":[{"document":"doc-a","record_id":42}]}`
	if string(got) != want {
		t.Fatalf("envelope json = %s, want %s", got, want)
	}
}

func TestResolveProcessorSelectionUsesCanonicalRegistry(t *testing.T) {
	got, err := resolveProcessorSelection(" generate-topics, extract-metadata ")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"generate_topics", "extract_doc_metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveProcessorSelectionDeduplicatesInRegistryOrder(t *testing.T) {
	got, err := resolveProcessorSelection("extract_metrics, generate-topics, extract-metadata, generate_summaries, extract_metrics")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"generate_summaries", "generate_topics", "extract_doc_metadata", "extract_metrics"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDryRunEnvelopeHasNoSelectedProcessors(t *testing.T) {
	ds := &docbenchmark.CorpusDataset{
		Manifest: docbenchmark.CorpusManifest{
			DatasetID:      "dataset-a",
			DatasetVersion: "1.2.3",
		},
	}
	corpusCase := &docbenchmark.CorpusCase{CaseID: "case-7", ContentHash: "sha256:abc123"}

	env := goldRunEnvelope(ds, corpusCase, true, []string{"extract_metrics"}, nil)
	if !env.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if len(env.SelectedProcessors) != 0 {
		t.Fatalf("SelectedProcessors = %v, want empty for dry-run", env.SelectedProcessors)
	}
	got, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"schema_version":2,"dataset":{"id":"dataset-a","version":"1.2.3","content_hash":"sha256:abc123"},"case_id":"case-7","selected_processors":[],"dry_run":true,"results":null}` {
		t.Fatalf("json = %s, want selected_processors as []", got)
	}
}

func TestGoldRunProcessorResultEmitsNonemptyRows(t *testing.T) {
	out, err := json.Marshal(goldRunRowsResult([]map[string]any{{"id": 1, "name": "x"}}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"state":"rows","rows":[{"id":1,"name":"x"}]}`
	if string(out) != want {
		t.Fatalf("json = %s, want %s", out, want)
	}
}

func TestGoldRunProcessorResultEmitsRegisteredEmptyRows(t *testing.T) {
	out, err := json.Marshal(goldRunRowsResult([]map[string]any{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"state":"rows","rows":[]}`
	if string(out) != want {
		t.Fatalf("json = %s, want %s", out, want)
	}
}

func TestGoldRunProcessorResultEmitsNotRegisteredWithoutRows(t *testing.T) {
	result := goldRunNotRegisteredResult()
	if result.Rows != nil {
		t.Fatalf("Rows = %#v, want nil", result.Rows)
	}
	out, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"state":"not_registered"}`
	if string(out) != want {
		t.Fatalf("json = %s, want %s", out, want)
	}
}

func TestGoldRunDocumentRunErrorHasNoProcessorResults(t *testing.T) {
	doc := goldRunDocumentRunError("doc-a", 42, assertError("boom"))
	if doc.RunError != "boom" {
		t.Fatalf("RunError = %q, want boom", doc.RunError)
	}
	if doc.Results != nil {
		t.Fatalf("Results = %#v, want nil", doc.Results)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"document":"doc-a","record_id":42,"run_error":"boom"}`
	if string(out) != want {
		t.Fatalf("json = %s, want %s", out, want)
	}
}

func TestProcessorResultTablesIncludeSummariesAndTopics(t *testing.T) {
	tests := map[string]struct {
		table string
		fkCol string
	}{
		"generate_summaries": {table: "kb.summaries", fkCol: "input_record_id"},
		"generate_topics":    {table: "kb.topics", fkCol: "input_record_id"},
	}
	for processor, want := range tests {
		got, ok := processorResultTables[processor]
		if !ok {
			t.Fatalf("%s not registered in processorResultTables", processor)
		}
		if got.table != want.table || got.fkCol != want.fkCol {
			t.Fatalf("%s mapping = {%q %q}, want {%q %q}", processor, got.table, got.fkCol, want.table, want.fkCol)
		}
	}
}

func TestFetchProcessorResultsReturnsNonemptyRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM kb.metrics WHERE input_record_id = $1 ORDER BY id`)).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "metric_id", "payload"}).AddRow(1, "m-1", []byte(`{"x":1}`)))

	got, err := fetchProcessorResults(context.Background(), db, "extract_metrics", 9)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := []map[string]any{{"id": int64(1), "metric_id": "m-1", "payload": `{"x":1}`}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestFetchProcessorResultsReturnsRegisteredEmptySlice(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM kb.summaries WHERE input_record_id = $1 ORDER BY id`)).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id"}))

	got, err := fetchProcessorResults(context.Background(), db, "generate_summaries", 9)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want nonnil empty slice for registered processor")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
