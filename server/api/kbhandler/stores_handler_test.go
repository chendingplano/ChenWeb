package kbhandler

import (
	"encoding/json"
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

func expectResolveKnowledgeStoreTable(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT\s+to_regclass\(\$1\)::text AS singular,\s+to_regclass\(\$2\)::text AS plural`).
		WithArgs("kb.knowledge_store", "kb.knowledge_stores").
		WillReturnRows(sqlmock.NewRows([]string{"singular", "plural"}).AddRow("kb.knowledge_store", nil))
}

func newKnowledgeStoreContext(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newKnowledgeStoreIDContext(t *testing.T, method, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/stores/" + id
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/stores/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestListKnowledgeStoresSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveKnowledgeStoreTable(mock)
	query := regexp.QuoteMeta(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM kb.knowledge_store
ORDER BY modify_time DESC, id DESC
`)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "ks_type", "ks_name", "ks_desc", "ks_sync_mode",
		"ks_sources", "status", "notes", "error_msg", "public_info", "private_info",
		"create_time", "modify_time",
	}).AddRow(
		int64(7), "tenant-a", "reference", "Standards Store", "Core standards", "manual",
		`{"docs","specs"}`, "active", "notes", "", `{"visibility":"team"}`, `{"secret":true}`,
		time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(query).WillReturnRows(rows)

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/stores", "")
	if err := ListKnowledgeStores(c); err != nil {
		t.Fatalf("ListKnowledgeStores returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listKnowledgeStoresResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].KSName != "Standards Store" {
		t.Fatalf("expected store name, got %+v", payload.Results[0])
	}
	if len(payload.Results[0].KSSources) != 2 || payload.Results[0].KSSources[0] != "docs" {
		t.Fatalf("expected sources, got %+v", payload.Results[0].KSSources)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateKnowledgeStoreSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveKnowledgeStoreTable(mock)
	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.knowledge_store (
    tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode, ks_sources, status, notes, public_info, private_info
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id
`)
	mock.ExpectQuery(insertQuery).
		WithArgs("tenant-a", "reference", "Standards Store", "Core standards", "manual", sqlmock.AnyArg(), "active", "notes", `{"visibility":"team"}`, `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))

	selectQuery := regexp.QuoteMeta(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM kb.knowledge_store
WHERE id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "ks_type", "ks_name", "ks_desc", "ks_sync_mode",
		"ks_sources", "status", "notes", "error_msg", "public_info", "private_info",
		"create_time", "modify_time",
	}).AddRow(
		int64(7), "tenant-a", "reference", "Standards Store", "Core standards", "manual",
		`{"docs","specs"}`, "active", "notes", "", `{"visibility":"team"}`, `{}`,
		time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(selectQuery).WithArgs(int64(7)).WillReturnRows(rows)

	c, rec := newKnowledgeStoreContext(t, http.MethodPost, "/api/v1/kb/stores", `{
		"tenant_id":"tenant-a",
		"ks_type":"reference",
		"ks_name":"Standards Store",
		"ks_desc":"Core standards",
		"ks_sync_mode":"manual",
		"ks_sources":["docs","specs"],
		"status":"active",
		"notes":"notes",
		"public_info":{"visibility":"team"},
		"private_info":null
	}`)
	if err := CreateKnowledgeStore(c); err != nil {
		t.Fatalf("CreateKnowledgeStore returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload knowledgeStoreDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.ID != 7 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateKnowledgeStoreDefaultsTenantID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveKnowledgeStoreTable(mock)
	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.knowledge_store (
    tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode, ks_sources, status, notes, public_info, private_info
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id
`)
	mock.ExpectQuery(insertQuery).
		WithArgs("-", nil, "Default Tenant Store", nil, "manual", sqlmock.AnyArg(), "active", nil, `{}`, `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(8)))

	selectQuery := regexp.QuoteMeta(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM kb.knowledge_store
WHERE id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "ks_type", "ks_name", "ks_desc", "ks_sync_mode",
		"ks_sources", "status", "notes", "error_msg", "public_info", "private_info",
		"create_time", "modify_time",
	}).AddRow(
		int64(8), "-", nil, "Default Tenant Store", nil, "manual",
		`{}`, "active", nil, nil, `{}`, `{}`,
		time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC), time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(selectQuery).WithArgs(int64(8)).WillReturnRows(rows)

	c, rec := newKnowledgeStoreContext(t, http.MethodPost, "/api/v1/kb/stores", `{
		"ks_name":"Default Tenant Store"
	}`)
	if err := CreateKnowledgeStore(c); err != nil {
		t.Fatalf("CreateKnowledgeStore returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateKnowledgeStoreSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveKnowledgeStoreTable(mock)
	updateQuery := regexp.QuoteMeta("UPDATE kb.knowledge_store SET modify_time = NOW(), ks_desc = $1, ks_sources = $2, status = $3 WHERE id = $4")
	mock.ExpectExec(updateQuery).
		WithArgs("Updated description", sqlmock.AnyArg(), "suspended", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM kb.knowledge_store
WHERE id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "tenant_id", "ks_type", "ks_name", "ks_desc", "ks_sync_mode",
		"ks_sources", "status", "notes", "error_msg", "public_info", "private_info",
		"create_time", "modify_time",
	}).AddRow(
		int64(7), "tenant-a", "reference", "Standards Store", "Updated description", "manual",
		`{"docs","specs","archive"}`, "suspended", "notes", "", `{"visibility":"team"}`, nil,
		time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(selectQuery).WithArgs(int64(7)).WillReturnRows(rows)

	c, rec := newKnowledgeStoreIDContext(t, http.MethodPut, "7", `{
		"ks_desc":"Updated description",
		"ks_sources":["docs","specs","archive"],
		"status":"suspended"
	}`)
	if err := UpdateKnowledgeStore(c); err != nil {
		t.Fatalf("UpdateKnowledgeStore returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeleteKnowledgeStoreNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveKnowledgeStoreTable(mock)
	deleteQuery := regexp.QuoteMeta("DELETE FROM kb.knowledge_store WHERE id = $1")
	mock.ExpectExec(deleteQuery).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newKnowledgeStoreIDContext(t, http.MethodDelete, "999", "")
	if err := DeleteKnowledgeStore(c); err != nil {
		t.Fatalf("DeleteKnowledgeStore returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
