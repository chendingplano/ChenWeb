package terms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Mapping is one version of a governed mapping between a governed term and
// either another governed term (ToTermID) or an external IRI (ToIRI). Mapping
// strengths are the DR13-conservative set -- exact, close, broad, narrow,
// related -- so lexical similarity can never be recorded as equivalence.
// Exact mappings require explicit approval (ApprovalStatus), per spec §9.1.
type Mapping struct {
	ID                int64           `json:"id"`
	MappingID         string          `json:"mapping_id"`
	Version           int             `json:"version"`
	FromTermID        string          `json:"from_term_id"`
	ToTermID          string          `json:"to_term_id"`
	ToIRI             string          `json:"to_iri"`
	Relation          string          `json:"relation"`
	Evidence          json.RawMessage `json:"evidence"`
	ApprovalStatus    string          `json:"approval_status"`
	ModuleID          string          `json:"module_id"`
	Status            string          `json:"status"`
	SourceCandidateID *int64          `json:"source_candidate_id"`
	CreateTime        time.Time       `json:"create_time"`
	CreateBy          string          `json:"create_by"`
	ModifyTime        time.Time       `json:"modify_time"`
	ModifyBy          string          `json:"modify_by"`
}

var AllowedRelations = map[string]bool{
	"exact": true, "close": true, "broad": true, "narrow": true, "related": true,
}

// MappingStore persists versioned mapping rows.
type MappingStore struct {
	DB DBX
}

const mappingColumns = `
	id, mapping_id, version, from_term_id, to_term_id, to_iri, relation,
	evidence, approval_status, module_id, status, source_candidate_id,
	create_time, create_by, modify_time, modify_by`

const mappingFrom = "FROM kb.ontology_mappings"

func scanMapping(scan func(dest ...any) error) (Mapping, error) {
	var (
		m               Mapping
		toTerm          sql.NullString
		toIRI           sql.NullString
		evidence        json.RawMessage
		sourceCandidate sql.NullInt64
		createBy        sql.NullString
		modifyBy        sql.NullString
	)
	if err := scan(
		&m.ID, &m.MappingID, &m.Version, &m.FromTermID, &toTerm, &toIRI, &m.Relation,
		&evidence, &m.ApprovalStatus, &m.ModuleID, &m.Status, &sourceCandidate,
		&m.CreateTime, &createBy, &m.ModifyTime, &modifyBy,
	); err != nil {
		return Mapping{}, err
	}
	if toTerm.Valid {
		m.ToTermID = toTerm.String
	}
	if toIRI.Valid {
		m.ToIRI = toIRI.String
	}
	if evidence != nil {
		m.Evidence = evidence
	}
	if sourceCandidate.Valid {
		v := sourceCandidate.Int64
		m.SourceCandidateID = &v
	}
	if createBy.Valid {
		m.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		m.ModifyBy = modifyBy.String
	}
	return m, nil
}

func validateMapping(m Mapping) error {
	if strings.TrimSpace(m.MappingID) == "" {
		return errors.New("mapping_id is required")
	}
	if strings.TrimSpace(m.FromTermID) == "" {
		return errors.New("from_term_id is required")
	}
	if strings.TrimSpace(m.ToTermID) == "" && strings.TrimSpace(m.ToIRI) == "" {
		return errors.New("to_term_id or to_iri is required")
	}
	if !AllowedRelations[m.Relation] {
		return errors.New("unsupported relation; must be one of exact, close, broad, narrow, related")
	}
	if strings.TrimSpace(m.ModuleID) == "" {
		return errors.New("module_id is required")
	}
	if m.ApprovalStatus == "" {
		m.ApprovalStatus = "pending"
	}
	if m.Status == "" {
		m.Status = "draft"
	}
	return nil
}

// CreateMapping inserts version 1 of a mapping. An exact mapping is created
// with ApprovalStatus "pending" -- it cannot be released until explicitly
// approved.
func (s MappingStore) CreateMapping(ctx context.Context, m Mapping) (Mapping, error) {
	if s.DB == nil {
		return Mapping{}, errors.New("db is nil")
	}
	if err := validateMapping(m); err != nil {
		return Mapping{}, err
	}
	if m.Relation == "exact" && m.ApprovalStatus == "" {
		m.ApprovalStatus = "pending"
	}
	evidence := m.Evidence
	if len(evidence) == 0 {
		evidence = []byte("null")
	}
	const stmt = `
INSERT INTO kb.ontology_mappings
	(mapping_id, version, from_term_id, to_term_id, to_iri, relation,
	 evidence, approval_status, module_id, status, source_candidate_id,
	 create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12)
RETURNING ` + mappingColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(m.MappingID), strings.TrimSpace(m.FromTermID),
		nullableString(m.ToTermID), nullableString(m.ToIRI), m.Relation,
		string(evidence), m.ApprovalStatus, strings.TrimSpace(m.ModuleID), m.Status,
		nullableInt64(m.SourceCandidateID), nullableString(m.CreateBy), nullableString(m.ModifyBy),
	)
	return scanMapping(row.Scan)
}

// ApproveMapping marks a mapping's approval_status approved (required for
// exact mappings before release). It is a distinct, audited act from the
// content-status lifecycle.
func (s MappingStore) ApproveMapping(ctx context.Context, mappingID string, version int, by string) (Mapping, error) {
	if s.DB == nil {
		return Mapping{}, errors.New("db is nil")
	}
	const stmt = `
UPDATE kb.ontology_mappings
SET approval_status = 'approved', modify_time = NOW(), modify_by = $3
WHERE mapping_id = $1 AND version = $2`
	if _, err := s.DB.ExecContext(ctx, stmt, strings.TrimSpace(mappingID), version, nullableString(by)); err != nil {
		return Mapping{}, err
	}
	const getStmt = `SELECT ` + mappingColumns + "\n" + mappingFrom + `
WHERE mapping_id = $1 AND version = $2`
	row := s.DB.QueryRowContext(ctx, getStmt, strings.TrimSpace(mappingID), version)
	return scanMapping(row.Scan)
}

// ListMappings returns mapping rows for a module, newest version first.
func (s MappingStore) ListMappings(ctx context.Context, moduleID string) ([]Mapping, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + mappingColumns + "\n" + mappingFrom + `
WHERE ($1 = '' OR module_id = $1)
ORDER BY mapping_id, version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mappings := make([]Mapping, 0)
	for rows.Next() {
		m, err := scanMapping(rows.Scan)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mappings, nil
}
