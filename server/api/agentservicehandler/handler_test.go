package agentservicehandler

import (
	"context"
	"database/sql"
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
	return &ProfileRegistry{versions: map[string]map[string]PiProfile{"knowledge-guide": {"v1": p}}, active: map[string]string{"knowledge-guide": "v1"}}
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
	registry.versions["knowledge-guide"]["v1"] = p
	e := echo.New()
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(nil, registry, nil))
	rec := callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "knowledge-guide") || !strings.Contains(rec.Body.String(), "model-1") {
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

func TestFilterResumeStateHidesDependentAnswerAfterRevocation(t *testing.T) {
	state := ResumeState{Messages: []Message{{ID: "u1", Role: "user", Content: "question", Status: "complete"}, {ID: "a1", Role: "assistant", Content: "protected answer", Status: "complete"}}}
	sources := map[string][]SourceRecord{"a1": {{DocumentID: "42", Fingerprint: "f"}}}
	filtered := FilterResumeState(context.Background(), state, sources, func(context.Context, SourceRecord) error { return sql.ErrNoRows })
	if len(filtered.Messages) != 1 || filtered.Messages[0].ID != "u1" || len(filtered.HiddenMessageIDs) != 1 || filtered.HiddenMessageIDs[0] != "a1" {
		t.Fatalf("filtered %+v", filtered)
	}
	if strings.Contains(filtered.OmissionNotice, "protected answer") {
		t.Fatalf("notice leaked answer: %s", filtered.OmissionNotice)
	}
}

func TestGetConversationHidesSavedAnswerWhenGrantIsRevoked(t *testing.T) {
	withAgentUser(t, "user-1")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT id, owner_user_id, service_slug`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "service_slug", "profile_slug", "profile_version", "model_name", "title", "status", "created_at", "updated_at"}).
			AddRow("conversation-1", "user-1", "knowledge-guide", "knowledge-guide", "v1", "model-1", "Question", "active", now, now))
	mock.ExpectQuery(`(?s)FROM kb.agentic_messages m.*owner_user_id`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "attempt_id", "sequence_no", "role", "content", "status", "created_at", "updated_at"}).
			AddRow("user-message", "conversation-1", nil, 1, "user", "What happened?", "complete", now, now).
			AddRow("answer-message", "conversation-1", "attempt-1", 2, "assistant", "Private answer", "complete", now, now))
	mock.ExpectQuery(`(?s)FROM kb.agentic_sources src.*owner_user_id`).
		WithArgs("conversation-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "document_id", "fingerprint", "version"}).
			AddRow("answer-message", "42", "old-fingerprint", "v1"))
	mock.ExpectQuery(`(?s)FROM kb.agentic_knowledge_grants g.*JOIN kb.inputs i`).
		WithArgs("user-1", sqlmock.AnyArg(), "42", "old-fingerprint", sqlmock.AnyArg(), "v1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	e := echo.New()
	checker := &CurrentSourceAccessChecker{DB: db}
	RegisterConversationRoutes(e.Group("/api/v1/agent-services"), NewConversationHandler(NewStore(db), testAgentProfileRegistry(), checker))
	rec := callAgentHandler(t, e, http.MethodGet, "/api/v1/agent-services/conversations/conversation-1", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "Private answer") || !strings.Contains(rec.Body.String(), "hidden_message_ids") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
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
