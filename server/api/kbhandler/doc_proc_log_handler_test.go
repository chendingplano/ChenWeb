package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListDocProcLogs_UsesMSUsedResponseShape(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE entry_type = $1")).
		WithArgs("doc_proc_summary").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\)\s+FROM kb\.doc_proc_logs\s+WHERE entry_type = \$1\s+ORDER BY create_time DESC\s+LIMIT \$2 OFFSET \$3`
	mock.ExpectQuery(listQuery).
		WithArgs("doc_proc_summary", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
		}).AddRow(
			int64(17), "summary run", "generate_topics", `{topic-model}`, "topic-prompt", int64(81), "66% (2/3)", "doc_proc_summary",
			nil, nil, nil, nil, nil, `{"topics_generated":5}`, int64(1800), nil, "2026-05-27T12:30:00+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?entry_type=doc_proc_summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	results, ok := payload["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results=%#v", payload["results"])
	}
	row, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("row=%#v", results[0])
	}
	if _, ok := row["ms_used"]; !ok {
		t.Fatalf("expected ms_used in response row: %#v", row)
	}
	if got := row["record_id"]; got != float64(81) {
		t.Fatalf("record_id=%v, want 81", got)
	}
	if got := row["proc_progress"]; got != "66% (2/3)" {
		t.Fatalf("proc_progress=%v, want 66%% (2/3)", got)
	}
	if _, ok := row["duration_ms"]; ok {
		t.Fatalf("did not expect duration_ms in response row: %#v", row)
	}
	if _, ok := row["start_time"]; ok {
		t.Fatalf("did not expect start_time in response row: %#v", row)
	}
	if _, ok := row["end_time"]; ok {
		t.Fatalf("did not expect end_time in response row: %#v", row)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcLogs_AcceptsOrderParams(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs ")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\)\s+FROM kb\.doc_proc_logs\s+ORDER BY doc_proc_name ASC\s+LIMIT \$1 OFFSET \$2`
	mock.ExpectQuery(listQuery).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
		}).AddRow(
			int64(18), "summary run", "blocking", `{}`, "topic-prompt", int64(18), nil, "doc_proc_summary",
			nil, nil, nil, nil, nil, `{}`, int64(0), nil, "2026-05-27T12:30:00+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?order_by=doc_proc_name&order_dir=asc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcLogs_FiltersByRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE doc_proc_name = $1 AND record_id = $2")).
		WithArgs("extract_entity_relation", int64(170)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\)\s+FROM kb\.doc_proc_logs\s+WHERE doc_proc_name = \$1 AND record_id = \$2\s+ORDER BY create_time DESC\s+LIMIT \$3 OFFSET \$4`
	mock.ExpectQuery(listQuery).
		WithArgs("extract_entity_relation", int64(170), 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
		}).AddRow(
			int64(21), "extract entity relation", "extract_entity_relation", `{entity-model}`, "entity-prompt", int64(170), "67% (8/12)", "doc_proc_summary",
			nil, nil, nil, nil, nil, `{}`, int64(1200), nil, "2026-05-30T15:43:04+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?doc_proc_name=extract_entity_relation&record_id=170", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
