package docprocessing

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Record 364: numeral-bearing workflow labels are not quantitative properties.
// If enrichment leaks excluded candidates as tagged rows, preserve their audit
// records while preventing them from reaching the live metric store.
func TestFinalizeChunkBatch_CategoryLabelsAndScopeAreSetAside(t *testing.T) {
	var raw []any
	for i, value := range []string{"1~4", "1", "2", "3", "1", "2", "3", "4"} {
		reason := "categorical_label"
		if i < 4 {
			reason = "applicability_scope"
		}
		raw = append(raw, map[string]any{
			"metric_name": fmt.Sprintf("转诊需求级别%d", i), "metric_value": value, "unit": "级",
			"value_class": "requirement", "value_range_type": "exact",
			"source_line_spans": []string{"2"}, "reasoning_tags": []string{reason},
		})
	}
	// Quantitative readings and requirements remain metrics even when a grade,
	// class or model selects their applicability. MEWS is a defined numeric scale.
	for _, row := range []map[string]any{
		{"metric_name": "MEWS心率评分", "metric_value": "2", "unit": "分", "condition": "心率：≤40次/min"},
		{"metric_name": "呼吸频率", "metric_value": "30", "unit": "次/min", "value_range_type": "lower_bound"},
		{"metric_name": "模型电阻阻值", "metric_value": "150", "unit": "Ω", "condition": "模型：模型1"},
		{"metric_name": "允许误差", "metric_value": "0.1", "unit": "%", "condition": "等级：2级"},
	} {
		row["source_line_spans"] = []string{"2"}
		row["value_class"] = "requirement"
		if row["value_range_type"] == nil {
			row["value_range_type"] = "exact"
		}
		raw = append(raw, row)
	}
	store := &fakeMetricsStore{}
	p := NewMetricsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 364}}, store,
		&fakeJSONExtractor{outs: []map[string]any{{"metrics": raw}}}, nil)
	p.OpenValueJudge = nil
	p.batchRecordID, p.batchForceClear = 364, true
	p.ObjectStore = &fakeArtifactObjectsStore{}
	p.batchChunks = metricsBlocksToChunks([]Block{makeMetricsBlock(0, 2)})
	p.batchMentions = []metricCandidateMention{{MetricNameHint: "转诊需求级别", SourceLineSpans: []string{"2"}, HasNormalEvidence: true}}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.lastSave.Metrics) != 4 || len(store.dropped) != 8 {
		t.Fatalf("live=%d dropped=%d, want 4 quantitative rows and 8 auditable exclusions", len(store.lastSave.Metrics), len(store.dropped))
	}
	for i, d := range store.dropped {
		wantReason := "categorical_label"
		if i < 4 {
			wantReason = "applicability_scope"
		}
		if d.Stage != metricDropStageLLMTag || d.Reason != wantReason || d.Row["metric_value"] == "" {
			t.Fatalf("drop[%d] lost its stage, reason or numeric source value: %+v", i, d)
		}
	}
}

// Drop ids continue after the record's highest existing one (merge mode), and each row is
// saved with its stage, reason, decision, full row and run id (spec metric-row-soft-drop).
func TestSaveDroppedMetricsNumbersAfterExistingDropIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(split_part\(drop_id, '_drp_', 2\)::int\), 0\)\s+FROM kb.metrics_dropped WHERE input_record_id = \$1`).
		WithArgs(int64(416)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(4))
	mock.ExpectExec(`INSERT INTO kb.metrics_dropped`).
		WithArgs(int64(416), "416_drp_5", "7_1", metricDropStageStatementKind, statementKindInspectionRequirement,
			nil, sqlmock.AnyArg(), "evt-1", int64(198)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO kb.metrics_dropped`).
		WithArgs(int64(416), "416_drp_6", "", metricDropStageDecision, openValueChoiceSchedule,
			`{"choice":"activity_schedule"}`, sqlmock.AnyArg(), "evt-1", int64(198)).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	rows := []droppedMetricRow{
		{Row: map[string]any{"candidate_id": "7_1", "metric_name": "垃圾桶加盖"}, Stage: metricDropStageStatementKind, Reason: statementKindInspectionRequirement},
		{Row: map[string]any{"metric_name": "餐厨垃圾收运时间和频次"}, Stage: metricDropStageDecision, Reason: openValueChoiceSchedule,
			Decision: map[string]any{"choice": "activity_schedule"}},
	}
	if err := (MetricsSQLStore{DB: db}).SaveDroppedMetrics(withRunID(context.Background(), 198), 416, "evt-1", rows); err != nil {
		t.Fatalf("SaveDroppedMetrics: %v", err)
	}
	if rows[0].DropID != "416_drp_5" || rows[1].DropID != "416_drp_6" {
		t.Fatalf("drop ids = %s, %s", rows[0].DropID, rows[1].DropID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Rows tagged with a drop reason are set aside as llm_tag, deduplicated across overlapping
// chunks, and saved with the live rows on a force_clear run.
func TestFinalizeChunkBatch_SetsAsideTaggedRowsOnce(t *testing.T) {
	tagged := map[string]any{"metric_name": "容积", "value_class": "requirement", "value_range_type": "exact",
		"metric_value": "50", "reasoning_tags": []any{"applicability_scope"}, "source_line_spans": []any{"2"}}
	live := map[string]any{"metric_name": "总砷", "value_class": "requirement", "value_range_type": "upper_bound",
		"metric_value": "15", "source_line_spans": []any{"2"}}
	extractor := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": []any{tagged, live}, "uncertain_metrics": []any{}},
		{"metrics": []any{cloneMetricMap(tagged)}, "uncertain_metrics": []any{}},
	}}
	metricsStore := &fakeMetricsStore{}
	p := NewMetricsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416}}, metricsStore, extractor, nil)
	p.OpenValueJudge = nil
	p.batchRecordID = 416
	p.batchForceClear = true
	p.ObjectStore = &fakeArtifactObjectsStore{}
	p.batchChunks = metricsBlocksToChunks([]Block{makeMetricsBlock(0, 2), makeMetricsBlock(1, 2)})
	p.batchMentions = []metricCandidateMention{
		{ChunkIndex: 0, MetricNameHint: "总砷", SourceLineSpans: []string{"2"}, HasNormalEvidence: true},
		{ChunkIndex: 1, MetricNameHint: "容积", SourceLineSpans: []string{"2"}, HasNormalEvidence: true},
	}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	if len(metricsStore.lastSave.Metrics) != 1 || metricsStore.lastSave.Metrics[0]["metric_name"] != "总砷" {
		t.Fatalf("saved = %v", metricsStore.lastSave.Metrics)
	}
	if len(metricsStore.dropped) != 1 {
		t.Fatalf("dropped %d rows, want 1 (deduplicated): %v", len(metricsStore.dropped), metricsStore.dropped)
	}
	d := metricsStore.dropped[0]
	if d.Stage != metricDropStageLLMTag || d.Reason != "applicability_scope" || d.DropID != "416_drp_1" {
		t.Fatalf("dropped row = %s/%s/%s", d.DropID, d.Stage, d.Reason)
	}
}

func TestExcludeRowsWithoutValue(t *testing.T) {
	rows := []map[string]any{
		{"metric_name": "总砷", "metric_value": "15", "value_range_type": "upper_bound"},
		{"metric_name": "比能耗", "metric_value": "", "value_range_type": "limit_absent", "value_class": "requirement"},
		{"metric_name": "量热计压力", "value_class": "definition", "formula_or_definition": "量热容器的二次流体侧压力"},
		{"metric_name": "名义风机功率", "value_class": "definition", "reasoning_tags": []any{"test_condition"},
			"formula_or_definition": "P_fan^(st) = P_fan × 1013.25 / P_atm"},
		{"metric_name": "测试时间", "value_class": "requirement", "formula_or_definition": "τ ≥ (200 × Δt_i × C) / P"},
		{"metric_name": "占位", "metric_value": " — "},
		{"metric_name": "污垢热阻", "metric_value": "0"},
	}
	kept, excluded := excludeRowsWithoutValue(rows)
	var keptNames, droppedNames []string
	for _, m := range kept {
		keptNames = append(keptNames, asString(m["metric_name"]))
	}
	for _, d := range excluded {
		droppedNames = append(droppedNames, asString(d.Row["metric_name"]))
		if d.Stage != metricDropStageNoValue || d.Reason != metricDropStageNoValue {
			t.Fatalf("dropped row stage/reason = %s/%s", d.Stage, d.Reason)
		}
	}
	if want := []string{"总砷", "污垢热阻"}; !equalStrings(keptNames, want) {
		t.Fatalf("kept = %v, want %v", keptNames, want)
	}
	// A formula without a value is a definition, not a metric.
	if want := []string{"比能耗", "量热计压力", "名义风机功率", "测试时间", "占位"}; !equalStrings(droppedNames, want) {
		t.Fatalf("dropped = %v, want %v", droppedNames, want)
	}
}
