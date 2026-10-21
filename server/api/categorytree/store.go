package categorytree

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ErrNotFound is returned when a tree, revision or classification does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a name is already taken.
var ErrConflict = errors.New("already exists")

// InputError marks an error caused by the request (bad operation, invalid tree, bad name).
type InputError struct{ Err error }

func (e *InputError) Error() string { return e.Err.Error() }
func (e *InputError) Unwrap() error { return e.Err }

func inputErr(format string, args ...any) error { return &InputError{Err: fmt.Errorf(format, args...)} }

// Store reads and writes the kb.category_* tables (migration 20261021000001).
type Store struct {
	DB    *sql.DB
	NewID func() string // node IDs; defaults to UUIDs
}

func (s *Store) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.NewString()
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// TreeInfo is a kb.category_trees row.
type TreeInfo struct {
	ID            string    `json:"id"`
	CanonicalName string    `json:"canonical_name"`
	Namespace     string    `json:"namespace"`
	PrefLabels    LangText  `json:"pref_labels"`
	Definitions   LangText  `json:"definitions"`
	Source        string    `json:"source"`
	CurrentSeq    int64     `json:"current_seq"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// InitialNode is one node of a new tree, with its children nested.
type InitialNode struct {
	NodeFields
	Children []InitialNode `json:"children,omitempty"`
}

// CreateTreeInput describes a new tree and its initial nodes.
type CreateTreeInput struct {
	CanonicalName string      `json:"canonical_name"`
	Namespace     string      `json:"namespace"`
	PrefLabels    LangText    `json:"pref_labels"`
	Definitions   LangText    `json:"definitions"`
	Source        string      `json:"source"`
	Root          InitialNode `json:"root"`
	Reason        string      `json:"reason"`
}

// ChangeSetResult summarizes an applied change set.
type ChangeSetResult struct {
	TreeID        string `json:"tree_id"`
	ChangeSetID   string `json:"change_set_id"`
	FromSeq       int64  `json:"from_seq"`
	ToSeq         int64  `json:"to_seq"`
	Entries       int    `json:"entries"`
	StaleBindings int64  `json:"stale_bindings"`
}

func jsonb(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func scanJSON(raw []byte, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

const treeColumns = `id, canonical_name, namespace, pref_labels, definitions, source, current_seq, created_by, created_at, updated_at`

func scanTree(row interface{ Scan(...any) error }) (*TreeInfo, error) {
	var t TreeInfo
	var labels, defs []byte
	if err := row.Scan(&t.ID, &t.CanonicalName, &t.Namespace, &labels, &defs, &t.Source, &t.CurrentSeq,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := scanJSON(labels, &t.PrefLabels); err != nil {
		return nil, err
	}
	return &t, scanJSON(defs, &t.Definitions)
}

// ListTrees returns every tree by name.
func (s *Store) ListTrees(ctx context.Context) ([]TreeInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+treeColumns+` FROM kb.category_trees ORDER BY canonical_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TreeInfo
	for rows.Next() {
		t, err := scanTree(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTree finds a tree by canonical name or ID.
func (s *Store) GetTree(ctx context.Context, ref string) (*TreeInfo, error) {
	return getTree(ctx, s.DB, ref, false)
}

func getTree(ctx context.Context, q querier, ref string, lock bool) (*TreeInfo, error) {
	stmt := `SELECT ` + treeColumns + ` FROM kb.category_trees WHERE canonical_name = $1 OR id = $1`
	if lock {
		stmt += ` FOR UPDATE`
	}
	return scanTree(q.QueryRowContext(ctx, stmt, strings.TrimSpace(ref)))
}

// CreateTree stores a tree and records its initial nodes as the first change set.
func (s *Store) CreateTree(ctx context.Context, in CreateTreeInput, author string) (*TreeInfo, *ChangeSetResult, error) {
	name := strings.TrimSpace(in.CanonicalName)
	if !canonicalNamePattern.MatchString(name) {
		return nil, nil, inputErr("tree canonical name %q must be a lower-case ASCII slug", name)
	}
	if strings.TrimSpace(in.Namespace) == "" {
		return nil, nil, inputErr("namespace is required")
	}
	for _, lang := range requiredLangs {
		if strings.TrimSpace(in.PrefLabels[lang]) == "" || strings.TrimSpace(in.Definitions[lang]) == "" {
			return nil, nil, inputErr("tree label and definition are required in %s", lang)
		}
	}
	var ops []Op
	var walk func(n InitialNode, parent string)
	walk = func(n InitialNode, parent string) {
		ops = append(ops, Op{Op: OpAdd, Parent: parent, Fields: n.NodeFields, Reason: in.Reason})
		var self string
		if n.CanonicalName != nil {
			self = *n.CanonicalName
		}
		for _, c := range n.Children {
			walk(c, self)
		}
	}
	walk(in.Root, "")

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO kb.category_trees
		(id, canonical_name, namespace, pref_labels, definitions, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, name, strings.TrimSpace(in.Namespace), jsonb(in.PrefLabels), jsonb(in.Definitions), strings.TrimSpace(in.Source), author); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, nil, fmt.Errorf("tree %s: %w", name, ErrConflict)
		}
		return nil, nil, err
	}
	info, err := getTree(ctx, tx, id, true)
	if err != nil {
		return nil, nil, err
	}
	res, err := s.applyInTx(ctx, tx, info, NewTree(id, info.Namespace), ops, author)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	info.CurrentSeq = res.ToSeq
	return info, res, nil
}

// LoadCurrent returns the current (non-retired) tree.
func (s *Store) LoadCurrent(ctx context.Context, info *TreeInfo) (*Tree, error) {
	return loadCurrent(ctx, s.DB, info)
}

func loadCurrent(ctx context.Context, q querier, info *TreeInfo) (*Tree, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, COALESCE(parent_id, ''), canonical_name, is_others,
		pref_labels, alt_labels, hidden_labels, definitions, scope_notes, source, matches
		FROM kb.category_nodes WHERE tree_id = $1 AND retired_at IS NULL`, info.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := NewTree(info.ID, info.Namespace)
	for rows.Next() {
		n := &Node{TreeID: info.ID}
		var pref, alt, hidden, defs, scope, matches []byte
		if err := rows.Scan(&n.ID, &n.ParentID, &n.CanonicalName, &n.IsOthers, &pref, &alt, &hidden, &defs, &scope, &n.Source, &matches); err != nil {
			return nil, err
		}
		for _, f := range []struct {
			raw []byte
			dst any
		}{{pref, &n.PrefLabels}, {alt, &n.AltLabels}, {hidden, &n.HiddenLabels}, {defs, &n.Definitions}, {scope, &n.ScopeNotes}, {matches, &n.Matches}} {
			if err := scanJSON(f.raw, f.dst); err != nil {
				return nil, fmt.Errorf("node %s: %w", n.ID, err)
			}
		}
		t.put(n)
	}
	return t, rows.Err()
}

// ApplyChangeSet applies ops to the current tree in one transaction: log entries, node rows,
// stale marks and the new sequence number are written together, after validation.
func (s *Store) ApplyChangeSet(ctx context.Context, treeRef string, ops []Op, author string) (*ChangeSetResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	info, err := getTree(ctx, tx, treeRef, true)
	if err != nil {
		return nil, err
	}
	cur, err := loadCurrent(ctx, tx, info)
	if err != nil {
		return nil, err
	}
	res, err := s.applyInTx(ctx, tx, info, cur, ops, author)
	if err != nil {
		return nil, err
	}
	return res, tx.Commit()
}

func (s *Store) applyInTx(ctx context.Context, tx *sql.Tx, info *TreeInfo, cur *Tree, ops []Op, author string) (*ChangeSetResult, error) {
	applied, err := Apply(cur, ops, author, s.newID)
	if err != nil {
		return nil, &InputError{Err: err}
	}
	res := &ChangeSetResult{TreeID: info.ID, ChangeSetID: uuid.NewString(), FromSeq: info.CurrentSeq + 1, Entries: len(applied.Entries)}
	seq := info.CurrentSeq
	changeIDs := make([]int64, len(applied.Entries))
	for i, e := range applied.Entries {
		seq++
		var before, after any
		if e.Before != nil {
			before = jsonb(e.Before)
		}
		if e.After != nil {
			after = jsonb(e.After)
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO kb.category_tree_changes
			(tree_id, change_set_id, seq, operation, node_id, before_value, after_value, author, reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			info.ID, res.ChangeSetID, seq, e.Op, e.NodeID, before, after, e.Author, e.Reason).Scan(&changeIDs[i]); err != nil {
			return nil, fmt.Errorf("log entry %d: %w", i+1, err)
		}
		if err := writeNode(ctx, tx, e); err != nil {
			return nil, fmt.Errorf("write node %s: %w", e.NodeID, err)
		}
	}
	for i, ids := range applied.Stale {
		r, err := tx.ExecContext(ctx, `UPDATE kb.category_bindings SET stale = TRUE, stale_change_id = $1
			WHERE node_id = ANY($2) AND NOT stale`, changeIDs[i], pq.Array(ids))
		if err != nil {
			return nil, fmt.Errorf("mark stale bindings: %w", err)
		}
		n, _ := r.RowsAffected()
		res.StaleBindings += n
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.category_trees SET current_seq = $2, updated_at = NOW() WHERE id = $1`, info.ID, seq); err != nil {
		return nil, err
	}
	res.ToSeq = seq
	return res, nil
}

func writeNode(ctx context.Context, tx *sql.Tx, e Entry) error {
	if e.Op == OpDelete {
		_, err := tx.ExecContext(ctx, `UPDATE kb.category_nodes SET retired_at = NOW(), updated_at = NOW() WHERE id = $1`, e.NodeID)
		return err
	}
	n := e.After
	var parent any
	if n.ParentID != "" {
		parent = n.ParentID
	}
	args := []any{n.ID, n.TreeID, parent, n.CanonicalName, n.IsOthers, jsonb(n.PrefLabels), jsonb(n.AltLabels),
		jsonb(n.HiddenLabels), jsonb(n.Definitions), jsonb(n.ScopeNotes), n.Source, jsonb(nonNilMatches(n.Matches))}
	if e.Op == OpAdd {
		_, err := tx.ExecContext(ctx, `INSERT INTO kb.category_nodes
			(id, tree_id, parent_id, canonical_name, is_others, pref_labels, alt_labels, hidden_labels, definitions, scope_notes, source, matches)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, args...)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE kb.category_nodes SET tree_id = $2, parent_id = $3, canonical_name = $4,
		is_others = $5, pref_labels = $6, alt_labels = $7, hidden_labels = $8, definitions = $9, scope_notes = $10,
		source = $11, matches = $12, updated_at = NOW() WHERE id = $1`, args...)
	return err
}

func nonNilMatches(m []Match) []Match {
	if m == nil {
		return []Match{}
	}
	return m
}

// ChangeRow is one stored log entry.
type ChangeRow struct {
	ID          int64     `json:"id"`
	ChangeSetID string    `json:"change_set_id"`
	Seq         int64     `json:"seq"`
	Entry       Entry     `json:"entry"`
	CreatedAt   time.Time `json:"created_at"`
}

// Changes returns log entries with afterSeq < seq <= uptoSeq (uptoSeq <= 0 = all), in order.
func (s *Store) Changes(ctx context.Context, treeID string, afterSeq, uptoSeq int64, limit int) ([]ChangeRow, error) {
	stmt := `SELECT id, change_set_id, seq, operation, node_id, before_value, after_value, author, reason, created_at
		FROM kb.category_tree_changes WHERE tree_id = $1 AND seq > $2 AND ($3::bigint <= 0 OR seq <= $3::bigint) ORDER BY seq`
	args := []any{treeID, afterSeq, uptoSeq}
	if limit > 0 {
		stmt += ` LIMIT $4`
		args = append(args, limit)
	}
	rows, err := s.DB.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangeRow
	for rows.Next() {
		var c ChangeRow
		var before, after []byte
		if err := rows.Scan(&c.ID, &c.ChangeSetID, &c.Seq, &c.Entry.Op, &c.Entry.NodeID, &before, &after,
			&c.Entry.Author, &c.Entry.Reason, &c.CreatedAt); err != nil {
			return nil, err
		}
		if len(before) > 0 {
			c.Entry.Before = &Node{}
			if err := json.Unmarshal(before, c.Entry.Before); err != nil {
				return nil, err
			}
		}
		if len(after) > 0 {
			c.Entry.After = &Node{}
			if err := json.Unmarshal(after, c.Entry.After); err != nil {
				return nil, err
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TreeAt replays the log up to seq (devdoc §4.2).
func (s *Store) TreeAt(ctx context.Context, info *TreeInfo, seq int64) (*Tree, error) {
	if seq <= 0 || seq > info.CurrentSeq {
		return nil, inputErr("seq %d is outside 1..%d", seq, info.CurrentSeq)
	}
	rows, err := s.Changes(ctx, info.ID, 0, seq, 0)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(rows))
	for i, r := range rows {
		entries[i] = r.Entry
	}
	return Replay(info.ID, info.Namespace, entries)
}

// Revision is a named point in a tree's log.
type Revision struct {
	Name      string    `json:"name"`
	Seq       int64     `json:"seq"`
	Note      string    `json:"note"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRevision names the tree's current sequence number.
func (s *Store) CreateRevision(ctx context.Context, info *TreeInfo, name, note, author string) (*Revision, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, inputErr("revision name is required")
	}
	r := Revision{Name: name, Note: strings.TrimSpace(note), CreatedBy: author}
	err := s.DB.QueryRowContext(ctx, `INSERT INTO kb.category_tree_revisions (tree_id, name, seq, note, created_by)
		SELECT id, $2, current_seq, $3, $4 FROM kb.category_trees WHERE id = $1
		RETURNING seq, created_at`, info.ID, r.Name, r.Note, author).Scan(&r.Seq, &r.CreatedAt)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return nil, fmt.Errorf("revision %s: %w", name, ErrConflict)
	}
	return &r, err
}

// Revisions lists a tree's revisions, oldest first.
func (s *Store) Revisions(ctx context.Context, treeID string) ([]Revision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, seq, note, created_by, created_at
		FROM kb.category_tree_revisions WHERE tree_id = $1 ORDER BY seq, id`, treeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Revision
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.Name, &r.Seq, &r.Note, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevisionSeq returns the sequence number a revision points at.
func (s *Store) RevisionSeq(ctx context.Context, treeID, name string) (int64, error) {
	var seq int64
	err := s.DB.QueryRowContext(ctx, `SELECT seq FROM kb.category_tree_revisions WHERE tree_id = $1 AND name = $2`,
		treeID, strings.TrimSpace(name)).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("revision %s: %w", name, ErrNotFound)
	}
	return seq, err
}

// TreeType binds an artifact type to a tree.
type TreeType struct {
	ArtifactType string    `json:"artifact_type"`
	TreeID       string    `json:"tree_id"`
	TreeName     string    `json:"tree_name"`
	UpdatedBy    string    `json:"updated_by"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SetTreeType binds artifactType to the tree.
func (s *Store) SetTreeType(ctx context.Context, artifactType string, info *TreeInfo, author string) error {
	artifactType = strings.TrimSpace(artifactType)
	if artifactType == "" {
		return inputErr("artifact type is required")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kb.category_tree_types (artifact_type, tree_id, updated_by)
		VALUES ($1, $2, $3) ON CONFLICT (artifact_type) DO UPDATE
		SET tree_id = EXCLUDED.tree_id, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		artifactType, info.ID, author)
	return err
}

// TreeTypes lists the artifact-type bindings.
func (s *Store) TreeTypes(ctx context.Context) ([]TreeType, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tt.artifact_type, tt.tree_id, t.canonical_name, tt.updated_by, tt.updated_at
		FROM kb.category_tree_types tt JOIN kb.category_trees t ON t.id = tt.tree_id ORDER BY tt.artifact_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TreeType
	for rows.Next() {
		var tt TreeType
		if err := rows.Scan(&tt.ArtifactType, &tt.TreeID, &tt.TreeName, &tt.UpdatedBy, &tt.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, tt)
	}
	return out, rows.Err()
}

// TreeForType returns the tree an artifact type is classified against.
func (s *Store) TreeForType(ctx context.Context, artifactType string) (*TreeInfo, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT tree_id FROM kb.category_tree_types WHERE artifact_type = $1`, artifactType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("artifact type %s is not bound to a category tree: %w", artifactType, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return s.GetTree(ctx, id)
}
