package docprocessing

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// pureRequirementTestRows are enriched rows of every relevant shape (spec
// metric-pure-requirement-exclusion): two pure requirements and three rows with
// a measurable property, one of them with a non-canonical range type.
func pureRequirementTestRows() []any {
	return []any{
		map[string]any{"metric_name": "垃圾桶加盖", "value_class": "requirement", "value_range_type": "qualitative",
			"threshold_or_target": "垃圾桶应加盖", "source_line_spans": []any{"2"}},
		map[string]any{"metric_name": "转运站设计要求", "value_class": "reference", "value_range_type": "qualitative",
			"reasoning_tags": []any{"external_reference", "cited_doc:CJJ 52"}, "source_line_spans": []any{"2"}},
		map[string]any{"metric_name": "总砷", "value_class": "requirement", "value_range_type": "upper_bound",
			"metric_value": "15", "source_line_spans": []any{"2"}},
		map[string]any{"metric_name": "比能耗", "value_class": "requirement", "value_range_type": "limit_absent",
			"source_line_spans": []any{"2"}},
		map[string]any{"metric_name": "含水率", "value_class": "requirement", "value_range_type": "maximum",
			"metric_value": "30", "source_line_spans": []any{"2"}},
	}
}

func TestExcludePureRequirements(t *testing.T) {
	rows := normalizeMetricList(pureRequirementTestRows())
	canonicalizeMetricValueRangeTypes(rows)
	kept, excluded := excludePureRequirements(rows)

	var keptNames []string
	for _, m := range kept {
		keptNames = append(keptNames, asString(m["metric_name"]))
	}
	if want := []string{"总砷", "比能耗", "含水率"}; !equalStrings(keptNames, want) {
		t.Fatalf("kept=%v, want %v", keptNames, want)
	}
	if len(excluded) != 2 {
		t.Fatalf("excluded=%v, want 2 rows", excluded)
	}
	if excluded[0]["kind"] != statementKindInspectionRequirement || excluded[1]["kind"] != statementKindDelegatedRequirement {
		t.Fatalf("excluded kinds=%v,%v", excluded[0]["kind"], excluded[1]["kind"])
	}
	if excluded[0]["threshold_or_target"] != "垃圾桶应加盖" {
		t.Fatalf("excluded row must carry its clause text, got %v", excluded[0])
	}
	for _, key := range []string{"metric_name", "subject", "threshold_or_target", "context", "source_line_spans"} {
		if _, ok := excluded[0][key]; !ok {
			t.Fatalf("excluded row missing %q: %v", key, excluded[0])
		}
	}
}

func TestFinalizeChunkBatch_WipeModeExcludesPureRequirements(t *testing.T) {
	metricsStore := &fakeMetricsStore{}
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": pureRequirementTestRows(), "uncertain_metrics": []any{}},
	}}
	p := NewMetricsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416}}, metricsStore, extractor, nil)
	p.batchRecordID = 416
	p.batchForceClear = true
	p.ObjectStore = &fakeArtifactObjectsStore{}
	p.batchChunks = metricsBlocksToChunks([]Block{makeMetricsBlock(0, 2)})
	p.batchMentions = []metricCandidateMention{{
		MetricNameHint: "总砷", SourceLineSpans: []string{"2"}, HasNormalEvidence: true,
	}}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	saved := metricsStore.lastSave.Metrics
	if len(saved) != 3 {
		t.Fatalf("saved %d rows, want 3 (pure requirements excluded): %v", len(saved), saved)
	}
	for i, m := range saved {
		if kind := metricStatementKind(m); isPureRequirementKind(kind) {
			t.Fatalf("saved a pure requirement: %v", m)
		}
		if want := "416_mtc_" + string(rune('1'+i)); m["metric_id"] != want {
			t.Fatalf("metric_id=%v, want %s (contiguous after exclusion)", m["metric_id"], want)
		}
	}
}

func TestLogExcludedPureRequirementsWritesOneEntry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_proc_logs")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	p.ProcLogger = DocProcLogger{DB: db}
	_, excluded := excludePureRequirements(normalizeMetricList(pureRequirementTestRows()))
	p.logExcludedPureRequirements(context.Background(), 416, excluded)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// Nothing excluded: no entry. sqlmock fails any unexpected Exec, and the
	// helper must not even try.
	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db2.Close()
	p.ProcLogger = DocProcLogger{DB: db2}
	p.logExcludedPureRequirements(context.Background(), 416, nil)
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsProcessorDefaultPromptsExcludePureRequirements(t *testing.T) {
	for _, key := range []string{"EXTRACT_METRIC_CANDIDATES_PROMPT", "ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"} {
		t.Setenv(key, "")
	}
	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	if p.MentionPromptRef != "prompt-extract-metric-candidates-v11.md" {
		t.Fatalf("MentionPromptRef=%q, want v11", p.MentionPromptRef)
	}
	if p.RelationPromptRef != "prompt-enrich-metrics-v8.md" {
		t.Fatalf("RelationPromptRef=%q, want v8", p.RelationPromptRef)
	}
	// Prompts load relative to the server's working directory; from this test,
	// check the files exist in the repo's prompts/ directory instead.
	for _, ref := range []string{p.MentionPromptRef, p.RelationPromptRef} {
		if _, err := os.Stat(filepath.Join("..", "..", "..", "prompts", ref)); err != nil {
			t.Fatalf("default prompt %s missing: %v", ref, err)
		}
	}
}
