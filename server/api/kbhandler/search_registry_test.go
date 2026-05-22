package kbhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestDeleteSearchRegistryRowsForRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.search_artifacts WHERE artifact_type = $1 AND input_record_id = $2`)).
		WithArgs("summary", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	deleted, err := kbsearch.DeleteSearchRegistryRowsForRecord(context.Background(), db, "summary", 42)
	if err != nil {
		t.Fatalf("DeleteSearchRegistryRowsForRecord returned error: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted=%d", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestInsertSearchRegistryRowsUpsertsNormalizedRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO kb.search_artifacts").
		WithArgs(
			"summary", "42_sum_1", int64(42), nil,
			"Energy summary", "Level 1",
			"Energy summary category performance",
			"Energy summary",
			"doc.pdf",
			"doc.pdf",
			`["performance"]`,
			`["10:11"]`,
			`{"kind":"summary"}`,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rows := []kbsearch.RegistryRow{{
		ArtifactType:    "summary",
		ArtifactID:      "42_sum_1",
		InputRecordID:   42,
		PrimaryLabel:    "Energy summary",
		SecondaryLabel:  "Level 1",
		SearchDocument:  "Energy summary category performance",
		SnippetBasis:    "Energy summary",
		SourceTitle:     "doc.pdf",
		SourceFilename:  "doc.pdf",
		CategoryPaths:   json.RawMessage(`["performance"]`),
		SourceLineSpans: json.RawMessage(`["10:11"]`),
		SemanticPayload: json.RawMessage(`{"kind":"summary"}`),
	}}

	inserted, err := kbsearch.InsertSearchRegistryRows(context.Background(), db, rows)
	if err != nil {
		t.Fatalf("InsertSearchRegistryRows returned error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted=%d", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func newRegistrySearchContext(t *testing.T, rawQuery string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/summaries/search?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSearchSummariesReturnsRegistryResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.search_artifacts sa WHERE").
		WithArgs("energy", "summary", int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery("WITH query_input AS").
		WithArgs("energy", "summary", int64(7), 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "score", "snippet",
		}).AddRow(
			"summary", "7_sum_1", int64(7), "Energy summary", "Level 1", "doc.pdf", "doc.pdf", `["10:11"]`, `{"kind":"summary"}`, 0.84, "Energy summary highlights the target",
		))

	c, rec := newRegistrySearchContext(t, "q=energy&input_record_id=7")
	if err := SearchSummaries(c); err != nil {
		t.Fatalf("SearchSummaries returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload artifactSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ArtifactType != "summary" {
		t.Fatalf("artifact_type=%q", payload.ArtifactType)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len=%d", len(payload.Results))
	}
	if payload.Results[0].PrimaryLabel != "Energy summary" {
		t.Fatalf("primary_label=%q", payload.Results[0].PrimaryLabel)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestSearchTopicsReturnsRegistryResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.search_artifacts sa WHERE").
		WithArgs("battery", "topic").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery("WITH query_input AS").
		WithArgs("battery", "topic", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "score", "snippet",
		}).AddRow(
			"topic", "9_tpc_1", int64(9), "Battery safety", "requirement", "battery.pdf", "battery.pdf", `["2:14"]`, `{"kind":"topic"}`, 0.73, "Battery safety appears in the charging requirements",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/topics/search?q=battery", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := SearchTopics(c); err != nil {
		t.Fatalf("SearchTopics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestSearchProductsReturnsRegistryResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.search_artifacts sa WHERE").
		WithArgs("pump", "product").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery("WITH query_input AS").
		WithArgs("pump", "product", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "score", "snippet",
		}).AddRow(
			"product", "12_prd_1", int64(12), "Infusion pump", "equipment", "products.pdf", "products.pdf", `["1:20"]`, `{"kind":"product"}`, 0.91, "Infusion pump requires monthly inspection",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/products/search?q=pump", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := SearchProducts(c); err != nil {
		t.Fatalf("SearchProducts returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestSearchAllArtifactsReturnsMixedRegistryResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.search_artifacts sa WHERE").
		WithArgs("safety", int64(88)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	mock.ExpectQuery("WITH query_input AS").
		WithArgs("safety", int64(88), 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "score", "snippet",
		}).
			AddRow("summary", "88_sum_1", int64(88), "Safety overview", "Level 1", "doc.pdf", "doc.pdf", `["1:10"]`, `{"kind":"summary"}`, 0.88, "Safety overview text").
			AddRow("provision", "88_prv_3", int64(88), "Protective enclosure", "mandatory", "doc.pdf", "doc.pdf", `["2:12"]`, `{"kind":"provision"}`, 0.81, "Protective enclosure shall remain closed"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/search?q=safety&input_record_id=88", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := SearchAllArtifacts(c); err != nil {
		t.Fatalf("SearchAllArtifacts returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload artifactSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("results len=%d", len(payload.Results))
	}
	if payload.Results[0].ArtifactType == "" || payload.Results[1].ArtifactType == "" {
		t.Fatalf("expected artifact_type on mixed results: %#v", payload.Results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestBuildRegistrySearchWhereClauseIncludesSemanticFilters(t *testing.T) {
	cfg := registrySearchConfig{
		dictionary:     "simple",
		phraseFriendly: true,
	}

	whereSQL, args := buildRegistrySearchWhereClause("topic", "battery", artifactSearchFilters{
		InputRecordID: ptrInt64(7),
		CategoryPath:  "safety",
		TopicType:     "requirement",
	}, cfg)

	if !strings.Contains(whereSQL, "sa.artifact_type = $2") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if !strings.Contains(whereSQL, "sa.category_paths::text ILIKE $4") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if !strings.Contains(whereSQL, "sa.semantic_payload->>'topic_type'") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if len(args) != 5 {
		t.Fatalf("args=%v", args)
	}
}

func TestBuildRegistrySearchWhereClauseSupportsGlobalArtifactTypes(t *testing.T) {
	cfg := registrySearchConfig{
		dictionary:     "simple",
		phraseFriendly: true,
	}

	whereSQL, args := buildRegistrySearchWhereClause("all", "safety", artifactSearchFilters{
		ArtifactTypes: []string{"summary", "provision"},
	}, cfg)

	if !strings.Contains(whereSQL, "sa.artifact_type IN ($2, $3)") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if len(args) != 3 {
		t.Fatalf("args=%v", args)
	}
}

func TestBuildRegistrySearchWhereClauseAddsCJKSubstringFallback(t *testing.T) {
	cfg := registrySearchConfig{
		dictionary:     "simple",
		phraseFriendly: true,
	}

	whereSQL, args := buildRegistrySearchWhereClause("metric", "平均瞬时日差", artifactSearchFilters{}, cfg)

	if !strings.Contains(whereSQL, "coalesce(sa.primary_label, '') ILIKE '%' || $1 || '%'") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if !strings.Contains(whereSQL, "coalesce(sa.search_document, '') ILIKE '%' || $1 || '%'") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if len(args) != 2 || args[0] != "平均瞬时日差" || args[1] != "metric" {
		t.Fatalf("args=%v", args)
	}
}

func ptrInt64(v int64) *int64 { return &v }
