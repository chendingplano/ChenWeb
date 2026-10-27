package wpi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
)

// decodeBody parses a descriptor body, keeping numbers as json.Number so
// integers survive unchanged.
func decodeBody(raw []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("descriptor must be a JSON object")
	}
	return m, nil
}

type store struct{ db *sql.DB }

// latest returns the latest active version of each requested descriptor, or
// of every descriptor of the kind when ids is nil.
func (s store) latest(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
	var rows *sql.Rows
	var err error
	if ids == nil {
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body FROM kb.wpi_descriptors WHERE kind = $1 AND status = 'active' ORDER BY descriptor_id, version DESC`, string(kind))
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body FROM kb.wpi_descriptors WHERE kind = $1 AND status = 'active' AND descriptor_id = ANY($2) ORDER BY descriptor_id, version DESC`, string(kind), pq.Array(ids))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Descriptor{}
	for rows.Next() {
		var d Descriptor
		var raw []byte
		if err := rows.Scan(&d.ID, &d.Version, &raw); err != nil {
			return nil, err
		}
		body, err := decodeBody(raw)
		if err != nil {
			return nil, fmt.Errorf("%s %q version %d: %w", kind, d.ID, d.Version, err)
		}
		d.Kind, d.Raw, d.Body = kind, raw, body
		out[d.ID] = d
	}
	return out, rows.Err()
}

// insertVersion stores body as the next active version of the descriptor.
func (s store) insertVersion(ctx context.Context, kind Kind, id string, raw []byte, actor string) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
		SELECT $1::text, $2::text, COALESCE(MAX(version), 0) + 1, $3::text, $4::jsonb, 'active', $5::text, $5::text
		FROM kb.wpi_descriptors WHERE kind = $1::text AND descriptor_id = $2::text
		RETURNING version`, string(kind), id, SchemaVersion, string(raw), actor).Scan(&version)
	return version, err
}
