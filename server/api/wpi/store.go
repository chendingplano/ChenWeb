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

// savePage serializes saves for a page and archives the row before updating it.
func (s store) savePage(ctx context.Context, id string, raw []byte, actor, recoveryID string, createOnly bool) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, id); err != nil {
		return 0, err
	}
	if createOnly {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.frontend_pages WHERE descriptor_id = $1)`, id).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			return 0, sql.ErrNoRows
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO archive.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at)
 SELECT descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at FROM kb.frontend_pages WHERE id = (SELECT id FROM kb.frontend_pages WHERE descriptor_id = $1 ORDER BY version DESC LIMIT 1)`, id); err != nil {
		return 0, err
	}
	var version int
	err = tx.QueryRowContext(ctx, `UPDATE kb.frontend_pages SET version = version + 1, schema_version = $2, body = $3::jsonb, status = 'active', updated_by = $4, updated_at = NOW()
 WHERE id = (SELECT id FROM kb.frontend_pages WHERE descriptor_id = $1 ORDER BY version DESC LIMIT 1) RETURNING version`, id, schemaVersionOf(KindPage), string(raw), actor).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES ($1, 1, $2, $3::jsonb, 'active', $4, $4) RETURNING version`, id, schemaVersionOf(KindPage), string(raw), actor).Scan(&version)
	}
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM archive.frontend_pages WHERE descriptor_id = $1 AND owner = $2 AND recovery`, id, actor); err != nil {
		return 0, err
	}
	if recoveryID != "" && recoveryID != id {
		if _, err = tx.ExecContext(ctx, `DELETE FROM archive.frontend_pages WHERE descriptor_id = $1 AND owner = $2 AND recovery AND version = 0`, recoveryID, actor); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return version, nil
}

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
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// pageSearch filters only after choosing the latest active version.
type pageSearch struct {
	Query, ID, Description                         string
	CreatedFrom, CreatedTo, UpdatedFrom, UpdatedTo *time.Time
	Limit, Offset                                  int
}

func (s store) searchPages(ctx context.Context, f pageSearch) ([]PageSummary, error) {
	query := `WITH latest AS (
 SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body, created_at, updated_at
 FROM kb.frontend_pages WHERE status = 'active' ORDER BY descriptor_id, version DESC
 ) SELECT descriptor_id, version, body->'page_name', body->'description', created_at, updated_at
 FROM latest WHERE
 ($1 = '' OR strpos(lower(descriptor_id), $1) > 0 OR strpos(lower(COALESCE(body->>'page_name', '')), $1) > 0 OR strpos(lower(COALESCE(body->>'description', '')), $1) > 0)
 AND ($2 = '' OR strpos(lower(descriptor_id), $2) > 0)
 AND ($3 = '' OR strpos(lower(COALESCE(body->>'description', '')), $3) > 0)
 AND ($4::timestamptz IS NULL OR created_at >= $4)
 AND ($5::timestamptz IS NULL OR created_at < $5)
 AND ($6::timestamptz IS NULL OR updated_at >= $6)
 AND ($7::timestamptz IS NULL OR updated_at < $7)
 ORDER BY descriptor_id LIMIT $8 OFFSET $9`
	rows, err := s.db.QueryContext(ctx, query, f.Query, f.ID, f.Description, f.CreatedFrom, f.CreatedTo, f.UpdatedFrom, f.UpdatedTo, f.Limit+1, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PageSummary{}
	for rows.Next() {
		var p PageSummary
		var name, desc []byte
		if err := rows.Scan(&p.DescriptorID, &p.Version, &name, &desc, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
	CodeModuleID string         `json:"code_module_id,omitempty"`
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
		c.CodeModuleID = str(body["code_module_id"])
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

// updateClass modifies the loaded element class row in place. Its version,
// status and creation audit fields stay unchanged; a missing row is an error.
func (s store) updateClass(ctx context.Context, id string, current ClassRow, raw []byte, actor string) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `UPDATE kb.wpi_descriptors
		SET schema_version = $2::text, body = $3::jsonb, updated_by = $4::text, updated_at = NOW()
		WHERE kind = 'element' AND descriptor_id = $1::text AND version = $5 AND status = $6::text
		RETURNING version`, id, ElementSchemaVersion, string(raw), actor, current.Version, current.Status).Scan(&version)
	return version, err
}

// Kinds of stored rows that query and load-file name (design D8), and the code
// modules class copies name: only active modules of the right type count
// (OpenSpec wpi-code-modules, design D9; wpi-pages-own-panes, design D6).
const (
	storedStatement             = "statement"
	storedFile                  = "file"
	storedModule                = "code_module"
	storedPageLayouterModule    = "page_layouter_module"
	storedElementLayouterModule = "element_layouter_module"
)

// moduleTypes is the kb.code_modules.module_type each module kind checks.
var moduleTypes = map[string]string{
	storedModule:                "element",
	storedPageLayouterModule:    "page-layouter",
	storedElementLayouterModule: "element-layouter",
}

const activeModuleSQL = `SELECT code_module_id FROM kb.code_modules WHERE module_type = '%s' AND status = 'active' AND code_module_id = ANY($1)`

var storedSQL = map[string]struct{ exist, row string }{
	storedModule:                {fmt.Sprintf(activeModuleSQL, "element"), ``},
	storedPageLayouterModule:    {fmt.Sprintf(activeModuleSQL, "page-layouter"), ``},
	storedElementLayouterModule: {fmt.Sprintf(activeModuleSQL, "element-layouter"), ``},
	storedStatement: {
		`SELECT stmt_name FROM kb.stored_stmts WHERE stmt_name = ANY($1)`,
		`SELECT sql_stmt, access_controls FROM kb.stored_stmts WHERE stmt_name = $1`,
	},
	storedFile: {
		`SELECT file_name FROM kb.stored_files WHERE file_name = ANY($1)`,
		`SELECT file_content::text, access_controls FROM kb.stored_files WHERE file_name = $1`,
	},
}

// CodeModule is a row of kb.code_modules (devdoc 2026102601 §3.5.2).
type CodeModule struct {
	ID          string `json:"code_module_id"`
	Name        string `json:"module_name"`
	Description string `json:"description"`
	Type        string `json:"module_type"`
	Doc         string `json:"module_doc"`
	Notes       string `json:"notes"`
	Status      string `json:"status"`
}

// listCodeModules returns every code module, by type and ID.
func (s store) listCodeModules(ctx context.Context) ([]CodeModule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code_module_id, module_name, description, module_type, module_doc, notes, status
		FROM kb.code_modules ORDER BY module_type, code_module_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CodeModule{}
	for rows.Next() {
		var m CodeModule
		if err := rows.Scan(&m.ID, &m.Name, &m.Description, &m.Type, &m.Doc, &m.Notes, &m.Status); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
