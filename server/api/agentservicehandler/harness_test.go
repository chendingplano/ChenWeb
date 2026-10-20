package agentservicehandler

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestHarnessRegistryRejectsInvalidAndDuplicateAdapters(t *testing.T) {
	registry := NewHarnessRegistry()
	if err := registry.Register(&fakeHarnessAdapter{capabilities: []HarnessCapability{CapabilityCancellation}}); err == nil {
		t.Fatal("adapter without streaming was registered")
	}
	if err := registry.Register(NewPiHarnessAdapter("", "", nil)); err != nil {
		t.Fatalf("register Pi: %v", err)
	}
	if err := registry.Register(&fakeHarnessAdapter{}); err == nil {
		t.Fatal("second adapter named pi was registered")
	}
	if names := registry.Names(); len(names) != 1 || names[0] != "pi" {
		t.Fatalf("names %v", names)
	}
}

func TestHarnessRegistryChecksRequiredCapabilities(t *testing.T) {
	profile := AgentServiceProfile{Slug: "knowledge-guide", PermissionDefault: PermissionAsk, SaveAndResumeConversations: true}
	if err := NewHarnessRegistry().CheckProfile(profile); !errors.Is(err, ErrHarnessNotRegistered) {
		t.Fatalf("unregistered harness err=%v", err)
	}
	profile.Harness = "codex"
	if err := testHarnesses(&fakeHarnessAdapter{}).CheckProfile(profile); !errors.Is(err, ErrHarnessNotRegistered) {
		t.Fatalf("other harness err=%v", err)
	}
	profile.Harness = ""
	limited := testHarnesses(&fakeHarnessAdapter{capabilities: []HarnessCapability{CapabilityStreaming, CapabilityCancellation, CapabilityHistoryReplay}})
	if err := limited.CheckProfile(profile); !errors.Is(err, ErrHarnessCapabilityMissing) {
		t.Fatalf("ask profile without approval err=%v", err)
	}
	profile.PermissionDefault = PermissionAuto
	if err := limited.CheckProfile(profile); err != nil {
		t.Fatalf("auto profile err=%v", err)
	}
}

func TestStoredProfileWithoutHarnessReadsAsPi(t *testing.T) {
	profile, err := decodeStoredProfile([]byte(`{"slug":"knowledge-guide","version":"v1","model":"model-1"}`), "prompt")
	if err != nil || profile.Harness != DefaultHarness {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
}

func limitedHarnessRun(t *testing.T, capabilities []HarnessCapability, body string) (*fakeRunStore, *fakeHarnessAdapter, int) {
	t.Helper()
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, state: ResumeState{Conversation: Conversation{ID: "conversation-1", OwnerUserID: "user-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	gateway := &fakeHarnessAdapter{capabilities: capabilities, window: 200000, summaryText: "summary",
		stream: `{"type":"answer_delta","text":"Answer."}` + "\n" + `{"type":"completion","status":"completed"}` + "\n"}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), allowRunSources{}, testHarnesses(gateway), signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", body)
	return store, gateway, rec.Code
}

func TestRunSkipsUndeclaredOptionalOperationsAndRecordsHarness(t *testing.T) {
	ran := false
	previous := runInBackground
	runInBackground = func(f func()) { ran = true; f() }
	t.Cleanup(func() { runInBackground = previous })

	store, gateway, code := limitedHarnessRun(t, []HarnessCapability{CapabilityStreaming, CapabilityCancellation}, `{"message":"Q","idempotency_key":"key-1"}`)
	if code != http.StatusOK || len(store.settled) != 1 || store.settled[0].Status != "completed" {
		t.Fatalf("status=%d settled=%+v", code, store.settled)
	}
	if gateway.windowCalls != 0 || ran || len(gateway.summarized) != 0 {
		t.Fatalf("optional operations called: window=%d fold=%v summaries=%d", gateway.windowCalls, ran, len(gateway.summarized))
	}
	if store.runHarness == nil || *store.runHarness != (RunHarness{Harness: "pi", AdapterVersion: "test-1", ModelName: "model-1"}) {
		t.Fatalf("recorded harness %+v", store.runHarness)
	}
}

func TestAskModeRefusedWithoutApprovalSupport(t *testing.T) {
	store, _, code := limitedHarnessRun(t, []HarnessCapability{CapabilityStreaming, CapabilityCancellation}, `{"message":"Q","idempotency_key":"key-1","permission_mode":"ask"}`)
	if code != http.StatusConflict || store.runHarness != nil {
		t.Fatalf("status=%d attempt created=%v", code, store.runHarness != nil)
	}
}

func TestRunRefusedWhenHarnessUnregistered(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, state: ResumeState{Conversation: Conversation{ID: "conversation-1", OwnerUserID: "user-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), allowRunSources{}, NewHarnessRegistry(), signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Q","idempotency_key":"key-1"}`)
	if rec.Code != http.StatusServiceUnavailable || store.runHarness != nil {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecisionRefusedWithoutApprovalSupport(t *testing.T) {
	withAgentUser(t, "user-1")
	gateway := &fakeHarnessAdapter{capabilities: []HarnessCapability{CapabilityStreaming, CapabilityCancellation}}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(&fakeRunStore{}, testAgentProfileRegistry(), allowRunSources{}, testHarnesses(gateway), signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs/run-1/permissions/request-1", `{"allowed":true}`)
	if rec.Code != http.StatusConflict || gateway.decision {
		t.Fatalf("status=%d decided=%v", rec.Code, gateway.decision)
	}
	cancel := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs/run-1/cancel", ``)
	if cancel.Code != http.StatusAccepted || !gateway.cancelled {
		t.Fatalf("cancel status=%d cancelled=%v", cancel.Code, gateway.cancelled)
	}
}

func TestConversationsRefusedAndServicesHiddenWhenHarnessUnavailable(t *testing.T) {
	withAgentUser(t, "user-1")
	e := echo.New()
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(&Store{}, testAgentProfileRegistry(), nil).WithHarnesses(NewHarnessRegistry()))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/knowledge-guide/conversations", `{}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	list := callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services", ``)
	if list.Code != http.StatusOK || strings.TrimSpace(list.Body.String()) != "[]" {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}
