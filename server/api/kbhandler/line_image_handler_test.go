package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestLineImage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARTIFACT_DIR", root)
	dir := filepath.Join(root, "0", "825", "images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "table.jpg"), []byte("image data"), 0644); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"images/table.jpg", http.StatusOK},
		{"../other.jpg", http.StatusBadRequest},
		{"images/missing.jpg", http.StatusNotFound},
		{"images/file.txt", http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodGet, "/?path="+tc.path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("825")
		if err := GetLineImage(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status {
			t.Errorf("%s: got %d, want %d", tc.path, rec.Code, tc.status)
		}
	}
}
