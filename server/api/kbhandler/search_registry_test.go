package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
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
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "")
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
			pq.Array([]string{}),
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

func TestInsertSearchRegistryRowsSemanticModeWritesEmbedding(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "true")
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
			pq.Array([]string{}),
			"Energy summary category performance",
			kbsearch.FormatVectorLiteral([]float64{0.125, -0.5, 1}),
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
		EmbeddingText:   "Energy summary category performance",
		Embedding:       []float64{0.125, -0.5, 1},
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

// searchCall records what a handler passed to the search layer.
type searchCallRecord struct {
	artifactType, query string
	filters             artifactSearchFilters
	page, pageSize      int
}

// stubRegistrySearch replaces the search layer for one test and returns what the handler asked for.
func stubRegistrySearch(t *testing.T, total int64, results []artifactSearchResult) *searchCallRecord {
	t.Helper()
	rec := &searchCallRecord{}
	oldCount, oldQuery := registrySearchCount, registrySearchQuery
	t.Cleanup(func() { registrySearchCount, registrySearchQuery = oldCount, oldQuery })
	registrySearchCount = func(_ *sql.DB, artifactType, query string, filters artifactSearchFilters, _ registrySearchConfig) (int64, error) {
		rec.artifactType, rec.query, rec.filters = artifactType, query, filters
		return total, nil
	}
	registrySearchQuery = func(_ *sql.DB, artifactType, query string, filters artifactSearchFilters, page, pageSize int, _ registrySearchConfig) ([]artifactSearchResult, error) {
		rec.artifactType, rec.query, rec.filters, rec.page, rec.pageSize = artifactType, query, filters, page, pageSize
		return results, nil
	}
	return rec
}

func TestSearchSummariesReturnsRegistryResults(t *testing.T) {
	rec := stubRegistrySearch(t, 1, []artifactSearchResult{{
		ArtifactType: "summary", ArtifactID: "7_sum_1", InputRecordID: 7, PrimaryLabel: "Energy summary", Score: 0.84,
		Snippet: "Energy summary highlights the target",
	}})
	c, resp := newRegistrySearchContext(t, "q=energy&input_record_id=7")
	if err := SearchSummaries(c); err != nil {
		t.Fatalf("SearchSummaries returned error: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	var payload artifactSearchResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ArtifactType != "summary" || payload.Total != 1 || len(payload.Results) != 1 || payload.Results[0].PrimaryLabel != "Energy summary" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if rec.artifactType != "summary" || rec.query != "energy" || rec.page != 1 || rec.pageSize != 20 ||
		rec.filters.InputRecordID == nil || *rec.filters.InputRecordID != 7 {
		t.Fatalf("search layer got %+v", rec)
	}
}

func TestSearchTopicsAndProductsPassTheirArtifactType(t *testing.T) {
	for _, tc := range []struct {
		path, want string
		call       func(echo.Context) error
	}{
		{"/api/v1/kb/topics/search?q=battery", "topic", SearchTopics},
		{"/api/v1/kb/products/search?q=pump", "product", SearchProducts},
	} {
		rec := stubRegistrySearch(t, 1, []artifactSearchResult{{ArtifactType: tc.want, ArtifactID: "1", InputRecordID: 1}})
		e := echo.New()
		resp := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, tc.path, nil), resp)
		if err := tc.call(c); err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if resp.Code != http.StatusOK || rec.artifactType != tc.want || rec.pageSize != 20 {
			t.Fatalf("%s: code=%d search layer got %+v", tc.path, resp.Code, rec)
		}
	}
}

func TestSearchAllArtifactsReturnsMixedRegistryResults(t *testing.T) {
	rec := stubRegistrySearch(t, 2, []artifactSearchResult{
		{ArtifactType: "summary", ArtifactID: "88_sum_1", InputRecordID: 88, Score: 0.88},
		{ArtifactType: "provision", ArtifactID: "88_prv_3", InputRecordID: 88, Score: 0.81},
	})
	e := echo.New()
	resp := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/search?q=safety&input_record_id=88", nil), resp)
	if err := SearchAllArtifacts(c); err != nil {
		t.Fatalf("SearchAllArtifacts returned error: %v", err)
	}
	var payload artifactSearchResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != http.StatusOK || len(payload.Results) != 2 || payload.Results[0].ArtifactType == "" || payload.Results[1].ArtifactType == "" {
		t.Fatalf("unexpected response %d: %s", resp.Code, resp.Body.String())
	}
	if rec.artifactType != "all" || rec.filters.InputRecordID == nil || *rec.filters.InputRecordID != 88 {
		t.Fatalf("search layer got %+v", rec)
	}
}

func TestRegistryLexicalBackendDefaultsToParadeDB(t *testing.T) {
	t.Setenv("SEARCH_LEXICAL_BACKEND", "")

	if got := registryLexicalBackend(); got != lexicalBackendParadeDB {
		t.Fatalf("backend=%q, want %q", got, lexicalBackendParadeDB)
	}
}

func TestRegistryLexicalBackendAllowsParadeDB(t *testing.T) {
	t.Setenv("SEARCH_LEXICAL_BACKEND", " ParadeDB ")

	if got := registryLexicalBackend(); got != lexicalBackendParadeDB {
		t.Fatalf("backend=%q, want %q", got, lexicalBackendParadeDB)
	}
}
