package categorytree

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Classification is a kb.category_classifications row.
type Classification struct {
	ID           int64             `json:"id"`
	ArtifactType string            `json:"artifact_type"`
	ArtifactID   string            `json:"artifact_id"`
	RecordID     int64             `json:"record_id"`
	TreeID       string            `json:"tree_id"`
	TreeSeq      int64             `json:"tree_seq"`
	Status       string            `json:"status"`
	Reason       string            `json:"reason"`
	Proposals    []string          `json:"proposals"`
	Evidence     map[string]any    `json:"evidence"`
	ModelName    string            `json:"model_name"`
	ModelProfile string            `json:"model_profile"`
	PromptRefs   map[string]string `json:"prompt_refs"`
	ReviewedBy   string            `json:"reviewed_by"`
	ReviewNote   string            `json:"review_note"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

const classificationColumns = `id, artifact_type, artifact_id, record_id, tree_id, tree_seq, status, reason,
	proposals, evidence, model_name, model_profile, prompt_refs, reviewed_by, review_note, created_at, updated_at`

func scanClassification(row interface{ Scan(...any) error }) (*Classification, error) {
	var c Classification
	var proposals, evidence, prompts []byte
	if err := row.Scan(&c.ID, &c.ArtifactType, &c.ArtifactID, &c.RecordID, &c.TreeID, &c.TreeSeq, &c.Status, &c.Reason,
		&proposals, &evidence, &c.ModelName, &c.ModelProfile, &prompts, &c.ReviewedBy, &c.ReviewNote, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	for _, f := range []struct {
		raw []byte
		dst any
	}{{proposals, &c.Proposals}, {evidence, &c.Evidence}, {prompts, &c.PromptRefs}} {
		if err := scanJSON(f.raw, f.dst); err != nil {
			return nil, err
		}
	}
	return &c, nil
}

// StartClassification records a pending attempt.
func (s *Store) StartClassification(ctx context.Context, a Artifact, info *TreeInfo, modelName, profile string, prompts map[string]string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO kb.category_classifications
		(artifact_type, artifact_id, record_id, tree_id, tree_seq, status, proposals, model_name, model_profile, prompt_refs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		a.Type, a.ID, a.RecordID, info.ID, info.CurrentSeq, StatusPending, jsonb(nonNilStrings(a.Proposals)),
		modelName, profile, jsonb(prompts)).Scan(&id)
	return id, err
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// FinishClassification stores a walk's outcome. A published walk replaces the artifact's
// bindings in the same transaction; any other outcome leaves them untouched.
func (s *Store) FinishClassification(ctx context.Context, id int64, a Artifact, treeID string, seq int64, res WalkResult) error {
	evidence := map[string]any{"walk": res}
	if res.Status == StatusPublished {
		return s.publish(ctx, id, a, treeID, seq, res.Leaves, evidence, "", "")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE kb.category_classifications SET status = $2, reason = $3, evidence = $4, updated_at = NOW()
		WHERE id = $1`, id, res.Status, res.Reason, jsonb(evidence))
	return err
}

// FailClassification ends an attempt that could not run.
func (s *Store) FailClassification(ctx context.Context, id int64, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE kb.category_classifications SET status = $2, reason = $3, updated_at = NOW()
		WHERE id = $1`, id, StatusFailed, reason)
	return err
}

// publish replaces the artifact's bindings with leaves and marks the classification published.
// When the tree changed after the walk read it at seq, the new bindings are published stale,
// pointing at the first later change, so they are reclassified like any other stale binding.
func (s *Store) publish(ctx context.Context, id int64, a Artifact, treeID string, seq int64, leaves []string, evidence map[string]any, reviewer, note string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT current_seq FROM kb.category_trees WHERE id = $1 FOR SHARE`, treeID).Scan(&current); err != nil {
		return err
	}
	var staleChange any
	if current != seq {
		var cid int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM kb.category_tree_changes WHERE tree_id = $1 AND seq > $2 ORDER BY seq LIMIT 1`,
			treeID, seq).Scan(&cid); err != nil {
			return err
		}
		staleChange = cid
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.category_bindings WHERE artifact_type = $1 AND artifact_id = $2`, a.Type, a.ID); err != nil {
		return err
	}
	for _, leaf := range leaves {
		if _, err := tx.ExecContext(ctx, `INSERT INTO kb.category_bindings
			(node_id, artifact_type, artifact_id, record_id, classification_id, tree_seq, stale, stale_change_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			leaf, a.Type, a.ID, a.RecordID, id, seq, staleChange != nil, staleChange); err != nil {
			return fmt.Errorf("insert binding %s: %w", leaf, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.category_classifications SET status = $2, reason = '', evidence = $3,
		reviewed_by = $4, review_note = $5, updated_at = NOW() WHERE id = $1`,
		id, StatusPublished, jsonb(evidence), reviewer, note); err != nil {
		return err
	}
	return tx.Commit()
}

// GetClassification returns one attempt.
func (s *Store) GetClassification(ctx context.Context, id int64) (*Classification, error) {
	return scanClassification(s.DB.QueryRowContext(ctx, `SELECT `+classificationColumns+` FROM kb.category_classifications WHERE id = $1`, id))
}

// ClassificationFilter selects attempts. Empty fields match everything.
type ClassificationFilter struct {
	ArtifactType string
	ArtifactID   string
	RecordID     int64
	Status       string
	LatestOnly   bool // only the latest attempt per artifact
	ReviewQueue  bool // latest attempts that are failed or in review
	Limit        int
}

// ListClassifications returns attempts, newest first.
func (s *Store) ListClassifications(ctx context.Context, f ClassificationFilter) ([]Classification, error) {
	var where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if f.ArtifactType != "" {
		where = append(where, "artifact_type = "+arg(f.ArtifactType))
	}
	if f.ArtifactID != "" {
		where = append(where, "artifact_id = "+arg(f.ArtifactID))
	}
	if f.RecordID > 0 {
		where = append(where, "record_id = "+arg(f.RecordID))
	}
	if f.Status != "" {
		where = append(where, "status = "+arg(f.Status))
	}
	if f.ReviewQueue {
		f.LatestOnly = true
		where = append(where, fmt.Sprintf("status IN ('%s', '%s')", StatusFailed, StatusInReview))
	}
	if f.LatestOnly {
		where = append(where, `id = (SELECT MAX(l.id) FROM kb.category_classifications l
			WHERE l.artifact_type = c.artifact_type AND l.artifact_id = c.artifact_id)`)
	}
	stmt := `SELECT ` + classificationColumns + ` FROM kb.category_classifications c`
	if len(where) > 0 {
		stmt += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	stmt += " ORDER BY id DESC LIMIT " + arg(limit)
	rows, err := s.DB.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Classification
	for rows.Next() {
		c, err := scanClassification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// CloseClassification ends an attempt as unclassified; bindings are untouched.
func (s *Store) CloseClassification(ctx context.Context, id int64, reviewer, note string) error {
	r, err := s.DB.ExecContext(ctx, `UPDATE kb.category_classifications SET status = $2, reviewed_by = $3, review_note = $4,
		updated_at = NOW() WHERE id = $1 AND status IN ($5, $6)`, id, StatusUnclassified, reviewer, note, StatusFailed, StatusInReview)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("classification %d is not in review: %w", id, ErrNotFound)
	}
	return nil
}

// ResolveClassification publishes a reviewer's choice of leaves for an attempt in review.
func (s *Store) ResolveClassification(ctx context.Context, id int64, leaves []string, reviewer, note string) error {
	c, err := s.GetClassification(ctx, id)
	if err != nil {
		return err
	}
	if c.Status != StatusFailed && c.Status != StatusInReview {
		return inputErr("classification %d is %s, not in review", id, c.Status)
	}
	info, err := s.GetTree(ctx, c.TreeID)
	if err != nil {
		return err
	}
	tree, err := s.LoadCurrent(ctx, info)
	if err != nil {
		return err
	}
	if err := ValidateLeaves(tree, leaves); err != nil {
		return &InputError{Err: err}
	}
	evidence := c.Evidence
	if evidence == nil {
		evidence = map[string]any{}
	}
	evidence["review"] = map[string]any{"reviewer": reviewer, "leaves": leaves, "note": note, "tree_seq": info.CurrentSeq,
		"previous_status": c.Status, "previous_reason": c.Reason}
	a := Artifact{Type: c.ArtifactType, ID: c.ArtifactID, RecordID: c.RecordID}
	return s.publish(ctx, id, a, info.ID, info.CurrentSeq, leaves, evidence, reviewer, note)
}

// Binding is a published direct membership with its node and path.
type Binding struct {
	NodeID        string    `json:"node_id"`
	CanonicalName string    `json:"canonical_name"`
	IsOthers      bool      `json:"is_others"`
	Path          []string  `json:"path"` // canonical names from the root
	ArtifactType  string    `json:"artifact_type"`
	ArtifactID    string    `json:"artifact_id"`
	RecordID      int64     `json:"record_id"`
	TreeSeq       int64     `json:"tree_seq"`
	Stale         bool      `json:"stale"`
	StaleChangeID *int64    `json:"stale_change_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ArtifactAssociations are an artifact's direct memberships and every ancestor of them.
type ArtifactAssociations struct {
	Direct    []Binding `json:"direct"`
	Ancestors []string  `json:"ancestors"` // node IDs, deduplicated, root first
}

const bindingSelect = `SELECT b.node_id, n.canonical_name, n.is_others, n.tree_id, b.artifact_type, b.artifact_id, b.record_id,
	b.tree_seq, b.stale, b.stale_change_id, b.created_at
	FROM kb.category_bindings b JOIN kb.category_nodes n ON n.id = b.node_id`

func (s *Store) scanBindings(ctx context.Context, stmt string, args ...any) ([]Binding, []string, error) {
	rows, err := s.DB.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []Binding
	var trees []string
	for rows.Next() {
		var b Binding
		var treeID string
		var staleChange sql.NullInt64
		if err := rows.Scan(&b.NodeID, &b.CanonicalName, &b.IsOthers, &treeID, &b.ArtifactType, &b.ArtifactID, &b.RecordID,
			&b.TreeSeq, &b.Stale, &staleChange, &b.CreatedAt); err != nil {
			return nil, nil, err
		}
		if staleChange.Valid {
			b.StaleChangeID = &staleChange.Int64
		}
		out = append(out, b)
		trees = append(trees, treeID)
	}
	return out, trees, rows.Err()
}

// ArtifactBindings returns an artifact's associations, with paths from the current tree.
func (s *Store) ArtifactBindings(ctx context.Context, artifactType, artifactID string) (*ArtifactAssociations, error) {
	direct, trees, err := s.scanBindings(ctx, bindingSelect+` WHERE b.artifact_type = $1 AND b.artifact_id = $2 ORDER BY n.canonical_name`,
		artifactType, artifactID)
	if err != nil {
		return nil, err
	}
	out := &ArtifactAssociations{Direct: direct, Ancestors: []string{}}
	loaded := map[string]*Tree{}
	seen := map[string]bool{}
	for i := range out.Direct {
		t := loaded[trees[i]]
		if t == nil {
			info, err := s.GetTree(ctx, trees[i])
			if err != nil {
				return nil, err
			}
			if t, err = s.LoadCurrent(ctx, info); err != nil {
				return nil, err
			}
			loaded[trees[i]] = t
		}
		path := t.Path(out.Direct[i].NodeID) // empty when the node was retired
		for j, n := range path {
			out.Direct[i].Path = append(out.Direct[i].Path, n.CanonicalName)
			if j < len(path)-1 && !seen[n.ID] {
				seen[n.ID] = true
				out.Ancestors = append(out.Ancestors, n.ID)
			}
		}
	}
	return out, nil
}

// Members lists artifacts bound to nodeID, or to any node of its subtree unless direct.
func (s *Store) Members(ctx context.Context, nodeID string, direct bool, limit int) ([]Binding, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	if direct {
		b, _, err := s.scanBindings(ctx, bindingSelect+` WHERE b.node_id = $1 ORDER BY b.id LIMIT $2`, nodeID, limit)
		return b, err
	}
	b, _, err := s.scanBindings(ctx, `WITH RECURSIVE sub AS (
			SELECT id FROM kb.category_nodes WHERE id = $1
			UNION ALL
			SELECT c.id FROM kb.category_nodes c JOIN sub ON c.parent_id = sub.id WHERE c.retired_at IS NULL
		) `+bindingSelect+` WHERE b.node_id IN (SELECT id FROM sub) ORDER BY b.id LIMIT $2`, nodeID, limit)
	return b, err
}

// OthersReportRow is one "others" leaf and its number of direct members.
type OthersReportRow struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"canonical_name"`
	ParentID   string `json:"parent_id"`
	ParentName string `json:"parent_name"`
	Members    int64  `json:"members"`
}

// OthersReport lists a tree's "others" leaves, largest first (devdoc §5).
func (s *Store) OthersReport(ctx context.Context, treeID string) ([]OthersReportRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT o.id, o.canonical_name, p.id, p.canonical_name, COUNT(b.id)
		FROM kb.category_nodes o JOIN kb.category_nodes p ON p.id = o.parent_id
		LEFT JOIN kb.category_bindings b ON b.node_id = o.id
		WHERE o.tree_id = $1 AND o.is_others AND o.retired_at IS NULL
		GROUP BY o.id, o.canonical_name, p.id, p.canonical_name
		ORDER BY COUNT(b.id) DESC, o.canonical_name`, treeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OthersReportRow
	for rows.Next() {
		var r OthersReportRow
		if err := rows.Scan(&r.NodeID, &r.Name, &r.ParentID, &r.ParentName, &r.Members); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ArtifactRef identifies an artifact to (re)classify.
type ArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	RecordID   int64  `json:"record_id"`
}

// StaleArtifacts returns artifacts of a type that have stale bindings.
func (s *Store) StaleArtifacts(ctx context.Context, artifactType string, limit int) ([]ArtifactRef, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT artifact_id, MIN(record_id) FROM kb.category_bindings
		WHERE artifact_type = $1 AND stale GROUP BY artifact_id ORDER BY MIN(id) LIMIT $2`, artifactType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactRef
	for rows.Next() {
		var r ArtifactRef
		if err := rows.Scan(&r.ArtifactID, &r.RecordID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RemoveVanished deletes bindings and classifications of a record's artifacts that no longer
// exist, as after re-extraction. It returns the number of bindings removed.
func (s *Store) RemoveVanished(ctx context.Context, artifactType string, recordID int64, current []string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `DELETE FROM kb.category_bindings WHERE artifact_type = $1 AND record_id = $2
		AND NOT (artifact_id = ANY($3))`, artifactType, recordID, pq.Array(nonNilStrings(current)))
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.category_classifications WHERE artifact_type = $1 AND record_id = $2
		AND NOT (artifact_id = ANY($3))`, artifactType, recordID, pq.Array(nonNilStrings(current))); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}
