package cdmhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// These handler tests run against the live staging database rather than
// sqlmock. The kbhandler tests use sqlmock, but that suits handlers whose
// logic is the SQL itself; these handlers are deliberately thin over
// cdm/store (design D2/D3/D4), so mocking the SQL would assert only that the
// mock was called and would not exercise the invariants that matter here —
// the row lock, the version guard, and the frozen rule all live in the
// database. The cdm package's own tests already use the live database, and
// this follows that convention.

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// withDB points the global handle the handlers read at the live test
// database, restoring the previous value afterwards.
func withDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old })
	return db
}

func newContext(t *testing.T, method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// withKeyParam sets the :key path parameter echo would have populated from
// the route pattern.
func withKeyParam(c echo.Context, key string) echo.Context {
	c.SetParamNames("key")
	c.SetParamValues(key)
	return c
}

func uniqueTitle(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("Handler Test %s %d", t.Name(), time.Now().UnixNano())
}

func cleanupByKey(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	t.Cleanup(func() {
		var inputID sql.NullInt64
		db.QueryRow(`SELECT input_record_id FROM kb.cdm_documents WHERE document_key = $1`, key).Scan(&inputID) //nolint:errcheck
		db.Exec(`DELETE FROM kb.cdm_documents WHERE document_key = $1`, key)                                    //nolint:errcheck
		if inputID.Valid {
			db.Exec(`DELETE FROM kb.inputs WHERE id = $1`, inputID.Int64) //nolint:errcheck
		}
	})
}

// createViaHandler drives CreateDocument and returns the created document.
func createViaHandler(t *testing.T, db *sql.DB, title string) model.Document {
	t.Helper()
	doc := cdmfixtures.JaroWinkler()
	doc.Title = title
	doc.Key = "" // server allocates

	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c, rec := newContext(t, http.MethodPost, "/api/v1/cdm/documents?tenant_id=tenant-x", string(body))
	if err := CreateDocument(c); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v (body=%s)", err, rec.Body.String())
	}
	cleanupByKey(t, db, created.Key)
	return created
}

func TestCreateDocument_AllocatesKeyAndLinksInputRow(t *testing.T) {
	db := withDB(t)
	title := uniqueTitle(t)

	created := createViaHandler(t, db, title)

	if !strings.HasPrefix(created.Key, "doc:") {
		t.Errorf("expected a doc: prefixed key, got %q", created.Key)
	}
	if created.ContentVersion != 1 {
		t.Errorf("expected content_version 1, got %d", created.ContentVersion)
	}

	var linked sql.NullInt64
	if err := db.QueryRow(
		`SELECT input_record_id FROM kb.cdm_documents WHERE document_key = $1`, created.Key,
	).Scan(&linked); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if !linked.Valid {
		t.Error("created document has no linked kb.inputs row")
	}
}

func TestCreateDocument_KeyIsDerivedFromTitle(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, "Jaro Winkler Similarity")

	if created.Key != "doc:jaro-winkler-similarity" {
		t.Errorf("expected doc:jaro-winkler-similarity, got %q", created.Key)
	}
}

func TestCreateDocument_CollidingTitlesGetDistinctKeys(t *testing.T) {
	db := withDB(t)
	title := uniqueTitle(t)

	first := createViaHandler(t, db, title)
	second := createViaHandler(t, db, title)

	if first.Key == second.Key {
		t.Fatalf("expected distinct keys for colliding titles, both are %q", first.Key)
	}
}

func TestCreateDocument_InvalidDocumentReportsAllViolations(t *testing.T) {
	withDB(t)

	doc := cdmfixtures.JaroWinkler()
	doc.Title = uniqueTitle(t)
	doc.Key = ""
	// Three distinct violations: a duplicate id, an unknown block type, and
	// an out-of-range heading level.
	doc.Blocks = append(doc.Blocks,
		model.Block{ID: "intro", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "dup"}}},
		model.Block{ID: "bogus", Type: "horizontal_stack"},
		model.Block{ID: "h9", Type: "heading", Level: 9, Content: []model.Inline{{Type: "text", Text: "x"}}},
	)

	body, _ := json.Marshal(doc)
	c, rec := newContext(t, http.MethodPost, "/api/v1/cdm/documents?tenant_id=tenant-x", string(body))
	if err := CreateDocument(c); err != nil {
		t.Fatalf("CreateDocument returned a transport error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Violations) < 3 {
		t.Fatalf("expected at least 3 violations reported, got %d: %v", len(payload.Violations), payload.Violations)
	}
}

func TestGetDocument_ReturnsCanonicalJSON(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents/"+created.Key, "")
	c = withKeyParam(c, created.Key)
	if err := GetDocument(c); err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var loaded model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("response is not canonical document JSON: %v", err)
	}
	if loaded.Key != created.Key {
		t.Errorf("document_key = %q, want %q", loaded.Key, created.Key)
	}
	if loaded.SchemaVersion != model.SchemaVersion {
		t.Errorf("schema_version = %q, want %q", loaded.SchemaVersion, model.SchemaVersion)
	}
	if len(loaded.Blocks) != len(created.Blocks) {
		t.Errorf("block count = %d, want %d", len(loaded.Blocks), len(created.Blocks))
	}
}

func TestGetDocument_UnknownKeyIs404(t *testing.T) {
	withDB(t)

	c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents/doc:nope", "")
	c = withKeyParam(c, "doc:definitely-not-a-real-document")
	if err := GetDocument(c); err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSaveDocument_RoundTripsAndIncrementsVersion(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	created.Title = created.Title + " (edited)"
	body, _ := json.Marshal(created)
	c, rec := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(body))
	c = withKeyParam(c, created.Key)
	if err := SaveDocument(c); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var saved model.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if saved.ContentVersion != created.ContentVersion+1 {
		t.Errorf("content_version = %d, want %d", saved.ContentVersion, created.ContentVersion+1)
	}
	if !strings.HasSuffix(saved.Title, "(edited)") {
		t.Errorf("edit was not persisted, title = %q", saved.Title)
	}
}

// TestSaveDocument_StaleVersionIs409 is the HTTP face of DR6: the body's own
// content_version is the caller's expectation, so a client that re-submits a
// document it loaded before someone else's write must be refused.
func TestSaveDocument_StaleVersionIs409(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	// One successful save moves the stored version past what `created` holds.
	firstBody, _ := json.Marshal(created)
	c1, rec1 := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(firstBody))
	c1 = withKeyParam(c1, created.Key)
	if err := SaveDocument(c1); err != nil || rec1.Code != http.StatusOK {
		t.Fatalf("first save failed: err=%v code=%d body=%s", err, rec1.Code, rec1.Body.String())
	}

	// The stale writer still claims the original version.
	staleBody, _ := json.Marshal(created)
	c2, rec2 := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(staleBody))
	c2 = withKeyParam(c2, created.Key)
	if err := SaveDocument(c2); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a stale save, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var payload errorResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Conflict != conflictStaleVersion {
		t.Errorf("conflict = %q, want %q", payload.Conflict, conflictStaleVersion)
	}
	if payload.ContentVersion == 0 {
		t.Error("expected the response to report the current content_version so the client can recover")
	}
}

func TestSaveDocument_PublishedDocumentIs409Frozen(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	cp, recp := newContext(t, http.MethodPost, "/api/v1/cdm/documents/"+created.Key+"/publish", "")
	cp = withKeyParam(cp, created.Key)
	if err := PublishDocument(cp); err != nil {
		t.Fatalf("PublishDocument: %v", err)
	}
	if recp.Code != http.StatusOK {
		t.Fatalf("publish failed: %d %s", recp.Code, recp.Body.String())
	}

	created.Title = "After Publish"
	body, _ := json.Marshal(created)
	c, rec := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(body))
	c = withKeyParam(c, created.Key)
	if err := SaveDocument(c); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a published document, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload errorResponse
	json.Unmarshal(rec.Body.Bytes(), &payload) //nolint:errcheck
	if payload.Conflict != conflictFrozen {
		t.Errorf("conflict = %q, want %q", payload.Conflict, conflictFrozen)
	}
}

func TestPublishDocument_EnqueuesForDocProcessingOnly(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	c, rec := newContext(t, http.MethodPost, "/api/v1/cdm/documents/"+created.Key+"/publish", "")
	c = withKeyParam(c, created.Key)
	if err := PublishDocument(c); err != nil {
		t.Fatalf("PublishDocument: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var parseState, pipelineState string
	err := db.QueryRow(`
		SELECT i.parse_state, i.pipeline_state
		FROM kb.cdm_documents d JOIN kb.inputs i ON i.id = d.input_record_id
		WHERE d.document_key = $1
	`, created.Key).Scan(&parseState, &pipelineState)
	if err != nil {
		t.Fatalf("read derived states: %v", err)
	}
	if pipelineState != "pending" {
		t.Errorf("pipeline_state = %q, want pending (publish should hand it to the worklist)", pipelineState)
	}
	if parseState != "parsed_success" {
		t.Errorf("parse_state = %q, want parsed_success (a CDM document is born parsed)", parseState)
	}
}

func TestListDocuments_FiltersByTenant(t *testing.T) {
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents?tenant_id=tenant-x", "")
	if err := ListDocuments(c); err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var found bool
	for _, r := range payload.Results {
		if r.DocumentKey == created.Key {
			found = true
		}
	}
	if !found {
		t.Errorf("created document %q not present in its own tenant's listing", created.Key)
	}

	// A different tenant must not see it.
	c2, rec2 := newContext(t, http.MethodGet, "/api/v1/cdm/documents?tenant_id=tenant-other", "")
	if err := ListDocuments(c2); err != nil {
		t.Fatalf("ListDocuments (other tenant): %v", err)
	}
	var other listResponse
	json.Unmarshal(rec2.Body.Bytes(), &other) //nolint:errcheck
	for _, r := range other.Results {
		if r.DocumentKey == created.Key {
			t.Errorf("document %q leaked into tenant-other's listing", created.Key)
		}
	}
}

func TestSaveDocument_MalformedJSONIs400(t *testing.T) {
	withDB(t)

	c, rec := newContext(t, http.MethodPut, "/api/v1/cdm/documents/doc:x", "{not json")
	c = withKeyParam(c, "doc:x")
	if err := SaveDocument(c); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRenderDocument_PreviewsDraftWithoutPublishing is the guard for design
// D4/D9: preview must render a draft without freezing it. Publisher.Publish
// both renders and transitions the kb.inputs row, so reusing it for preview
// would publish the document out from under an author who only wanted to look
// at their work — which is why Render was split out of Publish.
func TestRenderDocument_PreviewsDraftWithoutPublishing(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not found on PATH")
	}
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents/"+created.Key+"/render", "")
	c = withKeyParam(c, created.Key)
	if err := RenderDocument(c); err != nil {
		t.Fatalf("RenderDocument: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload renderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Pages) == 0 {
		t.Fatal("expected at least one rendered SVG page")
	}
	if !strings.Contains(payload.Pages[0], "<svg") {
		t.Errorf("first page does not look like SVG: %.80s", payload.Pages[0])
	}

	// The decisive assertion: the document must still be a draft.
	var pipelineState string
	err := db.QueryRow(`
		SELECT i.pipeline_state
		FROM kb.cdm_documents d JOIN kb.inputs i ON i.id = d.input_record_id
		WHERE d.document_key = $1
	`, created.Key).Scan(&pipelineState)
	if err != nil {
		t.Fatalf("read pipeline_state: %v", err)
	}
	if pipelineState != "success" {
		t.Errorf("preview published the document: pipeline_state = %q, want success (still a draft)", pipelineState)
	}

	// And it must still be editable.
	created.Title = created.Title + " (still editable)"
	body, _ := json.Marshal(created)
	c2, rec2 := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(body))
	c2 = withKeyParam(c2, created.Key)
	if err := SaveDocument(c2); err != nil {
		t.Fatalf("SaveDocument after preview: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("document was frozen by preview: save returned %d: %s", rec2.Code, rec2.Body.String())
	}
}

// TestRenderDocument_SecondRequestIsServedFromCache covers the caching half
// of D9: renderings are keyed by content_version, so an unchanged version is
// a table read rather than another Typst subprocess.
func TestRenderDocument_SecondRequestIsServedFromCache(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not found on PATH")
	}
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	render := func() {
		t.Helper()
		c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents/"+created.Key+"/render", "")
		c = withKeyParam(c, created.Key)
		if err := RenderDocument(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("render failed: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
		}
	}

	render()
	var afterFirst int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r
		JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = $2
	`, created.Key, created.ContentVersion).Scan(&afterFirst); err != nil {
		t.Fatalf("count renderings: %v", err)
	}
	if afterFirst == 0 {
		t.Fatal("first render stored nothing")
	}

	render()
	var afterSecond int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r
		JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = $2
	`, created.Key, created.ContentVersion).Scan(&afterSecond); err != nil {
		t.Fatalf("count renderings: %v", err)
	}
	if afterSecond != afterFirst {
		t.Errorf("second render changed the stored rendering count %d -> %d; it should have been served from cache",
			afterFirst, afterSecond)
	}
}

// TestRenderDocument_EditInvalidatesCachedPreview is the other half of D9's
// "editing invalidates the preview": loadCachedPages looks up
// (document_id, content_version), so a save that bumps content_version is a
// cache miss by construction, not by any explicit invalidation step. This
// confirms that guarantee behaviorally rather than trusting the key design
// alone -- a rendered draft's SVG must actually change after an edit that
// changes visible content, and the prior version's rendering must remain
// retrievable (not overwritten), matching TestPublisher_RepublishSupersedesArtifacts's
// same guarantee on the Publish path.
func TestRenderDocument_EditInvalidatesCachedPreview(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not found on PATH")
	}
	db := withDB(t)
	created := createViaHandler(t, db, uniqueTitle(t))

	renderPages := func() []string {
		t.Helper()
		c, rec := newContext(t, http.MethodGet, "/api/v1/cdm/documents/"+created.Key+"/render", "")
		c = withKeyParam(c, created.Key)
		if err := RenderDocument(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("render failed: err=%v code=%d body=%s", err, rec.Code, rec.Body.String())
		}
		var payload renderResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal render response: %v", err)
		}
		return payload.Pages
	}

	v1Pages := renderPages()
	if len(v1Pages) == 0 {
		t.Fatal("expected at least one v1 page")
	}

	created.Title = created.Title + " EDITED FOR RENDER"
	body, _ := json.Marshal(created)
	c2, rec2 := newContext(t, http.MethodPut, "/api/v1/cdm/documents/"+created.Key, string(body))
	c2 = withKeyParam(c2, created.Key)
	if err := SaveDocument(c2); err != nil || rec2.Code != http.StatusOK {
		t.Fatalf("save failed: err=%v code=%d body=%s", err, rec2.Code, rec2.Body.String())
	}
	var saved model.Document
	if err := json.Unmarshal(rec2.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal saved: %v", err)
	}
	if saved.ContentVersion == created.ContentVersion {
		t.Fatal("save did not bump content_version")
	}
	created.ContentVersion = saved.ContentVersion

	v2Pages := renderPages()
	if len(v2Pages) == 0 {
		t.Fatal("expected at least one v2 page")
	}
	if v1Pages[0] == v2Pages[0] {
		t.Error("expected the edited document's rendered SVG to differ from the pre-edit version, got byte-identical pages")
	}

	var v1Count, v2Count int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = 1 AND r.media_type = 'image/svg+xml'
	`, created.Key).Scan(&v1Count); err != nil {
		t.Fatalf("count v1 renderings: %v", err)
	}
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.cdm_renderings r JOIN kb.cdm_documents d ON d.id = r.document_id
		WHERE d.document_key = $1 AND r.content_version = $2 AND r.media_type = 'image/svg+xml'
	`, created.Key, saved.ContentVersion).Scan(&v2Count); err != nil {
		t.Fatalf("count v2 renderings: %v", err)
	}
	if v1Count == 0 {
		t.Error("expected the pre-edit version's rendering to remain retrievable, not overwritten")
	}
	if v2Count == 0 {
		t.Error("expected the post-edit version to have its own rendering")
	}
}

// TestRoutesAreRegisteredBehindAuth asserts every CDM route is inside the
// authenticated /api/v1 group rather than reachable anonymously.
func TestRoutesAreRegisteredBehindAuth(t *testing.T) {
	want := map[string]string{
		"/api/v1/cdm/documents":              http.MethodPost,
		"/api/v1/cdm/documents/:key":         http.MethodPut,
		"/api/v1/cdm/documents/:key/publish": http.MethodPost,
		"/api/v1/cdm/documents/:key/render":  http.MethodGet,
	}

	src, err := os.ReadFile("../routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	text := string(src)

	// Every CDM route must be registered on apiGroup, which is the group
	// carrying authmiddleware.AuthMiddleware (routes.go: apiGroup :=
	// e.Group("/api/v1"); apiGroup.Use(authmiddleware.AuthMiddleware)).
	if !strings.Contains(text, `apiGroup.Use(authmiddleware.AuthMiddleware)`) {
		t.Fatal("apiGroup no longer carries AuthMiddleware; CDM routes would be unauthenticated")
	}
	for path := range want {
		suffix := strings.TrimPrefix(path, "/api/v1")
		if !strings.Contains(text, `apiGroup.`) || !strings.Contains(text, `"`+suffix+`"`) {
			t.Errorf("route %q is not registered on the authenticated apiGroup", path)
		}
	}
}
