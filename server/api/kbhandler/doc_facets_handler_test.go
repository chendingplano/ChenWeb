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

func TestGetDocFacetsReturnsFacets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
SELECT record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number
FROM kb.doc_facets
WHERE record_id = $1`)
	mock.ExpectQuery(query).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{
			"record_id", "ks_store_id", "knowledge_store_binding", "input_doc_type", "source_language", "has_document_number",
		}).AddRow(int64(91), int64(42), "bound", "pdf", "en", true))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-facets?record_id=91", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetDocFacets(c); err != nil {
		t.Fatalf("GetDocFacets returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result=%#v", payload["result"])
	}
	if got := result["InputDocType"]; got != "pdf" {
		t.Fatalf("InputDocType=%v want pdf", got)
	}
	if got := result["KnowledgeStoreBinding"]; got != "bound" {
		t.Fatalf("KnowledgeStoreBinding=%v want bound", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetDocFacetsReturnsNilResultWhenNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
SELECT record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number
FROM kb.doc_facets
WHERE record_id = $1`)
	mock.ExpectQuery(query).WithArgs(int64(404)).WillReturnRows(sqlmock.NewRows([]string{
		"record_id", "ks_store_id", "knowledge_store_binding", "input_doc_type", "source_language", "has_document_number",
	}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-facets?record_id=404", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetDocFacets(c); err != nil {
		t.Fatalf("GetDocFacets returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if payload["result"] != nil {
		t.Fatalf("expected nil result, got %#v", payload["result"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetDocFacetsRejectsMissingRecordID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-facets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetDocFacets(c); err != nil {
		t.Fatalf("GetDocFacets returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
