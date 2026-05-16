package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestIsLocalFaviconPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/favicon.ico", want: true},
		{path: "/favicon.png", want: true},
		{path: "/integrations/openmetadata/favicon.ico", want: true},
		{path: "/integrations/openmetadata/favicon.png", want: true},
		{path: "/integrations/openmetadata/favicons/favicon-32x32.png", want: true},
		{path: "/assets/favicon.png", want: false},
	}

	for _, tt := range tests {
		if got := isLocalFaviconPath(tt.path); got != tt.want {
			t.Fatalf("isLocalFaviconPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestServeRootFavicon(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/favicon.png", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := serveRootFavicon(c); err != nil {
		t.Fatalf("serveRootFavicon returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(echo.HeaderContentType); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("expected SVG content type, got %q", got)
	}
	if got := rec.Body.String(); !strings.Contains(got, "<svg") {
		t.Fatalf("expected SVG body, got %q", got)
	}
}
