package terms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TermLabel is one version of a language-tagged label on a governed term.
// label_role follows the DR13 SKOS label discipline:
// prefLabel / altLabel / hiddenLabel.
type TermLabel struct {
	ID                int64     `json:"id"`
	TermID            string    `json:"term_id"`
	Version           int       `json:"version"`
	Label             string    `json:"label"`
	Lang              string    `json:"lang"`
	LabelRole         string    `json:"label_role"`
	Status            string    `json:"status"`
	SourceCandidateID *int64    `json:"source_candidate_id"`
	CreateTime        time.Time `json:"create_time"`
	CreateBy          string    `json:"create_by"`
	ModifyTime        time.Time `json:"modify_time"`
	ModifyBy          string    `json:"modify_by"`
}

var AllowedLabelRoles = map[string]bool{
	"prefLabel": true,
	"altLabel":  true,
	"hiddenLabel": true,
}

// LabelStore persists versioned term-label rows.
type LabelStore struct {
	DB *sql.DB
}

const labelColumns = `
	id, term_id, version, label, lang, label_role, status, source_candidate_id,
	create_time, create_by, modify_time, modify_by`

const labelFrom = "FROM kb.ontology_term_labels"

func scanTermLabel(scan func(dest ...any) error) (TermLabel, error) {
	var (
		l               TermLabel
		sourceCandidate sql.NullInt64
		createBy        sql.NullString
		modifyBy        sql.NullString
	)
	if err := scan(
		&l.ID, &l.TermID, &l.Version, &l.Label, &l.Lang, &l.LabelRole, &l.Status,
		&sourceCandidate, &l.CreateTime, &createBy, &l.ModifyTime, &modifyBy,
	); err != nil {
		return TermLabel{}, err
	}
	if sourceCandidate.Valid {
		v := sourceCandidate.Int64
		l.SourceCandidateID = &v
	}
	if createBy.Valid {
		l.CreateBy = createBy.String
	}
	if modifyBy.Valid {
		l.ModifyBy = modifyBy.String
	}
	return l, nil
}

func validateTermLabel(l TermLabel) error {
	if strings.TrimSpace(l.TermID) == "" {
		return errors.New("term_id is required")
	}
	if strings.TrimSpace(l.Label) == "" {
		return errors.New("label is required")
	}
	if l.Lang == "" {
		l.Lang = "en"
	}
	if !AllowedLabelRoles[l.LabelRole] {
		return fmt.Errorf("unsupported label_role %q", l.LabelRole)
	}
	if l.Status == "" {
		l.Status = "draft"
	}
	return nil
}

// CreateLabel inserts version 1 of a label on a term. A preferred label for a
// term+language may only be created when no prefLabel for that term+lang
// exists yet -- the "one released prefLabel per configured language" rule.
func (s LabelStore) CreateLabel(ctx context.Context, l TermLabel) (TermLabel, error) {
	if s.DB == nil {
		return TermLabel{}, errors.New("db is nil")
	}
	if err := validateTermLabel(l); err != nil {
		return TermLabel{}, err
	}
	if l.Lang == "" {
		l.Lang = "en"
	}
	if l.LabelRole == "" {
		l.LabelRole = "prefLabel"
	}
	if l.Status == "" {
		l.Status = "draft"
	}
	if l.LabelRole == "prefLabel" {
		exists, err := s.prefLabelExists(ctx, strings.TrimSpace(l.TermID), l.Lang)
		if err != nil {
			return TermLabel{}, err
		}
		if exists {
			return TermLabel{}, errors.New("a prefLabel already exists for this term and language")
		}
	}
	const stmt = `
INSERT INTO kb.ontology_term_labels
	(term_id, version, label, lang, label_role, status, source_candidate_id,
	 create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING ` + labelColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(l.TermID), strings.TrimSpace(l.Label), l.Lang, l.LabelRole,
		l.Status, nullableInt64(l.SourceCandidateID), nullableString(l.CreateBy), nullableString(l.ModifyBy),
	)
	return scanTermLabel(row.Scan)
}

func (s LabelStore) prefLabelExists(ctx context.Context, termID, lang string) (bool, error) {
	const stmt = `
SELECT EXISTS (
	SELECT 1 FROM kb.ontology_term_labels
	WHERE term_id = $1 AND lang = $2 AND label_role = 'prefLabel'
		AND status NOT IN ('rejected', 'superseded')
)`
	var exists bool
	if err := s.DB.QueryRowContext(ctx, stmt, termID, lang).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// CreateLabelVersion inserts a new version row for an existing term label.
func (s LabelStore) CreateLabelVersion(ctx context.Context, l TermLabel) (TermLabel, error) {
	if s.DB == nil {
		return TermLabel{}, errors.New("db is nil")
	}
	if err := validateTermLabel(l); err != nil {
		return TermLabel{}, err
	}
	const stmt = `
INSERT INTO kb.ontology_term_labels
	(term_id, version, label, lang, label_role, status, source_candidate_id,
	 create_by, modify_by)
VALUES ($1, (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.ontology_term_labels WHERE term_id = $1),
        $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING ` + labelColumns
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(l.TermID), strings.TrimSpace(l.Label), l.Lang, l.LabelRole,
		l.Status, nullableInt64(l.SourceCandidateID), nullableString(l.CreateBy), nullableString(l.ModifyBy),
	)
	return scanTermLabel(row.Scan)
}

// ListLabels returns all label versions for a term, newest first.
func (s LabelStore) ListLabels(ctx context.Context, termID string) ([]TermLabel, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `SELECT ` + labelColumns + "\n" + labelFrom + `
WHERE term_id = $1
ORDER BY version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(termID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := make([]TermLabel, 0)
	for rows.Next() {
		l, err := scanTermLabel(rows.Scan)
		if err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return labels, nil
}
