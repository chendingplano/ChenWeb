package assertions

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AllowedRelationKinds mirrors the kb.assertion_relations CHECK.
var AllowedRelationKinds = map[string]bool{
	"conflicts_with": true, "supersedes": true, "superseded_by": true,
}

// Relation records a conflict or supersession link between two assertions
// (spec §8.3).
type Relation struct {
	ID                 int64     `json:"id"`
	AssertionID        int64     `json:"assertion_id"`
	RelatedAssertionID int64     `json:"related_assertion_id"`
	RelationKind       string    `json:"relation_kind"`
	Rationale          string    `json:"rationale,omitempty"`
	CreateTime         time.Time `json:"create_time"`
	CreateBy           string    `json:"create_by,omitempty"`
}

// RelationStore persists assertion relations.
type RelationStore struct {
	DB DBX
}

const relationColumns = `id, assertion_id, related_assertion_id, relation_kind, rationale, create_time, create_by`
const relationFrom = "FROM kb.assertion_relations"

func scanRelation(scan func(dest ...any) error) (Relation, error) {
	var (
		r         Relation
		rationale sql.NullString
		createBy  sql.NullString
	)
	if err := scan(&r.ID, &r.AssertionID, &r.RelatedAssertionID, &r.RelationKind, &rationale, &r.CreateTime, &createBy); err != nil {
		return Relation{}, err
	}
	if rationale.Valid {
		r.Rationale = rationale.String
	}
	if createBy.Valid {
		r.CreateBy = createBy.String
	}
	return r, nil
}

// AddRelation records a conflict or supersession link between two
// assertions. Conflicting assertions remain independently queryable; this
// only records the relationship (spec §16.2 item 5).
func (s RelationStore) AddRelation(ctx context.Context, r Relation) (Relation, error) {
	if s.DB == nil {
		return Relation{}, errors.New("db is nil")
	}
	if !AllowedRelationKinds[r.RelationKind] {
		return Relation{}, errors.New("unsupported relation_kind")
	}
	if r.AssertionID == r.RelatedAssertionID {
		return Relation{}, errors.New("assertion_id and related_assertion_id must differ")
	}
	const stmt = `
INSERT INTO kb.assertion_relations (assertion_id, related_assertion_id, relation_kind, rationale, create_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (assertion_id, related_assertion_id, relation_kind) DO UPDATE SET rationale = EXCLUDED.rationale
RETURNING ` + relationColumns
	row := s.DB.QueryRowContext(ctx, stmt, r.AssertionID, r.RelatedAssertionID, r.RelationKind, nullableString(r.Rationale), nullableString(r.CreateBy))
	return scanRelation(row.Scan)
}

// ListForAssertion returns every relation touching the given assertion, in
// either direction.
func (s RelationStore) ListForAssertion(ctx context.Context, assertionID int64) ([]Relation, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + relationColumns + "\n" + relationFrom + `
WHERE assertion_id = $1 OR related_assertion_id = $1
ORDER BY id`
	rows, err := s.DB.QueryContext(ctx, stmt, assertionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Relation, 0)
	for rows.Next() {
		r, err := scanRelation(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
