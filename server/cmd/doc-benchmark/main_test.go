package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

func TestValidateEmitsMachineReadableSnapshot(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	t.Setenv("DOC_BENCHMARK_METRICS_MODEL_NAME", "deepseek-flash-chen")
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"validate", "--experiment", filepath.Join(root, "benchmark/doc-processors/experiments/example.toml"), "--datasets-root", filepath.Join(root, "benchmark/doc-processors/datasets")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got["dataset_hash"] == "" || got["request_hash"] == "" {
		t.Fatalf("output=%s err=%v", stdout.String(), err)
	}
}

// TestRunArtifactWebRootFlagSetsEnv confirms --artifact-web-root propagates
// to ARTIFACT_WEB_DIR the same way --artifact-root propagates to
// ARTIFACT_DIR (main.go), independent of whether the run itself succeeds --
// this environment has no live DB, so the run is expected to fail later; the
// behavior under test is the early flag-to-env propagation, not a full run.
func TestRunArtifactWebRootFlagSetsEnv(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	artifactRoot := t.TempDir()
	webRoot := filepath.Join(t.TempDir(), "web") // deliberately not pre-created
	t.Setenv("ARTIFACT_WEB_DIR", "")

	var stdout, stderr bytes.Buffer
	_ = execute(context.Background(), []string{
		"run",
		"--experiment", filepath.Join(root, "benchmark/doc-processors/experiments/example.toml"),
		"--artifact-root", artifactRoot,
		"--artifact-web-root", webRoot,
		"--allow-dirty",
	}, &stdout, &stderr)

	if got, want := os.Getenv("ARTIFACT_WEB_DIR"), filepath.Clean(webRoot); got != want {
		t.Fatalf("ARTIFACT_WEB_DIR = %q, want %q", got, want)
	}
}

func TestCommandValidationUsesStableJSONErrors(t *testing.T) {
	tests := [][]string{
		{"run"},
		{"compare", "--experiment-id", "x"},
		{"clean", "--discard-unverified", "--experiment-id", "x"},
		{"unknown"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if code := execute(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
		var envelope errorEnvelope
		if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Code == "" || envelope.Error.Message == "" {
			t.Fatalf("args=%v stderr=%q err=%v", args, stderr.String(), err)
		}
	}
}

func TestProfileReportCommandRejectsMissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"profile-report"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	assertValidationErrorContains(t, stderr.Bytes(), "--dataset, --case, and --results are required")
}

func TestProfileReportCommandRejectsDryRunEnvelope(t *testing.T) {
	root, runPath, _ := writeProfileReportFixture(t, func(env *docbenchmark.GoldRunEnvelope) {
		env.DryRun = true
	})

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{
		"profile-report",
		"--dataset", root,
		"--case", "case1",
		"--results", runPath,
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	assertValidationErrorContains(t, stderr.Bytes(), "dry_run")
}

func TestProfileReportCommandRendersJSON(t *testing.T) {
	root, runPath, _ := writeProfileReportFixture(t, nil)

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{
		"profile-report",
		"--dataset", root,
		"--case", "case1",
		"--results", runPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var report docbenchmark.ProfileReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != docbenchmark.ProfileReportSchemaVersion {
		t.Fatalf("schema_version=%d, want %d", report.SchemaVersion, docbenchmark.ProfileReportSchemaVersion)
	}
	if report.Dataset.ContentHash == "" || len(report.Rows) != 4 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestProfileReportCommandRendersMarkdownToFile(t *testing.T) {
	root, runPath, _ := writeProfileReportFixture(t, nil)
	outputPath := filepath.Join(t.TempDir(), "nested", "report.md")

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{
		"profile-report",
		"--dataset", root,
		"--case", "case1",
		"--results", runPath,
		"--format", "markdown",
		"--output", outputPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q, want empty when writing file", stdout.String())
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"# Store-profile report",
		"test-corpus",
		"product-specification",
		"extract_metrics",
		"Structural yield is not semantic correctness.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}

func TestParseGoldRunEnvelopeRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseGoldRunEnvelope([]byte(`{"schema_version":2`)); err == nil || !strings.Contains(err.Error(), "parse gold-run JSON") {
		t.Fatalf("err=%v, want parse gold-run JSON", err)
	}
}

func TestParseGoldRunEnvelopeRejectsUnknownTopLevelField(t *testing.T) {
	raw := []byte(`{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[],"extra":true}`)
	if _, err := ParseGoldRunEnvelope(raw); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%v, want unknown field", err)
	}
}

func TestParseGoldRunEnvelopeRejectsUnknownNestedField(t *testing.T) {
	raw := []byte(`{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[{"document":"doc:a","record_id":1,"results":{"extract_metrics":{"state":"rows","rows":[],"extra":true}}}]}`)
	if _, err := ParseGoldRunEnvelope(raw); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%v, want unknown field", err)
	}
}

func TestParseGoldRunEnvelopeRejectsMultipleJSONValues(t *testing.T) {
	raw := []byte(`{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[]} null`)
	if _, err := ParseGoldRunEnvelope(raw); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("err=%v, want multiple JSON values", err)
	}
}

func TestParseGoldRunEnvelopeRejectsUnversionedShape(t *testing.T) {
	raw := []byte(`{"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[]}`)
	if _, err := ParseGoldRunEnvelope(raw); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("err=%v, want schema_version error", err)
	}
}

func TestParseGoldRunEnvelopeRejectsInvalidStateCombinations(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "rows without rows field",
			raw:  `{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[{"document":"doc:a","record_id":1,"results":{"extract_metrics":{"state":"rows"}}}]}`,
			want: "rows state requires rows field",
		},
		{
			name: "not registered with rows field",
			raw:  `{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[{"document":"doc:a","record_id":1,"results":{"extract_metrics":{"state":"not_registered","rows":[]}}}]}`,
			want: "not_registered state must omit rows field",
		},
		{
			name: "run error with processor results",
			raw:  `{"schema_version":2,"dataset":{"id":"d","version":"1.0.0","content_hash":"sha256:x"},"case_id":"case1","selected_processors":["extract_metrics"],"dry_run":false,"results":[{"document":"doc:a","record_id":1,"run_error":"boom","results":{"extract_metrics":{"state":"rows","rows":[]}}}]}`,
			want: "run_error must not include processor results",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseGoldRunEnvelope([]byte(tt.raw)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestProfileReportCommandRejectsUnsupportedFormatAsValidationError(t *testing.T) {
	root, runPath, _ := writeProfileReportFixture(t, nil)

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{
		"profile-report",
		"--dataset", root,
		"--case", "case1",
		"--results", runPath,
		"--format", "html",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	assertValidationErrorContains(t, stderr.Bytes(), "--format must be json or markdown")
}

const testProfileReportGoldTOML = `
[[metric_definition]]
id = "m1"
quantity_kind = "Time"

[[authority_document]]
id = "doc:ent-q-syn-001-2026"
family = "enterprise"
title = "Subject standard"

[[authority_document]]
id = "doc:cn"
family = "cn_national"
title = "CN standard"

[[clause]]
id = "c1"
document = "doc:ent-q-syn-001-2026"
metric = "m1"
form = "upper_bound"
value = 120
unit = "ms"
text_template = "Subject clause."

[[clause]]
id = "c2"
document = "doc:cn"
metric = "m1"
form = "upper_bound"
value = 150
unit = "ms"
text_template = "CN clause."
`

func writeProfileReportFixture(t *testing.T, mutate func(*docbenchmark.GoldRunEnvelope)) (string, string, docbenchmark.GoldRunEnvelope) {
	t.Helper()
	root := t.TempDir()
	manifest := `{
		"schema_version": 1,
		"dataset_id": "test-corpus",
		"dataset_version": "1.0.0",
		"cases": [{
			"case_id": "case1",
			"gold": "gold.toml",
			"document_profiles": {
				"doc:ent-q-syn-001-2026": {
					"store_profile": "product-specification",
					"document_kind": "enterprise-standard",
					"expected_processors": {
						"extract_metrics": "required",
						"extract_provisions": "useful"
					}
				},
				"doc:cn": {
					"store_profile": "regulated-reference",
					"document_kind": "authority-standard",
					"expected_processors": {
						"extract_metrics": "useful",
						"extract_provisions": "required"
					}
				}
			}
		}]
	}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "gold.toml"), []byte(testProfileReportGoldTOML), 0o644); err != nil {
		t.Fatalf("write gold: %v", err)
	}
	ds, err := docbenchmark.LoadCorpusDataset(root)
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}
	run := docbenchmark.GoldRunEnvelope{
		SchemaVersion:      docbenchmark.GoldRunSchemaVersion,
		Dataset:            docbenchmark.GoldRunDatasetIdentity{ID: ds.Manifest.DatasetID, Version: ds.Manifest.DatasetVersion, ContentHash: ds.Cases[0].ContentHash},
		CaseID:             "case1",
		SelectedProcessors: []string{"extract_metrics", "extract_provisions"},
		DryRun:             false,
		Results: []docbenchmark.GoldRunDocumentResult{
			{
				Document: "doc:cn",
				RecordID: 1,
				Results: map[string]docbenchmark.GoldRunProcessorResult{
					"extract_metrics":    {State: docbenchmark.GoldRunResultRows, Rows: rowsPtr([]map[string]any{})},
					"extract_provisions": {State: docbenchmark.GoldRunResultRows, Rows: rowsPtr([]map[string]any{{"id": "p1"}})},
				},
			},
			{
				Document: "doc:ent-q-syn-001-2026",
				RecordID: 2,
				Results: map[string]docbenchmark.GoldRunProcessorResult{
					"extract_metrics":    {State: docbenchmark.GoldRunResultRows, Rows: rowsPtr([]map[string]any{{"id": "m1"}})},
					"extract_provisions": {State: docbenchmark.GoldRunResultNotRegistered},
				},
			},
		},
	}
	if mutate != nil {
		mutate(&run)
	}
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	runPath := filepath.Join(root, "run.json")
	if err := os.WriteFile(runPath, raw, 0o644); err != nil {
		t.Fatalf("write run: %v", err)
	}
	return root, runPath, run
}

func rowsPtr(rows []map[string]any) *[]map[string]any {
	return &rows
}

func assertValidationErrorContains(t *testing.T, raw []byte, want string) {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal error envelope: %v\n%s", err, string(raw))
	}
	if envelope.Error.Code != "validation_error" {
		t.Fatalf("error code=%q, want validation_error", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, want) {
		t.Fatalf("error message=%q, want substring %q", envelope.Error.Message, want)
	}
}
