package wpi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

var tinyPNG = string(pngSignature) + "rest-of-image"

func TestPageImageUpload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DATA_HOME_DIR", home)
	admin := testHandler(nil, &ApiTypes.UserInfo{Admin: true})

	rec := call(t, admin.PutPageImage, "PUT", tinyPNG, "pageId", "customers")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	name := decode(t, rec.Body.String())["page_image"].(string)
	if !strings.HasPrefix(name, "customers-") || !pageImageName.MatchString(name) {
		t.Fatalf("bad name %q", name)
	}
	if got, err := os.ReadFile(filepath.Join(home, "page-images", name)); err != nil || string(got) != tinyPNG {
		t.Fatalf("file not stored: %v", err)
	}

	rec = call(t, admin.GetPageImage, "GET", "", "name", name)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != tinyPNG {
		t.Fatalf("read back: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestPageImageRefusals(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DATA_HOME_DIR", home)
	admin := testHandler(nil, &ApiTypes.UserInfo{Admin: true})

	if rec := call(t, admin.PutPageImage, "PUT", "not a png", "pageId", "customers"); rec.Code != http.StatusBadRequest {
		t.Errorf("non-PNG: %d", rec.Code)
	}
	if rec := call(t, admin.PutPageImage, "PUT", tinyPNG+strings.Repeat("x", maxPageImageBytes), "pageId", "customers"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: %d", rec.Code)
	}
	if rec := call(t, admin.PutPageImage, "PUT", tinyPNG, "pageId", "Bad/ID"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", rec.Code)
	}
	user := testHandler(nil, &ApiTypes.UserInfo{})
	if rec := call(t, user.PutPageImage, "PUT", tinyPNG, "pageId", "customers"); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin: %d", rec.Code)
	}
	anon := testHandler(nil, nil)
	if rec := call(t, anon.PutPageImage, "PUT", tinyPNG, "pageId", "customers"); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	for _, name := range []string{"../../etc/passwd", "..%2Fx.png", "missing.png", "x.jpg"} {
		if rec := call(t, admin.GetPageImage, "GET", "", "name", name); rec.Code != http.StatusNotFound {
			t.Errorf("read %q: %d", name, rec.Code)
		}
	}
	t.Setenv("DATA_HOME_DIR", "")
	if rec := call(t, admin.PutPageImage, "PUT", tinyPNG, "pageId", "customers"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no data home: %d", rec.Code)
	}
}
