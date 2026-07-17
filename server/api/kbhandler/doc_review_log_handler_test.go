package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListDocReviewLogs_ReturnsRowsAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_logs ")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, unit_type, unit_key,.*FROM kb\.doc_review_logs\s+ORDER BY create_time DESC, id DESC\s+LIMIT \$1 OFFSET \$2`).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "run_id", "pass", "aspect", "unit_type", "unit_key", "unit_location", "matched_units", "findings", "outcome", "detail", "create_time"}).
			AddRow(int64(5), int64(12), int64(3), "P5", "metrics", "metric", "12_mtc_1", `{"line":8}`, nil, `[]`, "no_findings", `{"reason":"ok"}`, "2026-07-17T10:30:00+00:00"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-review-logs", nil)
	rec := httptest.NewRecorder()
	if err := ListDocReviewLogs(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListDocReviewLogs: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Status   bool              `json:"status"`
		Results  []json.RawMessage `json:"results"`
		Page     int               `json:"page"`
		PageSize int               `json:"page_size"`
		Total    int64             `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !response.Status || response.Page != 1 || response.PageSize != 50 || response.Total != 2 || len(response.Results) != 1 {
		t.Fatalf("response=%s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListDocReviewLogs_RejectsInvalidTimeRange(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-review-logs?create_start_time=2026-07-17T11:00:00Z&create_end_time=2026-07-17T10:00:00Z", nil)
	rec := httptest.NewRecorder()
	if err := ListDocReviewLogs(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListDocReviewLogs: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDocReviewLogs_RejectsMalformedPagination(t *testing.T) {
	for _, query := range []string{"page=not-a-number", "page_size=1.5"} {
		t.Run(query, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-review-logs?"+query, nil)
			rec := httptest.NewRecorder()
			if err := ListDocReviewLogs(e.NewContext(req, rec)); err != nil {
				t.Fatalf("ListDocReviewLogs: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListDocReviewLogs_RejectsInvalidNumericAndTimestampFilters(t *testing.T) {
	for _, query := range []string{"input_record_id=x", "run_id=x", "create_start_time=not-a-time", "create_end_time=not-a-time"} {
		t.Run(query, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-review-logs?"+query, nil)
			rec := httptest.NewRecorder()
			if err := ListDocReviewLogs(e.NewContext(req, rec)); err != nil {
				t.Fatalf("ListDocReviewLogs: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListDocReviewLogs_NormalizesPaginationBounds(t *testing.T) {
	for _, test := range []struct {
		query         string
		wantPage      int
		wantPageSize  int
		wantQueryArgs []driver.Value
	}{
		{"page=0&page_size=-1", 1, 50, []driver.Value{50, 0}},
		{"page=2&page_size=501", 2, 500, []driver.Value{500, 500}},
	} {
		t.Run(test.query, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()
			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_logs ")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, unit_type, unit_key,.*FROM kb\.doc_review_logs\s+ORDER BY create_time DESC, id DESC\s+LIMIT \$1 OFFSET \$2`).WithArgs(test.wantQueryArgs...).WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "run_id", "pass", "aspect", "unit_type", "unit_key", "unit_location", "matched_units", "findings", "outcome", "detail", "create_time"}))
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-review-logs?"+test.query, nil)
			rec := httptest.NewRecorder()
			if err := ListDocReviewLogs(e.NewContext(req, rec)); err != nil {
				t.Fatalf("ListDocReviewLogs: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var response struct {
				Page     int `json:"page"`
				PageSize int `json:"page_size"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if response.Page != test.wantPage || response.PageSize != test.wantPageSize {
				t.Fatalf("response=%s", rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}
