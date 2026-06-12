package api

import (
	"os"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRegisterRoutesIncludesMitmProxyIngest(t *testing.T) {
	t.Setenv("VITE_DEV_ONLY_URL", "http://localhost:5173")
	t.Setenv("APP_ENV", "development")
	e := echo.New()

	if err := RegisterRoutes(e); err != nil {
		t.Fatalf("RegisterRoutes returned error: %v", err)
	}

	found := false
	for _, route := range e.Routes() {
		if route.Method == "POST" && route.Path == "/api/internal/mitmproxy/ingest" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected POST /api/internal/mitmproxy/ingest to be registered")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
