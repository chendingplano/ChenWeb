package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestSortSkillVersionsDescIsNumeric(t *testing.T) {
	got := []string{"1.2.0", "1.10.0", "1.0.0", "2.0", "1.1.0"}
	sortSkillVersionsDesc(got)
	want := []string{"2.0", "1.10.0", "1.2.0", "1.1.0", "1.0.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListMetricsTestbedFiltersBySkillVersionAndModel(t *testing.T) {
	cases := []struct {
		name        string
		query       map[string]string
		mockNewest  bool
		wantVersion string
		wantModel   string
	}{
		{"explicit", map[string]string{"skill_version": "1.1.0", "model_name": "gpt-6.1-sol"}, false, "1.1.0", "gpt-6.1-sol"},
		{"defaults to newest version, any model", nil, true, "1.10.0", ""},
		{"selected earlier run", map[string]string{"skill_version": "6.0.0", "model_name": "gold-model", "run_id": "20261010_055329", "include_dropped": "true"}, false, "6.0.0", "gold-model"},
		{"benchmark drops", map[string]string{"skill_version": "6.0.0", "model_name": "gold-model", "include_dropped": "true"}, false, "6.0.0", "gold-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer db.Close()
			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()

			if tc.mockNewest {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT skill_version FROM testbed.metrics`)).
					WillReturnRows(sqlmock.NewRows([]string{"skill_version"}).AddRow("1.2.0").AddRow("1.10.0").AddRow("1.0.0"))
			}
			mock.ExpectQuery(regexp.QuoteMeta(selectBenchmarkRunQuery)).
				WithArgs(int64(244), tc.wantVersion, tc.wantModel, tc.query["run_id"]).
				WillReturnRows(sqlmock.NewRows([]string{"skill_name", "skill_version", "model_name", "run_id"}).
					AddRow("extract-metrics-benchmark", tc.wantVersion, "selected-model", "20261010_055329"))
			goldRows := sqlmock.NewRows([]string{"id"})
			if tc.query["include_dropped"] == "true" {
				columns := make([]string, 46)
				values := make([]driver.Value, 46)
				for i := range columns {
					columns[i] = strconv.Itoa(i)
				}
				values[0], values[1], values[2], values[4] = int64(8), int64(244), "244_gold_1", "input.pdf"
				values[5], values[7], values[16], values[35] = "Gold pressure", `["2"]`, "selected-model", "2026-10-10T05:53:29-05:00"
				goldRows = sqlmock.NewRows(columns).AddRow(values...)
			}
			mock.ExpectQuery(regexp.QuoteMeta(testbedMetricsQuery)).
				WithArgs(int64(244), "extract-metrics-benchmark", tc.wantVersion, "selected-model", "20261010_055329").
				WillReturnRows(goldRows)
			if tc.query["include_dropped"] == "true" {
				mock.ExpectQuery(regexp.QuoteMeta(testbedDroppedMetricsQuery)).
					WithArgs(int64(244), "extract-metrics-benchmark", tc.wantVersion, "selected-model", "20261010_055329").
					WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "candidate_id", "drop_reason", "candidate_data", "input_filename", "title", "doc_no", "created_at"}).
						AddRow(7, 244, "c7", "X18", `{"candidate_id":"c7","decision":"excluded","rule_id":"X18","reason":"Definition without a value","source_line_spans":["2"],"evidence_quote":"Pressure formula"}`, "input.pdf", nil, nil, "2026-10-10T05:53:29-05:00"))
			}

			c, rec := newListMetricsContext(t, "244")
			c.QueryParams().Set("source", "testbed")
			for k, v := range tc.query {
				c.QueryParams().Set(k, v)
			}
			if err := ListMetrics(c); err != nil {
				t.Fatalf("ListMetrics returned error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
			}
			if tc.query["include_dropped"] == "true" {
				var payload listMetricsResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Results) != 2 || payload.RunID != "20261010_055329" {
					t.Fatalf("wrong benchmark drop: %s", rec.Body.String())
				}
				for _, r := range payload.Results {
					if r.RunID != payload.RunID || r.SkillVersion != tc.wantVersion || *r.ModelName != "selected-model" {
						t.Fatalf("mixed benchmark runs: %s", rec.Body.String())
					}
				}
				if !payload.Results[0].Dropped || payload.Results[1].Dropped {
					t.Fatalf("expected one dropped and one gold row: %s", rec.Body.String())
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet db expectations: %v", err)
			}
		})
	}
}
