package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
)

// artifactCategoryRegistry is the DB-backed source of truth for the artifact
// category ontology, scoped by category_type. Writes are concurrency-safe upserts
// keyed on (category_type, category_key).
type artifactCategoryRegistry struct {
	DB *sql.DB
}

// loadActiveCategories returns the approved ∪ pending_review categories of a type as
// an in-memory snapshot for match-before-mint resolution.
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
	displayNamesJSON, _ := json.Marshal(c.DisplayNames)
	aliasesJSON, _ := json.Marshal(c.Aliases)
	acronymsJSON, _ := json.Marshal(c.Acronyms)
	keywordsJSON, _ := json.Marshal(c.Keywords)
	requiredJSON, _ := json.Marshal(orEmptySlice(c.RequiredAttrs))
	matchKeysJSON, _ := json.Marshal(matchKeys)
	specsJSON, _ := json.Marshal(orEmptyMap(c.Specs))
	rangesJSON, _ := json.Marshal(orEmptyMap(c.PlausibleRanges))
	embeddingJSON, _ := json.Marshal(embedding)

	const stmt = `
INSERT INTO kb.artifact_categories (
    category_key, category_type, status, display_names, aliases, acronyms,
    category_desc, category_keywords, match_keys, search_document, required_attrs,
    specs, plausible_ranges, embedding, seen_count
) VALUES ($1, $2, 'pending_review', $3::jsonb, $4::jsonb, $5::jsonb, $6, $7::jsonb,
    $8::jsonb, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb, 1)
ON CONFLICT (category_type, category_key) DO UPDATE SET
    seen_count   = kb.artifact_categories.seen_count + 1,
    last_seen_at = NOW()
RETURNING category_id`
	var id int64
	err := r.DB.QueryRowContext(ctx, stmt,
		key, categoryType, string(displayNamesJSON), string(aliasesJSON), string(acronymsJSON),
		c.Description, string(keywordsJSON), string(matchKeysJSON), searchDoc, string(requiredJSON),
		string(specsJSON), string(rangesJSON), string(embeddingJSON),
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

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// categoryEmbeddingMinDims guards against comparing empty/degenerate vectors.
const categoryEmbeddingMinDims = 2

// artifactCategoryRecord is an in-memory snapshot row of kb.artifact_categories
// used for match-before-mint resolution.
type artifactCategoryRecord struct {
	CategoryID   int64
	CategoryKey  string
	CategoryType string
	Status       string
	CanonicalOf  string
	MatchKeys    []string
	Embedding    []float64
}

// matchCategoryInSnapshot resolves a normalized key against an in-memory snapshot.
// Tier 1/2 (deterministic): the key appears in a record's match_keys (which holds
// the canonical key plus all normalized aliases/acronyms). Tier 3 (semantic): the
// best cosine similarity between queryEmbedding and a record's embedding clears
// cosineThreshold. Returns the matched record and true, or false when nothing
// matches.
func matchCategoryInSnapshot(normKey string, queryEmbedding []float64, snapshot []artifactCategoryRecord, cosineThreshold float64) (artifactCategoryRecord, bool) {
	for _, rec := range snapshot {
		for _, mk := range rec.MatchKeys {
			if mk == normKey {
				return rec, true
			}
		}
	}
	if len(queryEmbedding) >= categoryEmbeddingMinDims {
		var best artifactCategoryRecord
		bestScore := 0.0
		found := false
		for _, rec := range snapshot {
			if len(rec.Embedding) != len(queryEmbedding) {
				continue
			}
			if score := categoryCosine(queryEmbedding, rec.Embedding); score > bestScore {
				best, bestScore, found = rec, score, true
			}
		}
		if found && bestScore >= cosineThreshold {
			return best, true
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

// categoryCosine returns the cosine similarity of two equal-length vectors.
func categoryCosine(a, b []float64) float64 {
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

// createdCategory is the parsed output of the CREATE_ARTIFACT_CATEGORY LLM call.
type createdCategory struct {
	CategoryKey     string
	DisplayNames    []string
	Aliases         []string
	Acronyms        []string
	Description     string
	Keywords        []string
	RequiredAttrs   []string
	Specs           map[string]any
	PlausibleRanges map[string]any
	SearchDocument  string
}

// parseCreateCategoryResponse converts the LLM JSON payload into a createdCategory.
// It requires a non-empty category_key; all other fields are optional.
func parseCreateCategoryResponse(payload map[string]any) (createdCategory, error) {
	var c createdCategory
	c.CategoryKey = strings.TrimSpace(jsonString(payload["category_key"]))
	if c.CategoryKey == "" {
		return createdCategory{}, fmt.Errorf("(MID_26060401) create category response missing category_key")
	}
	c.DisplayNames = jsonStringSlice(payload["display_names"])
	c.Aliases = jsonStringSlice(payload["aliases"])
	c.Acronyms = jsonStringSlice(payload["acronyms"])
	c.Description = strings.TrimSpace(jsonString(payload["description"]))
	c.Keywords = jsonStringSlice(payload["keywords"])
	c.RequiredAttrs = jsonStringSlice(payload["required_attrs"])
	c.Specs, _ = payload["specs"].(map[string]any)
	c.PlausibleRanges, _ = payload["plausible_ranges"].(map[string]any)
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
