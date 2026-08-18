package classfoundation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ClassResolutionAlternative is one ranked candidate behind a decision (DR6:
// "records every candidate term, score, method, and evidence"). At least one
// of CandidateClassTermID or CandidateKey is required, matching the table's
// CHECK constraint -- an ungoverned candidate (no term yet) is still recorded
// by its raw key.
type ClassResolutionAlternative struct {
	CandidateClassTermID string
	CandidateKey         string
	Rank                 int
	Score                *float64
	Evidence             string
}

// ClassResolutionDecision is one row of the append-only
// kb.ontology_class_resolution_decisions history (ADR 2026081701 DR3/DR7,
// consumed by ADR 2026081801 DR5 item 2). The table itself rejects UPDATE/
// DELETE; superseding a decision means inserting a new row with
// SupersedesDecisionID set, never mutating the prior one.
type ClassResolutionDecision struct {
	ID                   int64
	SourceArtifactType   string
	SourceArtifactID     string
	SourceInputRecordID  *int64
	SourceAssertionID    *int64
	SelectedClassTermID  string
	IdentityState        string
	Method               string
	Confidence           *float64
	Evidence             string
	SupersedesDecisionID *int64
	CreateBy             string
}

// ClassResolutionDecisionStore persists class-resolution decisions and their
// alternatives.
type ClassResolutionDecisionStore struct {
	DB DBX
}

// Latest returns the most recent decision for one source scope, or
// sql.ErrNoRows if none exists yet.
func (s ClassResolutionDecisionStore) Latest(ctx context.Context, sourceArtifactType, sourceArtifactID string, sourceInputRecordID *int64) (ClassResolutionDecision, error) {
	if s.DB == nil {
		return ClassResolutionDecision{}, errors.New("db is nil")
	}
	row := s.DB.QueryRowContext(ctx, `
SELECT id, source_artifact_type, source_artifact_id, source_input_record_id, source_assertion_id,
       coalesce(selected_class_term_id, ''), identity_state, method, confidence,
       evidence::text, supersedes_decision_id, coalesce(create_by, '')
FROM kb.ontology_class_resolution_decisions
WHERE source_artifact_type = $1 AND source_artifact_id = $2
  AND source_input_record_id IS NOT DISTINCT FROM $3
ORDER BY create_time DESC, id DESC
LIMIT 1`, sourceArtifactType, sourceArtifactID, sourceInputRecordID)
	return scanClassResolutionDecision(row)
}

// RecordIfChanged records a new decision only when the source scope has no
// prior decision, or the prior one selected a different class/identity state.
// An unchanged retry (the common case when a stage is replayed with the same
// inputs) writes nothing, consistent with the append-only table's intent to
// hold real decision history rather than one row per attempt.
func (s ClassResolutionDecisionStore) RecordIfChanged(ctx context.Context, decision ClassResolutionDecision, alternatives []ClassResolutionAlternative) (ClassResolutionDecision, bool, error) {
	if s.DB == nil {
		return ClassResolutionDecision{}, false, errors.New("db is nil")
	}
	prior, err := s.Latest(ctx, decision.SourceArtifactType, decision.SourceArtifactID, decision.SourceInputRecordID)
	switch {
	case err == nil:
		if prior.SelectedClassTermID == decision.SelectedClassTermID && prior.IdentityState == decision.IdentityState {
			return prior, false, nil
		}
		id := prior.ID
		decision.SupersedesDecisionID = &id
	case errors.Is(err, sql.ErrNoRows):
		// No prior decision: nothing to supersede.
	default:
		return ClassResolutionDecision{}, false, err
	}
	recorded, err := s.record(ctx, decision, alternatives)
	if err != nil {
		return ClassResolutionDecision{}, false, err
	}
	return recorded, true, nil
}

func (s ClassResolutionDecisionStore) record(ctx context.Context, decision ClassResolutionDecision, alternatives []ClassResolutionAlternative) (ClassResolutionDecision, error) {
	if strings.TrimSpace(decision.SourceArtifactType) == "" || strings.TrimSpace(decision.SourceArtifactID) == "" {
		return ClassResolutionDecision{}, errors.New("source_artifact_type and source_artifact_id are required")
	}
	if strings.TrimSpace(decision.IdentityState) == "" || strings.TrimSpace(decision.Method) == "" {
		return ClassResolutionDecision{}, errors.New("identity_state and method are required")
	}
	row := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.ontology_class_resolution_decisions (
  source_artifact_type, source_artifact_id, source_input_record_id, source_assertion_id,
  selected_class_term_id, identity_state, method, confidence, evidence,
  supersedes_decision_id, create_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
RETURNING id, source_artifact_type, source_artifact_id, source_input_record_id, source_assertion_id,
          coalesce(selected_class_term_id, ''), identity_state, method, confidence,
          evidence::text, supersedes_decision_id, coalesce(create_by, '')`,
		decision.SourceArtifactType, decision.SourceArtifactID, decision.SourceInputRecordID, decision.SourceAssertionID,
		nullable(decision.SelectedClassTermID), decision.IdentityState, decision.Method, decision.Confidence,
		normalizedJSON(decision.Evidence), decision.SupersedesDecisionID, nullable(decision.CreateBy))
	recorded, err := scanClassResolutionDecision(row)
	if err != nil {
		return ClassResolutionDecision{}, fmt.Errorf("record class resolution decision: %w", err)
	}
	for i, alt := range alternatives {
		rank := alt.Rank
		if rank == 0 {
			rank = i + 1
		}
		if _, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_class_resolution_alternatives (
  decision_id, candidate_class_term_id, candidate_key, rank, score, evidence
)
VALUES ($1,$2,$3,$4,$5,$6::jsonb)`,
			recorded.ID, nullable(alt.CandidateClassTermID), alt.CandidateKey, rank, alt.Score, normalizedJSON(alt.Evidence)); err != nil {
			return ClassResolutionDecision{}, fmt.Errorf("record class resolution alternative: %w", err)
		}
	}
	return recorded, nil
}

func scanClassResolutionDecision(row *sql.Row) (ClassResolutionDecision, error) {
	var d ClassResolutionDecision
	if err := row.Scan(&d.ID, &d.SourceArtifactType, &d.SourceArtifactID, &d.SourceInputRecordID, &d.SourceAssertionID,
		&d.SelectedClassTermID, &d.IdentityState, &d.Method, &d.Confidence,
		&d.Evidence, &d.SupersedesDecisionID, &d.CreateBy); err != nil {
		return ClassResolutionDecision{}, err
	}
	return d, nil
}

