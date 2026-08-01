package profiles

import (
	"context"
	"errors"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// OntologyFinding is a deterministic P4 result with links to its frozen scope,
// governing rule, and (when present) evaluated assertion. RunID is the
// pre-existing, client-supplied cross-pass run identity shared with LLM
// review findings; ReviewRunID is the FK-integrous, server-generated
// kb.ontology_review_runs row that pins the assertion watermark this
// specific ontology finding was evaluated against.
type OntologyFinding struct {
	InputRecordID int64
	RunID         int64
	ReviewRunID   int64
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
	if f.InputRecordID == 0 || f.RunID == 0 || f.ReviewRunID == 0 || f.ScopeID == "" || f.ProfileRuleID == 0 || f.Category == "" || f.Severity == "" || f.Title == "" || f.Description == "" {
		return errors.New("finding provenance and content are required")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kb.doc_review_findings (input_record_id, run_id, review_run_id, pass, aspect, severity, finding_type, title, description, review_scope_id, profile_rule_id, assertion_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULLIF($12, 0))`, f.InputRecordID, f.RunID, f.ReviewRunID, "ontology_profile", "profile", f.Severity, string(f.Category), f.Title, f.Description, f.ScopeID, f.ProfileRuleID, f.AssertionID)
	return err
}
