package agentservicehandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestPiGatewayClientForwardsOnlyPrivateBearerAndBoundedRun(t *testing.T) {
	var gotAuthorization, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization, gotPath = r.Header.Get("Authorization"), r.URL.Path
		if r.Header.Get("Cookie") != "" {
			t.Fatal("browser cookie forwarded")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"type":"completion","status":"completed"}`+"\n")
	}))
	defer server.Close()
	client := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client())
	response, err := client.Start(context.Background(), GatewayRunRequest{RunID: "run-1", ConversationID: "c", UserID: "u", Message: "help", Capability: "cap", Profile: GatewayRunProfile{Slug: "guide", Version: "v1", Provider: "anthropic", Model: "model", SystemPrompt: "guide", AllowedTools: []string{"search_knowledge"}, MaxToolCalls: 2, MaxElapsedMs: 1000, MaxOutputTokens: 100, MaxEvidenceBytes: 1024, PermissionDefault: PermissionAuto, ThinkingLevel: "off"}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	if gotAuthorization != "Bearer private-secret-private-secret" || gotPath != "/v1/runs" {
		t.Fatalf("forwarded authorization=%q path=%q", gotAuthorization, gotPath)
	}
}

func TestSSEFramesSafeEventAndSeparatesAnswerFromActivity(t *testing.T) {
	var output bytes.Buffer
	if err := WriteAgentSSE(&output, AgentPublicEvent{Type: "answer_delta", Text: "It is a pump."}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSSE(&output, AgentPublicEvent{Type: "activity", Status: "started", Tool: "search_knowledge", ToolCallID: "call-1"}); err != nil {
		t.Fatal(err)
	}
	frames := output.String()
	if !strings.Contains(frames, "event: answer_delta\n") || !strings.Contains(frames, "event: activity\n") || strings.Contains(frames, "private tool payload") {
		t.Fatalf("unsafe SSE frames %q", frames)
	}
}

func TestRunCollectorStoresToolSourcesAndSettlesExactlyOnce(t *testing.T) {
	collector := NewRunEventCollector("run-1", 4096)
	for _, line := range []string{
		`{"type":"answer_delta","text":"Pump "}`,
		`{"type":"answer_delta","text":"flow."}`,
		`{"type":"sources","toolCallId":"call-1","toolName":"search_knowledge","sources":[{"knowledge_store_id":"7","document_id":"42","source_title":"Guide","source_version":"v1","source_fingerprint":"abc","line_start":10,"line_end":11}]}`,
		`{"type":"activity","status":"completed","tool":"search_knowledge","toolCallId":"call-1","error":false}`,
		`{"type":"usage","inputTokens":9,"outputTokens":4}`,
		`{"type":"completion","status":"completed"}`,
		`{"type":"completion","status":"failed"}`,
	} {
		if _, err := collector.Accept([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	result := collector.Result()
	if result.Answer != "Pump flow." || result.Status != "completed" || len(result.Sources) != 1 || len(result.ToolCalls) != 1 || result.InputTokens != 9 || result.OutputTokens != 4 {
		t.Fatalf("collector result %+v", result)
	}
	if result.Sources[0].ToolCallID != "call-1" || result.Sources[0].Fingerprint != "abc" {
		t.Fatalf("source %+v", result.Sources[0])
	}
}

func TestRunCollectorRejectsProtectedOrOversizedGatewayEvents(t *testing.T) {
	collector := NewRunEventCollector("run-1", 8)
	if _, err := collector.Accept([]byte(`{"type":"thinking_delta","text":"secret"}`)); err == nil {
		t.Fatal("thinking event accepted")
	}
	if _, err := collector.Accept([]byte(`{"type":"answer_delta","text":"much too long"}`)); err == nil {
		t.Fatal("oversized answer accepted")
	}
}

func TestRunCollectorAcceptsSafeRetryActivityButRejectsLateAnswer(t *testing.T) {
	collector := NewRunEventCollector("run-1", 4096)
	if event, err := collector.Accept([]byte(`{"type":"activity","status":"retrying","attempt":1}`)); err != nil || event.Status != "retrying" {
		t.Fatalf("retry event=%+v err=%v", event, err)
	}
	_, _ = collector.Accept([]byte(`{"type":"completion","status":"completed"}`))
	if _, err := collector.Accept([]byte(`{"type":"answer_delta","text":"late"}`)); err == nil {
		t.Fatal("accepted answer after completion")
	}
}

func TestGatewayProfileMappingUsesPinnedLimitsAndDisclosure(t *testing.T) {
	p := PiProfile{Slug: "guide", Version: "v1", Provider: "anthropic", Model: "model-1", SystemPrompt: "safe", ThinkingLevel: "medium", PermissionDefault: PermissionAsk,
		AllowedTools: []string{"search_knowledge"}, Limits: ProfileLimits{MaxToolCalls: 3, MaxElapsed: time.Minute, MaxOutputTokens: 100, MaxEvidenceBytes: 2048}}
	mapped := MapGatewayProfile(p, PermissionAsk)
	encoded, err := json.Marshal(mapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"maxElapsedMs":60000`)) || !bytes.Contains(encoded, []byte(`"permissionDefault":"ask"`)) || mapped.Model != "model-1" {
		t.Fatalf("mapped profile %s", encoded)
	}
}

type fakeRunStore struct {
	state         ResumeState
	created       bool
	answer        string
	settled       []AttemptOutcome
	sources       []SourceInput
	toolCalls     []ToolCallInput
	userTexts     []string
	finalizeErr   error
	attemptStatus string
}

func (f *fakeRunStore) LoadResumeState(context.Context, string, string) (ResumeState, error) {
	return f.state, nil
}
func (f *fakeRunStore) LoadSourceDependencies(context.Context, string, string) (map[string][]SourceRecord, error) {
	return map[string][]SourceRecord{}, nil
}
func (f *fakeRunStore) ListGrantedStoreIDs(context.Context, string, []string) ([]string, error) {
	return []string{"7"}, nil
}
func (f *fakeRunStore) CreateRunAttempt(context.Context, string, string, string) (ResponseAttempt, bool, error) {
	return ResponseAttempt{ID: "run-1", ConversationID: "conversation-1", Status: "running"}, f.created, nil
}
func (f *fakeRunStore) CreateMessage(_ context.Context, _ string, in CreateMessageInput) (Message, error) {
	if in.Role == "user" {
		f.userTexts = append(f.userTexts, in.Content)
		return Message{ID: "user-message", Role: "user", Status: "complete"}, nil
	}
	return Message{ID: "assistant-message", Role: "assistant", Status: "streaming"}, nil
}
func (f *fakeRunStore) AppendMessageDelta(_ context.Context, _, _, delta, _ string) (Message, error) {
	f.answer += delta
	return Message{ID: "assistant-message"}, nil
}
func (f *fakeRunStore) RecordToolCall(_ context.Context, _ string, in ToolCallInput) (string, error) {
	f.toolCalls = append(f.toolCalls, in)
	return "db-tool-1", nil
}
func (f *fakeRunStore) FinalizeAssistantMessage(_ context.Context, _, _ string, _ bool, sources []SourceInput) (Message, error) {
	f.sources = sources
	if f.finalizeErr != nil {
		return Message{}, f.finalizeErr
	}
	return Message{ID: "assistant-message", Status: "complete"}, nil
}
func (f *fakeRunStore) SetAttemptOutcome(_ context.Context, _, _ string, in AttemptOutcome) (ResponseAttempt, error) {
	f.settled = append(f.settled, in)
	return ResponseAttempt{ID: "run-1", Status: in.Status}, nil
}
func (f *fakeRunStore) GetRunAttempt(context.Context, string, string, string) (ResponseAttempt, error) {
	status := f.attemptStatus
	if status == "" {
		status = "running"
	}
	return ResponseAttempt{ID: "run-1", ConversationID: "conversation-1", Status: status}, nil
}

type fakeGatewayBridge struct {
	request   GatewayRunRequest
	stream    string
	startErr  error
	cancelled bool
	decision  bool
}
type allowRunSources struct{}

func (allowRunSources) CheckSourceWithGroups(context.Context, string, []string, []string, SourceRecord) error {
	return nil
}
func (g *fakeGatewayBridge) Start(_ context.Context, run GatewayRunRequest) (io.ReadCloser, error) {
	g.request = run
	if g.startErr != nil {
		return nil, g.startErr
	}
	return io.NopCloser(strings.NewReader(g.stream)), nil
}
func (g *fakeGatewayBridge) Cancel(context.Context, string) error { g.cancelled = true; return nil }
func (g *fakeGatewayBridge) Decide(_ context.Context, _, _ string, allowed bool) error {
	g.decision = allowed
	return nil
}

func TestRunEndpointMintsScopedCapabilityStreamsAndPersistsSourcesOnce(t *testing.T) {
	withAgentUser(t, "user-1")
	profile := testAgentProfileRegistry()
	store := &fakeRunStore{created: true, state: ResumeState{Conversation: Conversation{ID: "conversation-1", OwnerUserID: "user-1", ServiceSlug: "knowledge-guide", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"},
		Messages: []Message{{ID: "old-user", Role: "user", Content: "Old question", Status: "complete"}, {ID: "old-answer", Role: "assistant", Content: "Old answer", Status: "complete"}}}}
	gateway := &fakeGatewayBridge{stream: strings.Join([]string{
		`{"type":"answer_delta","text":"Flow is low."}`,
		`{"type":"sources","toolCallId":"call-1","toolName":"search_knowledge","sources":[{"knowledge_store_id":"7","document_id":"42","source_title":"Guide","source_version":"v1","source_fingerprint":"abc","line_start":10,"line_end":11}]}`,
		`{"type":"activity","status":"completed","tool":"search_knowledge","toolCallId":"call-1","error":false}`,
		`{"type":"usage","inputTokens":10,"outputTokens":5}`,
		`{"type":"completion","status":"completed"}`,
	}, "\n") + "\n"}
	signer, err := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRunHandler(store, profile, allowRunSources{}, gateway, signer)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), handler)
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Why is flow low?","idempotency_key":"key-1","permission_mode":"auto"}`)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Agent-Run-Id") != "run-1" || !strings.Contains(rec.Body.String(), "event: answer_delta") || !strings.Contains(rec.Body.String(), "event: activity") || strings.Contains(rec.Body.String(), "Old answer") {
		t.Fatalf("stream status=%d body=%s", rec.Code, rec.Body.String())
	}
	claims, err := signer.Verify(gateway.request.Capability, "run-1", "search_knowledge")
	if err != nil || claims.UserID != "user-1" || len(claims.KnowledgeStoreIDs) != 1 || claims.KnowledgeStoreIDs[0] != "7" {
		t.Fatalf("scoped capability %+v err=%v", claims, err)
	}
	if gateway.request.Message != "Why is flow low?" || len(gateway.request.History) != 2 || len(store.settled) != 1 || store.settled[0].Status != "completed" || store.answer != "Flow is low." || len(store.sources) != 1 || store.sources[0].ToolCallID != "db-tool-1" {
		t.Fatalf("run request=%+v settled=%+v answer=%q sources=%+v", gateway.request, store.settled, store.answer, store.sources)
	}
}

func TestDuplicateRunKeyDoesNotCreateMessagesOrCallGateway(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: false, state: ResumeState{Conversation: Conversation{ID: "conversation-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	gateway := &fakeGatewayBridge{}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), nil, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Question","idempotency_key":"same-key"}`)
	if rec.Code != http.StatusConflict || len(store.userTexts) != 0 || gateway.request.RunID != "" {
		t.Fatalf("duplicate status=%d messages=%v gateway=%+v", rec.Code, store.userTexts, gateway.request)
	}
}

func TestGrantedStoreScopeUsesCurrentUserGrantsAndStableIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*g.user_id=\$1.*ks.ks_name=ANY\(\$2\)`).
		WithArgs("user-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"store_id"}).AddRow("7").AddRow("9"))
	ids, err := NewStore(db).ListGrantedStoreIDs(context.Background(), "user-1", []string{"Research"})
	if err != nil || len(ids) != 2 || ids[0] != "7" || ids[1] != "9" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunDoesNotTellBrowserCompletedBeforeFinalization(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, finalizeErr: errors.New("database unavailable"), state: ResumeState{Conversation: Conversation{ID: "conversation-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	gateway := &fakeGatewayBridge{stream: `{"type":"answer_delta","text":"Draft"}` + "\n" + `{"type":"completion","status":"completed"}` + "\n"}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), allowRunSources{}, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Question","idempotency_key":"key-1"}`)
	if strings.Contains(rec.Body.String(), `"status":"completed"`) || !strings.Contains(rec.Body.String(), `"status":"failed"`) || len(store.settled) != 1 || store.settled[0].Status != "failed" {
		t.Fatalf("misleading completion: body=%s settled=%+v", rec.Body.String(), store.settled)
	}
}

func TestRunRejectsSourceFromFailedTool(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, state: ResumeState{Conversation: Conversation{ID: "conversation-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	gateway := &fakeGatewayBridge{stream: strings.Join([]string{
		`{"type":"answer_delta","text":"Possibly"}`,
		`{"type":"sources","toolCallId":"call-1","toolName":"search_knowledge","sources":[{"knowledge_store_id":"7","document_id":"42","source_title":"Guide","source_version":"v1","source_fingerprint":"abc","line_start":10,"line_end":11}]}`,
		`{"type":"activity","status":"completed","tool":"search_knowledge","toolCallId":"call-1","error":true}`,
		`{"type":"completion","status":"completed"}`,
	}, "\n") + "\n"}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), allowRunSources{}, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Question","idempotency_key":"key-1"}`)
	if len(store.sources) != 0 || len(store.settled) != 1 || store.settled[0].Status == "completed" || strings.Contains(rec.Body.String(), "event: sources") || strings.Contains(rec.Body.String(), "event: completion\ndata: {\"type\":\"completion\",\"status\":\"completed\"}") {
		t.Fatalf("failed-tool citation accepted: sources=%+v settled=%+v body=%s", store.sources, store.settled, rec.Body.String())
	}
}

func TestRunControlsRequireOwnedActiveAttempt(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true}
	gateway := &fakeGatewayBridge{}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), nil, gateway, signer))
	cancel := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs/run-1/cancel", `{}`)
	decision := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs/run-1/permissions/request-1", `{"allowed":true}`)
	if cancel.Code != http.StatusAccepted || decision.Code != http.StatusOK || !gateway.cancelled || !gateway.decision {
		t.Fatalf("controls cancel=%d decide=%d gateway=%+v", cancel.Code, decision.Code, gateway)
	}
	store.attemptStatus = "completed"
	stale := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs/run-1/cancel", `{}`)
	if stale.Code != http.StatusNotFound {
		t.Fatalf("stale control status=%d", stale.Code)
	}
}

func TestGatewayFailureSettlesRunAndReturnsRetryableUnavailable(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, state: ResumeState{Conversation: Conversation{ID: "conversation-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1"}}}
	gateway := &fakeGatewayBridge{startErr: errors.New("connection refused")}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), allowRunSources{}, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"Question","idempotency_key":"key-1"}`)
	if rec.Code != http.StatusServiceUnavailable || len(store.settled) != 1 || store.settled[0].ErrorCode != "gateway_unavailable" || strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatalf("gateway failure status=%d settled=%+v body=%s", rec.Code, store.settled, rec.Body.String())
	}
}
