package agentservicehandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var testNow = time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})
	return NewStore(db), mock
}

func conversationRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "owner_user_id", "service_slug", "profile_slug", "profile_version",
		"model_name", "title", "status", "created_at", "updated_at",
	})
}

func attemptRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "conversation_id", "idempotency_key", "status", "error_code",
		"error_message", "input_tokens", "output_tokens", "created_at", "updated_at",
	})
}

func messageRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "conversation_id", "attempt_id", "sequence_no", "role", "content",
		"status", "created_at", "updated_at",
	})
}

func TestCreateConversationPersistsPinnedProfileForOwner(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_conversations.*owner_user_id.*RETURNING`).
		WithArgs("user-1", "knowledge-guide", "knowledge-guide", "v1", "model-a", "Question").
		WillReturnRows(conversationRows().AddRow(
			"conv-1", "user-1", "knowledge-guide", "knowledge-guide", "v1",
			"model-a", "Question", "active", testNow, testNow,
		))

	got, err := store.CreateConversation(context.Background(), "user-1", CreateConversationInput{
		ServiceSlug: "knowledge-guide", ProfileSlug: "knowledge-guide", ProfileVersion: "v1",
		ModelName: "model-a", Title: "Question",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if got.ID != "conv-1" || got.OwnerUserID != "user-1" || got.ProfileVersion != "v1" {
		t.Fatalf("CreateConversation() = %+v", got)
	}
}

func TestListConversationsFiltersByOwner(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)FROM kb\.agentic_conversations.*owner_user_id = \$1.*ORDER BY updated_at DESC`).
		WithArgs("user-1").
		WillReturnRows(conversationRows().AddRow(
			"conv-1", "user-1", "knowledge-guide", "knowledge-guide", "v1",
			"model-a", "Question", "active", testNow, testNow,
		))

	got, err := store.ListConversations(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListConversations() error = %v", err)
	}
	if len(got) != 1 || got[0].OwnerUserID != "user-1" {
		t.Fatalf("ListConversations() = %+v", got)
	}
}

func TestGetConversationFiltersByOwnerAndID(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)FROM kb\.agentic_conversations.*id = \$1.*owner_user_id = \$2`).
		WithArgs("conv-1", "user-1").
		WillReturnRows(conversationRows().AddRow(
			"conv-1", "user-1", "knowledge-guide", "knowledge-guide", "v1",
			"model-a", "Question", "active", testNow, testNow,
		))

	got, err := store.GetConversation(context.Background(), "user-1", "conv-1")
	if err != nil {
		t.Fatalf("GetConversation() error = %v", err)
	}
	if got.ID != "conv-1" || got.OwnerUserID != "user-1" {
		t.Fatalf("GetConversation() = %+v", got)
	}
}

func TestLoadResumeStateReturnsOwnedConversationAndOrderedMessages(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)FROM kb\.agentic_conversations.*id = \$1.*owner_user_id = \$2`).
		WithArgs("conv-1", "user-1").
		WillReturnRows(conversationRows().AddRow(
			"conv-1", "user-1", "knowledge-guide", "knowledge-guide", "v1",
			"model-a", "Question", "active", testNow, testNow,
		))
	mock.ExpectQuery(`(?s)FROM kb\.agentic_messages m.*JOIN kb\.agentic_conversations c.*c\.owner_user_id = \$2.*ORDER BY m\.sequence_no ASC`).
		WithArgs("conv-1", "user-1").
		WillReturnRows(messageRows().
			AddRow("msg-1", "conv-1", nil, 1, "user", "hello", "complete", testNow, testNow).
			AddRow("msg-2", "conv-1", "attempt-1", 2, "assistant", "answer", "complete", testNow, testNow))

	got, err := store.LoadResumeState(context.Background(), "user-1", "conv-1")
	if err != nil {
		t.Fatalf("LoadResumeState() error = %v", err)
	}
	if len(got.Messages) != 2 || got.Messages[0].SequenceNo != 1 || got.Messages[1].Content != "answer" {
		t.Fatalf("LoadResumeState() = %+v", got)
	}
}

func TestCreateMessageRequiresOwnedConversation(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_messages.*SELECT.*FROM kb\.agentic_conversations c.*c\.owner_user_id = \$2.*EXISTS.*kb\.agentic_response_attempts.*a\.conversation_id = c\.id.*RETURNING`).
		WithArgs("conv-1", "user-1", nil, 1, "user", "hello", "complete").
		WillReturnRows(messageRows().AddRow(
			"msg-1", "conv-1", nil, 1, "user", "hello", "complete", testNow, testNow,
		))

	got, err := store.CreateMessage(context.Background(), "user-1", CreateMessageInput{
		ConversationID: "conv-1", SequenceNo: 1, Role: "user", Content: "hello", Status: "complete",
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if got.ID != "msg-1" || got.Role != "user" {
		t.Fatalf("CreateMessage() = %+v", got)
	}
}

func TestAppendMessageDeltaFiltersByOwner(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)UPDATE kb\.agentic_messages m.*FROM kb\.agentic_conversations c.*c\.owner_user_id = \$2.*RETURNING`).
		WithArgs("msg-2", "user-1", " more", "streaming").
		WillReturnRows(messageRows().AddRow(
			"msg-2", "conv-1", "attempt-1", 2, "assistant", "answer more", "streaming", testNow, testNow,
		))

	got, err := store.AppendMessageDelta(context.Background(), "user-1", "msg-2", " more", "streaming")
	if err != nil {
		t.Fatalf("AppendMessageDelta() error = %v", err)
	}
	if got.Content != "answer more" || got.Status != "streaming" {
		t.Fatalf("AppendMessageDelta() = %+v", got)
	}
}

func TestCreateAttemptIsIdempotentWithinOwnedConversation(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_response_attempts.*SELECT.*FROM kb\.agentic_conversations c.*c\.owner_user_id = \$2.*ON CONFLICT \(conversation_id, idempotency_key\).*RETURNING`).
		WithArgs("conv-1", "user-1", "turn-key-1").
		WillReturnRows(attemptRows().AddRow(
			"attempt-1", "conv-1", "turn-key-1", "running", "", "", 0, 0, testNow, testNow,
		))

	got, err := store.CreateAttempt(context.Background(), "user-1", "conv-1", "turn-key-1")
	if err != nil {
		t.Fatalf("CreateAttempt() error = %v", err)
	}
	if got.ID != "attempt-1" || got.IdempotencyKey != "turn-key-1" {
		t.Fatalf("CreateAttempt() = %+v", got)
	}
}

func TestSetAttemptOutcomeHandlesStoppedAndFailedStates(t *testing.T) {
	for _, status := range []string{"stopped", "failed"} {
		t.Run(status, func(t *testing.T) {
			store, mock := newMockStore(t)
			mock.ExpectQuery(`(?s)UPDATE kb\.agentic_response_attempts a.*FROM kb\.agentic_conversations c.*c\.owner_user_id = \$2.*RETURNING`).
				WithArgs("attempt-1", "user-1", status, "gateway_error", "details", int64(11), int64(7)).
				WillReturnRows(attemptRows().AddRow(
					"attempt-1", "conv-1", "turn-key-1", status, "gateway_error", "details", 11, 7, testNow, testNow,
				))

			got, err := store.SetAttemptOutcome(context.Background(), "user-1", "attempt-1", AttemptOutcome{
				Status: status, ErrorCode: "gateway_error", ErrorMessage: "details", InputTokens: 11, OutputTokens: 7,
			})
			if err != nil {
				t.Fatalf("SetAttemptOutcome() error = %v", err)
			}
			if got.Status != status {
				t.Fatalf("SetAttemptOutcome().Status = %q, want %q", got.Status, status)
			}
		})
	}
}

func TestRecordToolCallAndSourceRequireOwnedAttempt(t *testing.T) {
	store, mock := newMockStore(t)
	input := json.RawMessage(`{"q":"pump"}`)
	output := json.RawMessage(`{"count":2}`)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_tool_calls.*SELECT.*JOIN kb\.agentic_conversations c.*c\.owner_user_id = \$2.*RETURNING id`).
		WithArgs("attempt-1", "user-1", "search_knowledge", "complete", input, output, int64(25)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("tool-1"))

	toolID, err := store.RecordToolCall(context.Background(), "user-1", ToolCallInput{
		AttemptID: "attempt-1", ToolName: "search_knowledge", Status: "complete",
		InputSummary: input, OutputSummary: output, DurationMS: 25,
	})
	if err != nil {
		t.Fatalf("RecordToolCall() error = %v", err)
	}
	if toolID != "tool-1" {
		t.Fatalf("RecordToolCall() id = %q", toolID)
	}

	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_sources.*SELECT.*JOIN kb\.agentic_conversations c.*c\.owner_user_id = \$2.*RETURNING id`).
		WithArgs("attempt-1", "user-1", "msg-2", toolID, "42", "Manual", "2026-09-14", "sha256:abc", "metric", "42_mtc_1", 10, 14, 3, 4).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))

	sourceID, err := store.RecordSource(context.Background(), "user-1", SourceInput{
		AttemptID: "attempt-1", MessageID: "msg-2", ToolCallID: toolID, DocumentID: "42", DocumentTitle: "Manual",
		SourceVersion: "2026-09-14", Fingerprint: "sha256:abc",
		ArtifactType: "metric", ArtifactID: "42_mtc_1", LineStart: 10, LineEnd: 14, PageStart: 3, PageEnd: 4,
	})
	if err != nil {
		t.Fatalf("RecordSource() error = %v", err)
	}
	if sourceID != 7 {
		t.Fatalf("RecordSource() id = %d", sourceID)
	}
}

func TestRecordToolCallNormalizesMissingSummaries(t *testing.T) {
	store, mock := newMockStore(t)
	empty := json.RawMessage(`{}`)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_tool_calls.*input_summary.*output_summary.*RETURNING id`).
		WithArgs("attempt-1", "user-1", "search_knowledge", "running", empty, empty, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("tool-1"))
	if _, err := store.RecordToolCall(context.Background(), "user-1", ToolCallInput{
		AttemptID: "attempt-1", ToolName: "search_knowledge", Status: "running",
	}); err != nil {
		t.Fatalf("RecordToolCall() error = %v", err)
	}
}

func TestFinalizeAssistantMessageCommitsMessageAndSourcesAtomically(t *testing.T) {
	store, mock := newMockStore(t)
	toolID := "tool-1"
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE kb\.agentic_messages m.*m\.role = 'assistant'.*RETURNING`).
		WithArgs("msg-2", "user-1").
		WillReturnRows(messageRows().AddRow(
			"msg-2", "conv-1", "attempt-1", 2, "assistant", "answer", "complete", testNow, testNow,
		))
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_sources.*source_version.*source_fingerprint.*a\.conversation_id = \$14.*RETURNING id`).
		WithArgs("attempt-1", "msg-2", toolID, "42", "Manual", "v3", "sha256:abc", "metric", "42_mtc_1", 10, 14, 3, 4, "conv-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectCommit()

	got, err := store.FinalizeAssistantMessage(context.Background(), "user-1", "msg-2", true, []SourceInput{{
		AttemptID: "attempt-1", ToolCallID: toolID, DocumentID: "42", DocumentTitle: "Manual",
		SourceVersion: "v3", Fingerprint: "sha256:abc", ArtifactType: "metric", ArtifactID: "42_mtc_1",
		LineStart: 10, LineEnd: 14, PageStart: 3, PageEnd: 4,
	}})
	if err != nil {
		t.Fatalf("FinalizeAssistantMessage() error = %v", err)
	}
	if got.Status != "complete" {
		t.Fatalf("status = %q, want complete", got.Status)
	}
}

func TestAssistantCompletionMustUseFinalize(t *testing.T) {
	store, _ := newMockStore(t)
	_, err := store.CreateMessage(context.Background(), "user-1", CreateMessageInput{
		ConversationID: "conv-1", Role: "assistant", Content: "answer", Status: "complete",
	})
	if !errors.Is(err, ErrAssistantCompletionRequiresFinalize) {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	_, err = store.AppendMessageDelta(context.Background(), "user-1", "msg-2", "done", "complete")
	if !errors.Is(err, ErrAssistantCompletionRequiresFinalize) {
		t.Fatalf("AppendMessageDelta() error = %v", err)
	}
}

func TestFinalizeKnowledgeMessageRequiresSourceIdentity(t *testing.T) {
	store, _ := newMockStore(t)
	if _, err := store.FinalizeAssistantMessage(context.Background(), "user-1", "msg-2", true, nil); !errors.Is(err, ErrKnowledgeSourcesRequired) {
		t.Fatalf("missing sources error = %v", err)
	}
	if _, err := store.FinalizeAssistantMessage(context.Background(), "user-1", "msg-2", true, []SourceInput{{ToolCallID: "tool-1"}}); !errors.Is(err, ErrSourceIdentityRequired) {
		t.Fatalf("missing fingerprint error = %v", err)
	}
}

func TestFinalizeAssistantMessageRollsBackWhenSourceInsertFails(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE kb\.agentic_messages m.*m\.role = 'assistant'.*RETURNING`).
		WithArgs("msg-2", "user-1").
		WillReturnRows(messageRows().AddRow(
			"msg-2", "conv-1", "attempt-1", 2, "assistant", "answer", "complete", testNow, testNow,
		))
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_sources`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err := store.FinalizeAssistantMessage(context.Background(), "user-1", "msg-2", true, []SourceInput{{
		AttemptID: "attempt-1", ToolCallID: "tool-other-attempt", DocumentID: "42",
		Fingerprint: "sha256:abc",
	}})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("FinalizeAssistantMessage() error = %v", err)
	}
}

func TestUpsertFeedbackUsesAuthenticatedOwner(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`(?s)INSERT INTO kb\.agentic_feedback.*SELECT.*JOIN kb\.agentic_conversations c.*c\.owner_user_id = \$2.*ON CONFLICT.*RETURNING id`).
		WithArgs("msg-2", "user-1", "helpful", "clear answer").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("feedback-1"))

	id, err := store.UpsertFeedback(context.Background(), "user-1", FeedbackInput{
		MessageID: "msg-2", Rating: "helpful", Comment: "clear answer",
	})
	if err != nil {
		t.Fatalf("UpsertFeedback() error = %v", err)
	}
	if id != "feedback-1" {
		t.Fatalf("UpsertFeedback() id = %q", id)
	}
}

func TestDeleteConversationFiltersByOwner(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.agentic_conversations WHERE id = $1 AND owner_user_id = $2`)).
		WithArgs("conv-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.DeleteConversation(context.Background(), "user-1", "conv-1"); err != nil {
		t.Fatalf("DeleteConversation() error = %v", err)
	}
}

func TestDeleteConversationReturnsNotFoundWhenOwnerDoesNotMatch(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.agentic_conversations WHERE id = $1 AND owner_user_id = $2`)).
		WithArgs("conv-1", "other-user").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.DeleteConversation(context.Background(), "other-user", "conv-1")
	if err != sql.ErrNoRows {
		t.Fatalf("DeleteConversation() error = %v, want sql.ErrNoRows", err)
	}
}
