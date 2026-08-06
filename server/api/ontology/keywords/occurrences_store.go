package keywords

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Occurrence is one observed name occurrence — the corrected K4 shape
// (§9.5, §10.4). Unlike the old mention row it carries the observed string
// itself (raw_name), the derived norm key, consumer-supplied provenance
// (artifact_type/artifact_id/field_path — opaque to the resolver), the
// resolution outcome, and a link to the decision-log row from the same
// call. Append-only: the observe path writes occurrences, never updates or
// deletes them.
type Occurrence struct {
	OccurrenceID     int64     `json:"occurrence_id"`
	ArtifactType     string    `json:"artifact_type"`
	ArtifactID       string    `json:"artifact_id"`
	FieldPath        string    `json:"field_path"`
	RawName          string    `json:"raw_name"`
	NormKey          string    `json:"norm_key"`
	Scope            string    `json:"scope"`
	ContextText      string    `json:"context_text"`
	ChunkRef         *string   `json:"chunk_ref"`
	ConceptID        *string   `json:"concept_id"`
	TermID           *string   `json:"term_id"`
	ResolutionStatus string    `json:"resolution_status"`
	DecisionLogID    *int64    `json:"decision_log_id"`
	CreateTime       time.Time `json:"create_time"`
}

// AllowedResolutionStatuses mirrors the schema CHECK constraint (§9.5: five
// statuses, all normal results).
var AllowedResolutionStatuses = map[string]bool{
	"term_resolved":    true,
	"lexical_resolved": true,
	"ambiguous":        true,
	"unresolved":       true,
	"disabled":         true,
}

// OccurrenceStore persists keyword occurrence rows (append-only).
type OccurrenceStore struct {
	DB DBX
}

const occurrenceColumns = `occurrence_id, artifact_type, artifact_id, field_path, raw_name, norm_key, scope, context_text, chunk_ref, concept_id, term_id, resolution_status, decision_log_id, create_time`

const occurrenceFrom = `FROM kb.keyword_occurrences`

func scanOccurrence(scan func(dest ...any) error) (Occurrence, error) {
	var (
		o           Occurrence
		chunkRef    sql.NullString
		conceptID   sql.NullString
		termID      sql.NullString
		decisionLog sql.NullInt64
	)
	if err := scan(
		&o.OccurrenceID, &o.ArtifactType, &o.ArtifactID, &o.FieldPath,
		&o.RawName, &o.NormKey, &o.Scope, &o.ContextText,
		&chunkRef, &conceptID, &termID, &o.ResolutionStatus, &decisionLog,
		&o.CreateTime,
	); err != nil {
		return Occurrence{}, err
	}
	if chunkRef.Valid {
		o.ChunkRef = &chunkRef.String
	}
	if conceptID.Valid {
		o.ConceptID = &conceptID.String
	}
	if termID.Valid {
		o.TermID = &termID.String
	}
	if decisionLog.Valid {
		o.DecisionLogID = &decisionLog.Int64
	}
	return o, nil
}

// InsertOccurrence inserts a single occurrence and returns its id.
func (s OccurrenceStore) InsertOccurrence(ctx context.Context, o Occurrence) (int64, error) {
	if o.RawName == "" {
		return 0, fmt.Errorf("raw_name is required")
	}
	if o.NormKey == "" {
		return 0, fmt.Errorf("norm_key is required")
	}
	if !AllowedResolutionStatuses[o.ResolutionStatus] {
		return 0, fmt.Errorf("invalid resolution_status: %s", o.ResolutionStatus)
	}
	if o.Scope == "" {
		o.Scope = "_"
	}

	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.keyword_occurrences
			(artifact_type, artifact_id, field_path, raw_name, norm_key, scope,
			 context_text, chunk_ref, concept_id, term_id, resolution_status, decision_log_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING occurrence_id`,
		o.ArtifactType, o.ArtifactID, o.FieldPath, o.RawName, o.NormKey, o.Scope,
		o.ContextText, nullablePtrString(o.ChunkRef), nullablePtrString(o.ConceptID),
		nullablePtrString(o.TermID), o.ResolutionStatus, nullablePtrInt64(o.DecisionLogID))
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("insert occurrence: %w", err)
	}
	return id, nil
}

// ListOccurrences returns occurrences for a norm key within a scope — the
// harvest view over the evidence queue (R1).
func (s OccurrenceStore) ListOccurrences(ctx context.Context, normKey, scope string, limit int) ([]Occurrence, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+occurrenceColumns+`
		`+occurrenceFrom+`
		WHERE norm_key = $1 AND scope = $2
		ORDER BY occurrence_id DESC
		LIMIT $3`, normKey, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Occurrence
	for rows.Next() {
		o, err := scanOccurrence(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func nullablePtrString(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func nullablePtrInt64(p *int64) any {
	if p == nil || *p == 0 {
		return nil
	}
	return *p
}
