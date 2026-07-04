package docreviews

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestMetricsCompletenessBuildRostersEmptyMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// No metrics for this record.
	mock.ExpectQuery("SELECT id, COALESCE").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(metricRecordColumns()))

	r := &metricsCompletenessReviewer{db: db, logger: loggerutil.CreateDefaultLogger("TEST_COMPLETENESS")}
	rosters, err := r.buildRosters(context.Background(), 1)
	if err != nil {
		t.Fatalf("buildRosters err = %v", err)
	}
	if len(rosters) != 0 {
		t.Fatalf("rosters = %d, want 0 for empty metrics", len(rosters))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock: %v", err)
	}
}

func TestMetricsCompletenessBuildRostersNoObjectLinks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// One doc metric.
	docRows := sqlmock.NewRows(metricRecordColumns()).
		AddRow(10, "1_m_1", "压力", "管道", "", "", "", "", "", "", "1.6", "MPa", "", "", "", "maximum", "", "", "", "", "", "", `["pressure"]`, `["12"]`)
	mock.ExpectQuery("SELECT id, COALESCE").
		WithArgs(int64(1)).
		WillReturnRows(docRows)

	// No reconciled object links (NULL object_id).
	mock.ExpectQuery("SELECT ao.artifact_id").
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "object_id", "object_name", "object_type", "description"}))

	r := &metricsCompletenessReviewer{db: db, logger: loggerutil.CreateDefaultLogger("TEST_COMPLETENESS")}
	rosters, err := r.buildRosters(context.Background(), 1)
	if err != nil {
		t.Fatalf("buildRosters err = %v", err)
	}
	if len(rosters) != 0 {
		t.Fatalf("rosters = %d, want 0 (no reconciled objects)", len(rosters))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock: %v", err)
	}
}

func metricRecordColumns() []string {
	return []string{
		"id", "metric_id", "metric_name", "metric_subject",
		"metric_name_en", "metric_subject_en", "metric_desc", "metric_desc_en",
		"metric_context", "metric_context_en", "metric_value", "metric_unit",
		"metric_unit_en", "value_data_type", "value_range_type", "value_class",
		"value_class_en", "formula_or_definition", "threshold_or_target",
		"measurement_frequency", "location_type", "table_name_or_section",
		"metric_categories", "source_line_spans",
	}
}

func TestMetricsCompletenessReviewObjectPayloadShape(t *testing.T) {
	fake := &fakeJSONExtractor{out: map[string]any{"findings": []any{
		map[string]any{
			"title":               "Missing metric: Test Metric",
			"finding_type":        "missing_metric",
			"related_artifact_id": "2002_m_4",
			"related_record_id":   float64(2002),
		},
	}}}
	r := &metricsCompletenessReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_COMPLETENESS"),
	}

	ro := objectMetricRoster{
		objectID:   "NODE_abc",
		objectName: "压力容器",
		objectType: "equipment",
		docMetrics: []metricView{
			{MetricID: "1_m_1", MetricName: "设计压力", Value: "2.5", Unit: "MPa"},
		},
		docSpan: []string{"120-135"},
		peerDocs: []peerDocMetrics{
			{
				recordID:  2002,
				filename:  "GB_T_150.pdf",
				title:     "GB/T 150",
				docNo:     "GB/T 150.1-2024",
				authority: "standard",
				metrics: []metricView{
					{MetricID: "2002_m_3", MetricName: "设计压力", Value: "2.5", Unit: "MPa"},
					{MetricID: "2002_m_4", MetricName: "设计温度", Value: "350", Unit: "degC"},
				},
			},
		},
		totalPeerDocs:    1,
		totalPeerMetrics: 2,
	}

	findings := r.reviewObject(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "CHECK MISSING METRICS",
		PromptRef:  "prompt-review-metrics-missing-v1.md",
	}, ro)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" {
		t.Errorf("Pass = %q, want P5", f.Pass)
	}
	if f.Aspect != "metrics_completeness" {
		t.Errorf("Aspect = %q, want metrics_completeness", f.Aspect)
	}
	if f.FindingType != "missing_metric" {
		t.Errorf("FindingType = %q, want missing_metric", f.FindingType)
	}
	if f.Severity != "medium" {
		t.Errorf("Severity = %q, want medium", f.Severity)
	}
	if f.RelatedArtifactID != "2002_m_4" {
		t.Errorf("RelatedArtifactID = %q, want 2002_m_4", f.RelatedArtifactID)
	}
	if f.RelatedRecordID != 2002 {
		t.Errorf("RelatedRecordID = %d, want 2002", f.RelatedRecordID)
	}

	// Verify payload contents.
	in := fake.inputTexts[0]
	for _, want := range []string{"object", "\"object_id\":\"NODE_abc\"", "\"object_name\":\"压力容器\"",
		"doc_metrics", "\"metric_id\":\"1_m_1\"",
		"peer_docs", "\"source_record_id\":2002", "\"source_doc_authority\":\"standard\"",
		"total_peer_docs", "total_peer_metrics", "artifact_line_spans"} {
		if !strings.Contains(in, want) {
			t.Errorf("payload missing %q", want)
		}
	}
	// doc_metrics may be empty — the object could be mentioned without any
	// attached metrics (a strong signal itself).
	ro.docMetrics = nil
	r2 := &metricsCompletenessReviewer{
		client: &fakeJSONExtractor{out: map[string]any{"findings": []any{}}},
		logger: loggerutil.CreateDefaultLogger("TEST_COMPLETENESS"),
	}
	findings2 := r2.reviewObject(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "CHECK MISSING METRICS",
		PromptRef:  "prompt-review-metrics-missing-v1.md",
	}, ro)
	if len(findings2) != 0 {
		t.Fatalf("empty-object findings = %d, want 0", len(findings2))
	}
}

func TestMergeSpans(t *testing.T) {
	got := mergeSpans([]string{"12-14"}, []string{"20", "25:28"})
	if len(got) != 3 {
		t.Fatalf("mergeSpans len = %d, want 3", len(got))
	}
	if got[0] != "12-14" || got[1] != "20" || got[2] != "25:28" {
		t.Errorf("mergeSpans = %v", got)
	}
	empty := mergeSpans(nil, nil)
	if empty != nil {
		t.Errorf("mergeSpans(nil,nil) = %v, want nil", empty)
	}
}
