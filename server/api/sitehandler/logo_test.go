package sitehandler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestApplyLocalBrandingReplacesOnlyConfiguredValues(t *testing.T) {
	base := SiteConfig{Branding: Branding{LogoImage: "/images/logo-semos.svg"}, Footer: Footer{Email: "site@example.com"}}

	cfg := base
	applyLocalBranding(&cfg, "", "", "")
	if cfg.Branding.LogoImage != "/images/logo-semos.svg" || cfg.Branding.LogoImageWhite != "" || cfg.Footer.Email != "site@example.com" {
		t.Fatalf("nothing configured: branding = %+v, footer email = %q", cfg.Branding, cfg.Footer.Email)
	}

	cfg = base
	applyLocalBranding(&cfg, "/data/branding/logo.png", "", "")
	if cfg.Branding.LogoImage != logoURL || cfg.Branding.LogoImageWhite != "" || cfg.Footer.Email != "site@example.com" {
		t.Fatalf("logo_file only: branding = %+v, footer email = %q", cfg.Branding, cfg.Footer.Email)
	}

	cfg = base
	applyLocalBranding(&cfg, "", "/data/branding/logo-white.png", "")
	if cfg.Branding.LogoImage != "/images/logo-semos.svg" || cfg.Branding.LogoImageWhite != logoWhiteURL {
		t.Fatalf("logo_white_file only: branding = %+v", cfg.Branding)
	}

	cfg = base
	applyLocalBranding(&cfg, "", "", "contact@tenant.example")
	if cfg.Footer.Email != "contact@tenant.example" || cfg.Branding.LogoImage != "/images/logo-semos.svg" {
		t.Fatalf("footer_email only: branding = %+v, footer email = %q", cfg.Branding, cfg.Footer.Email)
	}
}

func TestServeLogoFile(t *testing.T) {
	dir := t.TempDir()
	logo := filepath.Join(dir, "logo.png")
	png := []byte("\x89PNG\r\n\x1a\nfake")
	if err := os.WriteFile(logo, png, 0o600); err != nil {
		t.Fatal(err)
	}
	serve := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, logoURL, nil), rec)
		if err := serveLogoFile(c, "logo_file", path); err != nil {
			t.Fatalf("serveLogoFile(%q): %v", path, err)
		}
		return rec
	}

	if rec := serve(logo); rec.Code != http.StatusOK || rec.Body.String() != string(png) ||
		rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("configured file: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	for _, path := range []string{"", filepath.Join(dir, "missing.png"), dir} {
		if rec := serve(path); rec.Code != http.StatusNotFound {
			t.Errorf("serveLogoFile(%q) = %d, want 404", path, rec.Code)
		}
	}
}
