package profiles

import (
	"context"
	"errors"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// OntologyFinding is a deterministic P4 result with links to its frozen scope,
// governing rule, and (when present) evaluated assertion.
type OntologyFinding struct {
	InputRecordID int64
	RunID         int64
	ScopeID       string
	ProfileRuleID int64
	AssertionID   int64
	Category      ResultCategory
	Severity      string
	Title         string
	Description   string
}

type FindingStore struct{ DB terms.DBX }

func (s FindingStore) Persist(ctx context.Context, f OntologyFinding) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if f.InputRecordID == 0 || f.RunID == 0 || f.ScopeID == "" || f.ProfileRuleID == 0 || f.Category == "" || f.Severity == "" || f.Title == "" || f.Description == "" {
		return errors.New("finding provenance and content are required")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_review_findings (input_record_id, run_id, pass, aspect, severity, finding_type, title, description, review_scope_id, profile_rule_id, assertion_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, 0))`, f.InputRecordID, f.RunID, "ontology_profile", "profile", f.Severity, string(f.Category), f.Title, f.Description, f.ScopeID, f.ProfileRuleID, f.AssertionID)
	return err
}
