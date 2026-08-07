package terminologyresourcehandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
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
