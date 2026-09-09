package productreviews

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
)

// standardDocKinds are the governed document.doc_kind facet values that earn the
// Path C boost (spec: Document scoping — Path C).
var standardDocKinds = map[string]string{
	"da:doc_kind_standard":      "standard",
	"da:doc_kind_specification": "specification",
	"da:doc_kind_regulation":    "regulation",
}

// ScopedDoc is one document in a run's scope set, with the nodes and paths that
// put it there and the fused score that ranks it.
type ScopedDoc struct {
	InputRecordID int64    `json:"input_record_id"`
	FusedScore    float64  `json:"fused_score"`
	NodeIDs       []int64  `json:"matching_node_ids"`
	Paths         []string `json:"matching_paths"`
	Reasons       []string `json:"match_reasons"`
	DocKind       string   `json:"doc_kind"`
}

// DocumentScoper computes the scored document scope set for a profile before
// retrieval (spec: Document scoping). Pure SQL + Go — no LLM.
type DocumentScoper struct {
	DB     *sql.DB
	Config *Config
}

// Scope runs Paths A, B, and C and returns the capped, ranked document set.
func (s DocumentScoper) Scope(ctx context.Context, nodes []ScopeNode) ([]ScopedDoc, error) {
	budgets := s.Config.Budgets.withDefaults()
	acc := map[int64]*ScopedDoc{}
	get := func(rec int64) *ScopedDoc {
		d := acc[rec]
		if d == nil {
			d = &ScopedDoc{InputRecordID: rec}
			acc[rec] = d
		}
		return d
	}

	if err := s.pathA(ctx, nodes, get); err != nil {
		return nil, err
	}
	if err := s.pathB(ctx, nodes, get); err != nil {
		return nil, err
	}
	if err := s.pathC(ctx, keysOfInt(acc), acc); err != nil {
		return nil, err
	}

	out := make([]ScopedDoc, 0, len(acc))
	for _, d := range acc {
		d.NodeIDs = dedupeInts(d.NodeIDs)
		d.Paths = dedupeStrings(d.Paths)
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FusedScore != out[j].FusedScore {
			return out[i].FusedScore > out[j].FusedScore
		}
		return out[i].InputRecordID < out[j].InputRecordID
	})
	if len(out) > budgets.MaxDocuments {
		out = out[:budgets.MaxDocuments]
	}
	return out, nil
}

// pathA — a kb.products row whose identity matches an accepted scope node,
// weighted by relation_type. Aspect nodes match a row whose identity is the
// root product and whose relation_type is one the aspect maps to.
func (s DocumentScoper) pathA(ctx context.Context, nodes []ScopeNode, get func(int64) *ScopedDoc) error {
	var allKeys []string
	rootKeys := map[string]bool{}
	keyToNodes := map[string][]ScopeNode{}
	var aspects []ScopeNode
	for _, n := range nodes {
		if n.isAspect() {
			if len(n.RelationTypes) > 0 {
				aspects = append(aspects, n)
			}
			continue
		}
		for _, k := range n.NameKeys {
			allKeys = append(allKeys, k, strings.ToLower(k))
			keyToNodes[k] = append(keyToNodes[k], n)
			if n.isProduct() {
				rootKeys[k] = true
			}
		}
	}
	allKeys = dedupeStrings(allKeys)
	if len(allKeys) == 0 {
		return nil
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT input_record_id, COALESCE(relation_type, ''),
		       lower(COALESCE(canonical_name, '')), lower(COALESCE(canonical_name_en, ''))
		FROM kb.products
		WHERE status = 'active'
		  AND (lower(canonical_name) = ANY($1) OR lower(canonical_name_en) = ANY($1))`,
		pq.Array(allKeys))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	weight := func(rt string) float64 {
		if w, ok := s.Config.Scoring.RelationTypeWeights[rt]; ok {
			return w
		}
		return 0.3
	}
	for rows.Next() {
		var rec int64
		var rt, name, nameEN string
		if err := rows.Scan(&rec, &rt, &name, &nameEN); err != nil {
			return err
		}
		rowKeys := normalizedKeys([]string{name, nameEN})
		matchedRoot := false
		for _, rk := range rowKeys {
			if rootKeys[rk] {
				matchedRoot = true
			}
			for _, n := range keyToNodes[rk] {
				d := get(rec)
				d.FusedScore += weight(rt)
				d.NodeIDs = append(d.NodeIDs, n.ID)
				d.Paths = append(d.Paths, "product_record")
				d.Reasons = append(d.Reasons, fmt.Sprintf("product record for %q (relation %q)", n.Label, rt))
			}
		}
		if matchedRoot {
			for _, a := range aspects {
				if containsFold(a.RelationTypes, rt) {
					d := get(rec)
					d.FusedScore += weight(rt)
					d.NodeIDs = append(d.NodeIDs, a.ID)
					d.Paths = append(d.Paths, "product_record")
					d.Reasons = append(d.Reasons, fmt.Sprintf("%s aspect: product record with relation %q", a.AspectKey, rt))
				}
			}
		}
	}
	return rows.Err()
}

// pathB — RRF hybrid over the product / summary / topic partitions using each
// node's labels. Join-mode aspect nodes are skipped (they scope via Path A).
func (s DocumentScoper) pathB(ctx context.Context, nodes []ScopeNode, get func(int64) *ScopedDoc) error {
	partitions := []string{"product", "summary", "topic"}
	for _, n := range nodes {
		if n.isAspect() && len(n.RelationTypes) > 0 {
			continue
		}
		hits, err := rrfSearch(ctx, s.DB, partitions, n.labelText(), n.Embedding, hybridCandidateLimit)
		if err != nil {
			return err
		}
		for _, h := range hits {
			d := get(h.InputRecordID)
			d.FusedScore += h.Score
			d.NodeIDs = append(d.NodeIDs, n.ID)
			d.Paths = append(d.Paths, "hybrid_document")
			d.Reasons = append(d.Reasons, fmt.Sprintf("hybrid document match for %q", n.Label))
		}
	}
	return nil
}

// pathC — a configured boost (never a filter) for standards, specifications and
// regulations, from the document.doc_kind facet with an input_doc_type fallback.
func (s DocumentScoper) pathC(ctx context.Context, recIDs []int64, acc map[int64]*ScopedDoc) error {
	if len(recIDs) == 0 {
		return nil
	}
	boost := s.Config.Scoring.StandardsBoost
	classified := map[int64]bool{}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT record_id, COALESCE(value, '')
		FROM kb.doc_facet_values
		WHERE path = 'document.doc_kind' AND record_id = ANY($1)`, pq.Array(recIDs))
	if err != nil {
		return err
	}
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var rec int64
			var val string
			if err = rows.Scan(&rec, &val); err != nil {
				return
			}
			classified[rec] = true
			if kind, ok := standardDocKinds[strings.TrimSpace(val)]; ok {
				acc[rec].DocKind = kind
				acc[rec].FusedScore += boost
			}
		}
		err = rows.Err()
	}()
	if err != nil {
		return err
	}

	var unclassified []int64
	for _, rec := range recIDs {
		if !classified[rec] {
			unclassified = append(unclassified, rec)
		}
	}
	if len(unclassified) == 0 {
		return nil
	}
	frows, err := s.DB.QueryContext(ctx, `
		SELECT record_id, lower(COALESCE(input_doc_type, ''))
		FROM kb.doc_facets WHERE record_id = ANY($1)`, pq.Array(unclassified))
	if err != nil {
		return err
	}
	defer func() { _ = frows.Close() }()
	for frows.Next() {
		var rec int64
		var docType string
		if err := frows.Scan(&rec, &docType); err != nil {
			return err
		}
		for needle, kind := range map[string]string{"standard": "standard", "specification": "specification", "regulation": "regulation"} {
			if strings.Contains(docType, needle) {
				acc[rec].DocKind = kind
				acc[rec].FusedScore += boost
				break
			}
		}
	}
	return frows.Err()
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

func dedupeInts(in []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func keysOfInt(m map[int64]*ScopedDoc) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
