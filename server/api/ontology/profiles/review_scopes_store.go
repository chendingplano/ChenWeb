package profiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// ReviewScope freezes every selection fact required to reproduce a normative
// review; callers cannot update a scope after creation.
type ReviewScope struct {
	ReviewScopeID       string          `json:"review_scope_id"`
	ReviewedDocumentIDs json.RawMessage `json:"reviewed_document_ids"`
	TargetObjectIDs     json.RawMessage `json:"target_object_ids"`
	TargetClassTermIDs  json.RawMessage `json:"target_class_term_ids"`
	AsOfDate            string          `json:"as_of_date"`
	Jurisdiction        string          `json:"jurisdiction"`
	OperatingContext    string          `json:"operating_context"`
	SelectedProfiles    json.RawMessage `json:"selected_profiles"`
	SelectionMode       string          `json:"selection_mode"`
	PrecedencePolicy    json.RawMessage `json:"precedence_policy"`
	ClosedDimensions    json.RawMessage `json:"closed_dimensions"`
	SelectedBy          string          `json:"selected_by"`
	SelectionReason     string          `json:"selection_reason"`
	CreateTime          time.Time       `json:"create_time"`
}

type ReviewScopeStore struct{ DB terms.DBX }

func (s ReviewScopeStore) Create(ctx context.Context, scope ReviewScope) (ReviewScope, error) {
	if s.DB == nil {
		return ReviewScope{}, errors.New("db is nil")
	}
	if strings.TrimSpace(scope.ReviewScopeID) == "" || scope.AsOfDate == "" || strings.TrimSpace(scope.Jurisdiction) == "" || len(scope.SelectedProfiles) == 0 {
		return ReviewScope{}, errors.New("review_scope_id, as_of_date, jurisdiction, and selected_profiles are required")
	}
	if scope.SelectionMode == "" {
		scope.SelectionMode = "explicit"
	}
	if scope.SelectionMode != "explicit" && scope.SelectionMode != "deterministic_rule" {
		return ReviewScope{}, errors.New("unsupported selection_mode")
	}
	if len(scope.ReviewedDocumentIDs) == 0 {
		scope.ReviewedDocumentIDs = json.RawMessage(`[]`)
	}
	if len(scope.TargetObjectIDs) == 0 {
		scope.TargetObjectIDs = json.RawMessage(`[]`)
	}
	if len(scope.TargetClassTermIDs) == 0 {
		scope.TargetClassTermIDs = json.RawMessage(`[]`)
	}
	if len(scope.PrecedencePolicy) == 0 {
		scope.PrecedencePolicy = json.RawMessage(`{}`)
	}
	if len(scope.ClosedDimensions) == 0 {
		scope.ClosedDimensions = json.RawMessage(`[]`)
	}
	const stmt = `INSERT INTO kb.ontology_review_scopes (review_scope_id, reviewed_document_ids, target_object_ids, target_class_term_ids, as_of_date, jurisdiction, operating_context, selected_profiles, selection_mode, precedence_policy, closed_dimensions, selected_by, selection_reason) VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::date, $6, $7, $8::jsonb, $9, $10::jsonb, $11::jsonb, $12, $13) RETURNING review_scope_id, reviewed_document_ids, target_object_ids, target_class_term_ids, as_of_date::text, jurisdiction, operating_context, selected_profiles, selection_mode, precedence_policy, closed_dimensions, COALESCE(selected_by, ''), COALESCE(selection_reason, ''), create_time`
	row := s.DB.QueryRowContext(ctx, stmt, scope.ReviewScopeID, string(scope.ReviewedDocumentIDs), string(scope.TargetObjectIDs), string(scope.TargetClassTermIDs), scope.AsOfDate, scope.Jurisdiction, nullableText(scope.OperatingContext), string(scope.SelectedProfiles), scope.SelectionMode, string(scope.PrecedencePolicy), string(scope.ClosedDimensions), nullableText(scope.SelectedBy), nullableText(scope.SelectionReason))
	var (
		out     ReviewScope
		context sql.NullString
	)
	err := row.Scan(&out.ReviewScopeID, &out.ReviewedDocumentIDs, &out.TargetObjectIDs, &out.TargetClassTermIDs, &out.AsOfDate, &out.Jurisdiction, &context, &out.SelectedProfiles, &out.SelectionMode, &out.PrecedencePolicy, &out.ClosedDimensions, &out.SelectedBy, &out.SelectionReason, &out.CreateTime)
	if context.Valid {
		out.OperatingContext = context.String
	}
	return out, err
}
