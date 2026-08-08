package terminologyresourcehandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListResourcesReportsNeverDownloaded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/terminology-resources", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListResources(c); err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Resources []resourceView `json:"resources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Resources) != len(terminology.Resources()) {
		t.Fatalf("got %d resources, want %d", len(body.Resources), len(terminology.Resources()))
	}
	for _, r := range body.Resources {
		if r.Downloaded {
			t.Fatalf("%s must start not downloaded", r.ID)
		}
	}
	iec := findView(body.Resources, string(terminology.ResourceIEC))
	if !iec.PermissionRequired || iec.CanDownload {
		t.Fatalf("IEC view must be permission-gated: %+v", iec)
	}
	qudt := findView(body.Resources, string(terminology.ResourceQUDT))
	if qudt.ExpectedSizeBytes <= 0 {
		t.Fatalf("QUDT expected_size_bytes = %d, want > 0", qudt.ExpectedSizeBytes)
	}
	wd := findView(body.Resources, string(terminology.ResourceWikidata))
	if wd.Cadence != "weekly" {
		t.Fatalf("Wikidata update_cadence = %q, want weekly", wd.Cadence)
	}
	if wd.MaxBytes <= 0 {
		t.Fatalf("Wikidata max_bytes = %d, want > 0", wd.MaxBytes)
	}
}

func TestListResourcesMergesPersistedStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)
	statusDir := filepath.Join(dir, string(terminology.ResourceSIRP))
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statusDir, "status.json"), []byte(`{"source":"sirp","release":"1.0.0","downloaded":true,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_bytes":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pending draft manifest means the resource awaits review.
	if err := os.WriteFile(filepath.Join(statusDir, "manifest.draft.json"), []byte(`{"adapter":"bipm-sirp-quantity","policy":{"license_review_status":"pending_review"},"artifacts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/terminology-resources", nil), rec)
	if err := ListResources(c); err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	var body struct {
		Resources []resourceView `json:"resources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	sirp := findView(body.Resources, "sirp")
	if !sirp.Downloaded || sirp.Release != "1.0.0" || sirp.SizeBytes != 42 {
		t.Fatalf("sirp view = %+v", sirp)
	}
	if sirp.ReviewStatus != "pending_review" {
		t.Fatalf("sirp review_status = %q, want pending_review", sirp.ReviewStatus)
	}
}

func TestListResourcesReviewStatusReflectsApproval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)

	for _, tc := range []struct {
		name  string
		draft string
		want  string
	}{
		{"approved draft in place", `{"adapter":"ucum","policy":{"license_review_status":"approved"},"artifacts":[]}`, "approved"},
		{"no draft after approval move", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
				t.Fatal(err)
			}
			draftPath := filepath.Join(sourceDir, "manifest.draft.json")
			if tc.draft != "" {
				if err := os.WriteFile(draftPath, []byte(tc.draft), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(draftPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}

			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/terminology-resources", nil), rec)
			if err := ListResources(c); err != nil {
				t.Fatalf("ListResources: %v", err)
			}
			var body struct {
				Resources []resourceView `json:"resources"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			ucum := findView(body.Resources, "ucum")
			if ucum.ReviewStatus != tc.want {
				t.Fatalf("ucum review_status = %q, want %q", ucum.ReviewStatus, tc.want)
			}
		})
	}
}

func TestListResourcesExposesDownloadingProgress(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)
	statusDir := filepath.Join(dir, string(terminology.ResourceUCUM))
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statusDir, "status.json"), []byte(`{"source":"ucum","downloading":true,"downloaded_bytes":1234,"total_bytes":8192}`), 0o644); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/terminology-resources", nil), rec)
	if err := ListResources(c); err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	var body struct {
		Resources []resourceView `json:"resources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ucum := findView(body.Resources, string(terminology.ResourceUCUM))
	if !ucum.Downloading || ucum.DownloadedBytes != 1234 || ucum.TotalBytes != 8192 {
		t.Fatalf("ucum view = %+v, want downloading progress", ucum)
	}
}

func TestListResourcesFailsClosedWithoutDir(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", "")
	t.Setenv("DATA_HOME_DIR", "")
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/terminology-resources", nil), rec)
	if err := ListResources(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestDownloadResourceIECForbidden(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", t.TempDir())
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/iec-60050-845/download", nil), rec)
	c.SetParamNames("source")
	c.SetParamValues(string(terminology.ResourceIEC))
	if err := DownloadResource(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDownloadResourceUnknownSource(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", t.TempDir())
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/nope/download", nil), rec)
	c.SetParamNames("source")
	c.SetParamValues("nope")
	if err := DownloadResource(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func findView(views []resourceView, id string) resourceView {
	for _, v := range views {
		if v.ID == id {
			return v
		}
	}
	return resourceView{}
}

func TestApproveResourceApprovesPendingDraft(t *testing.T) {
	// ApproveResource starts an import after approval; pin the global DB to
	// nil so the test never touches a real database and asserts the fallback.
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = nil
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)
	sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pending := `{"adapter":"ucum","policy":{"license_review_status":"pending_review"},"artifacts":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/approve", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, rec)
	c.SetParamNames("source")
	c.SetParamValues("ucum")
	if err := c.Request().ParseForm(); err != nil {
		t.Fatal(err)
	}
	c.Request().Form.Set("approved_by", "alice@example.test")
	c.Request().Form.Set("comments", "license and scope verified")

	if err := ApproveResource(c); err != nil {
		t.Fatalf("ApproveResource: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Resource resourceView `json:"resource"`
		Import   struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"import"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Resource.ReviewStatus != "approved" {
		t.Fatalf("review_status = %q, want approved", body.Resource.ReviewStatus)
	}
	if !body.Resource.Downloaded {
		t.Fatal("resource view must still show the download state")
	}
	if body.Resource.ReviewComments != "license and scope verified" {
		t.Fatalf("review_comments = %q, want the submitted comments", body.Resource.ReviewComments)
	}
	if body.Import.OK || !strings.Contains(body.Import.Error, "database handle is not configured") {
		t.Fatalf("import outcome = %+v, want the no-DB fallback error", body.Import)
	}

	// The draft on disk now carries the approval.
	b, err := os.ReadFile(filepath.Join(sourceDir, "manifest.draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"approved_by": "alice@example.test"`) {
		t.Fatalf("draft missing approved_by: %s", b)
	}
	if !strings.Contains(string(b), `"review_comments": "license and scope verified"`) {
		t.Fatalf("draft missing review_comments: %s", b)
	}
}

func TestApproveResourceFailsClosed(t *testing.T) {
	e := echo.New()

	t.Run("unknown source", func(t *testing.T) {
		t.Setenv("TERMINOLOGY_DIR", t.TempDir())
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/nope/approve", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("nope")
		if err := ApproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("missing approver", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("TERMINOLOGY_DIR", dir)
		sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"ucum","policy":{"license_review_status":"pending_review"},"artifacts":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/approve", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := ApproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("not downloaded", func(t *testing.T) {
		t.Setenv("TERMINOLOGY_DIR", t.TempDir())
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/approve", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := c.Request().ParseForm(); err != nil {
			t.Fatal(err)
		}
		c.Request().Form.Set("approved_by", "alice")
		if err := ApproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("already approved", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("TERMINOLOGY_DIR", dir)
		sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"ucum","policy":{"license_review_status":"approved","approved_by":"bob","approved_at":"2026-08-07T12:00:00Z"},"artifacts":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/approve", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := c.Request().ParseForm(); err != nil {
			t.Fatal(err)
		}
		c.Request().Form.Set("approved_by", "alice")
		if err := ApproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})
}

func TestDisapproveResourceRecordsRejection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)
	sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pending := `{"adapter":"ucum","policy":{"license_review_status":"pending_review"},"artifacts":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/disapprove", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, rec)
	c.SetParamNames("source")
	c.SetParamValues("ucum")
	if err := c.Request().ParseForm(); err != nil {
		t.Fatal(err)
	}
	c.Request().Form.Set("reviewed_by", "alice@example.test")
	c.Request().Form.Set("comments", "license terms not acceptable for redistribution")

	if err := DisapproveResource(c); err != nil {
		t.Fatalf("DisapproveResource: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Resource resourceView `json:"resource"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Resource.ReviewStatus != "disapproved" {
		t.Fatalf("review_status = %q, want disapproved", body.Resource.ReviewStatus)
	}
	if body.Resource.ReviewComments != "license terms not acceptable for redistribution" {
		t.Fatalf("review_comments = %q, want the submitted comments", body.Resource.ReviewComments)
	}

	// The draft on disk carries the rejection and the reviewer.
	b, err := os.ReadFile(filepath.Join(sourceDir, "manifest.draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"license_review_status": "disapproved"`,
		`"reviewed_by": "alice@example.test"`,
		`"review_comments": "license terms not acceptable for redistribution"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("draft missing %s: %s", want, b)
		}
	}
	if strings.Contains(string(b), `"approved_by"`) {
		t.Fatalf("disapproved draft must not carry approval fields: %s", b)
	}
}

func TestDisapproveResourceFailsClosed(t *testing.T) {
	e := echo.New()

	t.Run("unknown source", func(t *testing.T) {
		t.Setenv("TERMINOLOGY_DIR", t.TempDir())
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/nope/disapprove", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("nope")
		if err := DisapproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("missing reviewer", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("TERMINOLOGY_DIR", dir)
		sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"ucum","policy":{"license_review_status":"pending_review"},"artifacts":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/disapprove", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := DisapproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("not downloaded", func(t *testing.T) {
		t.Setenv("TERMINOLOGY_DIR", t.TempDir())
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/disapprove", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := c.Request().ParseForm(); err != nil {
			t.Fatal(err)
		}
		c.Request().Form.Set("reviewed_by", "alice")
		if err := DisapproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("already disapproved", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("TERMINOLOGY_DIR", dir)
		sourceDir := filepath.Join(dir, string(terminology.ResourceUCUM))
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"ucum","policy":{"license_review_status":"disapproved","reviewed_by":"bob","reviewed_at":"2026-08-07T12:00:00Z"},"artifacts":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/ucum/disapprove", nil), rec)
		c.SetParamNames("source")
		c.SetParamValues("ucum")
		if err := c.Request().ParseForm(); err != nil {
			t.Fatal(err)
		}
		c.Request().Form.Set("reviewed_by", "alice")
		if err := DisapproveResource(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})
}

// writeQUDTDraftFromFixture writes a pending QUDT draft manifest plus its
// artifact (from the terminology conformance fixture) into sourceDir, so the
// post-approval manifest passes ParseAndVerifyManifest.
func writeQUDTDraftFromFixture(t *testing.T, sourceDir string) {
	t.Helper()
	fixtureDir := filepath.Join("..", "ontology", "terminology", "testdata", "fixtures", "qudt")
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var draft map[string]any
	if err := json.Unmarshal(raw, &draft); err != nil {
		t.Fatal(err)
	}
	policy := draft["policy"].(map[string]any)
	policy["license_review_status"] = "pending_review"
	delete(policy, "approved_by")
	delete(policy, "approved_at")
	draftB, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), draftB, 0o644); err != nil {
		t.Fatal(err)
	}
	ttl, err := os.ReadFile(filepath.Join(fixtureDir, "quantity-kinds.ttl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "quantity-kinds.ttl"), ttl, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApproveResourceAlreadyImportedIsReplay(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINOLOGY_DIR", dir)
	sourceDir := filepath.Join(dir, string(terminology.ResourceQUDT))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"qudt","downloaded":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeQUDTDraftFromFixture(t, sourceDir)

	// The release is already registered with byte-identical content, so the
	// post-approval import must be an idempotent replay, not an error.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT content_checksum FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
		WithArgs("qudt-quantity-kind", "3.5.0").
		WillReturnRows(sqlmock.NewRows([]string{"content_checksum"}).AddRow("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminology-resources/qudt/approve", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, rec)
	c.SetParamNames("source")
	c.SetParamValues("qudt")
	if err := c.Request().ParseForm(); err != nil {
		t.Fatal(err)
	}
	c.Request().Form.Set("approved_by", "alice@example.test")

	if err := ApproveResource(c); err != nil {
		t.Fatalf("ApproveResource: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Resource resourceView `json:"resource"`
		Import   struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"import"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Resource.ReviewStatus != "approved" {
		t.Fatalf("review_status = %q, want approved", body.Resource.ReviewStatus)
	}
	if !body.Import.OK || body.Import.Error != "" {
		t.Fatalf("import outcome = %+v, want an ok replay without an error", body.Import)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestAlreadyImportedIdenticalRejectsDrift(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, string(terminology.ResourceQUDT))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The helper parses a valid approved manifest, so reuse the conformance
	// fixture as-is (approved policy + artifact) instead of a pending draft.
	fixtureDir := filepath.Join("..", "ontology", "terminology", "testdata", "fixtures", "qudt")
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ttl, err := os.ReadFile(filepath.Join(fixtureDir, "quantity-kinds.ttl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "quantity-kinds.ttl"), ttl, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(sourceDir, "manifest.draft.json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t.Run("missing release is not a replay", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT content_checksum FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
			WithArgs("qudt-quantity-kind", "3.5.0").
			WillReturnError(sql.ErrNoRows)
		if alreadyImportedIdentical(t.Context(), db, manifestPath) {
			t.Fatal("unregistered release must not be treated as a replay")
		}
	})

	t.Run("different checksum is drift, not a replay", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT content_checksum FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
			WithArgs("qudt-quantity-kind", "3.5.0").
			WillReturnRows(sqlmock.NewRows([]string{"content_checksum"}).AddRow("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
		if alreadyImportedIdentical(t.Context(), db, manifestPath) {
			t.Fatal("a different registered checksum must not be treated as a replay")
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
