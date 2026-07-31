package docbenchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestBuildProfileReportRepresentativeFixture(t *testing.T) {
	ds, run := representativeProfileReportFixture(t)

	report, err := BuildProfileReport(ds, "display-module-v1", run)
	if err != nil {
		t.Fatalf("BuildProfileReport: %v", err)
	}
	if report.SchemaVersion != ProfileReportSchemaVersion {
		t.Fatalf("SchemaVersion = %d", report.SchemaVersion)
	}
	if len(report.Rows) != 9 {
		t.Fatalf("got %d rows, want 9", len(report.Rows))
	}

	want := map[string]ProfileReportRow{
		"narrative-research|marketing-narrative|extract_inventory_items": {
			StoreProfile:        "narrative-research",
			DocumentKind:        "marketing-narrative",
			Processor:           "extract_inventory_items",
			Documents:           1,
			SuccessfulDocuments: 1,
			FailedDocuments:     0,
			DocumentsWithOutput: 1,
			OutputRows:          1,
			NotRegistered:       0,
			Applicability:       "not_required",
			EvidenceKind:        "structural_yield",
			Assessment:          "informational_not_required",
		},
		"narrative-research|marketing-narrative|extract_provisions": {
			StoreProfile:        "narrative-research",
			DocumentKind:        "marketing-narrative",
			Processor:           "extract_provisions",
			Documents:           1,
			SuccessfulDocuments: 1,
			FailedDocuments:     0,
			DocumentsWithOutput: 0,
			OutputRows:          0,
			NotRegistered:       1,
			Applicability:       "not_required",
			EvidenceKind:        "structural_yield",
			Assessment:          "informational_not_required",
		},
		"narrative-research|marketing-narrative|generate_summaries": {
			StoreProfile:        "narrative-research",
			DocumentKind:        "marketing-narrative",
			Processor:           "generate_summaries",
			Documents:           1,
			SuccessfulDocuments: 1,
			FailedDocuments:     0,
			DocumentsWithOutput: 1,
			OutputRows:          1,
			NotRegistered:       0,
			Applicability:       "required",
			EvidenceKind:        "structural_yield",
			Assessment:          "required_output_observed",
		},
		"product-specification|enterprise-standard|extract_inventory_items": {
			StoreProfile:        "product-specification",
			DocumentKind:        "enterprise-standard",
			Processor:           "extract_inventory_items",
			Documents:           3,
			SuccessfulDocuments: 2,
			FailedDocuments:     1,
			DocumentsWithOutput: 1,
			OutputRows:          1,
			NotRegistered:       0,
			Applicability:       "required",
			EvidenceKind:        "structural_yield",
			Assessment:          "required_failure",
		},
		"product-specification|enterprise-standard|extract_provisions": {
			StoreProfile:        "product-specification",
			DocumentKind:        "enterprise-standard",
			Processor:           "extract_provisions",
			Documents:           3,
			SuccessfulDocuments: 2,
			FailedDocuments:     1,
			DocumentsWithOutput: 2,
			OutputRows:          2,
			NotRegistered:       0,
			Applicability:       "useful",
			EvidenceKind:        "structural_yield",
			Assessment:          "useful_review_warning",
		},
		"product-specification|enterprise-standard|generate_summaries": {
			StoreProfile:        "product-specification",
			DocumentKind:        "enterprise-standard",
			Processor:           "generate_summaries",
			Documents:           3,
			SuccessfulDocuments: 2,
			FailedDocuments:     1,
			DocumentsWithOutput: 2,
			OutputRows:          2,
			NotRegistered:       0,
			Applicability:       "not_required",
			EvidenceKind:        "structural_yield",
			Assessment:          "informational_not_required",
		},
		"regulated-reference|authority-standard|extract_inventory_items": {
			StoreProfile:        "regulated-reference",
			DocumentKind:        "authority-standard",
			Processor:           "extract_inventory_items",
			Documents:           5,
			SuccessfulDocuments: 5,
			FailedDocuments:     0,
			DocumentsWithOutput: 5,
			OutputRows:          5,
			NotRegistered:       0,
			Applicability:       "not_required",
			EvidenceKind:        "structural_yield",
			Assessment:          "informational_not_required",
		},
		"regulated-reference|authority-standard|extract_provisions": {
			StoreProfile:        "regulated-reference",
			DocumentKind:        "authority-standard",
			Processor:           "extract_provisions",
			Documents:           5,
			SuccessfulDocuments: 5,
			FailedDocuments:     0,
			DocumentsWithOutput: 5,
			OutputRows:          5,
			NotRegistered:       0,
			Applicability:       "required",
			EvidenceKind:        "structural_yield",
			Assessment:          "required_output_observed",
		},
		"regulated-reference|authority-standard|generate_summaries": {
			StoreProfile:        "regulated-reference",
			DocumentKind:        "authority-standard",
			Processor:           "generate_summaries",
			Documents:           5,
			SuccessfulDocuments: 5,
			FailedDocuments:     0,
			DocumentsWithOutput: 5,
			OutputRows:          5,
			NotRegistered:       0,
			Applicability:       "not_required",
			EvidenceKind:        "structural_yield",
			Assessment:          "informational_not_required",
		},
	}
	for _, row := range report.Rows {
		key := row.StoreProfile + "|" + row.DocumentKind + "|" + row.Processor
		if !reflect.DeepEqual(row, want[key]) {
			t.Fatalf("row[%s] = %#v, want %#v", key, row, want[key])
		}
	}
}

func TestBuildProfileReportValidation(t *testing.T) {
	baseDS, baseRun := representativeProfileReportFixture(t)
	tests := []struct {
		name   string
		mutate func(ds *CorpusDataset, run *GoldRunEnvelope)
		want   string
	}{
		{"unsupported schema version", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.SchemaVersion = 99 }, "schema_version"},
		{"missing schema version", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.SchemaVersion = 0 }, "schema_version"},
		{"dry run", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.DryRun = true }, "dry_run"},
		{"dataset id mismatch", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Dataset.ID = "wrong" }, "dataset.id"},
		{"dataset version mismatch", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Dataset.Version = "wrong" }, "dataset.version"},
		{"case id mismatch", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.CaseID = "wrong" }, "case_id"},
		{"content hash mismatch", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Dataset.ContentHash = "sha256:wrong" }, "dataset.content_hash"},
		{"unknown selected processor", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.SelectedProcessors = append([]string{}, "unknown") }, "unknown selected processor"},
		{"duplicate selected processor", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			run.SelectedProcessors = []string{"generate_summaries", "generate_summaries"}
		}, "duplicate selected processor"},
		{"selected processors out of order", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			run.SelectedProcessors = []string{"extract_provisions", "generate_summaries"}
		}, "out of canonical order"},
		{"duplicate result document", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Results = append(run.Results, run.Results[0]) }, "duplicate result document"},
		{"missing result document", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Results = run.Results[:len(run.Results)-1] }, "missing result document"},
		{"unknown result document", func(_ *CorpusDataset, run *GoldRunEnvelope) { run.Results[0].Document = "doc:ghost" }, "unknown document"},
		{"result for unselected processor", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:cn-gb-syn-9706-1-2020")
			result.Results["generate_topics"] = result.Results["generate_summaries"]
		}, "processor not selected"},
		{"missing result for selected processor", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:cn-gb-syn-9706-1-2020")
			delete(result.Results, "extract_provisions")
		}, "missing result for selected processor"},
		{"unknown processor-result state", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:cn-gb-syn-9706-1-2020")
			bad := result.Results["generate_summaries"]
			bad.State = "mystery"
			result.Results["generate_summaries"] = bad
		}, "unknown processor-result state"},
		{"rows state missing rows field", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:cn-gb-syn-9706-1-2020")
			bad := result.Results["generate_summaries"]
			bad.Rows = nil
			result.Results["generate_summaries"] = bad
		}, "rows state requires rows field"},
		{"not_registered state with rows field", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:ent-mkt-syn-2025")
			bad := result.Results["extract_provisions"]
			rows := []map[string]any{}
			bad.Rows = &rows
			result.Results["extract_provisions"] = bad
		}, "not_registered state must omit rows field"},
		{"run_error with processor results", func(_ *CorpusDataset, run *GoldRunEnvelope) {
			result := fixtureResult(run, "doc:ent-q-syn-001-2026")
			result.RunError = "boom"
			result.Results = map[string]GoldRunProcessorResult{"generate_summaries": rowsResult([]map[string]any{{"id": 1}})}
		}, "run_error must not include processor results"},
		{"inconsistent applicability", func(ds *CorpusDataset, _ *GoldRunEnvelope) {
			ds.Cases[0].DocumentProfiles["doc:ent-q-syn-001-2019"] = mutateApplicability(ds.Cases[0].DocumentProfiles["doc:ent-q-syn-001-2019"], "extract_provisions", ProcessorRequired)
		}, "inconsistent applicability"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, run := cloneFixture(baseDS, baseRun)
			tt.mutate(ds, &run)
			_, err := BuildProfileReport(ds, "display-module-v1", run)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestProfileReportRenderDeterministicAndGolden(t *testing.T) {
	ds, run := representativeProfileReportFixture(t)
	report, err := BuildProfileReport(ds, "display-module-v1", run)
	if err != nil {
		t.Fatalf("BuildProfileReport: %v", err)
	}

	jsonA, err := RenderProfileReportJSON(report)
	if err != nil {
		t.Fatalf("RenderProfileReportJSON: %v", err)
	}
	jsonB, err := RenderProfileReportJSON(report)
	if err != nil {
		t.Fatalf("RenderProfileReportJSON second: %v", err)
	}
	if string(jsonA) != string(jsonB) {
		t.Fatal("non-deterministic JSON render")
	}
	if !strings.HasSuffix(string(jsonA), "\n") {
		t.Fatal("JSON must end with one newline")
	}

	mdA := RenderProfileReportMarkdown(report)
	mdB := RenderProfileReportMarkdown(report)
	if mdA != mdB {
		t.Fatal("non-deterministic markdown render")
	}
	if !strings.Contains(mdA, "Structural yield is not semantic correctness.") {
		t.Fatal("markdown missing structural yield note")
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "profile-report.golden.json"), jsonA, 0o644); err != nil {
			t.Fatalf("write JSON golden: %v", err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "profile-report.golden.md"), []byte(mdA), 0o644); err != nil {
			t.Fatalf("write Markdown golden: %v", err)
		}
	}

	jsonGolden, err := os.ReadFile(filepath.Join("testdata", "profile-report.golden.json"))
	if err != nil {
		t.Fatalf("read JSON golden: %v", err)
	}
	mdGolden, err := os.ReadFile(filepath.Join("testdata", "profile-report.golden.md"))
	if err != nil {
		t.Fatalf("read Markdown golden: %v", err)
	}
	if string(jsonA) != string(jsonGolden) {
		t.Fatalf("JSON render mismatch\nGOT:\n%s\nWANT:\n%s", jsonA, jsonGolden)
	}
	if mdA != string(mdGolden) {
		t.Fatalf("Markdown render mismatch\nGOT:\n%s\nWANT:\n%s", mdA, mdGolden)
	}
}

func representativeProfileReportFixture(t *testing.T) (*CorpusDataset, GoldRunEnvelope) {
	t.Helper()
	ds, err := LoadCorpusDataset("../../../benchmark/doc-processors/gold/display-module-v1")
	if err != nil {
		t.Fatalf("LoadCorpusDataset: %v", err)
	}
	corpusCase := findProfileReportCase(ds, "display-module-v1")
	if corpusCase == nil {
		t.Fatal("missing display-module-v1")
	}
	selected := []string{"generate_summaries", "extract_inventory_items", "extract_provisions"}
	results := make([]GoldRunDocumentResult, 0, len(corpusCase.DocumentProfiles))
	docKeys := make([]string, 0, len(corpusCase.DocumentProfiles))
	for docKey := range corpusCase.DocumentProfiles {
		docKeys = append(docKeys, docKey)
	}
	sort.Strings(docKeys)
	for i, docKey := range docKeys {
		switch docKey {
		case "doc:ent-q-syn-001-2026":
			results = append(results, GoldRunDocumentResult{
				Document: docKey,
				RecordID: int64(i + 1),
				RunError: "processor failed",
			})
		case "doc:ent-mkt-syn-2025":
			results = append(results, GoldRunDocumentResult{
				Document: docKey,
				RecordID: int64(i + 1),
				Results: map[string]GoldRunProcessorResult{
					"generate_summaries":      rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "generate_summaries"}}),
					"extract_inventory_items": rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "extract_inventory_items"}}),
					"extract_provisions":      {State: GoldRunResultNotRegistered},
				},
			})
		case "doc:ent-q-syn-001-2019":
			results = append(results, GoldRunDocumentResult{
				Document: docKey,
				RecordID: int64(i + 1),
				Results: map[string]GoldRunProcessorResult{
					"generate_summaries":      rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "generate_summaries"}}),
					"extract_inventory_items": rowsResult([]map[string]any{}),
					"extract_provisions":      rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "extract_provisions"}}),
				},
			})
		default:
			results = append(results, GoldRunDocumentResult{
				Document: docKey,
				RecordID: int64(i + 1),
				Results: map[string]GoldRunProcessorResult{
					"generate_summaries":      rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "generate_summaries"}}),
					"extract_inventory_items": rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "extract_inventory_items"}}),
					"extract_provisions":      rowsResult([]map[string]any{{"id": 1, "doc": docKey, "processor": "extract_provisions"}}),
				},
			})
		}
	}
	return ds, GoldRunEnvelope{
		SchemaVersion:      GoldRunSchemaVersion,
		Dataset:            GoldRunDatasetIdentity{ID: ds.Manifest.DatasetID, Version: ds.Manifest.DatasetVersion, ContentHash: corpusCase.ContentHash},
		CaseID:             corpusCase.CaseID,
		SelectedProcessors: selected,
		DryRun:             false,
		Results:            results,
	}
}

func rowsResult(rows []map[string]any) GoldRunProcessorResult {
	return GoldRunProcessorResult{State: GoldRunResultRows, Rows: &rows}
}

func cloneFixture(ds *CorpusDataset, run GoldRunEnvelope) (*CorpusDataset, GoldRunEnvelope) {
	cloned := *ds
	cloned.Cases = append([]CorpusCase(nil), ds.Cases...)
	for i := range cloned.Cases {
		profiles := make(map[string]DocumentProfile, len(ds.Cases[i].DocumentProfiles))
		for docKey, profile := range ds.Cases[i].DocumentProfiles {
			expected := make(map[string]ProcessorApplicability, len(profile.ExpectedProcessors))
			for processor, applicability := range profile.ExpectedProcessors {
				expected[processor] = applicability
			}
			profile.ExpectedProcessors = expected
			profiles[docKey] = profile
		}
		cloned.Cases[i].DocumentProfiles = profiles
	}
	roundTrip := func(in GoldRunEnvelope) GoldRunEnvelope {
		raw, err := json.Marshal(in)
		if err != nil {
			panic(err)
		}
		var out GoldRunEnvelope
		if err := json.Unmarshal(raw, &out); err != nil {
			panic(err)
		}
		return out
	}
	return &cloned, roundTrip(run)
}

func mutateApplicability(profile DocumentProfile, processor string, applicability ProcessorApplicability) DocumentProfile {
	expected := make(map[string]ProcessorApplicability, len(profile.ExpectedProcessors))
	for k, v := range profile.ExpectedProcessors {
		expected[k] = v
	}
	expected[processor] = applicability
	profile.ExpectedProcessors = expected
	return profile
}

func fixtureResult(run *GoldRunEnvelope, document string) *GoldRunDocumentResult {
	for i := range run.Results {
		if run.Results[i].Document == document {
			return &run.Results[i]
		}
	}
	panic("missing fixture document " + document)
}
