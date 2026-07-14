package kbhandler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

func expectDefaultStoreIDLookup(mock sqlmock.Sqlmock, ksName string, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1 ORDER BY id`)).
		WithArgs(ksName).
		WillReturnRows(rows)
}

func expectDefaultStoreFetch(mock sqlmock.Sqlmock, id int64, ksName string) {
	query := regexp.QuoteMeta(`
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
		id, "tenant-a", "research", ksName, "Personal research store", "manual",
		`{"Research"}`, "active", nil, nil, `{}`, `{}`,
		time.Date(2026, 4, 25, 15, 9, 18, 0, time.UTC), time.Date(2026, 4, 25, 15, 9, 18, 0, time.UTC),
	)
	mock.ExpectQuery(query).WithArgs(id).WillReturnRows(rows)
}

// withDefaultKnowledgeStoreName points [frontend].default_knowledge_store at
// ksName for the duration of the test.
func withDefaultKnowledgeStoreName(t *testing.T, ksName string) {
	t.Helper()
	old := appconfig.AppConfig.Frontend.DefaultKnowledgeStore
	appconfig.AppConfig.Frontend.DefaultKnowledgeStore = ksName
	t.Cleanup(func() { appconfig.AppConfig.Frontend.DefaultKnowledgeStore = old })
}

func TestGetDefaultKnowledgeStoreSelectsConfiguredStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	withDefaultKnowledgeStoreName(t, "Research")
	expectResolveKnowledgeStoreTable(mock)
	expectDefaultStoreIDLookup(mock, "Research", 4)
	expectDefaultStoreFetch(mock, 4, "Research")

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/default-store", "")
	if err := GetDefaultKnowledgeStore(c); err != nil {
		t.Fatalf("GetDefaultKnowledgeStore returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload defaultKnowledgeStoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record == nil {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Record.ID != 4 || payload.Record.KSName != "Research" {
		t.Fatalf("unexpected record: %+v", payload.Record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGetDefaultKnowledgeStoreReportsMissingConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// Whitespace-only is treated the same as an absent key.
	withDefaultKnowledgeStoreName(t, "   ")

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/default-store", "")
	if err := GetDefaultKnowledgeStore(c); err != nil {
		t.Fatalf("GetDefaultKnowledgeStore returned error: %v", err)
	}

	var payload defaultKnowledgeStoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Status || payload.Record != nil {
		t.Fatalf("expected no store to be selected, got %+v", payload)
	}
	if !strings.Contains(payload.ErrorMsg, "CWB_KB_S_401") {
		t.Fatalf("expected a not-configured error, got %q", payload.ErrorMsg)
	}
	// The database must not be touched when the key is missing.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGetDefaultKnowledgeStoreReportsNoMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	withDefaultKnowledgeStoreName(t, "Missing Store")
	expectResolveKnowledgeStoreTable(mock)
	expectDefaultStoreIDLookup(mock, "Missing Store")

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/default-store", "")
	if err := GetDefaultKnowledgeStore(c); err != nil {
		t.Fatalf("GetDefaultKnowledgeStore returned error: %v", err)
	}

	var payload defaultKnowledgeStoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Status || payload.Record != nil {
		t.Fatalf("expected no store to be selected, got %+v", payload)
	}
	if !strings.Contains(payload.ErrorMsg, "CWB_KB_S_406") {
		t.Fatalf("expected a no-match error, got %q", payload.ErrorMsg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestGetDefaultKnowledgeStoreReportsAmbiguousMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	withDefaultKnowledgeStoreName(t, "Research")
	expectResolveKnowledgeStoreTable(mock)
	expectDefaultStoreIDLookup(mock, "Research", 4, 9)

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/default-store", "")
	if err := GetDefaultKnowledgeStore(c); err != nil {
		t.Fatalf("GetDefaultKnowledgeStore returned error: %v", err)
	}

	var payload defaultKnowledgeStoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Status || payload.Record != nil {
		t.Fatalf("expected no store to be selected, got %+v", payload)
	}
	if !strings.Contains(payload.ErrorMsg, "CWB_KB_S_407") {
		t.Fatalf("expected an ambiguous-match error, got %q", payload.ErrorMsg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
