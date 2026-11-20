package sitehandler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/deepdoc/server/cmd/config"
)

func TestApplyLocalBrandingReplacesOnlyConfiguredValues(t *testing.T) {
	base := SiteConfig{
		Branding: Branding{LogoImage: "/images/logo-semos.svg"},
		Footer:   Footer{Email: "site@example.com", Address: "Site address"},
	}
	check := func(name string, b config.BrandingSection, want SiteConfig) {
		t.Helper()
		cfg := base
		applyLocalBranding(&cfg, b)
		if cfg.Branding != want.Branding || cfg.Footer.Email != want.Footer.Email || cfg.Footer.Address != want.Footer.Address {
			t.Fatalf("%s: branding = %+v, footer = %q / %q", name, cfg.Branding, cfg.Footer.Email, cfg.Footer.Address)
		}
	}

	check("nothing configured", config.BrandingSection{}, base)

	want := base
	want.Branding.LogoImage = logoURL
	check("logo_file", config.BrandingSection{LogoFile: "/data/branding/logo.png"}, want)

	want = base
	want.Branding.LogoImageWhite = logoWhiteURL
	check("logo_white_file", config.BrandingSection{LogoWhiteFile: "/data/branding/logo-white.png"}, want)

	want = base
	want.Footer.Email = "contact@tenant.example"
	check("footer_email", config.BrandingSection{FooterEmail: "contact@tenant.example"}, want)

	want = base
	want.Footer.Address = "1 Example Road\nExample City"
	check("footer_address", config.BrandingSection{FooterAddress: "1 Example Road\nExample City"}, want)
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
