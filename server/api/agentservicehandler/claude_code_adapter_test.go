package agentservicehandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestClaudeCodeAdapterSpeaksGatewayProtocolWithItsSecret(t *testing.T) {
	var paths []string
	var runBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer claude-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/runs":
			_ = json.NewDecoder(r.Body).Decode(&runBody)
			_, _ = io.WriteString(w, `{"type":"completion","status":"completed"}`+"\n")
		case "/v1/runs/run-1/cancel":
			w.WriteHeader(http.StatusAccepted)
		case "/v1/runs/run-1/permissions/toolu_1":
			w.WriteHeader(http.StatusOK)
		case "/v1/summaries":
			_, _ = io.WriteString(w, `{"summary":"Folded."}`)
		}
	}))
	defer server.Close()
	adapter := NewClaudeCodeHarnessAdapter(server.URL+"/", "claude-secret", server.Client())

	stream, err := adapter.Start(context.Background(), AgentRunRequest{RunID: "run-1", ConversationID: "c-1", Message: "Q", Profile: AgentRunProfile{Model: "claude-sonnet-5-5"}})
	if err != nil {
		t.Fatal(err)
	}
	events, _ := io.ReadAll(stream)
	stream.Close()
	if !strings.Contains(string(events), `"completion"`) || runBody["runId"] != "run-1" {
		t.Fatalf("events=%s body=%v", events, runBody)
	}
	if err := adapter.Cancel(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Decide(context.Background(), "run-1", "toolu_1", true); err != nil {
		t.Fatal(err)
	}
	summary, err := adapter.Summarize(context.Background(), AgentSummaryRequest{Provider: "anthropic", Model: "m", SystemPrompt: "s", Messages: []AgentHistoryMessage{{Role: "user", Content: "Q"}}, MaxTokens: 100})
	if err != nil || summary != "Folded." {
		t.Fatalf("summary=%q err=%v", summary, err)
	}
	if strings.Join(paths, ",") != "/v1/runs,/v1/runs/run-1/cancel,/v1/runs/run-1/permissions/toolu_1,/v1/summaries" {
		t.Fatalf("paths %v", paths)
	}
	if err := NewClaudeCodeHarnessAdapter(server.URL, "wrong", server.Client()).Cancel(context.Background(), "run-1"); err == nil {
		t.Fatal("wrong secret accepted")
	}
}

func TestClaudeCodeAdapterRunsOnlyAnthropicProfiles(t *testing.T) {
	registry := NewHarnessRegistry()
	if err := registry.Register(NewClaudeCodeHarnessAdapter("http://127.0.0.1:4318", "s", nil)); err != nil {
		t.Fatal(err)
	}
	profile := AgentServiceProfile{Slug: "knowledge-guide", Harness: ClaudeCodeHarness, Provider: "openai", PermissionDefault: PermissionAsk, SaveAndResumeConversations: true}
	if err := registry.CheckProfile(profile); !errors.Is(err, ErrHarnessProfileInvalid) {
		t.Fatalf("openai profile err=%v", err)
	}
	profile.Provider = "anthropic"
	if err := registry.CheckProfile(profile); err != nil {
		t.Fatalf("anthropic profile err=%v", err)
	}
	if NewClaudeCodeHarnessAdapter("", "", nil).Describe().Supports(CapabilityContextCapacityReporting) {
		t.Fatal("adapter must not claim context capacity reporting")
	}
}

func TestProfileCandidateHarnessDefaultsAndRejectsInvalidNames(t *testing.T) {
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte("Use evidence."), 0600); err != nil {
		t.Fatal(err)
	}
	candidate := ProfileCandidate{Slug: "knowledge-guide", Provider: "anthropic", Model: "m", ThinkingLevel: "medium", PromptFile: "prompt-agent-knowledge-guide-v1.md",
		AllowedKnowledgeStores: []string{"Research"}, PermissionDefault: PermissionAuto, Enabled: true,
		Limits: ProfileLimits{MaxToolCalls: 4, MaxElapsed: time.Minute, MaxOutputTokens: 1000, MaxEvidenceBytes: 65536, MaxHistoryTokens: 32000, MaxHistoryToolResultBytes: 4096}}
	validated, err := ValidateProfileCandidate(promptDir, candidate)
	if err != nil || validated.Profile.Harness != DefaultHarness {
		t.Fatalf("default harness %+v err=%v", validated.Profile.Harness, err)
	}
	candidate.Harness = ClaudeCodeHarness
	if validated, err = ValidateProfileCandidate(promptDir, candidate); err != nil || validated.Profile.Harness != ClaudeCodeHarness {
		t.Fatalf("claude-code harness %+v err=%v", validated.Profile.Harness, err)
	}
	candidate.Harness = "../pi"
	if _, err := ValidateProfileCandidate(promptDir, candidate); !errors.Is(err, ErrInvalidProfileCandidate) {
		t.Fatalf("invalid harness err=%v", err)
	}
}

func TestInternalToolsAcceptEachConfiguredGatewaySecret(t *testing.T) {
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"store-1"}, MaxEvidenceBytes: 65536}
	token, _ := signer.Mint(claims, time.Minute)
	h := NewInternalToolHandler("pi-secret", signer, NewKnowledgeToolService(&recordingBackend{}, &mutableAccessChecker{allowed: true})).WithGatewaySecret("claude-secret")
	e := echo.New()
	RegisterInternalToolRoutes(e, h)
	for secret, want := range map[string]int{"pi-secret": http.StatusOK, "claude-secret": http.StatusOK, "other-secret": http.StatusUnauthorized} {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/agent-tools/search_knowledge", strings.NewReader(`{"query":"q","knowledge_store_id":"store-1"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+secret)
		req.Header.Set(HeaderRunID, "r")
		req.Header.Set(HeaderRunCapability, token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s status=%d body=%s", secret, rec.Code, rec.Body.String())
		}
	}
}
