package docprocessing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	if excluded[0].Kind != statementKindInspectionRequirement || excluded[1].Kind != statementKindDelegatedRequirement {
		t.Fatalf("excluded kinds=%v,%v", excluded[0].Kind, excluded[1].Kind)
	}
	for _, e := range excluded {
		if e.Stage != metricDropStageStatementKind || e.Reason != e.Kind {
			t.Fatalf("excluded row stage/reason=%s/%s, want statement_kind/<kind>", e.Stage, e.Reason)
		}
	}
	if excluded[0].Row["threshold_or_target"] != "垃圾桶应加盖" {
		t.Fatalf("excluded row must keep the whole enriched row, got %v", excluded[0].Row)
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
	// Soft drop: the two pure requirements are kept in kb.metrics_dropped.
	if len(metricsStore.dropped) != 2 {
		t.Fatalf("dropped %d rows, want 2: %v", len(metricsStore.dropped), metricsStore.dropped)
	}
	for i, d := range metricsStore.dropped {
		if want := fmt.Sprintf("416_drp_%d", i+1); d.DropID != want || d.Stage != metricDropStageStatementKind {
			t.Fatalf("dropped[%d] = %s/%s, want %s/statement_kind", i, d.DropID, d.Stage, want)
		}
	}
}

func TestLogDroppedMetricRowsWritesOneEntry(t *testing.T) {
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
	p.saveDroppedMetricRows(context.Background(), 416, excluded)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// Nothing dropped: no entry. sqlmock fails any unexpected Exec, and the
	// helper must not even try.
	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db2.Close()
	p.ProcLogger = DocProcLogger{DB: db2}
	p.saveDroppedMetricRows(context.Background(), 416, nil)
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsProcessorDefaultPromptsExcludePureRequirements(t *testing.T) {
	for _, key := range []string{"EXTRACT_METRIC_CANDIDATES_PROMPT", "ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"} {
		t.Setenv(key, "")
	}
	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	if p.MentionPromptRef != "prompt-extract-metric-candidates-v12.md" {
		t.Fatalf("MentionPromptRef=%q, want v12", p.MentionPromptRef)
	}
	if p.RelationPromptRef != "prompt-enrich-metrics-v9.md" {
		t.Fatalf("RelationPromptRef=%q, want v9", p.RelationPromptRef)
	}
	// Prompts load relative to the server's working directory; from this test,
	// check the files exist in the repo's prompts/ directory instead.
	for _, ref := range []string{p.MentionPromptRef, p.RelationPromptRef} {
		if _, err := os.Stat(filepath.Join("..", "..", "..", "prompts", ref)); err != nil {
			t.Fatalf("default prompt %s missing: %v", ref, err)
		}
	}
}

func TestUnaccountedMetricCandidates(t *testing.T) {
	candidates := []metricCandidate{{CandidateID: "6_1"}, {CandidateID: "6_2"}, {CandidateID: "6_3"}, {CandidateID: "6_4"}}
	metrics := []map[string]any{{"candidate_id": "6_1"}, {"candidate_id": "6_1"}, {"candidate_id": ""}}
	dropped := []any{map[string]any{"candidate_id": "6_3", "reason": "applicability_scope"}, "junk"}
	got := unaccountedMetricCandidates(metrics, dropped, candidates)
	if strings.Join(got, ",") != "6_2,6_4" {
		t.Fatalf("unaccounted = %v, want [6_2 6_4]", got)
	}
}

func TestMetricRelationBatchPromptAsksForDroppedCandidates(t *testing.T) {
	prompt := buildMetricRelationBatchPrompt([]metricCandidate{{CandidateID: "6_2"}})
	for _, want := range []string{`"dropped_candidates"`, `"candidate_id":"string"`, `"candidate_id":"6_2"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("batch prompt missing %s", want)
		}
	}
}

func TestDropRowsTaggedWithDropReason(t *testing.T) {
	rows := []map[string]any{
		{"metric_name": "容积", "reasoning_tags": []string{"applicability_scope"}},
		{"metric_name": "比能耗", "reasoning_tags": []string{"open_value"}},
		{"metric_name": "根长", "reasoning_tags": []string{" Formula_Operand "}},
		{"metric_name": "pH"},
	}
	kept, dropped := dropRowsTaggedWithDropReason(rows)
	if len(kept) != 2 || kept[0]["metric_name"] != "比能耗" || kept[1]["metric_name"] != "pH" {
		t.Fatalf("kept = %v", kept)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestDropRowsKeepsUntaggedRowsForTheDecisionModel(t *testing.T) {
	// 416_mtc_3 as stored 2026-10-08: untagged, requirement + limit_absent. No word list
	// drops it here; the open-value decision judges it (spec metric-open-value-decision).
	rows := []map[string]any{{"metric_name": "餐厨垃圾收运时间和频次", "value_class": "requirement",
		"value_range_type": "limit_absent", "threshold_or_target": "由收运单位与集中供餐单位约定", "reasoning_tags": []any{}}}
	kept, dropped := dropRowsTaggedWithDropReason(rows)
	if len(kept) != 1 || len(dropped) != 0 {
		t.Fatalf("kept=%v dropped=%v", kept, dropped)
	}
	if got := metricDropReasonTag(map[string]any{"reasoning_tags": []string{"x", " Activity_Schedule "}}); got != "activity_schedule" {
		t.Fatalf("metricDropReasonTag = %q", got)
	}
}
