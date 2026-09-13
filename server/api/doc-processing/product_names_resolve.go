package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// productNameNormalizer is the same shared surface normalizer the keyword
// module's tiers 0-4 are built on (doc-2026080403 §6, §9.1) -- reused here,
// not re-implemented, so "normalized" means the same thing in both places.
var productNameNormalizer = semid.Normalizer{Version: semid.CurrentNormalizerVersion}

const (
	productNameFuzzyTopN          = 3
	productNameFuzzyMinSimilarity = 0.3
)

// ProductNameCandidate is one tier-5 (fuzzy trigram) near-match recorded on a
// newly proposed kb.product_names row for curator review. A fuzzy hit is
// never sufficient to attach an extraction to an existing row -- D10's bias
// toward under-merging -- it only gets surfaced here.
type ProductNameCandidate struct {
	ID          int64   `json:"id"`
	ProductName string  `json:"product_name"`
	Similarity  float64 `json:"similarity"`
}

// productNameMatch is a tiers-0-2 hit against kb.product_names, carrying the
// matched row's catalog category along so callers don't need a second
// round-trip to categorize the product (see categoryPathsFromCatalog).
type productNameMatch struct {
	ID          int64
	ProductName string
	Exact       bool
	SubCatalog  string
	CategoryL1  string
	CategoryL2  string
}

// productNameResolution is resolveProductName's result: the id to store as
// kb.products.product_name_id, plus that row's catalog category (empty for a
// newly proposed row, which by definition has none yet).
type productNameResolution struct {
	ID         int64
	SubCatalog string
	CategoryL1 string
	CategoryL2 string
}

// productNameKeySetJSON / productNameKeywordsJSON mirror the shape written by
// cmd/product-names-import's normalize.go -- kept as an independent copy
// rather than a shared package because the two callers (a one-off xlsx
// importer, and this pipeline processor) have no other reason to depend on
// each other. Keep the JSON shape in sync if either changes.
type productNameKeySetJSON struct {
	Norm     string `json:"norm"`
	Alnum    string `json:"alnum,omitempty"`
	Sorted   string `json:"sorted,omitempty"`
	Singular string `json:"singular,omitempty"`
	Initials string `json:"initials,omitempty"`
}

type productNameKeywordsJSON struct {
	Zh productNameKeySetJSON  `json:"zh"`
	En *productNameKeySetJSON `json:"en,omitempty"`
}

func toProductNameKeySetJSON(ks semid.KeySet) productNameKeySetJSON {
	return productNameKeySetJSON{Norm: ks.Norm, Alnum: ks.Alnum, Sorted: ks.Sorted, Singular: ks.Singular, Initials: ks.Initials}
}

// ProductNameStore resolves an extract_products-extracted product name
// against kb.product_names -- the keyword module's tier ladder
// (doc-2026080403), ported to this table instead of kb.keyword_surfaces:
// tier 0 exact, tier 1 normalized, tier 2 alternate keys (alnum/sorted/
// singular collapsed into one tier, unlike the 0.8-scored trio in the real
// ladder), tier 5 fuzzy trigram for review-only candidates. Tiers 3/4
// (rewrite rules, initials bridge) and 6 (embedding) are not ported: there is
// no rewrite-rule table or stored embedding for product names.
type ProductNameStore interface {
	LookupTiers(ctx context.Context, name string, keys semid.KeySet) (*productNameMatch, error)
	FuzzyCandidates(ctx context.Context, name string, topN int, minSimilarity float64) ([]ProductNameCandidate, error)
	AppendAlias(ctx context.Context, id int64, alias string) error
	CreateProposed(ctx context.Context, name, nameEN string, keywordsJSON []byte, candidates []ProductNameCandidate) (int64, error)
}

type ProductNameSQLStore struct {
	DB *sql.DB
}

// nullableProductNameID converts an int64 product_name_id (or its absence)
// into a SQL-ready value: a literal 0 must become NULL, never a real FK
// value (kb.product_names.id is a BIGSERIAL starting at 1).
func nullableProductNameID(v any) any {
	id, ok := v.(int64)
	if !ok || id <= 0 {
		return nil
	}
	return id
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// LookupTiers runs tiers 0-2 as one query, ordered by tier so the strongest
// hit wins. Every alternate-key parameter is passed as SQL NULL when the
// query's own key is empty (e.g. Singular is unset for CJK names) -- an
// empty-to-empty match on both sides would otherwise silently pair unrelated
// products that both have blank keys.
func (s ProductNameSQLStore) LookupTiers(ctx context.Context, name string, keys semid.KeySet) (*productNameMatch, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("project db handle is nil")
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, product_name, exact, sub_catalog, category_l1, category_l2 FROM (
			SELECT id, product_name, TRUE AS exact, sub_catalog, category_l1, category_l2, 0 AS tier
			FROM kb.product_names
			WHERE product_name = $1 OR product_name_en = $1
			UNION ALL
			SELECT id, product_name, FALSE, sub_catalog, category_l1, category_l2, 1
			FROM kb.product_names
			WHERE keywords->'zh'->>'norm' = $2 OR keywords->'en'->>'norm' = $2
			UNION ALL
			SELECT id, product_name, FALSE, sub_catalog, category_l1, category_l2, 2
			FROM kb.product_names
			WHERE keywords->'zh'->>'alnum' = $3 OR keywords->'en'->>'alnum' = $3
			   OR keywords->'zh'->>'sorted' = $4 OR keywords->'en'->>'sorted' = $4
			   OR keywords->'zh'->>'singular' = $5 OR keywords->'en'->>'singular' = $5
		) hits
		ORDER BY tier ASC
		LIMIT 1
	`, name, nullIfEmpty(keys.Norm), nullIfEmpty(keys.Alnum), nullIfEmpty(keys.Sorted), nullIfEmpty(keys.Singular))
	var m productNameMatch
	if err := row.Scan(&m.ID, &m.ProductName, &m.Exact, &m.SubCatalog, &m.CategoryL1, &m.CategoryL2); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup product name tiers: %w", err)
	}
	return &m, nil
}

// FuzzyCandidates is tier 5: trigram similarity over both product_name and
// product_name_en (pg_trgm; migration 20260912000003), review-only. It never
// authorizes a match by itself -- callers must treat any result as evidence
// to record, not attach.
func (s ProductNameSQLStore) FuzzyCandidates(ctx context.Context, name string, topN int, minSimilarity float64) ([]ProductNameCandidate, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("project db handle is nil")
	}
	if topN <= 0 {
		topN = productNameFuzzyTopN
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, product_name, GREATEST(similarity(product_name, $1), similarity(product_name_en, $1)) AS sim
		FROM kb.product_names
		WHERE GREATEST(similarity(product_name, $1), similarity(product_name_en, $1)) >= $2
		ORDER BY sim DESC
		LIMIT $3
	`, name, minSimilarity, topN)
	if err != nil {
		return nil, fmt.Errorf("fuzzy candidates: %w", err)
	}
	defer rows.Close()
	var out []ProductNameCandidate
	for rows.Next() {
		var c ProductNameCandidate
		if err := rows.Scan(&c.ID, &c.ProductName, &c.Similarity); err != nil {
			return nil, fmt.Errorf("scan fuzzy candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AppendAlias adds name to id's aliases array if it is not already present.
func (s ProductNameSQLStore) AppendAlias(ctx context.Context, id int64, alias string) error {
	if s.DB == nil {
		return fmt.Errorf("project db handle is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_names
		SET aliases = aliases || to_jsonb($2::text), update_time = NOW()
		WHERE id = $1 AND NOT (aliases @> to_jsonb($2::text))
	`, id, alias)
	if err != nil {
		return fmt.Errorf("append alias: %w", err)
	}
	return nil
}

// CreateProposed inserts a new status='proposed' kb.product_names row for a
// product name that missed tiers 0-2 (D11-style auto-first: a miss must not
// become a hole in the review queue). Race-safe: two concurrent Phase B runs
// proposing the same new name converge on one row via the partial unique
// index on (product_name) WHERE status='proposed' (migration
// 20260912000003) + ON CONFLICT DO NOTHING, then a re-select.
func (s ProductNameSQLStore) CreateProposed(ctx context.Context, name, nameEN string, keywordsJSON []byte, candidates []ProductNameCandidate) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("project db handle is nil")
	}
	extraInfo := []byte(`{}`)
	if len(candidates) > 0 {
		candidatesJSON, err := json.Marshal(candidates)
		if err != nil {
			return 0, fmt.Errorf("marshal fuzzy candidates: %w", err)
		}
		extraInfo, err = json.Marshal(map[string]json.RawMessage{"candidate_matches": candidatesJSON})
		if err != nil {
			return 0, fmt.Errorf("marshal extra_info: %w", err)
		}
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.product_names (seq_no, product_name, product_name_en, keywords, source, status, extra_info)
		VALUES (0, $1, $2, $3::jsonb, 'extract_products', 'proposed', $4::jsonb)
		ON CONFLICT (product_name) WHERE status = 'proposed' DO NOTHING
		RETURNING id
	`, name, nameEN, keywordsJSON, extraInfo).Scan(&id)
	if err == sql.ErrNoRows {
		err = s.DB.QueryRowContext(ctx,
			`SELECT id FROM kb.product_names WHERE product_name = $1 AND status = 'proposed'`, name,
		).Scan(&id)
	}
	if err != nil {
		return 0, fmt.Errorf("create proposed product name: %w", err)
	}
	return id, nil
}

// resolveProductName resolves one extracted product_name against
// kb.product_names and returns the id (for kb.products.product_name_id) plus
// that row's catalog category, if any (zero value if resolution could not
// run, e.g. no DB configured -- this is enrichment, not core extraction, and
// must never block saving the extracted product-relation rows themselves).
// nameEN, if already available from Pass 3a, is only used to enrich a newly
// proposed row -- matching is scoped to name (Pass 2's product_name field)
// alone.
func (p *ProductsProcessor) resolveProductName(ctx context.Context, name, nameEN string) productNameResolution {
	name = strings.TrimSpace(name)
	if name == "" || p.ProductNames == nil {
		return productNameResolution{}
	}
	nameEN = strings.TrimSpace(nameEN)
	keys := productNameNormalizer.Normalize(name)

	match, err := p.ProductNames.LookupTiers(ctx, name, keys)
	if err != nil {
		p.Logger.Warn("product name tier lookup failed; leaving product_name_id unset", "product_name", name, "error", err)
		return productNameResolution{}
	}
	if match != nil {
		if !match.Exact {
			if err := p.ProductNames.AppendAlias(ctx, match.ID, name); err != nil {
				p.Logger.Warn("append product name alias failed", "product_name", name, "id", match.ID, "error", err)
			}
		}
		return productNameResolution{ID: match.ID, SubCatalog: match.SubCatalog, CategoryL1: match.CategoryL1, CategoryL2: match.CategoryL2}
	}

	// Miss on tiers 0-2. A tier-5 fuzzy hit is recorded for review, never
	// auto-attached (D10). A newly proposed row has no catalog category yet.
	candidates, err := p.ProductNames.FuzzyCandidates(ctx, name, productNameFuzzyTopN, productNameFuzzyMinSimilarity)
	if err != nil {
		p.Logger.Warn("product name fuzzy lookup failed", "product_name", name, "error", err)
		candidates = nil
	}
	payload := productNameKeywordsJSON{Zh: toProductNameKeySetJSON(keys)}
	if nameEN != "" {
		enKeys := toProductNameKeySetJSON(productNameNormalizer.Normalize(nameEN))
		payload.En = &enKeys
	}
	keywordsJSON, err := json.Marshal(payload)
	if err != nil {
		p.Logger.Warn("marshal product name keywords failed", "product_name", name, "error", err)
		return productNameResolution{}
	}
	id, err := p.ProductNames.CreateProposed(ctx, name, nameEN, keywordsJSON, candidates)
	if err != nil {
		p.Logger.Warn("create proposed product name failed", "product_name", name, "error", err)
		return productNameResolution{}
	}
	return productNameResolution{ID: id}
}
