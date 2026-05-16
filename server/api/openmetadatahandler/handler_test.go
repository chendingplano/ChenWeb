package openmetadatahandler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
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

func TestRewriteRequestStripsTopLevelMarker(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/auth/login?redirectUri=http%3A%2F%2Fchenweb.local%3A8080%2Fcallback&om_top_level=1", nil)
	req.Host = "chenweb.local:8080"
	rewriteRequest(req, cfg)

	if req.URL.Path != "/api/v1/auth/login" {
		t.Fatalf("expected upstream auth login path, got %q", req.URL.Path)
	}
	if got := req.URL.Query().Get("om_top_level"); got != "" {
		t.Fatalf("expected om_top_level marker to be stripped, got %q", got)
	}
	if got := req.URL.Query().Get("redirectUri"); got != "http://chenweb.local:8080/integrations/openmetadata/auth/callback" {
		t.Fatalf("expected redirectUri to be preserved, got %q", got)
	}
}

func TestShouldPromoteAuthLogin(t *testing.T) {
	cfg := config{PublicBasePath: "/integrations/openmetadata/"}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/auth/login?redirectUri=http%3A%2F%2Fchenweb.local%3A8080%2Fcallback", nil)
	if !shouldPromoteAuthLogin(req, cfg) {
		t.Fatal("expected embedded auth login request to be promoted")
	}

	req = httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/auth/login?om_top_level=1", nil)
	if shouldPromoteAuthLogin(req, cfg) {
		t.Fatal("expected top-level marked auth login request to pass through")
	}
}

func TestRewriteRequestRewritesAuthLoginRedirectURIToPrefixedCallback(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/auth/login?redirectUri=http%3A%2F%2Fchenweb.local%3A8080%2Fauth%2Fcallback", nil)
	req.Host = "chenweb.local:8080"
	rewriteRequest(req, cfg)

	if got := req.URL.Query().Get("redirectUri"); got != "http://chenweb.local:8080/integrations/openmetadata/auth/callback" {
		t.Fatalf("expected prefixed callback redirectUri, got %q", got)
	}
}

func TestAuthLoginPromotionHTMLTargetsTopLevelMarkedURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/integrations/openmetadata/api/v1/auth/login?redirectUri=http%3A%2F%2Fchenweb.local%3A8080%2Fcallback", nil)

	got := authLoginPromotionHTML(req)

	if !strings.Contains(got, "window.top.location.replace") {
		t.Fatalf("expected top-level navigation script, got %s", got)
	}
	if !strings.Contains(got, "om_top_level=1") {
		t.Fatalf("expected top-level marker in generated HTML, got %s", got)
	}
	if !strings.Contains(got, "redirectUri=http%3A%2F%2Fchenweb.local%3A8080%2Fcallback") {
		t.Fatalf("expected redirectUri to be preserved, got %s", got)
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

func TestRedirectAuthCallbackMapsToPrefixedFrontendCallback(t *testing.T) {
	t.Setenv("OPENMETADATA_UPSTREAM_URL", "http://localhost:8585")

	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?id_token=test-token&email=alex%40example.com", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := RedirectAuthCallback(c); err != nil {
		t.Fatal(err)
	}

	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect status 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/integrations/openmetadata/auth/callback?") {
		t.Fatalf("expected redirect to prefixed OpenMetadata auth callback with query, got %q", location)
	}
	if !strings.Contains(location, "id_token=test-token") {
		t.Fatalf("expected id_token query to be preserved, got %q", location)
	}
	if !strings.Contains(location, "email=alex%40example.com") {
		t.Fatalf("expected email query to be preserved, got %q", location)
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

func TestRewriteOpenMetadataHTMLRewritesRootAssetURLs(t *testing.T) {
	const original = `
<script>window.BASE_PATH = '/';</script>
<script type="module" src="/assets/index.js"></script>
<link rel="stylesheet" href="/assets/index.css">
<img src="/images/governance.png">
<link rel="manifest" href="/manifest.json">
<link rel="icon" href="/favicon.png">
<link rel="shortcut icon" href="/favicon.ico">
<link rel="icon" href="/favicons/favicon-32x32.png">
`

	got := rewriteOpenMetadataHTML(original, "/integrations/openmetadata/", "")

	expectedSnippets := []string{
		"window.BASE_PATH = '/integrations/openmetadata/';",
		`src="/integrations/openmetadata/assets/index.js"`,
		`href="/integrations/openmetadata/assets/index.css"`,
		`src="/integrations/openmetadata/images/governance.png"`,
		`href="/integrations/openmetadata/manifest.json"`,
		`href="/integrations/openmetadata/favicon.png"`,
		`href="/integrations/openmetadata/favicon.ico"`,
		`href="/integrations/openmetadata/favicons/favicon-32x32.png"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(got, snippet) {
			t.Fatalf("expected rewritten html to contain %q, got %s", snippet, got)
		}
	}
}

func TestRewriteOpenMetadataHTMLRemovesMissingLoginPreloadImages(t *testing.T) {
	const original = `
<script>
let images = [
  '/images/governance.png',
  '/images/data-collaboration.png',
];
images.forEach((image) => {
  let img = new Image();
  img.src = image;
});
</script>
`

	got := rewriteOpenMetadataHTML(original, "/integrations/openmetadata/", "")

	if strings.Contains(got, "governance.png") {
		t.Fatalf("expected governance preload to be removed, got %s", got)
	}
	if strings.Contains(got, "data-collaboration.png") {
		t.Fatalf("expected data collaboration preload to be removed, got %s", got)
	}
}

func TestRewriteHTMLResponseUpdatesBodyAndLength(t *testing.T) {
	cfg := config{PublicBasePath: "/integrations/openmetadata/"}
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Request: &http.Request{
			Header: http.Header{},
		},
		Body: io.NopCloser(strings.NewReader(`<script>window.BASE_PATH = '/';</script><script src="/assets/index.js"></script>`)),
	}

	if err := rewriteHTMLResponse(resp, cfg); err != nil {
		t.Fatalf("rewriteHTMLResponse: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "/integrations/openmetadata/assets/index.js") {
		t.Fatalf("expected rewritten asset path in body, got %s", text)
	}
	if got := resp.Header.Get("Content-Length"); got == "" {
		t.Fatalf("expected content-length to be updated")
	}
}

func TestRewriteHTMLResponseHandlesGzipEncodedHTML(t *testing.T) {
	cfg := config{PublicBasePath: "/integrations/openmetadata/"}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte(`<script>window.BASE_PATH = '/';</script><script src="/assets/index.js"></script>`))
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	resp := &http.Response{
		Header: http.Header{
			"Content-Type":     []string{"text/html; charset=utf-8"},
			"Content-Encoding": []string{"gzip"},
		},
		Request: &http.Request{
			Header: http.Header{},
		},
		Body: io.NopCloser(bytes.NewReader(compressed.Bytes())),
	}

	if err := rewriteHTMLResponse(resp, cfg); err != nil {
		t.Fatalf("rewriteHTMLResponse: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "/integrations/openmetadata/assets/index.js") {
		t.Fatalf("expected rewritten asset path in body, got %s", text)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("expected content-encoding to be cleared, got %q", got)
	}
}

func TestRewriteOpenMetadataHTMLInjectsDarkThemeBootstrap(t *testing.T) {
	got := rewriteOpenMetadataHTML(`<html><head></head><body><div id="root"></div></body></html>`, "/integrations/openmetadata/", "dark")

	expectedSnippets := []string{
		`id="chenweb-openmetadata-theme"`,
		`window.localStorage.setItem("ui-theme","dark")`,
		`root.classList.add("dark-mode")`,
		`root.style.colorScheme="dark"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(got, snippet) {
			t.Fatalf("expected dark theme bootstrap to contain %q, got %s", snippet, got)
		}
	}
}

func TestRewriteHTMLResponseUsesThemeHeaderForBootstrap(t *testing.T) {
	cfg := config{PublicBasePath: "/integrations/openmetadata/"}
	reqHeader := http.Header{}
	reqHeader.Set(internalThemeHeader, "dark")
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Request: &http.Request{
			Header: reqHeader,
		},
		Body: io.NopCloser(strings.NewReader(`<html><head></head><body><div id="root"></div></body></html>`)),
	}

	if err := rewriteHTMLResponse(resp, cfg); err != nil {
		t.Fatalf("rewriteHTMLResponse: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `window.localStorage.setItem("ui-theme","dark")`) {
		t.Fatalf("expected dark theme bootstrap in body, got %s", text)
	}
}
