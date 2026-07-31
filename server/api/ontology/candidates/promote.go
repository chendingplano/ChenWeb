package candidates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// PromoteToContent materializes an approved candidate into the governed
// content tables (kb.ontology_terms / kb.ontology_term_labels /
// kb.ontology_mappings / kb.ontology_axioms) and links the content row back
// via source_candidate_id. The candidate itself stays `approved`: per spec
// §16.3 item 3, an approved candidate remains inactive until included in a
// module release (chunk B owns that transition and the release linkage).
//
// The operation is idempotent: if content for this candidate already exists
// (source_candidate_id), it is a no-op for that table, so a retry after a
// transient failure does not duplicate rows (spec §16.3 item 13).
//
// Only term, label, mapping, and axiom candidates are promotable in this
// chunk; profile / profile_rule / module_change need the release mechanism.
func (s CandidateStore) PromoteToContent(ctx context.Context, candidateID int64, by string) (Candidate, error) {
	if s.DB == nil {
		return Candidate{}, errors.New("db is nil")
	}
	c, err := s.GetCandidate(ctx, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	if c.Status != StatusApproved {
		return Candidate{}, fmt.Errorf("candidate is %s, not approved", c.Status)
	}

	switch c.CandidateKind {
	case "term":
		err = promoteTerm(ctx, s.DB, c, by)
	case "label":
		err = promoteLabel(ctx, s.DB, c, by)
	case "mapping":
		err = promoteMapping(ctx, s.DB, c, by)
	case "axiom":
		err = promoteAxiom(ctx, s.DB, c, by)
	default:
		return Candidate{}, fmt.Errorf("candidate_kind %q is not promotable in this chunk", c.CandidateKind)
	}
	if err != nil {
		return Candidate{}, err
	}
	return c, nil
}

func contentExists(ctx context.Context, db *sql.DB, table string, candidateID int64) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+table+" WHERE source_candidate_id = $1)",
		candidateID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func promoteTerm(ctx context.Context, db *sql.DB, c Candidate, by string) error {
	var p struct {
		TermID     string `json:"term_id"`
		TermKind   string `json:"term_kind"`
		ModuleID   string `json:"module_id"`
		Definition string `json:"definition"`
		Scope      string `json:"scope"`
	}
	if err := json.Unmarshal(c.ProposedPayload, &p); err != nil {
		return fmt.Errorf("decode term candidate payload: %w", err)
	}
	exists, err := contentExists(ctx, db, "kb.ontology_terms", c.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil // already materialized
	}
	store := terms.TermStore{DB: db}
	_, err = store.CreateTerm(ctx, terms.Term{
		TermID:            p.TermID,
		TermKind:          p.TermKind,
		ModuleID:          p.ModuleID,
		Definition:        p.Definition,
		Scope:             p.Scope,
		Status:            "approved",
		SourceCandidateID: &c.ID,
		CreateBy:          by,
		ModifyBy:          by,
	})
	return err
}

func promoteLabel(ctx context.Context, db *sql.DB, c Candidate, by string) error {
	var p struct {
		TermID    string `json:"term_id"`
		Label     string `json:"label"`
		Lang      string `json:"lang"`
		LabelRole string `json:"label_role"`
	}
	if err := json.Unmarshal(c.ProposedPayload, &p); err != nil {
		return fmt.Errorf("decode label candidate payload: %w", err)
	}
	exists, err := contentExists(ctx, db, "kb.ontology_term_labels", c.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	store := terms.LabelStore{DB: db}
	_, err = store.CreateLabel(ctx, terms.TermLabel{
		TermID:            p.TermID,
		Label:             p.Label,
		Lang:              p.Lang,
		LabelRole:         p.LabelRole,
		Status:            "approved",
		SourceCandidateID: &c.ID,
		CreateBy:          by,
		ModifyBy:          by,
	})
	return err
}

func promoteMapping(ctx context.Context, db *sql.DB, c Candidate, by string) error {
	var p struct {
		MappingID  string `json:"mapping_id"`
		FromTermID string `json:"from_term_id"`
		ToTermID   string `json:"to_term_id"`
		ToIRI      string `json:"to_iri"`
		Relation   string `json:"relation"`
		ModuleID   string `json:"module_id"`
	}
	if err := json.Unmarshal(c.ProposedPayload, &p); err != nil {
		return fmt.Errorf("decode mapping candidate payload: %w", err)
	}
	exists, err := contentExists(ctx, db, "kb.ontology_mappings", c.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	// A mapping candidate reached `approved`, which is the explicit approval
	// exact mappings require (spec §9.1).
	approval := "approved"
	store := terms.MappingStore{DB: db}
	_, err = store.CreateMapping(ctx, terms.Mapping{
		MappingID:        p.MappingID,
		FromTermID:       p.FromTermID,
		ToTermID:         p.ToTermID,
		ToIRI:            p.ToIRI,
		Relation:         p.Relation,
		ApprovalStatus:   approval,
		ModuleID:         p.ModuleID,
		Status:           "approved",
		SourceCandidateID: &c.ID,
		CreateBy:         by,
		ModifyBy:         by,
	})
	return err
}

func promoteAxiom(ctx context.Context, db *sql.DB, c Candidate, by string) error {
	var p struct {
		AxiomID        string `json:"axiom_id"`
		AxiomKind      string `json:"axiom_kind"`
		SubjectTermID  string `json:"subject_term_id"`
		PredicateTermID string `json:"predicate_term_id"`
		ObjectTermID   string `json:"object_term_id"`
		ObjectIRI      string `json:"object_iri"`
		ModuleID       string `json:"module_id"`
	}
	if err := json.Unmarshal(c.ProposedPayload, &p); err != nil {
		return fmt.Errorf("decode axiom candidate payload: %w", err)
	}
	exists, err := contentExists(ctx, db, "kb.ontology_axioms", c.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	store := terms.AxiomStore{DB: db}
	_, err = store.CreateAxiom(ctx, terms.Axiom{
		AxiomID:           p.AxiomID,
		AxiomKind:         p.AxiomKind,
		SubjectTermID:     p.SubjectTermID,
		PredicateTermID:   p.PredicateTermID,
		ObjectTermID:      p.ObjectTermID,
		ObjectIRI:         p.ObjectIRI,
		ModuleID:          p.ModuleID,
		Status:            "approved",
		SourceCandidateID: &c.ID,
		CreateBy:          by,
		ModifyBy:          by,
	})
	return err
}
