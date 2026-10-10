package kbhandler

import (
	"encoding/json"
	"errors"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAttachGoldMetricRuns(t *testing.T) {
	for _, model := range []string{"", "model-a"} {
		t.Run("model="+model, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			records := []inputRecord{{ID: 753}, {ID: 800}}
			query := "SELECT DISTINCT input_record_id, skill_version, model_name FROM testbed.metrics WHERE input_record_id IN ($1, $2) AND skill_version = $3"
			if model != "" {
				query += " AND model_name = $4"
			}
			query += " ORDER BY input_record_id, skill_version, model_name"
			rows := sqlmock.NewRows([]string{"input_record_id", "skill_version", "model_name"}).AddRow(753, "6.0.0", "model-a")
			if model == "" {
				rows.AddRow(753, "6.0.0", "model-b").AddRow(800, "6.0.0", "model-b")
			}
			expectation := mock.ExpectQuery(regexp.QuoteMeta(query))
			if model == "" {
				expectation.WithArgs(int64(753), int64(800), "6.0.0")
			} else {
				expectation.WithArgs(int64(753), int64(800), "6.0.0", model)
			}
			expectation.WillReturnRows(rows)
			if err := attachGoldMetricRuns(db, records, listInputsFilters{GoldSkillVersion: "6.0.0", GoldModelName: model}); err != nil {
				t.Fatal(err)
			}
			if len(records) != 2 || records[0].ID != 753 {
				t.Fatalf("documents duplicated or reordered: %+v", records)
			}
			wantRuns := 1
			if model == "" {
				wantRuns = 2
			}
			if len(records[0].GoldMetricRuns) != wantRuns || records[0].GoldMetricRuns[0].ModelName != "model-a" || records[0].GoldMetricRuns[0].SkillVersion != "6.0.0" {
				t.Fatalf("incorrect matching runs: %+v", records)
			}
			if model == "" && (len(records[1].GoldMetricRuns) != 1 || records[1].GoldMetricRuns[0].ModelName != "model-b") {
				t.Fatalf("runs attached to wrong document: %+v", records)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttachGoldMetricRunsEmptyAndFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := attachGoldMetricRuns(db, nil, listInputsFilters{}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("database unavailable")
	mock.ExpectQuery("SELECT DISTINCT input_record_id").WithArgs(int64(753)).WillReturnError(want)
	if err := attachGoldMetricRuns(db, []inputRecord{{ID: 753}}, listInputsFilters{}); !errors.Is(err, want) {
		t.Fatalf("expected database failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListInputsSeparateBenchmarkEntries(t *testing.T) {
	for _, model := range []string{"", "gpt-6-tuna"} {
		t.Run("model="+model, func(t *testing.T) {
			stubListInputsUser(t, &ApiTypes.UserInfo{UserId: "admin-1", Roles: []string{"admin"}})
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()
			expectResolveInputTablePlural(mock)
			where := "i.benchmark_skill_version = $1"
			models := []string{"claude-opus-5-5", "gpt-6-tuna"}
			if model != "" {
				where += " AND i.benchmark_model_name = $2"
				models = []string{model}
			}
			countQuery := "SELECT COUNT(1) FROM " + goldBenchmarkInputTable("kb.inputs") + " i WHERE " + where
			count := mock.ExpectQuery(regexp.QuoteMeta(countQuery))
			if model == "" {
				count.WithArgs("6.0.0")
			} else {
				count.WithArgs("6.0.0", model)
			}
			count.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(len(models)))
			expectResolveNameColumnStaging(mock)
			expectResolveParserNameColumn(mock, true)
			rows := sqlmock.NewRows([]string{
				"id", "name", "parser_name", "type", "user_id", "ks_store_id", "title", "doc_no", "ks_desc", "processing_mode", "source", "file_name",
				"backup_filename", "result_filename", "publish_date", "authors", "owner", "status", "create_time", "modify_time", "public_info", "private_info", "doc_metadata", "notes", "error_msg", "benchmark_skill_version", "benchmark_model_name",
			})
			for _, name := range models {
				rows.AddRow(int64(753), "Input 753", "mineru", "pdf", "admin-1", nil, "Document", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "[]", time.Now(), time.Now(), nil, nil, nil, nil, nil, "6.0.0", name)
			}
			data := mock.ExpectQuery("SELECT.*i.benchmark_skill_version, i.benchmark_model_name.*" + regexp.QuoteMeta("FROM "+goldBenchmarkInputTable("kb.inputs")+" i WHERE "+where) + ".*" + regexp.QuoteMeta("i.benchmark_skill_version ASC, i.benchmark_model_name ASC") + ".*LIMIT")
			if model == "" {
				data.WithArgs("6.0.0", 50, 0)
			} else {
				data.WithArgs("6.0.0", model, 50, 0)
			}
			data.WillReturnRows(rows)
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?has_gold_metrics=true&gold_benchmark_entries=true&gold_skill_version=6.0.0&gold_model_name="+model, nil)
			rec := httptest.NewRecorder()
			if err := ListInputs(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var response listInputsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Total != int64(len(models)) || len(response.Results) != len(models) {
				t.Fatalf("benchmarks merged or miscounted: %+v", response)
			}
			for i, record := range response.Results {
				if record.ID != 753 || len(record.GoldMetricRuns) != 1 || record.GoldMetricRuns[0].ModelName != models[i] || record.GoldMetricRuns[0].SkillVersion != "6.0.0" {
					t.Fatalf("wrong benchmark identity: %+v", record)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
