package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestQuestionGenerationJobsPagination(t *testing.T) {
	_, mock := installPolicyDB(t)
	columns := strings.Split(questionRunColumns, ",")
	for i := range columns {
		columns[i] = strings.TrimSpace(columns[i])
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM testbed.generated_question_runs.*ORDER BY id DESC LIMIT \\$2 OFFSET \\$3").WithArgs(int64(0), 25, 50).WillReturnRows(sqlmock.NewRows(columns).AddRow(12, 753, "search_metrics", "test job", 4, 4, 4, "mixed", "done", "", "model", "prompt", "", time.Now(), time.Now(), 4, 4, 4, 4, 4, 4, 1, time.Now().Add(time.Hour)))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM testbed.generated_question_runs").WithArgs(int64(0)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(61))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?page=3&page_size=25", nil), rec)
	if err := ListQuestionGenerationRuns(c); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Runs     []questionGenerationRun `json:"runs"`
		Total    int                     `json:"total"`
		Page     int                     `json:"page"`
		PageSize int                     `json:"page_size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(result.Runs) != 1 || result.Total != 61 || result.Page != 3 || result.PageSize != 25 {
		t.Fatalf("status=%d result=%+v", rec.Code, result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGenerationJobsRejectInvalidPagination(t *testing.T) {
	for _, query := range []string{"page=0", "page=-1", "page=no", "page=1000001", "page_size=0", "page_size=101", "page_size=no"} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?"+query, nil), rec)
		if err := ListQuestionGenerationRuns(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", query, rec.Code)
		}
	}
}

func TestQuestionGenerationJobsPreservesRecordFilter(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM testbed.generated_question_runs.*ORDER BY id DESC LIMIT \\$2 OFFSET \\$3").WithArgs(int64(753), 10, 10).WillReturnRows(sqlmock.NewRows(strings.Split(questionRunColumns, ",")))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM testbed.generated_question_runs").WithArgs(int64(753)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?page=2&page_size=10&input_record_id=753", nil), rec)
	if err := ListQuestionGenerationRuns(c); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Runs  []questionGenerationRun `json:"runs"`
		Total int                     `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || result.Runs == nil || len(result.Runs) != 0 || result.Total != 10 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
