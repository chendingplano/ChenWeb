package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRegisterRoutes_EmbeddedFrontendServesLoginPage(t *testing.T) {
	t.Setenv("USE_EMBED_FRONTEND", "true")
	t.Setenv("APP_ENV", "production")
	// Route registration loads the Pi profiles, which need an explicit (here synthetic) model each.
	t.Setenv("PI_KNOWLEDGE_GUIDE_MODEL", "synthetic-pi-model")
	t.Setenv("PI_PROBLEM_DIAGNOSTICS_MODEL", "synthetic-pi-model")

	e := echo.New()
	if err := RegisterRoutes(e); err != nil {
		t.Fatalf("RegisterRoutes returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /login status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("GET /login content-type = %q, want text/html", contentType)
	}
}
