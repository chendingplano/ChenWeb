package agentservicehandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

var (
	ErrAssistantCompletionRequiresFinalize = errors.New("assistant completion requires FinalizeAssistantMessage")
	ErrKnowledgeSourcesRequired            = errors.New("knowledge-backed assistant message requires sources")
	ErrSourceIdentityRequired              = errors.New("source tool call and fingerprint are required")
	ErrRunAlreadyActive                    = errors.New("conversation already has an active run")
)

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type Conversation struct {
	ID             string    `json:"id"`
	OwnerUserID    string    `json:"-"`
	ServiceSlug    string    `json:"service_slug"`
	ProfileSlug    string    `json:"profile_slug"`
	ProfileVersion string    `json:"profile_version"`
	ModelName      string    `json:"model_name"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateConversationInput struct {
	ServiceSlug    string
	ProfileSlug    string
	ProfileVersion string
	ModelName      string
	Title          string
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	AttemptID      *string   `json:"attempt_id,omitempty"`
	SequenceNo     int       `json:"sequence_no"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateMessageInput struct {
	ConversationID string
	AttemptID      *string
	SequenceNo     int
	Role           string
	Content        string
	Status         string
}

type ResumeState struct {
	Conversation Conversation `json:"conversation"`
	Messages     []Message    `json:"messages"`
}

type ResponseAttempt struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Status         string    `json:"status"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	InputTokens    int64     `json:"input_tokens"`
	OutputTokens   int64     `json:"output_tokens"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AttemptOutcome struct {
	Status       string
	ErrorCode    string
	ErrorMessage string
	InputTokens  int64
	OutputTokens int64
}

type ToolCallInput struct {
	AttemptID string
	ToolName  string
	Status    string
	// Summaries must contain identifiers/counts only, never retrieved passages.
	InputSummary  json.RawMessage
	OutputSummary json.RawMessage
	DurationMS    int64
}

type SourceInput struct {
	AttemptID     string
	MessageID     string
	ToolCallID    string
	DocumentID    string
	DocumentTitle string
	SourceVersion string
	Fingerprint   string
	ArtifactType  string
	ArtifactID    string
	LineStart     int
	LineEnd       int
	PageStart     int
	PageEnd       int
}

type FeedbackInput struct {
	MessageID string
	Rating    string
	Comment   string
}

const conversationColumns = `id, owner_user_id, service_slug, profile_slug, profile_version,
       model_name, title, status, created_at, updated_at`

func scanConversation(row interface{ Scan(...any) error }) (Conversation, error) {
	var out Conversation
	err := row.Scan(
		&out.ID, &out.OwnerUserID, &out.ServiceSlug, &out.ProfileSlug, &out.ProfileVersion,
		&out.ModelName, &out.Title, &out.Status, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var out Message
	err := row.Scan(
		&out.ID, &out.ConversationID, &out.AttemptID, &out.SequenceNo, &out.Role,
		&out.Content, &out.Status, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func scanAttempt(row interface{ Scan(...any) error }) (ResponseAttempt, error) {
	var out ResponseAttempt
	err := row.Scan(
		&out.ID, &out.ConversationID, &out.IdempotencyKey, &out.Status, &out.ErrorCode,
		&out.ErrorMessage, &out.InputTokens, &out.OutputTokens, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (s *Store) CreateConversation(ctx context.Context, ownerUserID string, in CreateConversationInput) (Conversation, error) {
	const query = `INSERT INTO kb.agentic_conversations (
    owner_user_id, service_slug, profile_slug, profile_version, model_name, title
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING ` + conversationColumns
	return scanConversation(s.db.QueryRowContext(ctx, query,
		ownerUserID, in.ServiceSlug, in.ProfileSlug, in.ProfileVersion, in.ModelName, in.Title,
	))
}

func (s *Store) ListConversations(ctx context.Context, ownerUserID string) ([]Conversation, error) {
	const query = `SELECT ` + conversationColumns + `
FROM kb.agentic_conversations
WHERE owner_user_id = $1
ORDER BY updated_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Conversation, 0)
	for rows.Next() {
		item, scanErr := scanConversation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetConversation(ctx context.Context, ownerUserID, conversationID string) (Conversation, error) {
	const query = `SELECT ` + conversationColumns + `
FROM kb.agentic_conversations
WHERE id = $1 AND owner_user_id = $2`
	return scanConversation(s.db.QueryRowContext(ctx, query, conversationID, ownerUserID))
}

func (s *Store) LoadResumeState(ctx context.Context, ownerUserID, conversationID string) (ResumeState, error) {
	conversation, err := s.GetConversation(ctx, ownerUserID, conversationID)
	if err != nil {
		return ResumeState{}, err
	}

	const query = `SELECT m.id, m.conversation_id, m.attempt_id, m.sequence_no, m.role,
       m.content, m.status, m.created_at, m.updated_at
FROM kb.agentic_messages m
JOIN kb.agentic_conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1 AND c.owner_user_id = $2
ORDER BY m.sequence_no ASC`
	rows, err := s.db.QueryContext(ctx, query, conversationID, ownerUserID)
	if err != nil {
		return ResumeState{}, err
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		message, scanErr := scanMessage(rows)
		if scanErr != nil {
			return ResumeState{}, scanErr
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return ResumeState{}, err
	}
	return ResumeState{Conversation: conversation, Messages: messages}, nil
}

func (s *Store) LoadSourceDependencies(ctx context.Context, ownerUserID, conversationID string) (map[string][]SourceRecord, error) {
	const query = `SELECT src.message_id, src.document_id, src.source_fingerprint, src.source_version
FROM kb.agentic_sources src
JOIN kb.agentic_messages m ON m.id=src.message_id
JOIN kb.agentic_conversations c ON c.id=m.conversation_id
WHERE c.id=$1 AND c.owner_user_id=$2
ORDER BY src.message_id, src.id`
	rows, err := s.db.QueryContext(ctx, query, conversationID, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]SourceRecord)
	for rows.Next() {
		var item SourceRecord
		if err := rows.Scan(&item.MessageID, &item.DocumentID, &item.Fingerprint, &item.SourceVersion); err != nil {
			return nil, err
		}
		out[item.MessageID] = append(out[item.MessageID], item)
	}
	return out, rows.Err()
}

func (s *Store) CreateMessage(ctx context.Context, ownerUserID string, in CreateMessageInput) (Message, error) {
	if in.Role == "assistant" && in.Status == "complete" {
		return Message{}, ErrAssistantCompletionRequiresFinalize
	}
	const query = `INSERT INTO kb.agentic_messages (
    conversation_id, attempt_id, sequence_no, role, content, status
)
SELECT c.id, $3, $4, $5, $6, $7
FROM kb.agentic_conversations c
WHERE c.id = $1 AND c.owner_user_id = $2
  AND ($3::text IS NULL OR EXISTS (
      SELECT 1 FROM kb.agentic_response_attempts a
      WHERE a.id = $3 AND a.conversation_id = c.id
  ))
RETURNING id, conversation_id, attempt_id, sequence_no, role, content, status, created_at, updated_at`
	return scanMessage(s.db.QueryRowContext(ctx, query,
		in.ConversationID, ownerUserID, in.AttemptID, in.SequenceNo, in.Role, in.Content, in.Status,
	))
}

func (s *Store) AppendMessageDelta(ctx context.Context, ownerUserID, messageID, delta, status string) (Message, error) {
	if status == "complete" {
		return Message{}, ErrAssistantCompletionRequiresFinalize
	}
	const query = `UPDATE kb.agentic_messages m
SET content = m.content || $3, status = $4, updated_at = NOW()
FROM kb.agentic_conversations c
WHERE m.id = $1 AND m.conversation_id = c.id AND c.owner_user_id = $2
RETURNING m.id, m.conversation_id, m.attempt_id, m.sequence_no, m.role,
          m.content, m.status, m.created_at, m.updated_at`
	return scanMessage(s.db.QueryRowContext(ctx, query, messageID, ownerUserID, delta, status))
}

func (s *Store) CreateAttempt(ctx context.Context, ownerUserID, conversationID, idempotencyKey string) (ResponseAttempt, error) {
	const query = `INSERT INTO kb.agentic_response_attempts (conversation_id, idempotency_key)
SELECT c.id, $3
FROM kb.agentic_conversations c
WHERE c.id = $1 AND c.owner_user_id = $2
ON CONFLICT (conversation_id, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, conversation_id, idempotency_key, status, error_code, error_message,
          input_tokens, output_tokens, created_at, updated_at`
	return scanAttempt(s.db.QueryRowContext(ctx, query, conversationID, ownerUserID, idempotencyKey))
}

func (s *Store) ListGrantedStoreIDs(ctx context.Context, userID string, allowedStoreNames []string) ([]string, error) {
	const query = `SELECT DISTINCT ks.id::text
FROM kb.agentic_knowledge_grants g
JOIN kb.knowledge_store ks ON ks.id=g.knowledge_store_id
WHERE g.user_id=$1 AND ks.ks_name=ANY($2) AND ks.status='active'
  AND g.active AND (g.expires_at IS NULL OR g.expires_at>now())
ORDER BY ks.id::text`
	rows, err := s.db.QueryContext(ctx, query, userID, pq.Array(allowedStoreNames))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CreateRunAttempt(ctx context.Context, ownerUserID, conversationID, idempotencyKey string) (ResponseAttempt, bool, error) {
	const insert = `INSERT INTO kb.agentic_response_attempts (conversation_id, idempotency_key)
SELECT c.id, $3 FROM kb.agentic_conversations c
WHERE c.id=$1 AND c.owner_user_id=$2
ON CONFLICT DO NOTHING
RETURNING id, conversation_id, idempotency_key, status, error_code, error_message,
          input_tokens, output_tokens, created_at, updated_at`
	created, err := scanAttempt(s.db.QueryRowContext(ctx, insert, conversationID, ownerUserID, idempotencyKey))
	if err == nil {
		return created, true, nil
	}
	if err != sql.ErrNoRows {
		return ResponseAttempt{}, false, err
	}
	const existing = `SELECT a.id, a.conversation_id, a.idempotency_key, a.status, a.error_code,
       a.error_message, a.input_tokens, a.output_tokens, a.created_at, a.updated_at
FROM kb.agentic_response_attempts a
JOIN kb.agentic_conversations c ON c.id=a.conversation_id
WHERE c.id=$1 AND c.owner_user_id=$2 AND a.idempotency_key=$3`
	attempt, err := scanAttempt(s.db.QueryRowContext(ctx, existing, conversationID, ownerUserID, idempotencyKey))
	if err == nil {
		return attempt, false, nil
	}
	if err != sql.ErrNoRows {
		return ResponseAttempt{}, false, err
	}
	var running bool
	const active = `SELECT EXISTS(SELECT 1 FROM kb.agentic_response_attempts a
JOIN kb.agentic_conversations c ON c.id=a.conversation_id
WHERE c.id=$1 AND c.owner_user_id=$2 AND a.status='running')`
	if err := s.db.QueryRowContext(ctx, active, conversationID, ownerUserID).Scan(&running); err != nil {
		return ResponseAttempt{}, false, err
	}
	if running {
		return ResponseAttempt{}, false, ErrRunAlreadyActive
	}
	return ResponseAttempt{}, false, sql.ErrNoRows
}

func (s *Store) GetRunAttempt(ctx context.Context, ownerUserID, conversationID, attemptID string) (ResponseAttempt, error) {
	const query = `SELECT a.id, a.conversation_id, a.idempotency_key, a.status, a.error_code,
       a.error_message, a.input_tokens, a.output_tokens, a.created_at, a.updated_at
FROM kb.agentic_response_attempts a
JOIN kb.agentic_conversations c ON c.id=a.conversation_id
WHERE a.id=$1 AND c.id=$2 AND c.owner_user_id=$3`
	return scanAttempt(s.db.QueryRowContext(ctx, query, attemptID, conversationID, ownerUserID))
}

func (s *Store) SetAttemptOutcome(ctx context.Context, ownerUserID, attemptID string, in AttemptOutcome) (ResponseAttempt, error) {
	const query = `UPDATE kb.agentic_response_attempts a
SET status = $3, error_code = $4, error_message = $5,
    input_tokens = $6, output_tokens = $7, updated_at = NOW(),
    completed_at = CASE WHEN $3 = 'running' THEN NULL ELSE NOW() END
FROM kb.agentic_conversations c
WHERE a.id = $1 AND a.conversation_id = c.id AND c.owner_user_id = $2
  AND a.status = 'running'
RETURNING a.id, a.conversation_id, a.idempotency_key, a.status, a.error_code,
          a.error_message, a.input_tokens, a.output_tokens, a.created_at, a.updated_at`
	return scanAttempt(s.db.QueryRowContext(ctx, query,
		attemptID, ownerUserID, in.Status, in.ErrorCode, in.ErrorMessage, in.InputTokens, in.OutputTokens,
	))
}

func (s *Store) RecordToolCall(ctx context.Context, ownerUserID string, in ToolCallInput) (string, error) {
	const query = `INSERT INTO kb.agentic_tool_calls (
    attempt_id, tool_name, status, input_summary, output_summary, duration_ms
)
SELECT a.id, $3, $4, $5, $6, $7
FROM kb.agentic_response_attempts a
JOIN kb.agentic_conversations c ON c.id = a.conversation_id
WHERE a.id = $1 AND c.owner_user_id = $2
RETURNING id`
	var id string
	inputSummary := jsonOrEmpty(in.InputSummary)
	outputSummary := jsonOrEmpty(in.OutputSummary)
	err := s.db.QueryRowContext(ctx, query,
		in.AttemptID, ownerUserID, in.ToolName, in.Status, inputSummary, outputSummary, in.DurationMS,
	).Scan(&id)
	return id, err
}

func (s *Store) RecordSource(ctx context.Context, ownerUserID string, in SourceInput) (int64, error) {
	if strings.TrimSpace(in.ToolCallID) == "" || strings.TrimSpace(in.Fingerprint) == "" {
		return 0, ErrSourceIdentityRequired
	}
	const query = `INSERT INTO kb.agentic_sources (
    attempt_id, message_id, tool_call_id, document_id, document_title, source_version,
    source_fingerprint, artifact_type, artifact_id, line_start, line_end, page_start, page_end
)
SELECT a.id, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
FROM kb.agentic_response_attempts a
JOIN kb.agentic_conversations c ON c.id = a.conversation_id
WHERE a.id = $1 AND c.owner_user_id = $2
  AND EXISTS (
      SELECT 1 FROM kb.agentic_messages m
      WHERE m.id = $3 AND m.conversation_id = a.conversation_id
        AND m.attempt_id = a.id AND m.role = 'assistant'
  )
RETURNING id`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		in.AttemptID, ownerUserID, in.MessageID, in.ToolCallID, in.DocumentID, in.DocumentTitle,
		in.SourceVersion, in.Fingerprint, in.ArtifactType, in.ArtifactID,
		in.LineStart, in.LineEnd, in.PageStart, in.PageEnd,
	).Scan(&id)
	return id, err
}

func jsonOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}

// FinalizeAssistantMessage makes completion and all of its source dependencies
// visible atomically, preventing a completed sourced answer without citations.
func (s *Store) FinalizeAssistantMessage(ctx context.Context, ownerUserID, messageID string, knowledgeUsed bool, sources []SourceInput) (Message, error) {
	if knowledgeUsed && len(sources) == 0 {
		return Message{}, ErrKnowledgeSourcesRequired
	}
	for _, source := range sources {
		if strings.TrimSpace(source.ToolCallID) == "" || strings.TrimSpace(source.Fingerprint) == "" {
			return Message{}, ErrSourceIdentityRequired
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	const update = `UPDATE kb.agentic_messages m
SET status = 'complete', updated_at = NOW()
FROM kb.agentic_conversations c
WHERE m.id = $1 AND m.conversation_id = c.id AND c.owner_user_id = $2
  AND m.role = 'assistant'
RETURNING m.id, m.conversation_id, m.attempt_id, m.sequence_no, m.role,
          m.content, m.status, m.created_at, m.updated_at`
	message, err := scanMessage(tx.QueryRowContext(ctx, update, messageID, ownerUserID))
	if err != nil {
		return Message{}, err
	}
	const insertSource = `INSERT INTO kb.agentic_sources (
    attempt_id, message_id, tool_call_id, document_id, document_title, source_version,
    source_fingerprint, artifact_type, artifact_id, line_start, line_end, page_start, page_end
)
SELECT a.id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
FROM kb.agentic_response_attempts a
WHERE a.id = $1 AND a.conversation_id = $14
  AND EXISTS (
      SELECT 1 FROM kb.agentic_messages m
      WHERE m.id = $2 AND m.attempt_id = a.id AND m.role = 'assistant'
  )
RETURNING id`
	for _, source := range sources {
		var id int64
		if err := tx.QueryRowContext(ctx, insertSource,
			source.AttemptID, messageID, source.ToolCallID, source.DocumentID, source.DocumentTitle,
			source.SourceVersion, source.Fingerprint, source.ArtifactType, source.ArtifactID,
			source.LineStart, source.LineEnd, source.PageStart, source.PageEnd, message.ConversationID,
		).Scan(&id); err != nil {
			return Message{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Store) UpsertFeedback(ctx context.Context, ownerUserID string, in FeedbackInput) (string, error) {
	const query = `INSERT INTO kb.agentic_feedback (message_id, owner_user_id, rating, comment)
SELECT m.id, $2, $3, $4
FROM kb.agentic_messages m
JOIN kb.agentic_conversations c ON c.id = m.conversation_id
WHERE m.id = $1 AND c.owner_user_id = $2
ON CONFLICT (message_id, owner_user_id) DO UPDATE
SET rating = EXCLUDED.rating, comment = EXCLUDED.comment, updated_at = NOW()
RETURNING id`
	var id string
	err := s.db.QueryRowContext(ctx, query, in.MessageID, ownerUserID, in.Rating, in.Comment).Scan(&id)
	return id, err
}

func (s *Store) UpsertConversationFeedback(ctx context.Context, ownerUserID, conversationID string, in FeedbackInput) (string, error) {
	const query = `INSERT INTO kb.agentic_feedback (message_id, owner_user_id, rating, comment)
SELECT m.id, $2, $4, $5
FROM kb.agentic_messages m
JOIN kb.agentic_conversations c ON c.id=m.conversation_id
WHERE m.id=$3 AND c.id=$1 AND c.owner_user_id=$2 AND m.role='assistant' AND m.status='complete'
ON CONFLICT (message_id, owner_user_id) DO UPDATE
SET rating=EXCLUDED.rating, comment=EXCLUDED.comment, updated_at=NOW()
RETURNING id`
	var id string
	err := s.db.QueryRowContext(ctx, query, conversationID, ownerUserID, in.MessageID, in.Rating, in.Comment).Scan(&id)
	return id, err
}

func (s *Store) DeleteConversation(ctx context.Context, ownerUserID, conversationID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM kb.agentic_conversations WHERE id = $1 AND owner_user_id = $2`,
		conversationID, ownerUserID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
