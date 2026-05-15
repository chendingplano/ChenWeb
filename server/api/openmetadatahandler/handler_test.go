package openmetadatahandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newEcho() *echo.Echo { return echo.New() }

func TestGetSessionReturnsForbiddenWhenUnauthenticated(t *testing.T) {
	originalResolver := resolveCurrentUser
	t.Cleanup(func() {
		resolveCurrentUser = originalResolver
	})
	resolveCurrentUser = func(c echo.Context, loc string) (*CurrentUser, bool) {
		return nil, false
	}

	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/openmetadata/session", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestGetSessionReturnsLaunchPayloadForAuthenticatedUser(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_PUBLIC_BASE_PATH", "/integrations/openmetadata/")

	originalResolver := resolveCurrentUser
	t.Cleanup(func() {
		resolveCurrentUser = originalResolver
	})
	resolveCurrentUser = func(c echo.Context, loc string) (*CurrentUser, bool) {
		return &CurrentUser{
			UserID:      "user-123",
			Email:       "alex@example.com",
			DisplayName: "Alex",
		}, true
	}

	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/openmetadata/session", nil)
	req.Host = "chenweb.local:8080"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.LaunchURL != "/integrations/openmetadata/" {
		t.Fatalf("expected launch URL /integrations/openmetadata/, got %q", resp.LaunchURL)
	}
	if resp.UserID != "user-123" {
		t.Fatalf("expected user id user-123, got %q", resp.UserID)
	}
	if len(resp.Capabilities) == 0 {
		t.Fatalf("expected capabilities to be populated")
	}
	if resp.SSOMode != "proxy-only" {
		t.Fatalf("expected default sso mode proxy-only, got %q", resp.SSOMode)
	}
	if resp.AuthBoundaryNote == "" {
		t.Fatalf("expected auth boundary note to be populated")
	}
}

func TestGetSessionReturnsCallbackURLForSharedIDP(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_SSO_MODE", "shared-idp")

	originalResolver := resolveCurrentUser
	t.Cleanup(func() {
		resolveCurrentUser = originalResolver
	})
	resolveCurrentUser = func(c echo.Context, loc string) (*CurrentUser, bool) {
		return &CurrentUser{
			UserID:      "user-456",
			Email:       "jamie@example.com",
			DisplayName: "Jamie",
		}, true
	}

	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/openmetadata/session", nil)
	req.Host = "chenweb.local:8080"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetSession(c); err != nil {
		t.Fatal(err)
	}

	var resp SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.CallbackURL != "http://chenweb.local:8080/callback" {
		t.Fatalf("expected callback URL http://chenweb.local:8080/callback, got %q", resp.CallbackURL)
	}
}

func TestNewProxyReturnsErrorForInvalidUpstream(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "://bad-url")

	proxy, err := NewProxy()
	if err == nil {
		t.Fatal("expected error for invalid upstream URL")
	}
	if proxy != nil {
		t.Fatal("expected nil proxy on error")
	}
}

func TestProxyStripsIntegrationPrefixBeforeForwarding(t *testing.T) {
	t.Setenv("OPENMETADATA_PUBLIC_BASE_PATH", "/integrations/openmetadata/")

	var gotPath string
	var gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	t.Setenv("OPENMETADATA_UPSTREAM_URL", upstream.URL)

	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/tables?limit=5", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/api/v1/tables" {
		t.Fatalf("expected upstream path /api/v1/tables, got %q", gotPath)
	}
	if gotQuery != "limit=5" {
		t.Fatalf("expected query limit=5, got %q", gotQuery)
	}
}

func TestProxyRootRequestMapsToSlash(t *testing.T) {
	t.Setenv("OPENMETADATA_PUBLIC_BASE_PATH", "/integrations/openmetadata/")

	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	t.Setenv("OPENMETADATA_UPSTREAM_URL", upstream.URL)

	proxy, err := NewProxy()
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(gotPath) != "/" {
		t.Fatalf("expected upstream root path /, got %q", gotPath)
	}
}

func TestProxyInjectsAuthorizationHeaderForSessionBootstrap(t *testing.T) {
	t.Setenv("OPENMETADATA_PUBLIC_BASE_PATH", "/integrations/openmetadata/")
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://example.invalid")
	t.Setenv("OPENMETADATA_SSO_MODE", "session-bootstrap")
	t.Setenv("OPENMETADATA_BEARER_TOKEN", "test-token")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/tables", nil)
	req.Header.Set("Authorization", "Bearer should-not-pass-through")
	rewriteRequest(req, cfg)

	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("expected injected bearer token, got %q", got)
	}
}

func TestRewriteRequestAddsForwardedHeadersForSharedIDP(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_SSO_MODE", "shared-idp")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata", nil)
	req.Host = "chenweb.local:8080"
	rewriteRequest(req, cfg)

	if got := req.Header.Get("X-Forwarded-Host"); got != "chenweb.local:8080" {
		t.Fatalf("expected X-Forwarded-Host chenweb.local:8080, got %q", got)
	}
	if got := req.Header.Get("X-Forwarded-Proto"); got != "http" {
		t.Fatalf("expected X-Forwarded-Proto http, got %q", got)
	}
	if got := req.Header.Get("X-Forwarded-Prefix"); got != "/integrations/openmetadata" {
		t.Fatalf("expected X-Forwarded-Prefix /integrations/openmetadata, got %q", got)
	}
}

func TestRewriteCallbackRequestMapsToRootCallback(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_SSO_MODE", "shared-idp")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=123", nil)
	req.Host = "chenweb.local:8080"
	rewriteCallbackRequest(req, cfg)

	if req.URL.Path != "/callback" {
		t.Fatalf("expected callback path /callback, got %q", req.URL.Path)
	}
	if req.URL.RawQuery != "code=abc&state=123" {
		t.Fatalf("expected callback query to be preserved, got %q", req.URL.RawQuery)
	}
}

func TestLoadConfigRequiresUpstreamURL(t *testing.T) {
	original := os.Getenv("OPENMETADATA_UPSTREAM_URL")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("OPENMETADATA_UPSTREAM_URL")
			return
		}
		_ = os.Setenv("OPENMETADATA_UPSTREAM_URL", original)
	})
	_ = os.Unsetenv("OPENMETADATA_UPSTREAM_URL")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected loadConfig to fail without OPENMETADATA_UPSTREAM_URL")
	}
}

func TestLoadConfigDefaultsSSOModeToProxyOnly(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	_ = os.Unsetenv("OPENMETADATA_SSO_MODE")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.SSOMode != "proxy-only" {
		t.Fatalf("expected default sso mode proxy-only, got %q", cfg.SSOMode)
	}
}

func TestLoadConfigRejectsInvalidSSOMode(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_SSO_MODE", "banana")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected invalid OPENMETADATA_SSO_MODE to fail")
	}
}

func TestLoadConfigRequiresBearerTokenForSessionBootstrap(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")
	t.Setenv("OPENMETADATA_SSO_MODE", "session-bootstrap")
	_ = os.Unsetenv("OPENMETADATA_BEARER_TOKEN")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected session-bootstrap mode without bearer token to fail")
	}
}
