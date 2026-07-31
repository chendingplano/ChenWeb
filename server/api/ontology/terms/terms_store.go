// Package terms provides versioned stores for governed ontology content:
// terms, term labels, axioms, and mappings (ADR 2026072901 DR1, DR13; spec
// 2026072702 §9.3). Content is authored and versioned in the database -- the
// 2026-07-31 storage decision -- so every accepted change inserts a new
// version row and the latest version is the current state.
package terms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Term is one version of a governed ontology term. TermID is the immutable,
// namespaced identity (e.g. `core:assertion`); a material meaning change
// creates a replacement TermID rather than redefining the existing one.
type Term struct {
	ID                int64     `json:"id"`
	TermID            string    `json:"term_id"`
	Version           int       `json:"version"`
	TermKind          string    `json:"term_kind"`
	ModuleID          string    `json:"module_id"`
	Status            string    `json:"status"`
	Definition        string    `json:"definition"`
	Scope             string    `json:"scope"`
	SourceCandidateID *int64    `json:"source_candidate_id"`
	CreateTime        time.Time `json:"create_time"`
	CreateBy          string    `json:"create_by"`
	ModifyTime        time.Time `json:"modify_time"`
	ModifyBy          string    `json:"modify_by"`
}

// DBX is the subset of database/sql that the content stores need. Both
// *sql.DB and *sql.Tx satisfy it, so store methods can run against a live
// connection or inside a caller's transaction (the module release flow tags
// content rows atomically in one tx).
type DBX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// AllowedTermKinds is the governed set of term kinds, mirroring the schema
// CHECK on kb.ontology_terms. Only compiler-approved kinds may be authored.
var AllowedTermKinds = map[string]bool{
	"class":            true,
	"property":         true,
	"individual":       true,
	"concept":          true,
	"metric_definition": true,
	"quantity_kind":    true,
	"unit":             true,
	"dimension":        true,
}

// AllowedTermStatuses is the governed content lifecycle (spec §9.3). The
// included_in_release transition and the release linkage arrive in chunk B.
var AllowedTermStatuses = map[string]bool{
	"draft":             true,
	"in_review":         true,
	"approved":          true,
	"included_in_release": true,
	"superseded":        true,
	"rejected":          true,
}

// errIllegalTermTransition is returned when a content-row status change does
// not follow the governed lifecycle (spec §9.3).
var errIllegalTermTransition = errors.New("illegal term status transition")

// termStatusTransitions lists the allowed content-row status changes. The
// governed candidate state machine (spec §9.3) is the same lifecycle; content
// rows follow the same arcs once materialized.
var termStatusTransitions = map[string]map[string]bool{
	"draft":       {"in_review": true, "rejected": true},
	"in_review":   {"approved": true, "rejected": true},
	"approved":    {"included_in_release": true, "superseded": true},
	"included_in_release": {"superseded": true},
}

// TermStore persists versioned term rows.
type TermStore struct {
	DB DBX
}

const termColumns = `
	id, term_id, version, term_kind, module_id, status, definition, scope,
	source_candidate_id, create_time, create_by, modify_time, modify_by`

const termFrom = "FROM kb.ontology_terms"

func scanTerm(scan func(dest ...any) error) (Term, error) {
	var (
		t                Term
		definition       sql.NullString
		scope            sql.NullString
		sourceCandidate  sql.NullInt64
		createBy         sql.NullString
		modifyBy         sql.NullString
	)
	if err := scan(
		&t.ID, &t.TermID, &t.Version, &t.TermKind, &t.ModuleID, &t.Status,
		&definition, &scope, &sourceCandidate, &t.CreateTime, &createBy, &t.ModifyTime, &modifyBy,
	); err != nil {
		return Term{}, err
	}
	if definition.Valid {
		t.Definition = definition.String
	}
	if scope.Valid {
		t.Scope = scope.String
	}
	if sourceCandidate.Valid {
		v := sourceCandidate.Int64
		t.SourceCandidateID = &v
	}
	if createBy.Valid {
		t.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		t.ModifyBy = modifyBy.String
	}
	return t, nil
}

func validateTerm(t Term) error {
	if strings.TrimSpace(t.TermID) == "" {
		return errors.New("term_id is required")
	}
	if !AllowedTermKinds[t.TermKind] {
		return fmt.Errorf("unsupported term_kind %q", t.TermKind)
	}
	if strings.TrimSpace(t.ModuleID) == "" {
		return errors.New("module_id is required")
	}
	if t.Status == "" {
		t.Status = "draft"
	}
	if !AllowedTermStatuses[t.Status] {
		return fmt.Errorf("unsupported term status %q", t.Status)
	}
	return nil
}

// CreateTerm inserts version 1 of a new term.
func (s TermStore) CreateTerm(ctx context.Context, t Term) (Term, error) {
	if s.DB == nil {
		return Term{}, errors.New("db is nil")
	}
	if err := validateTerm(t); err != nil {
		return Term{}, err
	}
	if t.Status == "" {
		t.Status = "draft"
	}
	const stmt = `
INSERT INTO kb.ontology_terms
	(term_id, version, term_kind, module_id, status, definition, scope,
	 source_candidate_id, create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING ` + termColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(t.TermID), t.TermKind, strings.TrimSpace(t.ModuleID),
		t.Status, nullableString(t.Definition), nullableString(t.Scope),
		nullableInt64(t.SourceCandidateID), nullableString(t.CreateBy), nullableString(t.ModifyBy),
	)
	return scanTerm(row.Scan)
}

// CreateTermVersion inserts a new version row for an existing term_id,
// preserving every prior version (the version column is the data version,
// per the 2026-07-31 storage decision). The new row inherits the fields of
// the supplied Term; callers that want to carry forward unmodified fields
// should load the latest version first.
func (s TermStore) CreateTermVersion(ctx context.Context, t Term) (Term, error) {
	if s.DB == nil {
		return Term{}, errors.New("db is nil")
	}
	if err := validateTerm(t); err != nil {
		return Term{}, err
	}
	const stmt = `
INSERT INTO kb.ontology_terms
	(term_id, version, term_kind, module_id, status, definition, scope,
	 source_candidate_id, create_by, modify_by)
VALUES ($1, (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.ontology_terms WHERE term_id = $1),
        $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING ` + termColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(t.TermID), t.TermKind, strings.TrimSpace(t.ModuleID),
		t.Status, nullableString(t.Definition), nullableString(t.Scope),
		nullableInt64(t.SourceCandidateID), nullableString(t.CreateBy), nullableString(t.ModifyBy),
	)
	return scanTerm(row.Scan)
}

// GetTermLatest returns the current (highest-version) row for a term_id.
func (s TermStore) GetTermLatest(ctx context.Context, termID string) (Term, error) {
	if s.DB == nil {
		return Term{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + termColumns + "\n" + termFrom + `
WHERE term_id = $1
ORDER BY version DESC
LIMIT 1`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(termID))
	return scanTerm(row.Scan)
}

// GetTerm returns the specific version of a term.
func (s TermStore) GetTerm(ctx context.Context, termID string, version int) (Term, error) {
	if s.DB == nil {
		return Term{}, errors.New("db is nil")
	}
	if version < 1 {
		return Term{}, errors.New("version must be >= 1")
	}
	const stmt = `SELECT ` + termColumns + "\n" + termFrom + `
WHERE term_id = $1 AND version = $2`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(termID), version)
	return scanTerm(row.Scan)
}

// ListTerms returns term rows filtered by optional module_id and status,
// newest version first per term (ORDER BY term_id, version DESC).
func (s TermStore) ListTerms(ctx context.Context, moduleID, status string) ([]Term, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + termColumns + "\n" + termFrom + `
WHERE ($1 = '' OR module_id = $1)
  AND ($2 = '' OR status = $2)
ORDER BY term_id, version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID), strings.TrimSpace(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	terms := make([]Term, 0)
	for rows.Next() {
		t, err := scanTerm(rows.Scan)
		if err != nil {
			return nil, err
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return terms, nil
}

// TransitionStatus moves a term version between allowed content-lifecycle
// states. Included_in_release is only reachable from the chunk B release
// path; this method enforces the same transition table so no call site can
// shortcut the lifecycle.
func (s TermStore) TransitionStatus(ctx context.Context, termID string, version int, to, by string) (Term, error) {
	if s.DB == nil {
		return Term{}, errors.New("db is nil")
	}
	cur, err := s.GetTerm(ctx, termID, version)
	if err != nil {
		return Term{}, err
	}
	allowed := termStatusTransitions[cur.Status]
	if !allowed[to] {
		return Term{}, fmt.Errorf("%w: %s -> %s", errIllegalTermTransition, cur.Status, to)
	}
	const stmt = `
UPDATE kb.ontology_terms
SET status = $3, modify_time = NOW(), modify_by = $4
WHERE term_id = $1 AND version = $2`
	if _, err := s.DB.ExecContext(ctx, stmt, strings.TrimSpace(termID), version, to, nullableString(by)); err != nil {
		return Term{}, err
	}
	return s.GetTerm(ctx, termID, version)
}
