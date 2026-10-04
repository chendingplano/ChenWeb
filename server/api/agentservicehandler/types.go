package agentservicehandler

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

type SourceRecord struct {
	MessageID     string `json:"message_id"`
	DocumentID    string `json:"document_id"`
	Fingerprint   string `json:"-"`
	SourceVersion string `json:"source_version,omitempty"`
	DocumentTitle string `json:"document_title,omitempty"`
	ArtifactType  string `json:"artifact_type,omitempty"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	LineStart     int    `json:"line_start,omitempty"`
	LineEnd       int    `json:"line_end,omitempty"`
	PageStart     int    `json:"page_start,omitempty"`
	PageEnd       int    `json:"page_end,omitempty"`
}

type CurrentSourceAccessChecker struct{ DB *sql.DB }

func (a CurrentSourceAccessChecker) CheckSource(ctx context.Context, userID string, allowedStoreNames []string, source SourceRecord) error {
	return a.CheckSourceWithGroups(ctx, userID, allowedStoreNames, nil, source)
}

func (a CurrentSourceAccessChecker) CheckSourceWithGroups(ctx context.Context, userID string, allowedStoreNames, allowedDocumentGroups []string, source SourceRecord) error {
	if a.DB == nil || userID == "" || source.DocumentID == "" || source.Fingerprint == "" || len(allowedStoreNames) == 0 {
		return ErrKnowledgeAccessDenied
	}
	var allowed bool
	err := a.DB.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM kb.agentic_knowledge_grants g
JOIN kb.knowledge_store ks ON ks.id=g.knowledge_store_id
JOIN kb.inputs i ON i.ks_store_id=ks.id
WHERE g.user_id=$1 AND ks.ks_name=ANY($2) AND i.id::text=$3
  AND ks.status='active' AND g.active AND (g.expires_at IS NULL OR g.expires_at>now())
  AND (g.document_id IS NULL OR g.document_id=i.id)
  AND COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text)=$4
  AND (CARDINALITY($5::text[])=0 OR i.type=ANY($5))
  AND ($6='' OR i.modify_time::text=$6)
)`, userID, pq.Array(allowedStoreNames), source.DocumentID, source.Fingerprint, pq.Array(allowedDocumentGroups), source.SourceVersion).Scan(&allowed)
	if err != nil || !allowed {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

// CheckDocumentAccess checks only whether the user may still access a
// document, not whether it changed. It applies the snapshot rule to history
// older than the snapshot period.
func (a CurrentSourceAccessChecker) CheckDocumentAccess(ctx context.Context, userID string, allowedStoreNames, allowedDocumentGroups []string, documentID string) error {
	if a.DB == nil || userID == "" || documentID == "" || len(allowedStoreNames) == 0 {
		return ErrKnowledgeAccessDenied
	}
	var allowed bool
	err := a.DB.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM kb.agentic_knowledge_grants g
JOIN kb.knowledge_store ks ON ks.id=g.knowledge_store_id
JOIN kb.inputs i ON i.ks_store_id=ks.id
WHERE g.user_id=$1 AND ks.ks_name=ANY($2) AND i.id::text=$3
  AND ks.status='active' AND g.active AND (g.expires_at IS NULL OR g.expires_at>now())
  AND (g.document_id IS NULL OR g.document_id=i.id)
  AND (CARDINALITY($4::text[])=0 OR i.type=ANY($4))
)`, userID, pq.Array(allowedStoreNames), documentID, pq.Array(allowedDocumentGroups)).Scan(&allowed)
	if err != nil || !allowed {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

// VisibleResumeState is a conversation as the user may see it, and as it is
// resent to Pi, under the snapshot rule.
type VisibleResumeState struct {
	Conversation     Conversation              `json:"conversation"`
	Messages         []Message                 `json:"messages"`
	SourcesByMessage map[string][]SourceRecord `json:"sources_by_message,omitempty"`
	// ToolResults holds stored tool calls by attempt; they are not sent to
	// the browser.
	ToolResults        map[string][]ToolResultRecord `json:"-"`
	RemovedSources     int                           `json:"removed_sources"`
	RemovedToolResults int                           `json:"removed_tool_results"`
	SnapshotHours      int                           `json:"snapshot_hours"`
}

// FilterResumeState applies the snapshot rule. Records younger than window
// are kept exactly as saved. In older records, a source whose document the
// user can no longer access is removed, and so is a tool result referencing
// any such document. Content changes are not checked, and messages (answers)
// are never removed.
func FilterResumeState(ctx context.Context, state ResumeState, sources map[string][]SourceRecord, toolResults map[string][]ToolResultRecord,
	now time.Time, window time.Duration, canAccess func(context.Context, string) error) VisibleResumeState {
	out := VisibleResumeState{Conversation: state.Conversation, Messages: append([]Message(nil), state.Messages...),
		SourcesByMessage: make(map[string][]SourceRecord), ToolResults: make(map[string][]ToolResultRecord), SnapshotHours: int(window / time.Hour)}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	access := make(map[string]bool)
	accessible := func(documentID string) bool {
		if allowed, ok := access[documentID]; ok {
			return allowed
		}
		allowed := canAccess != nil && canAccess(ctx, documentID) == nil
		access[documentID] = allowed
		return allowed
	}
	expired := func(saved time.Time) bool { return now.Sub(saved) > window }
	for _, message := range state.Messages {
		for _, source := range sources[message.ID] {
			if expired(message.CreatedAt) && !accessible(source.DocumentID) {
				out.RemovedSources++
				continue
			}
			out.SourcesByMessage[message.ID] = append(out.SourcesByMessage[message.ID], source)
		}
	}
	for attemptID, records := range toolResults {
	records:
		for _, record := range records {
			if expired(record.CreatedAt) {
				for _, documentID := range record.DocumentIDs {
					if !accessible(documentID) {
						out.RemovedToolResults++
						continue records
					}
				}
			}
			out.ToolResults[attemptID] = append(out.ToolResults[attemptID], record)
		}
	}
	return out
}

var ErrSourceDependenciesUnavailable = errors.New("source dependencies unavailable")
