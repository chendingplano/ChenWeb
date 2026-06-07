package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// artifactCategoryRegistry is the DB-backed source of truth for the artifact
// category ontology, scoped by category_type. Writes are concurrency-safe upserts
// keyed on (category_type, category_key).
type artifactCategoryRegistry struct {
	DB *sql.DB
}

// loadIntoIndex loads all approved ∪ pending_review rows for categoryType from the DB,
// populates idx with their match_keys, and returns any alias conflicts found. Records
// are processed in descending seen_count order so the higher-seen_count entry wins on
// conflict.
func (r artifactCategoryRegistry) loadIntoIndex(ctx context.Context, categoryType string, idx *categoryIndex) ([]aliasConflict, error) {
	const q = `
SELECT category_id, category_key, status, COALESCE(canonical_of, ''), match_keys, seen_count
FROM kb.artifact_categories
WHERE category_type = $1 AND status IN ('approved', 'pending_review')
ORDER BY seen_count DESC`
	rows, err := r.DB.QueryContext(ctx, q, categoryType)
	if err != nil {
		return nil, fmt.Errorf("(MID_26060701) load categories into index: %w", err)
	}
	defer rows.Close()

	// conflictMap accumulates all IDs seen for a conflicting key so logAliasConflict
	// gets the full picture, not just the first two.
	conflictMap := map[string]map[int64]struct{}{}
	for rows.Next() {
		var (
			rec       artifactCategoryRecord
			matchKeys []byte
			seenCount int64
		)
		if err := rows.Scan(&rec.CategoryID, &rec.CategoryKey, &rec.Status, &rec.CanonicalOf, &matchKeys, &seenCount); err != nil {
			return nil, err
		}
		// rec.CategoryType = categoryType
		_ = json.Unmarshal(matchKeys, &rec.MatchKeys)

		for _, mk := range rec.MatchKeys {
			if conflict := idx.put(categoryType, mk, rec.CategoryID, seenCount); conflict {
				// Record both IDs in the conflict map.
				if conflictMap[mk] == nil {
					conflictMap[mk] = map[int64]struct{}{}
					// The existing winner is already in the index; pull it out.
					if winner, ok := idx.lookup(categoryType, mk); ok {
						conflictMap[mk][winner] = struct{}{}
					}
				}
				conflictMap[mk][rec.CategoryID] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	idx.markLoaded(categoryType)

	var out []aliasConflict
	for alias, idSet := range conflictMap {
		ids := make([]int64, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		out = append(out, aliasConflict{Alias: alias, IDs: ids})
	}
	return out, nil
}

// loadActiveCategories returns the approved ∪ pending_review categories of a type as
// an in-memory snapshot. Retained for the inventory registry which uses the old
// snapshot-style resolution path.
func (r artifactCategoryRegistry) loadActiveCategories(ctx context.Context, categoryType string) ([]artifactCategoryRecord, error) {
	const q = `
SELECT category_id, category_key, status, COALESCE(canonical_of, ''), match_keys, embedding
FROM kb.artifact_categories
WHERE category_type = $1 AND status IN ('approved', 'pending_review')`
	rows, err := r.DB.QueryContext(ctx, q, categoryType)
	if err != nil {
		return nil, fmt.Errorf("(MID_26060410) load active categories: %w", err)
	}
	defer rows.Close()
	var out []artifactCategoryRecord
	for rows.Next() {
		var (
			rec       artifactCategoryRecord
			matchKeys []byte
			embedding []byte
		)
		if err := rows.Scan(&rec.CategoryID, &rec.CategoryKey, &rec.Status, &rec.CanonicalOf, &matchKeys, &embedding); err != nil {
			return nil, err
		}
		rec.CategoryType = categoryType
		_ = json.Unmarshal(matchKeys, &rec.MatchKeys)
		_ = json.Unmarshal(embedding, &rec.Embedding)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// mintCategory inserts a new category (status pending_review) and returns its
// category_id. It is idempotent under concurrency: a racing insert of the same
// (category_type, category_key) collapses via ON CONFLICT and RETURNING yields the
// surviving row's id. The category_key is normalized; match_keys and
// search_document are derived from the LLM output.
func (r artifactCategoryRegistry) mintCategory(ctx context.Context, c createdCategory, categoryType string, embedding []float64) (int64, error) {
	key := normalizeCategoryKey(c.CategoryKey)
	if key == "" {
		return 0, fmt.Errorf("(MID_26060411) empty category key")
	}
	matchKeys := buildArtifactMatchKeys(key, c.DisplayNames, c.Aliases, c.Acronyms)
	searchDoc := c.SearchDocument
	if searchDoc == "" {
		searchDoc = buildCategorySearchDocument(key, c.DisplayNames, c.Aliases, c.Acronyms, c.Keywords, c.Description)
	}
	displayNamesJSON, _ := json.Marshal(orEmptySlice(c.DisplayNames))
	aliasesJSON, _ := json.Marshal(orEmptySlice(c.Aliases))
	acronymsJSON, _ := json.Marshal(orEmptySlice(c.Acronyms))
	keywordsJSON, _ := json.Marshal(orEmptySlice(c.Keywords))
	requiredJSON, _ := json.Marshal(orEmptyJSONArray(c.RequiredAttrs))
	matchKeysJSON, _ := json.Marshal(matchKeys)
	specsJSON, _ := json.Marshal(orEmptyJSONObject(c.Specs))
	rangesJSON, _ := json.Marshal(orEmptyJSONValue(c.PlausibleRanges, map[string]any{}))
	parentCategoriesJSON, _ := json.Marshal(orEmptySlice(c.ParentCategories))
	relatedCategoriesJSON, _ := json.Marshal(orEmptySlice(c.RelatedCategories))
	embeddingJSON, _ := json.Marshal(embedding)

	const stmt = `
INSERT INTO kb.artifact_categories (
    category_key, category_type, status, display_names, aliases, acronyms,
    category_desc, category_keywords, match_keys, search_document, required_attrs,
    specs, plausible_ranges, parent_categories, related_categories, embedding, seen_count
) VALUES ($1, $2, 'pending_review', $3::jsonb, $4::jsonb, $5::jsonb, $6, $7::jsonb,
    $8::jsonb, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb, $14::jsonb, $15::jsonb, 1)
ON CONFLICT (category_type, category_key) DO UPDATE SET
    seen_count   = kb.artifact_categories.seen_count + 1,
    last_seen_at = NOW()
RETURNING category_id`
	var id int64
	err := r.DB.QueryRowContext(ctx, stmt,
		key, categoryType, string(displayNamesJSON), string(aliasesJSON), string(acronymsJSON),
		c.Description, string(keywordsJSON), string(matchKeysJSON), searchDoc, string(requiredJSON),
		string(specsJSON), string(rangesJSON), string(parentCategoriesJSON), string(relatedCategoriesJSON), string(embeddingJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("(MID_26060412) mint category %q: %w", key, err)
	}
	return id, nil
}

// absorbAlias idempotently adds a normalized alias to an existing category's
// match_keys so the next lookup hits the deterministic exact/alias layer. The
// conditional WHERE makes concurrent absorbs of the same alias a no-op.
func (r artifactCategoryRegistry) absorbAlias(ctx context.Context, categoryID int64, alias string) error {
	k := normalizeCategoryKey(alias)
	if k == "" {
		return nil
	}
	const stmt = `
UPDATE kb.artifact_categories
SET match_keys = match_keys || to_jsonb($2::text), modify_time = NOW()
WHERE category_id = $1 AND NOT (match_keys @> to_jsonb($2::text))`
	if _, err := r.DB.ExecContext(ctx, stmt, categoryID, k); err != nil {
		return fmt.Errorf("(MID_26060413) absorb alias %q: %w", k, err)
	}
	return nil
}

// logAliasConflict upserts a conflict record for alias within categoryType. categoryIDs
// is the full set of category IDs that share the alias. Stored as jsonb.
func (r artifactCategoryRegistry) logAliasConflict(ctx context.Context, categoryType, alias string, categoryIDs []int64) error {
	idsJSON, err := json.Marshal(categoryIDs)
	if err != nil {
		return fmt.Errorf("(MID_26060702) marshal conflict ids for %q: %w", alias, err)
	}
	const stmt = `
INSERT INTO kb.category_alias_conflicts (alias, category_type, category_ids, detected_at)
VALUES ($1, $2, $3::jsonb, NOW())
ON CONFLICT (category_type, alias) DO UPDATE
    SET category_ids = EXCLUDED.category_ids,
        detected_at  = NOW()`
	if _, err := r.DB.ExecContext(ctx, stmt, alias, categoryType, string(idsJSON)); err != nil {
		return fmt.Errorf("(MID_26060703) log alias conflict %q: %w", alias, err)
	}
	return nil
}

// normalizeCategoryKeys normalizes a slice of raw keys, dropping empties.
func normalizeCategoryKeys(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if k := normalizeCategoryKey(s); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// categoryStatuses returns the current review status of every category within one
// category_type, keyed by canonical category_key.
func (r artifactCategoryRegistry) categoryStatuses(ctx context.Context, categoryType string) (map[string]string, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT category_key, status FROM kb.artifact_categories WHERE category_type = $1`, categoryType)
	if err != nil {
		return nil, fmt.Errorf("(MID_26060601) load category statuses: %w", err)
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

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyJSONArray(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

func orEmptyJSONObject(v any) any {
	return orEmptyJSONValue(v, map[string]any{})
}

func orEmptyJSONValue(v any, empty any) any {
	if v == nil {
		return empty
	}
	return v
}

// artifactCategoryRecord is an in-memory snapshot row of kb.artifact_categories
// used by the inventory registry's snapshot-style resolution path.
type artifactCategoryRecord struct {
	CategoryID   int64
	CategoryKey  string
	CategoryType string
	Status       string
	CanonicalOf  string
	MatchKeys    []string
	Embedding    []float64
}

// aliasConflict records a normalized key that maps to two or more category IDs.
type aliasConflict struct {
	Alias string
	IDs   []int64
}

// categoryIndex is the process-wide in-memory lookup table that maps
// (categoryType, normalizedKey) → category_id. All writes are protected by a
// RWMutex; reads use RLock. Tests inject a fresh instance per test to avoid
// cross-test pollution via globalCategoryIndex.
type categoryIndex struct {
	mu     sync.RWMutex
	loaded map[string]bool
	byType map[string]map[string]int64 // [categoryType][normalizedKey] → categoryID
	seenBy map[string]map[string]int64 // [categoryType][normalizedKey] → seen_count (conflict resolution)
}

func newCategoryIndex() *categoryIndex {
	return &categoryIndex{
		loaded: map[string]bool{},
		byType: map[string]map[string]int64{},
		seenBy: map[string]map[string]int64{},
	}
}

// globalCategoryIndex is the process-wide singleton used by all categoryResolver
// instances. Tests inject a fresh categoryIndex per test to avoid cross-test pollution.
var globalCategoryIndex = newCategoryIndex()

// lookup returns the category_id for normKey within categoryType, or false if absent.
func (ci *categoryIndex) lookup(categoryType, normKey string) (int64, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	m := ci.byType[categoryType]
	if m == nil {
		return 0, false
	}
	id, ok := m[normKey]
	return id, ok
}

// isLoaded reports whether the index has been populated from the DB for categoryType.
func (ci *categoryIndex) isLoaded(categoryType string) bool {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.loaded[categoryType]
}

// markLoaded marks categoryType as fully loaded from the DB.
func (ci *categoryIndex) markLoaded(categoryType string) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.loaded[categoryType] = true
}

// put adds normKey→id to the index. seenCount is used to resolve conflicts: the entry
// with the higher seen_count wins. Returns true if a conflict was detected (a different
// id already occupied this key).
func (ci *categoryIndex) put(categoryType, normKey string, id, seenCount int64) (conflict bool) {
	if normKey == "" {
		return false
	}
	ci.mu.Lock()
	defer ci.mu.Unlock()
	m := ci.byType[categoryType]
	if m == nil {
		m = map[string]int64{}
		ci.byType[categoryType] = m
		ci.seenBy[categoryType] = map[string]int64{}
	}
	sc := ci.seenBy[categoryType]
	if existing, ok := m[normKey]; ok && existing != id {
		conflict = true
		if seenCount <= sc[normKey] {
			return // existing has higher or equal seen_count; do not overwrite
		}
	}
	m[normKey] = id
	sc[normKey] = seenCount
	return
}

// putAll calls put for each key, using seenCount=0 (appropriate for newly minted
// categories). Empty strings are skipped.
func (ci *categoryIndex) putAll(categoryType string, keys []string, id int64) {
	for _, k := range keys {
		ci.put(categoryType, k, id, 0)
	}
}

// matchCategoryInSnapshot resolves a normalized key against an in-memory snapshot via
// exact match on each record's match_keys (canonical key + all aliases/acronyms).
func matchCategoryInSnapshot(normKey string, snapshot []artifactCategoryRecord) (artifactCategoryRecord, bool) {
	for _, rec := range snapshot {
		for _, mk := range rec.MatchKeys {
			if mk == normKey {
				return rec, true
			}
		}
	}
	return artifactCategoryRecord{}, false
}

// resolveCanonicalCategory follows canonical_of for merged categories so callers
// never attach to a retired category. byKey is keyed by category_key. A visited
// set guards against cycles.
func resolveCanonicalCategory(rec artifactCategoryRecord, byKey map[string]artifactCategoryRecord) artifactCategoryRecord {
	visited := map[string]struct{}{}
	for rec.Status == "merged" && strings.TrimSpace(rec.CanonicalOf) != "" {
		if _, seen := visited[rec.CategoryKey]; seen {
			break
		}
		visited[rec.CategoryKey] = struct{}{}
		next, ok := byKey[rec.CanonicalOf]
		if !ok {
			break
		}
		rec = next
	}
	return rec
}

// createdCategory is the parsed output of the CREATE_ARTIFACT_CATEGORY LLM call.
type createdCategory struct {
	CategoryKey       string
	DisplayNames      []string
	Aliases           []string
	Acronyms          []string
	Description       string
	Keywords          []string
	RequiredAttrs     []any
	Specs             any
	PlausibleRanges   any
	ParentCategories  []string
	RelatedCategories []string
	SearchDocument    string
}

// parseCreateCategoryResponse converts the LLM JSON payload into a createdCategory.
// It requires a non-empty category_key; all other fields are optional.
func parseCreateCategoryResponse(payload map[string]any) (createdCategory, error) {
	var c createdCategory
	c.CategoryKey = strings.TrimSpace(jsonString(payload["category_key"]))
	if c.CategoryKey == "" {
		c.CategoryKey = normalizeCategoryKey(strings.ReplaceAll(strings.TrimSpace(jsonString(payload["canonical_key"])), "_", " "))
		if c.CategoryKey == "" {
			return createdCategory{}, fmt.Errorf("(MID_26060601) create category response missing category_key/canonical_key")
		}
	}
	c.DisplayNames = jsonStringSlice(firstNonNil(payload["display_names"], payload["canonical_name"]))
	c.Aliases = jsonStringSlice(payload["aliases"])
	c.Acronyms = jsonStringSlice(payload["acronyms"])
	c.Description = strings.TrimSpace(jsonString(payload["description"]))
	c.Keywords = jsonStringSlice(payload["keywords"])
	c.RequiredAttrs = jsonArray(firstNonNil(payload["required_attrs"], payload["typical_attributes"]))
	c.Specs = payload["specs"]
	if c.Specs == nil {
		c.Specs = payload["typical_specs"]
	}
	c.PlausibleRanges = firstNonNil(payload["plausible_ranges"], payload["common_value_ranges"])
	c.ParentCategories = normalizeCategoryStringSlice(firstNonNil(payload["parent_categories"], payload["subcategory_of"]))
	c.RelatedCategories = normalizeCategoryStringSlice(payload["related_categories"])
	c.SearchDocument = strings.TrimSpace(jsonString(payload["search_document"]))
	return c, nil
}

// jsonString coerces an arbitrary JSON value to a string (empty for non-strings).
func jsonString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// jsonStringSlice coerces a JSON array (or single string) into a []string of
// trimmed, non-empty values.
func jsonStringSlice(v any) []string {
	var out []string
	switch t := v.(type) {
	case []string:
		for _, s := range t {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	case []any:
		for _, e := range t {
			if s := strings.TrimSpace(jsonString(e)); s != "" {
				out = append(out, s)
			}
		}
	case string:
		if s := strings.TrimSpace(t); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func jsonArray(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	case []string:
		out := make([]any, 0, len(t))
		for _, s := range t {
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func normalizeCategoryStringSlice(v any) []string {
	raw := jsonStringSlice(v)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		norm := normalizeCategoryKey(strings.ReplaceAll(item, "_", " "))
		if norm != "" {
			out = append(out, norm)
		}
	}
	return out
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// normalizeCategoryKey produces the canonical lookup form of a category key:
// lowercase, with every run of non-alphanumeric characters collapsed to a single
// space, and surrounding spaces trimmed. Unicode letters (incl. CJK) are kept, so
// non-English keys survive normalization until the create LLM canonicalizes them.
func normalizeCategoryKey(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// buildCategorySearchDocument assembles the dense English text used by the hybrid
// (lexical + semantic) match channel: category key, display names, aliases,
// acronyms, keywords, then the description. Phrases are joined with spaces in that
// order; case-insensitive duplicate phrases are dropped (first occurrence kept) so
// repeated surface forms do not skew lexical ranking.
func buildCategorySearchDocument(categoryKey string, displayNames, aliases, acronyms, keywords []string, desc string) string {
	parts := make([]string, 0, 2+len(displayNames)+len(aliases)+len(acronyms)+len(keywords))
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		lk := strings.ToLower(s)
		if _, ok := seen[lk]; ok {
			return
		}
		seen[lk] = struct{}{}
		parts = append(parts, s)
	}
	add(categoryKey)
	for _, s := range displayNames {
		add(s)
	}
	for _, s := range aliases {
		add(s)
	}
	for _, s := range acronyms {
		add(s)
	}
	for _, s := range keywords {
		add(s)
	}
	add(desc)
	return strings.Join(parts, " ")
}

// buildArtifactMatchKeys returns the deterministic lookup set for a category: the
// normalized category key plus every normalized display name, alias, and acronym,
// de-duplicated and with empties dropped. Order is key first, then display names,
// aliases, acronyms (first-seen order preserved).
func buildArtifactMatchKeys(categoryKey string, displayNames, aliases, acronyms []string) []string {
	out := make([]string, 0, 1+len(displayNames)+len(aliases)+len(acronyms))
	seen := map[string]struct{}{}
	add := func(s string) {
		k := normalizeCategoryKey(s)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	add(categoryKey)
	for _, s := range displayNames {
		add(s)
	}
	for _, s := range aliases {
		add(s)
	}
	for _, s := range acronyms {
		add(s)
	}
	return out
}
