package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ObjectType is a broad classification shared by all artifact object producers.
// Proposed types are usable immediately but do not increase deterministic
// type-based matching confidence.
type ObjectType struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	RawValue        string     `json:"raw_value"`
	SourceRecordID  int64      `json:"source_record_id"`
	SourceRunID     int64      `json:"source_run_id"`
	SourceProcessor string     `json:"source_processor"`
	SourceModel     string     `json:"source_model"`
	SourcePrompt    string     `json:"source_prompt"`
	SourceSpans     []string   `json:"source_spans"`
	CreatedAt       time.Time  `json:"created_at"`
	ApprovedAt      *time.Time `json:"approved_at"`
	ApprovedBy      string     `json:"approved_by"`
}

type ObjectTypeSource struct {
	RecordID, RunID          int64
	Processor, Model, Prompt string
	Spans                    []string
}

type ObjectTypeRegistry interface {
	Resolve(context.Context, string, ObjectTypeSource) (ObjectType, error)
}

type ObjectTypeSQLStore struct{ DB *sql.DB }

const objectTypeColumns = `id, name, status, raw_value, source_record_id, source_run_id,
 source_processor, source_model, source_prompt, source_spans, created_at, approved_at, approved_by`

func scanObjectType(row interface{ Scan(...any) error }) (ObjectType, error) {
	var out ObjectType
	var spans []byte
	err := row.Scan(&out.ID, &out.Name, &out.Status, &out.RawValue, &out.SourceRecordID, &out.SourceRunID,
		&out.SourceProcessor, &out.SourceModel, &out.SourcePrompt, &spans, &out.CreatedAt, &out.ApprovedAt, &out.ApprovedBy)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(spans, &out.SourceSpans); err != nil {
		return out, err
	}
	if out.SourceSpans == nil {
		out.SourceSpans = []string{}
	}
	return out, nil
}

// Resolve keeps the first observation and never overwrites approval on conflict.
// The no-op conflict update also returns a row when another worker inserts the
// same type concurrently, unlike an INSERT DO NOTHING followed by a snapshot read.
func (s ObjectTypeSQLStore) Resolve(ctx context.Context, raw string, source ObjectTypeSource) (ObjectType, error) {
	name := normalizeObjectToken(raw)
	if name == "" {
		return ObjectType{}, fmt.Errorf("object type must not be blank")
	}
	if s.DB == nil {
		return ObjectType{}, fmt.Errorf("object type registry db is nil")
	}
	spans, _ := json.Marshal(orEmptySlice(source.Spans))
	return scanObjectType(s.DB.QueryRowContext(ctx, `
INSERT INTO kb.object_types (name, status, raw_value, source_record_id, source_run_id,
 source_processor, source_model, source_prompt, source_spans)
VALUES (COALESCE((SELECT name FROM kb.object_types WHERE legacy_code = $1), $1),
 'proposed', $2, $3, $4, $5, $6, $7, $8::jsonb)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
RETURNING `+objectTypeColumns, name, strings.TrimSpace(raw), source.RecordID, source.RunID, source.Processor, source.Model, source.Prompt, string(spans)))
}

func (s ObjectTypeSQLStore) List(ctx context.Context) ([]ObjectType, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("object type registry db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+objectTypeColumns+` FROM kb.object_types ORDER BY status DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ObjectType{}
	for rows.Next() {
		item, err := scanObjectType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s ObjectTypeSQLStore) Approve(ctx context.Context, id int64, actor string) (ObjectType, error) {
	if s.DB == nil {
		return ObjectType{}, fmt.Errorf("object type registry db is nil")
	}
	if strings.TrimSpace(actor) == "" {
		return ObjectType{}, fmt.Errorf("approver is required")
	}
	return scanObjectType(s.DB.QueryRowContext(ctx, `UPDATE kb.object_types
SET status = 'approved', approved_at = COALESCE(approved_at, NOW()),
 approved_by = CASE WHEN status = 'approved' THEN approved_by ELSE $2 END
WHERE id = $1 RETURNING `+objectTypeColumns, id, actor))
}

func (s ObjectTypeSQLStore) approvedNames(ctx context.Context, names []string) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name FROM kb.object_types WHERE status = 'approved' AND name = ANY($1)`, pq.Array(uniqueStrings(names)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	approved := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		approved[name] = true
	}
	return approved, rows.Err()
}

func approvedObjectTypesCompatible(a, b string, approved map[string]bool) bool {
	a, b = normalizeObjectToken(a), normalizeObjectToken(b)
	if a != "" && !approved[a] || b != "" && !approved[b] {
		return false
	}
	return objectTypesCompatible(a, b)
}
