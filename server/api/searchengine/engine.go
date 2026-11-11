package searchengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/lib/pq"
)

// Search runs one hybrid search: a keyword list and a vector list, both filtered before ranking,
// fused by reciprocal rank, then reduced by the caller's criteria.
func Search(ctx context.Context, db *sql.DB, cfg Config, req Request) (*Response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	limit := req.Options.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var ref *refRow
	if req.Query.Ref != nil {
		var err error
		if ref, err = loadRef(ctx, db, *req.Query.Ref); err != nil {
			return nil, err
		}
	}

	vecLiteral := queryVector(ctx, cfg, req, ref)
	if req.Criteria.MinCosine > 0 && vecLiteral == "" {
		return nil, fmt.Errorf("searchengine: a minimum cosine needs a query vector, and none is available (semantic search off, or no embedding model)")
	}
	spec := resolveKeywordList(ctx, cfg, req, ref)

	resp := &Response{}
	for _, k := range spec.keywords {
		resp.Keywords = append(resp.Keywords, k.Term)
	}

	var lexical []lexHit
	if !spec.empty() {
		var err error
		lexical, err = lexicalList(ctx, db, cfg, spec, req.Filters, req.Query.Ref)
		if err != nil {
			return nil, fmt.Errorf("searchengine: keyword list: %w", err)
		}
	}
	var vector []vecHit
	if vecLiteral != "" {
		var err error
		vector, err = vectorList(ctx, db, vecLiteral, req.Filters, req.Query.Ref)
		if err != nil {
			return nil, fmt.Errorf("searchengine: vector list: %w", err)
		}
	}

	fused := fuse(cfg, lexical, vector)
	resp.Candidates = len(fused)
	if err := applyCriteria(ctx, db, cfg, vecLiteral, req.Criteria, fused, resp); err != nil {
		return nil, err
	}
	kept := fused[:0:0]
	for _, c := range fused {
		if !c.removed {
			kept = append(kept, c)
		}
	}
	if len(kept) > limit {
		kept = kept[:limit]
	}
	results, err := loadResults(ctx, db, kept)
	if err != nil {
		return nil, err
	}
	resp.Results = results
	return resp, nil
}

// lexicalList runs the keyword list on the configured backend. A ParadeDB failure falls back to
// Postgres full text, as the HTTP search does.
func lexicalList(ctx context.Context, db *sql.DB, cfg Config, s lexSpec, f Filters, self *ArtifactRef) ([]lexHit, error) {
	if cfg.Backend != BackendPostgres {
		hits, err := lexicalParadeDB(ctx, db, s, f, self)
		if err == nil {
			return hits, nil
		}
		if s.mlt != "" || s.raw == "" && len(s.keywords) == 0 {
			return nil, err
		}
	}
	return lexicalPostgres(ctx, db, cfg.Dictionary, s, f, self)
}

// refRow is the stored data of a referenced artifact.
type refRow struct {
	doc       string
	keywords  []string
	embedding string // pgvector text form, empty when not stored
}

func loadRef(ctx context.Context, db *sql.DB, r ArtifactRef) (*refRow, error) {
	var doc string
	var kws []string
	var emb sql.NullString
	err := db.QueryRowContext(ctx, `SELECT COALESCE(search_document, ''), COALESCE(keywords, '{}'), embedding::text
		FROM kb.search_artifacts WHERE artifact_type = $1 AND artifact_id = $2`, r.Type, r.ID).
		Scan(&doc, pq.Array(&kws), &emb)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s/%s", ErrReferenceNotIndexed, r.Type, r.ID)
	}
	if err != nil {
		return nil, err
	}
	return &refRow{doc: doc, keywords: kws, embedding: emb.String}, nil
}

// queryVector returns the query vector as a pgvector literal, or "" when there is none (semantic
// search off, no embedding model, or nothing to embed). A reference uses its stored embedding.
func queryVector(ctx context.Context, cfg Config, req Request, ref *refRow) string {
	if !cfg.SemanticEnabled {
		return ""
	}
	text := strings.TrimSpace(req.Query.Text)
	if ref != nil {
		if ref.embedding != "" {
			return ref.embedding
		}
		text = strings.TrimSpace(ref.doc)
	}
	embed := currentEmbedder()
	if text == "" || embed == nil {
		return ""
	}
	vec, ok := embed(ctx, text)
	if !ok || len(vec) == 0 {
		return ""
	}
	return kbsearch.FormatVectorLiteral(vec)
}

// resolveKeywordList chooses what the keyword list searches for: the caller's keywords; else, for a
// text query, extracted keywords (unless off or unavailable) and then the raw text; else, for a
// reference, its stored keywords, then terms selected from its stored text.
func resolveKeywordList(ctx context.Context, cfg Config, req Request, ref *refRow) lexSpec {
	if kws := req.Criteria.listKeywords(); len(kws) > 0 {
		return lexSpec{keywords: kws}
	}
	if ref == nil {
		text := strings.TrimSpace(req.Query.Text)
		if req.Options.KeywordExtraction != KeywordExtractionOff {
			if extract := currentExtractor(); extract != nil {
				if kws := cleanKeywords(extract(ctx, text, req.Options.KeywordExtraction == KeywordExtractionOn)); len(kws) > 0 {
					return lexSpec{keywords: weighted(kws)}
				}
			}
		}
		return lexSpec{raw: text}
	}
	if kws := cleanKeywords(ref.keywords); len(kws) > 0 {
		return lexSpec{keywords: weighted(kws)}
	}
	if strings.TrimSpace(ref.doc) == "" {
		return lexSpec{}
	}
	if cfg.Backend == BackendPostgres {
		return lexSpec{keywords: weighted(frequentTerms(ref.doc, 8))}
	}
	return lexSpec{mlt: ref.doc}
}

func weighted(terms []string) []Keyword {
	out := make([]Keyword, len(terms))
	for i, t := range terms {
		out[i] = Keyword{Term: t, Weight: 1}
	}
	return out
}

// fusedCand is one candidate after fusion.
type fusedCand struct {
	candKey
	score   float64
	diag    Diagnostics
	removed bool
}

// fuse merges the two lists by reciprocal rank: wl/(60+lexical_rank) + wv/(60+vector_rank), a
// missing rank adding 0. Ties break by artifact id.
func fuse(cfg Config, lexical []lexHit, vector []vecHit) []*fusedCand {
	byKey := map[candKey]*fusedCand{}
	get := func(k candKey) *fusedCand {
		c := byKey[k]
		if c == nil {
			c = &fusedCand{candKey: k}
			byKey[k] = c
		}
		return c
	}
	for i, h := range lexical {
		c := get(candKey{h.typ, h.id})
		rank, score := i+1, h.score
		c.diag.LexicalRank, c.diag.LexicalScore = &rank, &score
		c.score += cfg.LexicalWeight / float64(rrfK+rank)
	}
	for i, h := range vector {
		c := get(h.candKey)
		rank := i + 1
		c.diag.VectorRank = &rank
		c.score += cfg.SemanticWeight / float64(rrfK+rank)
	}
	out := make([]*fusedCand, 0, len(byKey))
	for _, c := range byKey {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].id < out[j].id
	})
	return out
}

// applyCriteria computes diagnostics for every fused candidate (cosine, matched keywords) and marks
// candidates that fail a criterion, counting each under the first criterion it fails: excluded,
// required, minimum matched, minimum cosine.
func applyCriteria(ctx context.Context, db *sql.DB, cfg Config, vecLiteral string, c Criteria, cands []*fusedCand, resp *Response) error {
	if len(cands) == 0 {
		return nil
	}
	keys := make([]candKey, len(cands))
	for i, fc := range cands {
		keys[i] = fc.candKey
	}

	var cos map[candKey]float64
	if vecLiteral != "" {
		var err error
		if cos, err = cosines(ctx, db, vecLiteral, keys); err != nil {
			return fmt.Errorf("searchengine: cosine: %w", err)
		}
		for _, fc := range cands {
			if v, ok := cos[fc.candKey]; ok {
				v := v
				fc.diag.Cosine = &v
			}
		}
	}

	var terms []string
	for _, k := range c.Keywords {
		if t := strings.TrimSpace(k.Term); t != "" {
			terms = append(terms, t)
		}
	}
	var matched []map[candKey]bool
	if len(terms) > 0 {
		var err error
		if matched, err = keywordMatches(ctx, db, cfg.Backend, cfg.Dictionary, terms, keys); err != nil {
			return fmt.Errorf("searchengine: keyword criteria: %w", err)
		}
	}

	for _, fc := range cands {
		var hit []string
		excluded, requiredMissing := false, false
		ti := 0
		for _, k := range c.Keywords {
			if strings.TrimSpace(k.Term) == "" {
				continue
			}
			m := matched[ti][fc.candKey]
			ti++
			switch {
			case k.Excluded && m:
				excluded = true
			case !k.Excluded && m:
				hit = append(hit, strings.TrimSpace(k.Term))
			case !k.Excluded && k.Required && !m:
				requiredMissing = true
			}
		}
		fc.diag.MatchedKeywords = hit
		switch {
		case excluded:
			resp.Removed.Excluded++
			fc.removed = true
		case requiredMissing:
			resp.Removed.Required++
			fc.removed = true
		case c.MinMatched > 0 && len(hit) < c.MinMatched:
			resp.Removed.MinMatched++
			fc.removed = true
		case c.MinCosine > 0 && (fc.diag.Cosine == nil || *fc.diag.Cosine < c.MinCosine):
			resp.Removed.MinCosine++
			fc.removed = true
		}
	}
	return nil
}

// loadResults loads the display fields of the kept candidates and assigns ranks.
func loadResults(ctx context.Context, db *sql.DB, kept []*fusedCand) ([]Result, error) {
	if len(kept) == 0 {
		return nil, nil
	}
	types := make([]string, len(kept))
	ids := make([]string, len(kept))
	for i, c := range kept {
		types[i], ids[i] = c.typ, c.id
	}
	rows, err := db.QueryContext(ctx, `
SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
	COALESCE(sa.primary_label, ''), COALESCE(sa.secondary_label, ''),
	COALESCE(sa.source_title, ''), COALESCE(sa.source_filename, ''),
	COALESCE(sa.source_line_spans, '[]'::jsonb), COALESCE(sa.semantic_payload, '{}'::jsonb), COALESCE(sa.keywords, '{}'),
	COALESCE(NULLIF(sa.snippet_basis, ''), NULLIF(sa.search_document, ''), COALESCE(sa.primary_label, ''))
FROM kb.search_artifacts sa
JOIN unnest($1::text[], $2::text[]) u(t, i) ON sa.artifact_type = u.t AND sa.artifact_id = u.i`,
		pq.Array(types), pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("searchengine: load results: %w", err)
	}
	defer rows.Close()
	byKey := map[candKey]Result{}
	for rows.Next() {
		var r Result
		var spans, payload []byte
		if err := rows.Scan(&r.ArtifactType, &r.ArtifactID, &r.InputRecordID, &r.PrimaryLabel, &r.SecondaryLabel,
			&r.SourceTitle, &r.SourceFilename, &spans, &payload, pq.Array(&r.Keywords), &r.Snippet); err != nil {
			return nil, err
		}
		r.SourceLineSpans, r.SemanticPayload = json.RawMessage(spans), json.RawMessage(payload)
		byKey[candKey{r.ArtifactType, r.ArtifactID}] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(kept))
	for _, c := range kept {
		r, ok := byKey[c.candKey]
		if !ok {
			continue // the row vanished between the lists and this read
		}
		r.Rank, r.Score, r.Diag = len(out)+1, c.score, c.diag
		out = append(out, r)
	}
	return out, nil
}
