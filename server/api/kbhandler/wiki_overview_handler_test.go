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

func TestWikiOverviewUsesADRCountTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(`SELECT\s+to_regclass\(\$1\)::text AS singular,\s+to_regclass\(\$2\)::text AS plural`).
		WithArgs("kb.input", "kb.inputs").
		WillReturnRows(sqlmock.NewRows([]string{"singular", "plural"}).AddRow("kb.input", "kb.inputs"))

	expectCount := func(table string, value int64) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM " + table)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(value))
	}

	expectCount("kb.inputs", 208)
	expectCount("kb.chunks", 51)
	expectCount("kb.topics", 2639)
	expectCount("kb.semantic_projections", 645)
	expectCount("kb.metrics", 2671)
	expectCount("kb.provisions", 4502)
	expectCount("kb.inventory_items", 17)
	expectCount("kb.scene_objects", 1921)
	expectCount("kb.entities", 22111)
	expectCount("kb.relations", 15582)

	mock.ExpectQuery(`SELECT i\.id,`).
		WithArgs(recentLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "type", "ts"}))
	mock.ExpectQuery(`SELECT i\.id,`).
		WithArgs(recentLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "type", "ts"}))
	mock.ExpectQuery(`SELECT l\.record_id,`).
		WithArgs(recentLimit).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "title", "doc_proc_name", "create_time"}))
	mock.ExpectQuery(`SELECT l\.record_id,`).
		WithArgs(recentLimit).
		WillReturnRows(sqlmock.NewRows([]string{"record_id", "title", "doc_proc_name", "errors", "create_time"}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/wiki-overview", nil)
	rec := httptest.NewRecorder()

	if err := WikiOverview(e.NewContext(req, rec)); err != nil {
		t.Fatalf("WikiOverview returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp wikiOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Counts.Documents != 208 {
		t.Fatalf("documents=%d, want 208", resp.Counts.Documents)
	}
	if resp.Counts.PartsComponents != 17 {
		t.Fatalf("parts_components=%d, want 17", resp.Counts.PartsComponents)
	}
	if resp.Counts.Entities != 22111 || resp.Counts.Relations != 15582 {
		t.Fatalf("entity/relation counts=%d/%d, want 22111/15582", resp.Counts.Entities, resp.Counts.Relations)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
