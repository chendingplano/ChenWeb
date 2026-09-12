package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrMultipleRoots is returned when an edit would give a profile a second
// `product`-kind node. A profile has exactly one root (spec: profile model).
var ErrMultipleRoots = errors.New("a profile may have only one product root node")

// ErrNodeNotFound is returned when a node id does not belong to the profile.
var ErrNodeNotFound = errors.New("scope node not found in profile")

// queryer is the subset of *sql.DB / *sql.Tx the store uses, so every helper
// runs the same whether or not it is inside a transaction.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is the persistence layer for profiles and their scope nodes.
type Store struct {
	DB *sql.DB
}

const nodeColumns = `id, profile_id, parent_node_id, node_kind, label, label_en,
	COALESCE(aliases, '[]'::jsonb), depth, origin, status, confidence, rationale,
	COALESCE(object_id, ''), COALESCE(concept_id, ''), COALESCE(term_id, ''),
	grounding, reconcile_status, aspect_key,
	COALESCE(relation_types, '[]'::jsonb), match_mode,
	COALESCE(source_refs, '[]'::jsonb), (embedding IS NOT NULL)`

// scannedNode is a ProfileNode plus the derived has-embedding flag.
type scannedNode struct {
	ProfileNode
	HasEmbedding bool
}

func scanNodes(rows *sql.Rows) ([]scannedNode, error) {
	defer func() { _ = rows.Close() }()
	var out []scannedNode
	for rows.Next() {
		var (
			n                          scannedNode
			parent                     sql.NullInt64
			aliasesRaw, relRaw, refRaw []byte
		)
		if err := rows.Scan(
			&n.ID, &n.ProfileID, &parent, &n.NodeKind, &n.Label, &n.LabelEN,
			&aliasesRaw, &n.Depth, &n.Origin, &n.Status, &n.Confidence, &n.Rationale,
			&n.ObjectID, &n.ConceptID, &n.TermID,
			&n.Grounding, &n.ReconcileStatus, &n.AspectKey,
			&relRaw, &n.MatchMode, &refRaw, &n.HasEmbedding,
		); err != nil {
			return nil, err
		}
		if parent.Valid {
			p := parent.Int64
			n.ParentNodeID = &p
		}
		_ = json.Unmarshal(aliasesRaw, &n.Aliases)
		_ = json.Unmarshal(relRaw, &n.RelationTypes)
		_ = json.Unmarshal(refRaw, &n.SourceRefs)
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateProfile inserts a draft profile at version 1 and its single accepted
// `product` root node whose label is the product name.
func (s Store) CreateProfile(ctx context.Context, in NewProfileInput) (*Profile, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("product name is required")
	}
	tenant := strings.TrimSpace(in.TenantID)
	if tenant == "" {
		tenant = "-"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	keywords, _ := json.Marshal(orEmpty(in.Keywords))
	p := Profile{}
	var keywordsRaw []byte
	err = tx.QueryRowContext(ctx, `
		INSERT INTO kb.product_profiles (tenant_id, name, product_description, keywords, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, name, product_description, keywords, notes, version, status,
		          truncated, truncated_count, created_at, updated_at`,
		tenant, name, strings.TrimSpace(in.ProductDescription), keywords, strings.TrimSpace(in.Notes),
	).Scan(&p.ID, &p.TenantID, &p.Name, &p.ProductDescription, &keywordsRaw, &p.Notes, &p.Version, &p.Status,
		&p.Truncated, &p.TruncatedCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(keywordsRaw, &p.Keywords)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO kb.product_profile_nodes
			(profile_id, parent_node_id, node_kind, label, label_en, depth, origin, status)
		VALUES ($1, NULL, $2, $3, $3, 0, $4, $5)`,
		p.ID, KindProduct, name, OriginUserAdded, StatusAccepted,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProfile loads one profile row.
func (s Store) GetProfile(ctx context.Context, id int64) (*Profile, error) {
	p := Profile{}
	var keywordsRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, product_description, keywords, notes, version, status,
		       truncated, truncated_count, created_at, updated_at
		FROM kb.product_profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.TenantID, &p.Name, &p.ProductDescription, &keywordsRaw, &p.Notes, &p.Version, &p.Status,
		&p.Truncated, &p.TruncatedCount, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(keywordsRaw, &p.Keywords)
	return &p, nil
}

// FindProfileByName looks up a tenant's profile by product name, matching on a
// trimmed, case-insensitive comparison. Returns nil, nil when no profile
// matches (spec: product-review-intake — duplicate detection is exact
// normalized-name match, not fuzzy).
func (s Store) FindProfileByName(ctx context.Context, tenantID, name string) (*Profile, error) {
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		tenant = "-"
	}
	p := Profile{}
	var keywordsRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, product_description, keywords, notes, version, status,
		       truncated, truncated_count, created_at, updated_at
		FROM kb.product_profiles
		WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))
		ORDER BY updated_at DESC LIMIT 1`, tenant, name,
	).Scan(&p.ID, &p.TenantID, &p.Name, &p.ProductDescription, &keywordsRaw, &p.Notes, &p.Version, &p.Status,
		&p.Truncated, &p.TruncatedCount, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(keywordsRaw, &p.Keywords)
	return &p, nil
}

// ListProfiles returns a tenant's profiles, most-recently-updated first.
func (s Store) ListProfiles(ctx context.Context, tenantID string) ([]Profile, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "-"
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, tenant_id, name, product_description, version, status,
		       truncated, truncated_count, created_at, updated_at
		FROM kb.product_profiles WHERE tenant_id = $1
		ORDER BY updated_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.ProductDescription, &p.Version,
			&p.Status, &p.Truncated, &p.TruncatedCount, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LoadNodes returns every node of a profile, shallowest first.
func (s Store) LoadNodes(ctx context.Context, profileID int64) ([]ProfileNode, error) {
	scanned, err := s.loadScanned(ctx, s.DB, profileID)
	if err != nil {
		return nil, err
	}
	out := make([]ProfileNode, len(scanned))
	for i, n := range scanned {
		out[i] = n.ProfileNode
	}
	return out, nil
}

func (s Store) loadScanned(ctx context.Context, q queryer, profileID int64) ([]scannedNode, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+nodeColumns+`
		FROM kb.product_profile_nodes WHERE profile_id = $1
		ORDER BY depth, id`, profileID)
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func bumpVersion(ctx context.Context, q queryer, profileID int64) error {
	_, err := q.ExecContext(ctx, `
		UPDATE kb.product_profiles
		SET version = version + 1, updated_at = NOW()
		WHERE id = $1`, profileID)
	return err
}

// SetProfileStatus sets a profile's status (draft ↔ ready). Not a node-set
// mutation, so it does not bump the version.
func (s Store) SetProfileStatus(ctx context.Context, id int64, status string) error {
	if status != ProfileDraft && status != ProfileReady {
		return fmt.Errorf("invalid profile status %q", status)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_profiles SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, status)
	return err
}

// RecordTruncation flags a profile whose construction discarded nodes past the
// node budget.
func (s Store) RecordTruncation(ctx context.Context, q queryer, profileID int64, count int) error {
	if q == nil {
		q = s.DB
	}
	_, err := q.ExecContext(ctx, `
		UPDATE kb.product_profiles
		SET truncated = TRUE, truncated_count = truncated_count + $2, updated_at = NOW()
		WHERE id = $1`, profileID, count)
	return err
}

// AddNode inserts a user-added, accepted node under parentID (nil = attach to
// the root level). It bumps the profile version and rejects a cycle or a second
// product root.
func (s Store) AddNode(ctx context.Context, profileID int64, parentID *int64, in ProfileNode) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	nodes, err := s.loadScanned(ctx, tx, profileID)
	if err != nil {
		return 0, err
	}
	kind := in.NodeKind
	if kind == "" {
		kind = KindPart
	}
	if kind == KindProduct && hasProductRoot(nodes, 0) {
		return 0, ErrMultipleRoots
	}
	depth := 0
	if parentID != nil {
		parent := findNode(nodes, *parentID)
		if parent == nil {
			return 0, ErrNodeNotFound
		}
		depth = parent.Depth + 1
	}

	newID, err := insertNode(ctx, tx, ProfileNode{
		ProfileID:    profileID,
		ParentNodeID: parentID,
		NodeKind:     kind,
		Label:        strings.TrimSpace(in.Label),
		LabelEN:      strings.TrimSpace(in.LabelEN),
		Aliases:      in.Aliases,
		Depth:        depth,
		Origin:       OriginUserAdded,
		Status:       StatusAccepted,
		Confidence:   in.Confidence,
		Rationale:    strings.TrimSpace(in.Rationale),
	})
	if err != nil {
		return 0, err
	}
	if err := bumpVersion(ctx, tx, profileID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newID, nil
}

// UpdateNode applies an edit to one node and bumps the profile version. A
// re-parent that would create a cycle is rejected with CWB_KB_PMR_010; turning
// a node into a second product root is rejected with ErrMultipleRoots.
func (s Store) UpdateNode(ctx context.Context, profileID, nodeID int64, edit NodeEdit) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	nodes, err := s.loadScanned(ctx, tx, profileID)
	if err != nil {
		return err
	}
	target := findNode(nodes, nodeID)
	if target == nil {
		return ErrNodeNotFound
	}

	set := []string{"updated_at = NOW()"}
	args := []any{nodeID, profileID}
	add := func(expr string, val any) {
		args = append(args, val)
		set = append(set, fmt.Sprintf("%s = $%d", expr, len(args)))
	}
	if edit.Label != nil {
		add("label", strings.TrimSpace(*edit.Label))
	}
	if edit.LabelEN != nil {
		add("label_en", strings.TrimSpace(*edit.LabelEN))
	}
	if edit.Aliases != nil {
		raw, _ := json.Marshal(*edit.Aliases)
		add("aliases", raw)
	}
	if edit.Status != nil {
		if *edit.Status != StatusProposed && *edit.Status != StatusAccepted && *edit.Status != StatusRejected {
			return fmt.Errorf("invalid node status %q", *edit.Status)
		}
		add("status", *edit.Status)
	}
	newKind := target.NodeKind
	if edit.NodeKind != nil {
		newKind = *edit.NodeKind
		add("node_kind", newKind)
	}
	if newKind == KindProduct && hasProductRoot(nodes, nodeID) {
		return ErrMultipleRoots
	}
	if edit.ParentNodeID != nil {
		if wouldCreateCycle(nodes, nodeID, *edit.ParentNodeID) {
			return ErrCycle(nodeID)
		}
		if findNode(nodes, *edit.ParentNodeID) == nil {
			return ErrNodeNotFound
		}
		add("parent_node_id", *edit.ParentNodeID)
	}

	if len(set) > 1 {
		q := fmt.Sprintf(`UPDATE kb.product_profile_nodes SET %s WHERE id = $1 AND profile_id = $2`,
			strings.Join(set, ", "))
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	if err := bumpVersion(ctx, tx, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteNode removes a node (its subtree cascades via the self-FK) and bumps
// the profile version.
func (s Store) DeleteNode(ctx context.Context, profileID, nodeID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		DELETE FROM kb.product_profile_nodes
		WHERE id = $1 AND profile_id = $2 AND node_kind <> 'product'`, nodeID, profileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNodeNotFound
	}
	if err := bumpVersion(ctx, tx, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// AcceptAllProposed accepts every `proposed` node on a profile in one step and
// bumps the version once. Used by the self-service intake flow (spec:
// product-review-intake), which has no manual curation step of its own — the
// full curation page still gates on individual accept/reject.
func (s Store) AcceptAllProposed(ctx context.Context, profileID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE kb.product_profile_nodes
		SET status = 'accepted', updated_at = NOW()
		WHERE profile_id = $1 AND status = 'proposed'`, profileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return tx.Commit()
	}
	if err := bumpVersion(ctx, tx, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// insertNode inserts one node and returns its id. JSONB columns are marshalled
// from the Go slices.
func insertNode(ctx context.Context, q queryer, n ProfileNode) (int64, error) {
	aliases, _ := json.Marshal(orEmpty(n.Aliases))
	rels, _ := json.Marshal(orEmpty(n.RelationTypes))
	refs, _ := json.Marshal(orEmptyRefs(n.SourceRefs))
	grounding := n.Grounding
	if grounding == "" {
		grounding = GroundingUngrounded
	}
	var parent any
	if n.ParentNodeID != nil {
		parent = *n.ParentNodeID
	}
	var objectID, conceptID, termID any
	if n.ObjectID != "" {
		objectID = n.ObjectID
	}
	if n.ConceptID != "" {
		conceptID = n.ConceptID
	}
	if n.TermID != "" {
		termID = n.TermID
	}
	var id int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO kb.product_profile_nodes
			(profile_id, parent_node_id, node_kind, label, label_en, aliases, depth,
			 origin, status, confidence, rationale, object_id, concept_id, term_id,
			 grounding, reconcile_status, aspect_key, relation_types, match_mode, source_refs)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id`,
		n.ProfileID, parent, n.NodeKind, n.Label, n.LabelEN, aliases, n.Depth,
		n.Origin, n.Status, n.Confidence, n.Rationale, objectID, conceptID, termID,
		grounding, n.ReconcileStatus, n.AspectKey, rels, n.MatchMode, refs,
	).Scan(&id)
	return id, err
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyRefs(s []SourceRef) []SourceRef {
	if s == nil {
		return []SourceRef{}
	}
	return s
}

func findNode(nodes []scannedNode, id int64) *scannedNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

// hasProductRoot reports whether a product-kind node other than exceptID exists.
func hasProductRoot(nodes []scannedNode, exceptID int64) bool {
	for _, n := range nodes {
		if n.NodeKind == KindProduct && n.ID != exceptID {
			return true
		}
	}
	return false
}

// wouldCreateCycle reports whether re-parenting nodeID under newParentID would
// make nodeID its own ancestor.
func wouldCreateCycle(nodes []scannedNode, nodeID, newParentID int64) bool {
	if nodeID == newParentID {
		return true
	}
	parentOf := make(map[int64]*int64, len(nodes))
	for _, n := range nodes {
		parentOf[n.ID] = n.ParentNodeID
	}
	seen := map[int64]bool{}
	for cur := &newParentID; cur != nil; {
		if *cur == nodeID {
			return true
		}
		if seen[*cur] {
			return false // a pre-existing loop that doesn't involve nodeID
		}
		seen[*cur] = true
		cur = parentOf[*cur]
	}
	return false
}
