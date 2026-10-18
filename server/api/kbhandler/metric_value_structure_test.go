package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestExtractedMetricCollectionPersistsRelationship(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw := map[string]any{"metric_name": "Diameter", "metric_value": "10, 20 or 30", "metric_unit": "mm", "value_structure": map[string]any{"kind": "alternatives", "values": []any{"10", "20", "30"}}}
	metrics := normalizeExtractedMetrics([]any{raw})
	if metrics[0]["value_structure"] == nil {
		t.Fatal("normalization lost collection semantics")
	}
	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COUNT").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	args := make([]driver.Value, 40)
	for i := range args {
		args[i] = sqlmock.AnyArg()
	}
	args[19] = "10, 20 or 30"
	args[39] = `{"kind":"alternatives","values":["10","20","30"]}`
	mock.ExpectExec("INSERT INTO kb.metrics").WithArgs(args...).WillReturnResult(sqlmock.NewResult(1, 1))
	n, err := saveExtractedMetrics(db, 7, metrics)
	if err != nil || n != 1 {
		t.Fatalf("save = %d, %v", n, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateMetricRejectsInvalidCollection(t *testing.T) {
	for _, structure := range []string{`{"kind":"alternatives","values":["10"]}`, `{"kind":"required_points","values":["10",""]}`, `{"kind":"unknown"}`} {
		c, rec := newUpdateMetricContext(t, "11", `{"value_structure":`+structure+`}`)
		if err := UpdateMetric(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid structure accepted: %s (%d)", structure, rec.Code)
		}
	}
}

func TestMetricRecordSerializesStructure(t *testing.T) {
	r := metricRecord{ValueStructure: json.RawMessage(`{"kind":"required_points","values":["10","30","50"]}`)}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	v, ok := out["value_structure"].(map[string]any)
	if !ok || v["kind"] != "required_points" || len(v["values"].([]any)) != 3 {
		t.Fatalf("structure missing: %s", b)
	}
}

func TestUpdateMetricRejectsContradictoryDisplay(t *testing.T) {
	c, rec := newUpdateMetricContext(t, "11", `{"metric_value":"10, 20 or 30","value_structure":{"kind":"alternatives","values":["10","20","99"]}}`)
	if err := UpdateMetric(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("contradictory structure accepted: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMetricPartialDisplayChecksRetainedCollection(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()
	mock.ExpectQuery("SELECT metric_value, value_structure FROM kb.metrics WHERE id").WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"metric_value", "value_structure"}).AddRow("10 or 20", `{"kind":"alternatives","values":["10","20"]}`))
	c, rec := newUpdateMetricContext(t, "11", `{"metric_value":"10"}`)
	if err := UpdateMetric(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("partial scalar edit lost retained collection: %d %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
