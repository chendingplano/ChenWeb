package terms

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Axiom is one version of a governed ontology axiom. AxiomKind is restricted
// to compiler-approved kinds in chunk B's module compiler; the column itself
// is open so the compiler owns the approved-kinds vocabulary.
type Axiom struct {
	ID                int64     `json:"id"`
	AxiomID           string    `json:"axiom_id"`
	Version           int       `json:"version"`
	AxiomKind         string    `json:"axiom_kind"`
	SubjectTermID     string    `json:"subject_term_id"`
	PredicateTermID   string    `json:"predicate_term_id"`
	ObjectTermID      string    `json:"object_term_id"`
	ObjectIRI         string    `json:"object_iri"`
	ModuleID          string    `json:"module_id"`
	Status            string    `json:"status"`
	SourceCandidateID *int64    `json:"source_candidate_id"`
	CreateTime        time.Time `json:"create_time"`
	CreateBy          string    `json:"create_by"`
	ModifyTime        time.Time `json:"modify_time"`
	ModifyBy          string    `json:"modify_by"`
}

// AxiomStore persists versioned axiom rows.
type AxiomStore struct {
	DB *sql.DB
}

const axiomColumns = `
	id, axiom_id, version, axiom_kind, subject_term_id, predicate_term_id,
	object_term_id, object_iri, module_id, status, source_candidate_id,
	create_time, create_by, modify_time, modify_by`

const axiomFrom = "FROM kb.ontology_axioms"

func scanAxiom(scan func(dest ...any) error) (Axiom, error) {
	var (
		a               Axiom
		predicate       sql.NullString
		objectTerm      sql.NullString
		objectIRI       sql.NullString
		sourceCandidate sql.NullInt64
		createBy        sql.NullString
		modifyBy        sql.NullString
	)
	if err := scan(
		&a.ID, &a.AxiomID, &a.Version, &a.AxiomKind, &a.SubjectTermID, &predicate,
		&objectTerm, &objectIRI, &a.ModuleID, &a.Status, &sourceCandidate,
		&a.CreateTime, &createBy, &a.ModifyTime, &modifyBy,
	); err != nil {
		return Axiom{}, err
	}
	if predicate.Valid {
		a.PredicateTermID = predicate.String
	}
	if objectTerm.Valid {
		a.ObjectTermID = objectTerm.String
	}
	if objectIRI.Valid {
		a.ObjectIRI = objectIRI.String
	}
	if sourceCandidate.Valid {
		v := sourceCandidate.Int64
		a.SourceCandidateID = &v
	}
	if createBy.Valid {
		a.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		a.ModifyBy = modifyBy.String
	}
	return a, nil
}

func validateAxiom(a Axiom) error {
	if strings.TrimSpace(a.AxiomID) == "" {
		return errors.New("axiom_id is required")
	}
	if strings.TrimSpace(a.AxiomKind) == "" {
		return errors.New("axiom_kind is required")
	}
	if strings.TrimSpace(a.SubjectTermID) == "" {
		return errors.New("subject_term_id is required")
	}
	if strings.TrimSpace(a.ObjectTermID) == "" && strings.TrimSpace(a.ObjectIRI) == "" {
		return errors.New("object_term_id or object_iri is required")
	}
	if strings.TrimSpace(a.ModuleID) == "" {
		return errors.New("module_id is required")
	}
	if a.Status == "" {
		a.Status = "draft"
	}
	return nil
}

// CreateAxiom inserts version 1 of an axiom.
func (s AxiomStore) CreateAxiom(ctx context.Context, a Axiom) (Axiom, error) {
	if s.DB == nil {
		return Axiom{}, errors.New("db is nil")
	}
	if err := validateAxiom(a); err != nil {
		return Axiom{}, err
	}
	const stmt = `
INSERT INTO kb.ontology_axioms
	(axiom_id, version, axiom_kind, subject_term_id, predicate_term_id,
	 object_term_id, object_iri, module_id, status, source_candidate_id,
	 create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING ` + axiomColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(a.AxiomID), a.AxiomKind, strings.TrimSpace(a.SubjectTermID),
		nullableString(a.PredicateTermID), nullableString(a.ObjectTermID), nullableString(a.ObjectIRI),
		strings.TrimSpace(a.ModuleID), a.Status, nullableInt64(a.SourceCandidateID),
		nullableString(a.CreateBy), nullableString(a.ModifyBy),
	)
	return scanAxiom(row.Scan)
}

// ListAxioms returns axiom rows for a module, newest version first.
func (s AxiomStore) ListAxioms(ctx context.Context, moduleID string) ([]Axiom, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + axiomColumns + "\n" + axiomFrom + `
WHERE ($1 = '' OR module_id = $1)
ORDER BY axiom_id, version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	axioms := make([]Axiom, 0)
	for rows.Next() {
		a, err := scanAxiom(rows.Scan)
		if err != nil {
			return nil, err
		}
		axioms = append(axioms, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return axioms, nil
}
