package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Review status values for inventory rows in kb.artifact_categories.status.
const (
	InventoryCategoryStatusPending  = "pending_review"
	InventoryCategoryStatusApproved = "approved"
	InventoryCategoryStatusRejected = "rejected"
	InventoryCategoryStatusMerged   = "merged"
)

// inventoryCategoryEmbeddingMinDims guards against comparing empty/degenerate vectors.
const inventoryCategoryEmbeddingMinDims = 2

// InventoryCategoryRecord is a single row of the category registry.
type InventoryCategoryRecord struct {
	CategoryKey     string
	Status          string
	CanonicalOf     string
	DisplayNames    []string
	RequiredAttrs   []string
	Specs           map[string]InventorySpecSchema
	PlausibleRanges map[string]InventoryPlausibleRange
	Embedding       []float64
	SeenCount       int64
}

// InventoryCategoryRegistry is the DB-backed source of truth for the inventory
// category ontology. Unlike the static JSON dictionary it is mutated at runtime
// (concurrency-safe via upserts) so the ontology can grow from the corpus while
// staying a controlled vocabulary.
type InventoryCategoryRegistry struct {
	DB *sql.DB
}

func (r InventoryCategoryRegistry) ensureTables(ctx context.Context) error {
	if r.DB == nil {
		return fmt.Errorf("(MID_26053160) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;
CREATE TABLE IF NOT EXISTS kb.artifact_categories (
    category_key      TEXT NOT NULL,
    category_type     TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending_review',
    canonical_of      TEXT,
    display_names     JSONB NOT NULL DEFAULT '[]'::jsonb,
    required_attrs    JSONB NOT NULL DEFAULT '[]'::jsonb,
    specs             JSONB NOT NULL DEFAULT '{}'::jsonb,
    plausible_ranges  JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding         JSONB NOT NULL DEFAULT '[]'::jsonb,
    seen_count        BIGINT NOT NULL DEFAULT 0,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    category_id       BIGINT GENERATED ALWAYS AS IDENTITY,
    search_document   TEXT NOT NULL DEFAULT '',
    CONSTRAINT artifact_categories_pkey PRIMARY KEY (category_key)
);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_status ON kb.artifact_categories(status);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_seen_count ON kb.artifact_categories(seen_count DESC);
`
	_, err := r.DB.ExecContext(ctx, ddl)
	return err
}

// SeedApprovedCategories upserts the curated dictionary categories as approved.
// It is idempotent and never downgrades human edits: an existing row keeps its
// status, and its schema body is refreshed from the dictionary only while it is
// still approved (so reviewers' in-DB edits to pending rows are preserved).
func (r InventoryCategoryRegistry) SeedApprovedCategories(ctx context.Context, dict inventoryDictionary) error {
	if err := r.ensureTables(ctx); err != nil {
		return err
	}
	for category, schema := range dict.Categories {
		key := normalizeInventoryToken(category)
		if key == "" {
			continue
		}
		requiredAttrs, _ := json.Marshal(uniqueSortedStrings(schema.RequiredAttrs))
		specs, _ := json.Marshal(schema.Specs)
		ranges, _ := json.Marshal(dict.PlausibleRanges[key])
		displayNames, _ := json.Marshal([]string{category})
		const stmt = `
INSERT INTO kb.artifact_categories (
    category_key, category_type, status, display_names, required_attrs, specs, plausible_ranges, search_document
) VALUES ($1, 'inventory', 'approved', $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb, $6)
ON CONFLICT (category_key) DO UPDATE SET
    category_type    = 'inventory',
    required_attrs   = CASE WHEN kb.artifact_categories.status = 'approved' THEN EXCLUDED.required_attrs ELSE kb.artifact_categories.required_attrs END,
    specs            = CASE WHEN kb.artifact_categories.status = 'approved' THEN EXCLUDED.specs ELSE kb.artifact_categories.specs END,
    plausible_ranges = CASE WHEN kb.artifact_categories.status = 'approved' THEN EXCLUDED.plausible_ranges ELSE kb.artifact_categories.plausible_ranges END`
		if _, err := r.DB.ExecContext(ctx, stmt, key, string(displayNames), string(requiredAttrs), string(specs), string(ranges), key); err != nil {
			return fmt.Errorf("(MID_26053161) seed inventory category %q: %w", key, err)
		}
	}
	return nil
}

// LoadActiveCategories returns every category that should be treated as known
// vocabulary during extraction: approved ∪ pending_review. Rejected and merged
// categories are excluded. Keyed by normalized category_key.
func (r InventoryCategoryRegistry) LoadActiveCategories(ctx context.Context) (map[string]InventoryCategoryRecord, error) {
	if err := r.ensureTables(ctx); err != nil {
		return nil, err
	}
	const q = `
SELECT category_key, status, COALESCE(canonical_of, ''), display_names,
       required_attrs, specs, plausible_ranges, embedding, seen_count
FROM kb.artifact_categories
WHERE category_type = 'inventory' AND status IN ('approved', 'pending_review')`
	rows, err := r.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053162) load active categories: %w", err)
	}
	defer rows.Close()
	out := map[string]InventoryCategoryRecord{}
	for rows.Next() {
		rec, scanErr := scanInventoryCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out[rec.CategoryKey] = rec
	}
	return out, rows.Err()
}

// ObserveCategory admits a surface form seen in the corpus into the registry,
// matching it against existing categories before minting a new one. The
// embedding is optional; when present it enables fuzzy (synonym) matching.
// existing is the in-memory snapshot of active categories (caller-loaded once
// per pass) used for match-before-mint; the actual write is an upsert so it is
// safe under concurrency. Returns the canonical key the surface form resolved to.
func (r InventoryCategoryRegistry) ObserveCategory(
	ctx context.Context,
	surfaceForm string,
	embedding []float64,
	existing map[string]InventoryCategoryRecord,
	fuzzyThreshold float64,
) (string, error) {
	if err := r.ensureTables(ctx); err != nil {
		return "", err
	}
	key := normalizeInventoryToken(surfaceForm)
	if key == "" {
		return "", nil
	}

	// Tier 1: exact normalized-key match.
	matchKey := ""
	if _, ok := existing[key]; ok {
		matchKey = key
	}
	// Tier 2: display-name / surface-form match against any existing category.
	if matchKey == "" {
		for k, rec := range existing {
			for _, name := range rec.DisplayNames {
				if normalizeInventoryToken(name) == key {
					matchKey = k
					break
				}
			}
			if matchKey != "" {
				break
			}
		}
	}
	// Tier 3: embedding similarity (synonym merge), if we have a usable vector.
	if matchKey == "" && len(embedding) >= inventoryCategoryEmbeddingMinDims {
		best, bestScore := "", 0.0
		for k, rec := range existing {
			if len(rec.Embedding) < inventoryCategoryEmbeddingMinDims {
				continue
			}
			if score := inventoryCategoryCosine(embedding, rec.Embedding); score > bestScore {
				best, bestScore = k, score
			}
		}
		if best != "" && bestScore >= fuzzyThreshold {
			matchKey = best
		}
	}

	if matchKey != "" {
		return matchKey, r.touchCategory(ctx, matchKey, surfaceForm)
	}
	return key, r.mintCategory(ctx, key, surfaceForm, embedding)
}

// touchCategory records another sighting of an already-known category: bumps the
// seen count, refreshes last_seen, and appends the surface form to display_names.
func (r InventoryCategoryRegistry) touchCategory(ctx context.Context, key, surfaceForm string) error {
	const stmt = `
UPDATE kb.artifact_categories SET
    seen_count    = seen_count + 1,
    last_seen_at  = NOW(),
    display_names = CASE
        WHEN display_names @> to_jsonb($2::text) THEN display_names
        ELSE display_names || to_jsonb($2::text)
    END
WHERE category_key = $1 AND category_type = 'inventory'`
	_, err := r.DB.ExecContext(ctx, stmt, key, strings.TrimSpace(surfaceForm))
	if err != nil {
		return fmt.Errorf("(MID_26053163) touch inventory category %q: %w", key, err)
	}
	return nil
}

// mintCategory inserts a brand-new pending_review category. Concurrency-safe:
// a racing insert of the same key collapses into a touch via ON CONFLICT.
func (r InventoryCategoryRegistry) mintCategory(ctx context.Context, key, surfaceForm string, embedding []float64) error {
	displayNames, _ := json.Marshal([]string{strings.TrimSpace(surfaceForm)})
	embJSON, _ := json.Marshal(embedding)
	const stmt = `
INSERT INTO kb.artifact_categories (
    category_key, category_type, status, display_names, embedding, seen_count, search_document
) VALUES ($1, 'inventory', 'pending_review', $2::jsonb, $3::jsonb, 1, $1)
ON CONFLICT (category_key) DO UPDATE SET
    category_type = 'inventory',
    seen_count   = kb.artifact_categories.seen_count + 1,
    last_seen_at = NOW(),
    embedding    = CASE
        WHEN jsonb_array_length(kb.artifact_categories.embedding) = 0 THEN EXCLUDED.embedding
        ELSE kb.artifact_categories.embedding
    END`
	if _, err := r.DB.ExecContext(ctx, stmt, key, string(displayNames), string(embJSON)); err != nil {
		return fmt.Errorf("(MID_26053164) mint inventory category %q: %w", key, err)
	}
	return nil
}

// CategoryStatuses returns the current review status of every category, keyed by
// normalized category_key. Merged categories report the surviving key's status
// is resolved by the caller via CanonicalOf if needed; here we return raw status.
func (r InventoryCategoryRegistry) CategoryStatuses(ctx context.Context) (map[string]string, error) {
	if err := r.ensureTables(ctx); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT category_key, status FROM kb.artifact_categories WHERE category_type = 'inventory'`)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053165) load category statuses: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, status string
		if err := rows.Scan(&key, &status); err != nil {
			return nil, err
		}
		out[key] = status
	}
	return out, rows.Err()
}

// ListPendingForReview returns pending categories ordered by how often they have
// been seen, so reviewers curate the highest-impact ontology gaps first.
func (r InventoryCategoryRegistry) ListPendingForReview(ctx context.Context, limit int) ([]InventoryCategoryRecord, error) {
	if err := r.ensureTables(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	const q = `
SELECT category_key, status, COALESCE(canonical_of, ''), display_names,
       required_attrs, specs, plausible_ranges, embedding, seen_count
FROM kb.artifact_categories
WHERE category_type = 'inventory' AND status = 'pending_review'
ORDER BY seen_count DESC, first_seen_at ASC
LIMIT $1`
	rows, err := r.DB.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053166) list pending categories: %w", err)
	}
	defer rows.Close()
	out := make([]InventoryCategoryRecord, 0, limit)
	for rows.Next() {
		rec, scanErr := scanInventoryCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanInventoryCategory(rows *sql.Rows) (InventoryCategoryRecord, error) {
	var (
		rec           InventoryCategoryRecord
		displayNames  []byte
		requiredAttrs []byte
		specs         []byte
		ranges        []byte
		embedding     []byte
	)
	if err := rows.Scan(
		&rec.CategoryKey, &rec.Status, &rec.CanonicalOf,
		&displayNames, &requiredAttrs, &specs, &ranges, &embedding, &rec.SeenCount,
	); err != nil {
		return InventoryCategoryRecord{}, err
	}
	_ = json.Unmarshal(displayNames, &rec.DisplayNames)
	_ = json.Unmarshal(requiredAttrs, &rec.RequiredAttrs)
	_ = json.Unmarshal(specs, &rec.Specs)
	_ = json.Unmarshal(ranges, &rec.PlausibleRanges)
	_ = json.Unmarshal(embedding, &rec.Embedding)
	return rec, nil
}

// inventoryCategoryCosine is a local cosine-similarity helper (uniquely named to
// avoid colliding with other helpers in the package).
func inventoryCategoryCosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// deriveInventoryCategoryStatus maps a stored item_category to a review status
// for the read path. It reflects the registry's current state, so a category
// approved after extraction is immediately seen as approved without touching the
// instance rows. A missing entry means the category predates curation (or the
// registry was unavailable) and is treated as needing review.
func deriveInventoryCategoryStatus(statuses map[string]string, itemCategory string) string {
	if len(statuses) == 0 {
		return InventoryCategoryStatusPending
	}
	key := normalizeInventoryToken(itemCategory)
	if key == "" {
		return InventoryCategoryStatusPending
	}
	switch statuses[key] {
	case InventoryCategoryStatusApproved:
		return InventoryCategoryStatusApproved
	case InventoryCategoryStatusRejected:
		return InventoryCategoryStatusRejected
	default:
		// pending_review, merged, or absent → still needs human attention.
		return InventoryCategoryStatusPending
	}
}

// inventoryCategorySchemaFromRecord converts a registry row into the in-memory
// dictionary schema shape so admitted categories merge into the live dictionary.
func inventoryCategorySchemaFromRecord(rec InventoryCategoryRecord) inventoryCategorySchema {
	return inventoryCategorySchema{
		RequiredAttrs: rec.RequiredAttrs,
		Specs:         rec.Specs,
	}
}

// ErrInventoryCategoryNotFound is returned by PatchCategory when the key does not exist.
var ErrInventoryCategoryNotFound = errors.New("inventory category not found")

// NormalizeInventoryToken is the exported form of normalizeInventoryToken for
// use by API handlers that need to canonicalize a category key before a DB lookup.
func NormalizeInventoryToken(raw string) string { return normalizeInventoryToken(raw) }

// InventoryCategoryPatch carries the fields that can be updated via the review endpoint.
// Only non-nil/non-empty fields are applied.
type InventoryCategoryPatch struct {
	Status          *string
	CanonicalOf     *string
	RequiredAttrs   []string
	Specs           map[string]InventorySpecSchema
	PlausibleRanges map[string]InventoryPlausibleRange
}

// ListCategoriesByStatus returns categories filtered by status (empty = all),
// ordered by seen_count DESC, capped at limit rows.
func (r InventoryCategoryRegistry) ListCategoriesByStatus(ctx context.Context, status string, limit int) ([]InventoryCategoryRecord, error) {
	if err := r.ensureTables(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = r.DB.QueryContext(ctx, `
SELECT category_key, status, COALESCE(canonical_of, ''), display_names,
       required_attrs, specs, plausible_ranges, embedding, seen_count
FROM kb.artifact_categories
WHERE category_type = 'inventory'
ORDER BY seen_count DESC, first_seen_at ASC
LIMIT $1`, limit)
	} else {
		rows, err = r.DB.QueryContext(ctx, `
SELECT category_key, status, COALESCE(canonical_of, ''), display_names,
       required_attrs, specs, plausible_ranges, embedding, seen_count
FROM kb.artifact_categories
WHERE category_type = 'inventory' AND status = $1
ORDER BY seen_count DESC, first_seen_at ASC
LIMIT $2`, status, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("(MID_26053167) list inventory categories: %w", err)
	}
	defer rows.Close()
	out := make([]InventoryCategoryRecord, 0, limit)
	for rows.Next() {
		rec, scanErr := scanInventoryCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// PatchCategory applies a partial update to an existing category row.
// Returns ErrInventoryCategoryNotFound if the key does not exist.
func (r InventoryCategoryRegistry) PatchCategory(ctx context.Context, key string, patch InventoryCategoryPatch) (InventoryCategoryRecord, error) {
	if err := r.ensureTables(ctx); err != nil {
		return InventoryCategoryRecord{}, err
	}

	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if patch.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *patch.Status)
		argIdx++
	}
	if patch.CanonicalOf != nil {
		if *patch.CanonicalOf == "" {
			setClauses = append(setClauses, "canonical_of = NULL")
		} else {
			setClauses = append(setClauses, fmt.Sprintf("canonical_of = $%d", argIdx))
			args = append(args, *patch.CanonicalOf)
			argIdx++
		}
	}
	if patch.RequiredAttrs != nil {
		b, _ := json.Marshal(patch.RequiredAttrs)
		setClauses = append(setClauses, fmt.Sprintf("required_attrs = $%d::jsonb", argIdx))
		args = append(args, string(b))
		argIdx++
	}
	if patch.Specs != nil {
		b, _ := json.Marshal(patch.Specs)
		setClauses = append(setClauses, fmt.Sprintf("specs = $%d::jsonb", argIdx))
		args = append(args, string(b))
		argIdx++
	}
	if patch.PlausibleRanges != nil {
		b, _ := json.Marshal(patch.PlausibleRanges)
		setClauses = append(setClauses, fmt.Sprintf("plausible_ranges = $%d::jsonb", argIdx))
		args = append(args, string(b))
		argIdx++
	}

	if len(setClauses) > 0 {
		setClauses = append(setClauses, "modify_time = NOW()")
		query := fmt.Sprintf(
			`UPDATE kb.artifact_categories SET %s WHERE category_type = 'inventory' AND category_key = $%d`,
			strings.Join(setClauses, ", "), argIdx,
		)
		args = append(args, key)
		res, err := r.DB.ExecContext(ctx, query, args...)
		if err != nil {
			return InventoryCategoryRecord{}, fmt.Errorf("(MID_26053168) patch inventory category %q: %w", key, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return InventoryCategoryRecord{}, ErrInventoryCategoryNotFound
		}
	}

	// Re-fetch the updated row.
	rows, err := r.DB.QueryContext(ctx, `
SELECT category_key, status, COALESCE(canonical_of, ''), display_names,
       required_attrs, specs, plausible_ranges, embedding, seen_count
FROM kb.artifact_categories WHERE category_type = 'inventory' AND category_key = $1`, key)
	if err != nil {
		return InventoryCategoryRecord{}, fmt.Errorf("(MID_26053169) re-fetch inventory category %q: %w", key, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return InventoryCategoryRecord{}, ErrInventoryCategoryNotFound
	}
	rec, scanErr := scanInventoryCategory(rows)
	if scanErr != nil {
		return InventoryCategoryRecord{}, scanErr
	}
	return rec, rows.Err()
}
