package wpi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

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

// Pages live in kb.frontend_pages, the other kinds in kb.wpi_descriptors
// (devdoc 2026102601 §6, design D1).
const (
	latestSQL      = `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body FROM kb.wpi_descriptors WHERE kind = $1 AND status = 'active'`
	latestPagesSQL = `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body FROM kb.frontend_pages WHERE status = 'active'`
	latestOrder    = ` ORDER BY descriptor_id, version DESC`
)

// latest returns the latest active version of each requested descriptor, or
// of every descriptor of the kind when ids is nil.
func (s store) latest(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
	q, args := latestSQL, []any{string(kind)}
	if kind == KindPage {
		q, args = latestPagesSQL, nil
	}
	if ids != nil {
		args = append(args, pq.Array(ids))
		q += " AND descriptor_id = ANY($" + strconv.Itoa(len(args)) + ")"
	}
	rows, err := s.db.QueryContext(ctx, q+latestOrder, args...)
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
	if kind == KindPage {
		err := s.db.QueryRowContext(ctx, `INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by)
		SELECT $1::text, COALESCE(MAX(version), 0) + 1, $2::text, $3::jsonb, 'active', $4::text, $4::text
		FROM kb.frontend_pages WHERE descriptor_id = $1::text
		RETURNING version`, id, SchemaVersion, string(raw), actor).Scan(&version)
		return version, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
		SELECT $1::text, $2::text, COALESCE(MAX(version), 0) + 1, $3::text, $4::jsonb, 'active', $5::text, $5::text
		FROM kb.wpi_descriptors WHERE kind = $1::text AND descriptor_id = $2::text
		RETURNING version`, string(kind), id, schemaVersionOf(kind), string(raw), actor).Scan(&version)
	return version, err
}

// PageSummary describes the latest active version of a page to an author
// looking for one to reuse (devdoc 2026102601 §3.2.3).
type PageSummary struct {
	DescriptorID string    `json:"descriptor_id"`
	Version      int       `json:"version"`
	PageName     any       `json:"page_name"`
	Description  any       `json:"description"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// matches reports whether q (lower case) occurs in the page's ID, or in its
// name or description in any language.
func (p PageSummary) matches(q string) bool {
	return strings.Contains(strings.ToLower(p.DescriptorID), q) || textContains(p.PageName, q) || textContains(p.Description, q)
}

func textContains(t any, q string) bool {
	switch x := t.(type) {
	case string:
		return strings.Contains(strings.ToLower(x), q)
	case map[string]any:
		for _, s := range x {
			if textContains(s, q) {
				return true
			}
		}
	}
	return false
}

// pageSummaries returns every page's latest active version, ordered by ID.
// Pages are few, so callers filter them in Go (design D10).
func (s store) pageSummaries(ctx context.Context) ([]PageSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body->'page_name', body->'description', updated_at
		FROM kb.frontend_pages WHERE status = 'active' ORDER BY descriptor_id, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PageSummary
	for rows.Next() {
		var p PageSummary
		var name, desc []byte
		if err := rows.Scan(&p.DescriptorID, &p.Version, &name, &desc, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if name != nil {
			_ = json.Unmarshal(name, &p.PageName)
		}
		if desc != nil {
			_ = json.Unmarshal(desc, &p.Description)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// errClassExists reports a create for a class ID that already has a row.
var errClassExists = errors.New("class already exists")

// insertDraftClass stores body as version 1 of a new element class, as a draft
// (OpenSpec wpi-create-element-class, design D3). Any row of the ID, whatever
// its version or status, makes it a conflict; so does a concurrent create.
func (s store) insertDraftClass(ctx context.Context, id string, raw []byte, actor string) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
		SELECT 'element', $1::text, 1, $2::text, $3::jsonb, 'draft', $4::text, $4::text
		WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = $1::text)`,
		id, ElementSchemaVersion, string(raw), actor)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return errClassExists
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errClassExists
	}
	return nil
}

// ClassSummary describes the latest draft or active version of a class to the
// editor's palette and properties panel. Attributes are the row's instance
// attributes, each {help_tip, default_value} (format 0.3).
type ClassSummary struct {
	DescriptorID string         `json:"descriptor_id"`
	Version      int            `json:"version"`
	Status       string         `json:"status"`
	Description  any            `json:"description"`
	Attributes   map[string]any `json:"attributes"`
}

// elementClasses returns the latest draft or active version of every element
// class, ordered by ID; classes with only retired rows are left out.
func (s store) elementClasses(ctx context.Context) ([]ClassSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT ON (descriptor_id) descriptor_id, version, status, body
		FROM kb.wpi_descriptors WHERE kind = 'element' AND status IN ('draft', 'active') ORDER BY descriptor_id, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClassSummary{}
	for rows.Next() {
		var c ClassSummary
		var raw []byte
		if err := rows.Scan(&c.DescriptorID, &c.Version, &c.Status, &raw); err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("element class %s version %d: %w", c.DescriptorID, c.Version, err)
		}
		c.Description = body["description"]
		c.Attributes = map[string]any{}
		for k, v := range body {
			if !slices.Contains(classAttrs, k) {
				c.Attributes[k] = v
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClassRow is the latest draft or active version of an element class, whole,
// for the editor's edit form.
type ClassRow struct {
	Version int             `json:"version"`
	Status  string          `json:"status"`
	Class   json.RawMessage `json:"class"`
}

// elementClass returns the latest draft or active version of an element class;
// found is false when it has none.
func (s store) elementClass(ctx context.Context, id string) (c ClassRow, found bool, err error) {
	var raw []byte
	err = s.db.QueryRowContext(ctx, `SELECT version, status, body FROM kb.wpi_descriptors
		WHERE kind = 'element' AND descriptor_id = $1 AND status IN ('draft', 'active') ORDER BY version DESC LIMIT 1`, id).
		Scan(&c.Version, &c.Status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ClassRow{}, false, nil
	}
	c.Class = raw
	return c, err == nil, err
}

// insertClassVersion stores body as the next version of an element class with
// the given status, so an edited draft stays a draft.
func (s store) insertClassVersion(ctx context.Context, id, status string, raw []byte, actor string) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
		SELECT 'element', $1::text, COALESCE(MAX(version), 0) + 1, $2::text, $3::jsonb, $4::text, $5::text, $5::text
		FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = $1::text
		RETURNING version`, id, ElementSchemaVersion, string(raw), status, actor).Scan(&version)
	return version, err
}

// Kinds of stored rows that query and load-file name (design D8).
const (
	storedStatement = "statement"
	storedFile      = "file"
)

var storedSQL = map[string]struct{ exist, row string }{
	storedStatement: {
		`SELECT stmt_name FROM kb.stored_stmts WHERE stmt_name = ANY($1)`,
		`SELECT sql_stmt, access_controls FROM kb.stored_stmts WHERE stmt_name = $1`,
	},
	storedFile: {
		`SELECT file_name FROM kb.stored_files WHERE file_name = ANY($1)`,
		`SELECT file_content::text, access_controls FROM kb.stored_files WHERE file_name = $1`,
	},
}

// registered returns which of names exist as stored rows of kind.
func (s store) registered(ctx context.Context, kind string, names []string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, storedSQL[kind].exist, pq.Array(names))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// stored returns a stored statement's SQL or a stored file's JSON, and the
// roles that may use it; found is false when there is no such row.
func (s store) stored(ctx context.Context, kind, name string) (content string, access []string, found bool, err error) {
	var roles pq.StringArray
	err = s.db.QueryRowContext(ctx, storedSQL[kind].row, name).Scan(&content, &roles)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	return content, roles, err == nil, err
}
