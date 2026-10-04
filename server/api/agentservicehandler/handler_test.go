package agentservicehandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func withAgentUser(t *testing.T, id string) {
	t.Helper()
	previous := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		if id == "" {
			return nil, nil
		}
		return &ApiTypes.UserInfo{UserId: id}, nil
	}
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = previous })
}

func testAgentProfileRegistry() *ProfileRegistry {
	p := PiProfile{Slug: "knowledge-guide", Version: "v1", FriendlyName: "Knowledge Guide", Provider: "anthropic", Model: "model-1", Enabled: true,
		AllowedTools: []string{"search_knowledge"}, AllowedKnowledgeStores: []string{"Research"}, PermissionDefault: PermissionAuto,
		Limits: ProfileLimits{MaxToolCalls: 2, MaxElapsed: time.Minute, MaxOutputTokens: 100, MaxEvidenceBytes: 1024}}
	return &ProfileRegistry{versions: map[string]map[string]PiProfile{"knowledge-guide": {"v1": p}}, active: map[string]string{"knowledge-guide": "v1"},
		knowledgeContext: mustKnowledgeContextTemplate()}
}

func callAgentHandler(t *testing.T, e *echo.Echo, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestAgentServiceListingUsesAuthenticatedPilotAvailability(t *testing.T) {
	withAgentUser(t, "pilot")
	registry := testAgentProfileRegistry()
	p := registry.versions["knowledge-guide"]["v1"]
	p.PilotUsers = []string{"pilot"}
	p.PilotUsers = append(p.PilotUsers, "private-pilot-user")
	p.SystemPrompt = "private system instructions"
	registry.versions["knowledge-guide"]["v1"] = p
	e := echo.New()
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(nil, registry, nil))
	rec := callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "knowledge-guide") || !strings.Contains(rec.Body.String(), "model-1") || strings.Contains(rec.Body.String(), "private-pilot-user") || strings.Contains(rec.Body.String(), "private system instructions") {
		t.Fatalf("pilot listing status=%d body=%s", rec.Code, rec.Body.String())
	}
	withAgentUser(t, "other")
	rec = callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "knowledge-guide") {
		t.Fatalf("nonpilot listing status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateConversationPinsProfileAndDerivesOwnerFromAuthentication(t *testing.T) {
	withAgentUser(t, "user-1")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`INSERT INTO kb.agentic_conversations`).
		WithArgs("user-1", "knowledge-guide", "knowledge-guide", "v1", "model-1", "Pump problem").
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "service_slug", "profile_slug", "profile_version", "model_name", "title", "status", "created_at", "updated_at"}).
			AddRow("conversation-1", "user-1", "knowledge-guide", "knowledge-guide", "v1", "model-1", "Pump problem", "active", now, now))
	e := echo.New()
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(NewStore(db), testAgentProfileRegistry(), nil))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/knowledge-guide/conversations", `{"title":"Pump problem","owner_user_id":"attacker","model_name":"wrong"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got Conversation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProfileVersion != "v1" || got.ModelName != "model-1" {
		t.Fatalf("unpinned conversation %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentSourceAccessRejectsRevokedMovedAndChangedSources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		exists bool
	}{{"accessible", true}, {"revoked_or_moved", false}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*JOIN kb.inputs i.*i.id::text=\$3`).
				WithArgs("user-1", sqlmock.AnyArg(), "42", "fingerprint-1", sqlmock.AnyArg(), "").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.exists))
			checker := CurrentSourceAccessChecker{DB: db}
			allowed := checker.CheckSource(context.Background(), "user-1", []string{"Research"}, SourceRecord{DocumentID: "42", Fingerprint: "fingerprint-1"}) == nil
			if allowed != tc.exists {
				t.Fatalf("allowed=%t want=%t", allowed, tc.exists)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCurrentSourceAccessEnforcesProfileDocumentGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*i.type=ANY\(\$5\)`).
		WithArgs("user-1", sqlmock.AnyArg(), "42", "fingerprint-1", sqlmock.AnyArg(), "").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	checker := CurrentSourceAccessChecker{DB: db}
	if err := checker.CheckSourceWithGroups(context.Background(), "user-1", []string{"Research"}, []string{"manual"}, SourceRecord{DocumentID: "42", Fingerprint: "fingerprint-1"}); err == nil {
		t.Fatal("source outside profile group was allowed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentSourceAccessRejectsReprocessedDocumentWithSameMD5(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*i.modify_time::text=\$6`).
		WithArgs("user-1", sqlmock.AnyArg(), "42", "same-md5", sqlmock.AnyArg(), "old-version").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	checker := CurrentSourceAccessChecker{DB: db}
	if err := checker.CheckSourceWithGroups(context.Background(), "user-1", []string{"Research"}, nil,
		SourceRecord{DocumentID: "42", Fingerprint: "same-md5", SourceVersion: "old-version"}); err == nil {
		t.Fatal("changed source version was allowed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFilterResumeStateKeepsRecentHistoryAsSaved(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	saved := now.Add(-time.Hour)
	state := ResumeState{Messages: []Message{{ID: "u1", Role: "user", Content: "question", Status: "complete", CreatedAt: saved}, {ID: "a1", Role: "assistant", Content: "answer", Status: "complete", CreatedAt: saved}}}
	sources := map[string][]SourceRecord{"a1": {{DocumentID: "42", Fingerprint: "f"}}}
	results := map[string][]ToolResultRecord{"attempt-1": {{ToolCallID: "t1", DocumentIDs: []string{"42"}, CreatedAt: saved}}}
	filtered := FilterResumeState(context.Background(), state, sources, results, now, 48*time.Hour, func(context.Context, string) error { return ErrKnowledgeAccessDenied })
	if len(filtered.Messages) != 2 || len(filtered.SourcesByMessage["a1"]) != 1 || len(filtered.ToolResults["attempt-1"]) != 1 ||
		filtered.RemovedSources != 0 || filtered.RemovedToolResults != 0 || filtered.SnapshotHours != 48 {
		t.Fatalf("recent history changed: %+v", filtered)
	}
}

func TestFilterResumeStateRemovesOnlyInaccessibleOldRecords(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	state := ResumeState{Messages: []Message{{ID: "a1", Role: "assistant", Content: "answer citing a revoked document", Status: "complete", CreatedAt: old}}}
	sources := map[string][]SourceRecord{"a1": {{DocumentID: "42", DocumentTitle: "Guide"}, {DocumentID: "43", DocumentTitle: "Private"}}}
	results := map[string][]ToolResultRecord{"attempt-1": {
		{ToolCallID: "t1", DocumentIDs: []string{"42"}, CreatedAt: old},
		{ToolCallID: "t2", DocumentIDs: []string{"42", "43"}, CreatedAt: old},
		{ToolCallID: "t3", CreatedAt: old},
	}}
	checks := 0
	visible := FilterResumeState(context.Background(), state, sources, results, now, 48*time.Hour, func(_ context.Context, documentID string) error {
		checks++
		if documentID == "43" {
			return ErrKnowledgeAccessDenied
		}
		return nil
	})
	if len(visible.Messages) != 1 || visible.Messages[0].Content != "answer citing a revoked document" {
		t.Fatalf("answer was removed: %+v", visible.Messages)
	}
	if len(visible.SourcesByMessage["a1"]) != 1 || visible.SourcesByMessage["a1"][0].DocumentTitle != "Guide" || visible.RemovedSources != 1 {
		t.Fatalf("sources %+v removed=%d", visible.SourcesByMessage, visible.RemovedSources)
	}
	kept := visible.ToolResults["attempt-1"]
	if len(kept) != 2 || kept[0].ToolCallID != "t1" || kept[1].ToolCallID != "t3" || visible.RemovedToolResults != 1 {
		t.Fatalf("tool results %+v removed=%d", kept, visible.RemovedToolResults)
	}
	if checks != 2 {
		t.Fatalf("access checked %d times, want one per document", checks)
	}
}

func TestGetConversationRemovesOldInaccessibleSourceButKeepsAnswer(t *testing.T) {
	withAgentUser(t, "user-1")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved := time.Now().Add(-72 * time.Hour)
	mock.ExpectQuery(`SELECT id, owner_user_id, service_slug`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "service_slug", "profile_slug", "profile_version", "model_name", "title", "status", "created_at", "updated_at"}).
			AddRow("conversation-1", "user-1", "knowledge-guide", "knowledge-guide", "v1", "model-1", "Question", "active", saved, saved))
	mock.ExpectQuery(`(?s)FROM kb.agentic_messages m.*owner_user_id`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "attempt_id", "sequence_no", "role", "content", "status", "created_at", "updated_at"}).
			AddRow("user-message", "conversation-1", "attempt-1", 1, "user", "What happened?", "complete", saved, saved).
			AddRow("answer-message", "conversation-1", "attempt-1", 2, "assistant", "Saved answer", "complete", saved, saved))
	mock.ExpectQuery(`(?s)FROM kb.agentic_sources src.*owner_user_id`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "document_id", "fingerprint", "version", "title", "artifact_type", "artifact_id", "line_start", "line_end", "page_start", "page_end"}).
			AddRow("answer-message", "42", "old-fingerprint", "v1", "Private guide", "", "", 1, 2, 0, 0))
	mock.ExpectQuery(`(?s)FROM kb.agentic_tool_results r.*owner_user_id`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"attempt_id", "gateway_tool_call_id", "tool_name", "arguments", "result", "is_error", "document_ids", "created_at"}))
	mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*JOIN kb.inputs i`).
		WithArgs("user-1", sqlmock.AnyArg(), "42", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	e := echo.New()
	checker := &CurrentSourceAccessChecker{DB: db}
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(NewStore(db), testAgentProfileRegistry(), checker))
	rec := callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services/conversations/conversation-1", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Saved answer") || strings.Contains(body, "Private guide") ||
		!strings.Contains(body, `"removed_sources":1`) || !strings.Contains(body, `"snapshot_hours":48`) {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAndFeedbackAreScopedToAuthenticatedConversation(t *testing.T) {
	withAgentUser(t, "user-1")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)INSERT INTO kb.agentic_feedback.*c.id=\$1 AND c.owner_user_id=\$2`).
		WithArgs("conversation-1", "user-1", "answer-1", "helpful", "Useful").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("feedback-1"))
	mock.ExpectExec(`DELETE FROM kb.agentic_conversations WHERE id = \$1 AND owner_user_id = \$2`).
		WithArgs("conversation-1", "user-1").WillReturnResult(sqlmock.NewResult(0, 1))
	e := echo.New()
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(NewStore(db), testAgentProfileRegistry(), nil))
	feedback := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/messages/answer-1/feedback", `{"rating":"helpful","comment":"Useful"}`)
	if feedback.Code != http.StatusOK {
		t.Fatalf("feedback status=%d body=%s", feedback.Code, feedback.Body.String())
	}
	deleted := callAgentHandler(t, e, http.MethodDelete, "/api/v1/agent-services/conversations/conversation-1", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
