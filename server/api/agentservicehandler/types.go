package agentservicehandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
  AND i.tenant_id=ks.tenant_id
  AND COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text)=$4
  AND (CARDINALITY($5::text[])=0 OR i.type=ANY($5))
  AND ($6='' OR i.modify_time::text=$6)
)`, userID, pq.Array(allowedStoreNames), source.DocumentID, source.Fingerprint, pq.Array(allowedDocumentGroups), source.SourceVersion).Scan(&allowed)
	if err != nil || !allowed {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

type VisibleResumeState struct {
	Conversation     Conversation              `json:"conversation"`
	Messages         []Message                 `json:"messages"`
	SourcesByMessage map[string][]SourceRecord `json:"sources_by_message,omitempty"`
	HiddenMessageIDs []string                  `json:"hidden_message_ids,omitempty"`
	OmissionNotice   string                    `json:"omission_notice,omitempty"`
}

func FilterResumeState(ctx context.Context, state ResumeState, sources map[string][]SourceRecord, check func(context.Context, SourceRecord) error) VisibleResumeState {
	out := VisibleResumeState{Conversation: state.Conversation, Messages: make([]Message, 0, len(state.Messages)), SourcesByMessage: make(map[string][]SourceRecord)}
	for _, message := range state.Messages {
		if message.Role == "assistant" && message.Status == "complete" {
			blocked := false
			for _, source := range sources[message.ID] {
				if check == nil || check(ctx, source) != nil {
					blocked = true
					break
				}
			}
			if blocked {
				out.HiddenMessageIDs = append(out.HiddenMessageIDs, message.ID)
				continue
			}
			if len(sources[message.ID]) > 0 {
				out.SourcesByMessage[message.ID] = append([]SourceRecord(nil), sources[message.ID]...)
			}
		}
		out.Messages = append(out.Messages, message)
	}
	if len(out.HiddenMessageIDs) > 0 {
		out.OmissionNotice = fmt.Sprintf("%d saved answer(s) were hidden because their sources are no longer accessible or current.", len(out.HiddenMessageIDs))
	}
	return out
}

var ErrSourceDependenciesUnavailable = errors.New("source dependencies unavailable")
