package agentservicehandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type adminProfileStoreStub struct {
	profiles    []AgentServiceProfile
	active      AgentServiceProfile
	previous    string
	seedCalls   int
	initErr     error
	listErr     error
	activateErr error
}

func (s *adminProfileStoreStub) EnsureSeeded(context.Context, *ProfileRegistry) error {
	s.seedCalls++
	return s.initErr
}
func (s *adminProfileStoreStub) ListActive(context.Context) ([]AgentServiceProfile, error) {
	return s.profiles, s.listErr
}
func (s *adminProfileStoreStub) LoadVersion(context.Context, string, string) (AgentServiceProfile, error) {
	return AgentServiceProfile{}, ErrProfileNotFound
}
func (s *adminProfileStoreStub) ActivateVersion(_ context.Context, p AgentServiceProfile, _, _ string) (AgentServiceProfile, string, error) {
	if s.activateErr != nil {
		return AgentServiceProfile{}, s.previous, s.activateErr
	}
	p.Version = "v2"
	return p, s.previous, nil
}
func (s *adminProfileStoreStub) RestoreActiveVersion(context.Context, string, string, string) error {
	return nil
}

func installProfileAdminAuthenticator(t *testing.T, user *ApiTypes.UserInfo) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
}

func profileAdminContext(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestProfileAdminStateContractAndAdminGuard(t *testing.T) {
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}})
	registry := profileRegistryFixture(t)
	handler := NewProfileAdminHandler(registry, nil, "", "https://gateway.example", "secret")
	c, rec := profileAdminContext(http.MethodGet, "/", "")
	if err := handler.GetState(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var state struct {
		GatewayURL    string              `json:"gateway_url"`
		Initialized   bool                `json:"initialized"`
		Profiles      []map[string]any    `json:"profiles"`
		PromptChoices map[string][]string `json:"prompt_choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.GatewayURL != "https://gateway.example" || state.Initialized || len(state.Profiles) != 2 || len(state.PromptChoices) != 2 {
		t.Fatalf("unexpected state payload: %#v", state)
	}
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "user"})
	c, rec = profileAdminContext(http.MethodGet, "/", "")
	_ = handler.GetState(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestProfileAdminRejectsUnknownUpdateFields(t *testing.T) {
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "admin", IsOwner: true})
	registry := profileRegistryFixture(t)
	store := &adminProfileStoreStub{}
	registry.WithVersionStore(store)
	registry.versionStoreReady.Store(true)
	handler := NewProfileAdminHandler(registry, store, t.TempDir(), "", "")
	c, rec := profileAdminContext(http.MethodPut, "/profiles/knowledge-guide", `{"expected_active_version":"v1","provider":"x","unknown":true}`)
	c.SetParamNames("slug")
	c.SetParamValues("knowledge-guide")
	_ = handler.UpdateProfile(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestProfileAdminUpdateCreatesNewVersionAndReturnsStaleConflict(t *testing.T) {
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte("Use evidence."), 0600); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"slug": "knowledge-guide", "friendly_name": "Guide", "description": "Answers questions",
		"provider": "anthropic", "model": "claude", "thinking_level": "medium",
		"prompt_file": "prompt-agent-knowledge-guide-v1.md", "allowed_knowledge_stores": []string{"Research"},
		"allowed_document_groups": []string{}, "permission_default": "ask", "enabled": true,
		"pilot_users": []string{}, "expected_active_version": "v1",
		"limits": map[string]int{"max_tool_calls": 12, "max_elapsed": 90, "max_output_tokens": 1200,
			"max_evidence_bytes": 65536, "max_history_tokens": 32000, "max_history_tool_result_bytes": 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	newRegistry := func(store *adminProfileStoreStub) *ProfileRegistry {
		registry := profileRegistryFixture(t)
		registry.WithVersionStore(store)
		registry.versionStoreReady.Store(true)
		return registry
	}

	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}})
	store := &adminProfileStoreStub{previous: "v1"}
	handler := NewProfileAdminHandler(newRegistry(store), store, promptDir, "", "")
	c, rec := profileAdminContext(http.MethodPut, "/profiles/knowledge-guide", string(body))
	c.SetParamNames("slug")
	c.SetParamValues("knowledge-guide")
	if err := handler.UpdateProfile(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":"v2"`) {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}

	// The body asks for permission_default "ask"; a harness without
	// interactive approval cannot run it, so activation is refused.
	limitedStore := &adminProfileStoreStub{previous: "v1"}
	limited := testHarnesses(&fakeHarnessAdapter{capabilities: []HarnessCapability{CapabilityStreaming, CapabilityCancellation, CapabilityHistoryReplay}})
	handler = NewProfileAdminHandler(newRegistry(limitedStore), limitedStore, promptDir, "", "").WithHarnesses(limited)
	c, rec = profileAdminContext(http.MethodPut, "/profiles/knowledge-guide", string(body))
	c.SetParamNames("slug")
	c.SetParamValues("knowledge-guide")
	_ = handler.UpdateProfile(c)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "interactive_tool_approval") {
		t.Fatalf("incompatible update status=%d body=%s", rec.Code, rec.Body.String())
	}

	conflictStore := &adminProfileStoreStub{previous: "v2", activateErr: ErrProfileVersionConflict}
	handler = NewProfileAdminHandler(newRegistry(conflictStore), conflictStore, promptDir, "", "")
	c, rec = profileAdminContext(http.MethodPut, "/profiles/knowledge-guide", string(body))
	c.SetParamNames("slug")
	c.SetParamValues("knowledge-guide")
	_ = handler.UpdateProfile(c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale update status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProfileAdminInitializationAndAuthOnEveryEndpoint(t *testing.T) {
	profile := AgentServiceProfile{Slug: "knowledge-guide", Version: "v1", Enabled: true}
	store := &adminProfileStoreStub{profiles: []AgentServiceProfile{profile}}
	registry := profileRegistryFixture(t)
	registry.WithVersionStore(store)
	handler := NewProfileAdminHandler(registry, store, "", "", "")
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "root", Roles: []string{"root"}})
	for range 2 {
		c, rec := profileAdminContext(http.MethodPost, "/initialize", "")
		if err := handler.Initialize(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("initialize status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
		}
	}
	if store.seedCalls != 2 || !registry.VersionStoreReady() {
		t.Fatalf("seed calls=%d ready=%v", store.seedCalls, registry.VersionStoreReady())
	}

	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "ordinary"})
	for name, invoke := range map[string]func(echo.Context){
		"read":       func(c echo.Context) { _ = handler.GetState(c) },
		"save":       func(c echo.Context) { _ = handler.UpdateProfile(c) },
		"initialize": func(c echo.Context) { _ = handler.Initialize(c) },
		"gateway":    func(c echo.Context) { _ = handler.CheckGateway(c) },
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := profileAdminContext(http.MethodGet, "/", "")
			invoke(c)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d, want forbidden", rec.Code)
			}
		})
	}
}

func TestProfileAdminInitializationFailureLeavesProfilesReadOnly(t *testing.T) {
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}})
	store := &adminProfileStoreStub{initErr: errors.New("database unavailable")}
	registry := profileRegistryFixture(t)
	registry.WithVersionStore(store)
	handler := NewProfileAdminHandler(registry, store, t.TempDir(), "", "")
	c, rec := profileAdminContext(http.MethodPost, "/initialize", "")
	_ = handler.Initialize(c)
	if rec.Code != http.StatusServiceUnavailable || registry.VersionStoreReady() {
		t.Fatalf("init status=%d ready=%v body=%s", rec.Code, registry.VersionStoreReady(), rec.Body.String())
	}
	c, rec = profileAdminContext(http.MethodGet, "/", "")
	_ = handler.GetState(c)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"initialized":true`) {
		t.Fatalf("degraded state status=%d body=%s", rec.Code, rec.Body.String())
	}
	c, rec = profileAdminContext(http.MethodPut, "/profiles/knowledge-guide", `{}`)
	_ = handler.UpdateProfile(c)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded update status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProfileElapsedSecondsRejectsOverflowValues(t *testing.T) {
	for _, value := range []int{-1, 0, 601, int(^uint(0) >> 1)} {
		if _, err := profileElapsedDuration(value); err == nil {
			t.Errorf("profileElapsedDuration(%d) accepted an invalid value", value)
		}
	}
	for _, value := range []int{1, 600} {
		got, err := profileElapsedDuration(value)
		if err != nil || got != time.Duration(value)*time.Second {
			t.Errorf("profileElapsedDuration(%d) = %v, %v", value, got, err)
		}
	}
}

func TestProfileAdminGatewayCheckUsesSecretAndReturnsSafeMetadata(t *testing.T) {
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "root", Roles: []string{"root"}})
	secret := "gateway-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/health" || r.Method != http.MethodGet {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	handler := NewProfileAdminHandler(nil, nil, "", server.URL+"/base", secret)
	handler.now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	c, rec := profileAdminContext(http.MethodPost, "/gateway-check", "")
	if err := handler.CheckGateway(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "healthy" || result["elapsed_ms"] == nil || strings.Contains(rec.Body.String(), secret) || result["http_status"] != nil {
		t.Fatalf("unsafe or unexpected gateway result: %s", rec.Body.String())
	}
}

func TestSafeGatewayURLRemovesCredentialsAndQuery(t *testing.T) {
	if got := safeGatewayURL("https://user:secret@gateway.example/base?token=hidden#frag"); got != "https://gateway.example/base" {
		t.Fatalf("safeGatewayURL() = %q", got)
	}
	if got := safeGatewayURL("file:///tmp/gateway"); got != "" {
		t.Fatalf("safeGatewayURL() accepted unsupported scheme: %q", got)
	}
}

func TestProfileAdminGatewayCheckReportsUnavailableGateway(t *testing.T) {
	installProfileAdminAuthenticator(t, &ApiTypes.UserInfo{UserId: "admin", Admin: true})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	handler := NewProfileAdminHandler(nil, nil, "", server.URL, "long-enough-secret")
	c, rec := profileAdminContext(http.MethodPost, "/gateway-check", "")
	_ = handler.CheckGateway(c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"unavailable"`) {
		t.Fatalf("gateway status=%d body=%s", rec.Code, rec.Body.String())
	}
}
