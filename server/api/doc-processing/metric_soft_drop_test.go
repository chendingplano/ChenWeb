package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Drop ids continue after the record's highest existing one (merge mode), and each row is
// saved with its stage, reason, decision and full row (spec metric-row-soft-drop).
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
			nil, sqlmock.AnyArg(), "evt-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO kb.metrics_dropped`).
		WithArgs(int64(416), "416_drp_6", "", metricDropStageDecision, openValueChoiceSchedule,
			`{"choice":"activity_schedule"}`, sqlmock.AnyArg(), "evt-1").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	rows := []droppedMetricRow{
		{Row: map[string]any{"candidate_id": "7_1", "metric_name": "垃圾桶加盖"}, Stage: metricDropStageStatementKind, Reason: statementKindInspectionRequirement},
		{Row: map[string]any{"metric_name": "餐厨垃圾收运时间和频次"}, Stage: metricDropStageDecision, Reason: openValueChoiceSchedule,
			Decision: map[string]any{"choice": "activity_schedule"}},
	}
	if err := (MetricsSQLStore{DB: db}).SaveDroppedMetrics(context.Background(), 416, "evt-1", rows); err != nil {
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
