package kbhandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestBuildWhereClauseDocTypeAndFileName(t *testing.T) {
	whereSQL, args, err := buildWhereClause("pdf", "all", "report", nil, nil)
	if err != nil {
		t.Fatalf("buildWhereClause returned error: %v", err)
	}

	if !strings.Contains(whereSQL, "LOWER(i.type) = LOWER($1)") {
		t.Fatalf("expected doc type filter in whereSQL, got: %s", whereSQL)
	}
	if !strings.Contains(whereSQL, "COALESCE(i.file_name, '') ILIKE $2") {
		t.Fatalf("expected file_name filter in whereSQL, got: %s", whereSQL)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClauseParseStates(t *testing.T) {
	tests := []struct {
		name       string
		parseState string
		wantPart   string
	}{
		{
			name:       "pending",
			parseState: "pending",
			wantPart:   "NOT EXISTS",
		},
		{
			name:       "parsed_success",
			parseState: "parsed_success",
			wantPart:   "LOWER(COALESCE(st->>'status', '')) = 'success'",
		},
		{
			name:       "parsed_failed",
			parseState: "parsed_failed",
			wantPart:   "LOWER(COALESCE(st->>'status', '')) <> 'success'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			whereSQL, _, err := buildWhereClause("all", tc.parseState, "", nil, nil)
			if err != nil {
				t.Fatalf("buildWhereClause returned error: %v", err)
			}
			if !strings.Contains(whereSQL, "LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')") {
				t.Fatalf("expected whereSQL to include parse/parsing operation matching, got: %s", whereSQL)
			}
			if !strings.Contains(whereSQL, tc.wantPart) {
				t.Fatalf("expected whereSQL to contain %q, got: %s", tc.wantPart, whereSQL)
			}
		})
	}
}

func TestBuildWhereClauseRejectsInvalidParseState(t *testing.T) {
	_, _, err := buildWhereClause("all", "bad-state", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid parse_state")
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := parsePositiveInt("3", 1); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := parsePositiveInt("-7", 1); got != 1 {
		t.Fatalf("expected default 1, got %d", got)
	}
	if got := parsePositiveInt("abc", 9); got != 9 {
		t.Fatalf("expected default 9, got %d", got)
	}
}

func TestNormalizeParseStateAliases(t *testing.T) {
	if got := normalizeParseState(""); got != "all" {
		t.Fatalf("expected all, got %q", got)
	}
	if got := normalizeParseState("success"); got != "parsed_success" {
		t.Fatalf("expected parsed_success, got %q", got)
	}
	if got := normalizeParseState("failed"); got != "parsed_failed" {
		t.Fatalf("expected parsed_failed, got %q", got)
	}
}

func TestParseTimeQuery(t *testing.T) {
	valid := []string{
		"2026-03-26T14:22:00Z",
		"2026-03-26T14:22",
		"2026-03-26 14:22:33",
		"2026-03-26",
	}
	for _, input := range valid {
		if _, err := parseTimeQuery(input); err != nil {
			t.Fatalf("parseTimeQuery(%q) unexpected error: %v", input, err)
		}
	}
	if _, err := parseTimeQuery("not-a-time"); err == nil {
		t.Fatal("expected error for invalid time input")
	}
}

func TestBuildWhereClauseWithTimeRange(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)
	whereSQL, args, err := buildWhereClause("all", "all", "", &start, &end)
	if err != nil {
		t.Fatalf("buildWhereClause returned error: %v", err)
	}
	if !strings.Contains(whereSQL, "i.create_time >= $1") {
		t.Fatalf("expected start time clause, got: %s", whereSQL)
	}
	if !strings.Contains(whereSQL, "i.create_time <= $2") {
		t.Fatalf("expected end time clause, got: %s", whereSQL)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestListInputsSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	countQuery := regexp.QuoteMeta(`SELECT COUNT(1) FROM kb.inputs i WHERE LOWER(i.type) = LOWER($1) AND EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')
			  AND LOWER(COALESCE(st->>'status', '')) = 'success'
		) AND COALESCE(i.file_name, '') ILIKE $2`)
	mock.ExpectQuery(countQuery).
		WithArgs("pdf", "%report%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	dataQuery := regexp.QuoteMeta(`
SELECT
    i.id,
    i.name,
    i.type,
    i.title,
    i.doc_no,
    i.source,
    i.file_name,
    i.backup_filename,
    i.result_filename,
    i.publish_date,
    i.authors,
    i.owner,
    COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time,
    i.modify_time,
    i.public_info,
    i.private_info,
    i.notes,
    i.error_msg
FROM kb.inputs i
 WHERE LOWER(i.type) = LOWER($1) AND EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(i.status, '[]'::jsonb)) AS st
			WHERE LOWER(COALESCE(st->>'operation', '')) IN ('parsing', 'parse')
			  AND LOWER(COALESCE(st->>'status', '')) = 'success'
		) AND COALESCE(i.file_name, '') ILIKE $2 ORDER BY i.create_time DESC LIMIT $3 OFFSET $4`)
	rows := sqlmock.NewRows([]string{
		"id", "name", "type", "title", "doc_no", "source", "file_name",
		"backup_filename", "result_filename", "publish_date", "authors", "owner",
		"status", "create_time", "modify_time", "public_info", "private_info",
		"notes", "error_msg",
	}).AddRow(
		int64(101), "Report A", "pdf", "Annual Report", nil, "upload", "/tmp/report-a.pdf",
		"/backup/report-a.pdf", "/result/report-a.json", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "Alice", int64(7),
		`[{"operation":"parsing","status":"success"}]`, time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 2, 12, 10, 0, 0, time.UTC),
		`{"visibility":"public"}`, `{"internal":"yes"}`, "note", "",
	)
	mock.ExpectQuery(dataQuery).
		WithArgs("pdf", "%report%", 50, 0).
		WillReturnRows(rows)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?doc_type=pdf&parse_state=parsed_success&file_name=report&page=1&page_size=50", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload listInputsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, got false")
	}
	if payload.Total != 1 {
		t.Fatalf("expected total=1, got %d", payload.Total)
	}
	if payload.PageSize != 50 {
		t.Fatalf("expected page_size=50, got %d", payload.PageSize)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(payload.Results))
	}
	if payload.Results[0].ID != 101 {
		t.Fatalf("expected result id=101, got %d", payload.Results[0].ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListInputsInvalidStartTime(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?start_time=bad-time", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListInputsInvalidParseState(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?parse_state=unknown", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListInputsCountQueryFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM kb.inputs i`)).
		WillReturnError(errors.New("count failed"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestListInputsDataQueryFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM kb.inputs i`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
    i.id,
    i.name,
    i.type,
    i.title,
    i.doc_no,
    i.source,
    i.file_name,
    i.backup_filename,
    i.result_filename,
    i.publish_date,
    i.authors,
    i.owner,
    COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time,
    i.modify_time,
    i.public_info,
    i.private_info,
    i.notes,
    i.error_msg
FROM kb.inputs i
 ORDER BY i.create_time DESC LIMIT $1 OFFSET $2`)).
		WithArgs(50, 0).
		WillReturnError(errors.New("data query failed"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestListInputsPageSizeCap(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM kb.inputs i`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(`ORDER BY i.create_time DESC LIMIT $1 OFFSET $2`)).
		WithArgs(500, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "title", "doc_no", "source", "file_name",
			"backup_filename", "result_filename", "publish_date", "authors", "owner",
			"status", "create_time", "modify_time", "public_info", "private_info",
			"notes", "error_msg",
		}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?page_size=9999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListInputsWithDateQueryParams(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM kb.inputs i WHERE i.create_time >= $1 AND i.create_time <= $2`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta(`ORDER BY i.create_time DESC LIMIT $3 OFFSET $4`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "title", "doc_no", "source", "file_name",
			"backup_filename", "result_filename", "publish_date", "authors", "owner",
			"status", "create_time", "modify_time", "public_info", "private_info",
			"notes", "error_msg",
		}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inputs?start_time=2026-03-01&end_time=2026-03-31&page=2&page_size=20", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListInputs(c); err != nil {
		t.Fatalf("ListInputs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
