package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// entitiesReviewer is the cross-document entity consistency reviewer (P5, aspect
// "entities"; ADR 2026063004). Like the metric and provisions reviewers it does not read
// the document body: it loads the document's extracted entities and compares each against
// related entities in OTHER documents. Semantic similarity is discovered LIVE at review
// time via docprocessing.FindSimilarArtifactsOnTheFly (Branch A) rather than from
// precomputed hybrid_search/semantically_related edges — on-the-fly search is always fresh
// and needs no inbound/outbound edge bookkeeping. Branch B is a corpus-wide name scan
// (same-named entities in other documents).
//
// ReviewStrategy StrategyDocument + Input="artifact" routes it to runReviewersLegacy,
// which calls ReviewDocument directly.
type entitiesReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	db         *sql.DB
	maxTasks   int
	maxMatches int // cap on matching entities per doc entity (ENTITY_REVIEW_MAX_MATCHES)
	maxEntity  int // cap on doc entities reviewed; 0 = no cap (ENTITY_REVIEW_MAX_ENTITIES)
}

func (r *entitiesReviewer) Name() string             { return "entities" }
func (r *entitiesReviewer) Group() string            { return "P5" }
func (r *entitiesReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// entityView is the JSON-serializable subset of an entity sent to the LLM. Entities are
// bilingual so both languages are passed for conflict detection across source languages.
type entityView struct {
	EntityID   string   `json:"entity_id,omitempty"`
	Name       string   `json:"entity,omitempty"`
	NameEN     string   `json:"entity_en,omitempty"`
	Type       string   `json:"entity_type,omitempty"`
	TypeEN     string   `json:"entity_type_en,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Desc       string   `json:"desc_text,omitempty"`
	DescEN     string   `json:"desc_text_en,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

// docEntity is one entity extracted from the document under review.
type docEntity struct {
	view  entityView
	spans []string
}

// matchedEntity is a candidate match from another document, with provenance.
type matchedEntity struct {
	view       entityView
	recordID   int64
	filename   string
	via        string // "hybrid_search" | "name"
	confidence float64
}

func (r *entitiesReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("(MID_26063040) entities reviewer: nil db handle")
	}

	docEntities, err := r.loadRecordEntities(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063041) load entities for record %d: %w", recordID, err)
	}
	if len(docEntities) == 0 {
		r.logger.Info("entities review skipped: no entities", "record_id", recordID)
		return nil, nil
	}
	if r.maxEntity > 0 && len(docEntities) > r.maxEntity {
		docEntities = docEntities[:r.maxEntity]
	}

	matches, err := r.buildMatches(ctx, recordID, docEntities)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063042) build entity matches for record %d: %w", recordID, err)
	}

	type reviewUnit struct {
		de      docEntity
		matches []matchedEntity
	}
	var units []reviewUnit
	for i, de := range docEntities {
		if ms := matches[i]; len(ms) > 0 {
			units = append(units, reviewUnit{de: de, matches: ms})
		}
	}
	if len(units) == 0 {
		r.logger.Info("entities review: no cross-document matches", "record_id", recordID, "entities", len(docEntities))
		return nil, nil
	}

	r.logger.Info("entities review running",
		"record_id", recordID,
		"entities", len(docEntities),
		"reviewed_entities", len(units),
	)

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(units), cfg.OnProgress,
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.reviewEntity(workerCtx, recordID, i, cfg, units[i].de, units[i].matches), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	var all []ReviewFinding
	for _, wf := range results {
		all = append(all, wf...)
	}
	return all, nil
}

// reviewEntity runs one LLM comparison for a single doc entity and its matches.
func (r *entitiesReviewer) reviewEntity(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	de docEntity,
	ms []matchedEntity,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"entity_under_review": de.view,
		"matching_entities":   matchedEntitiesPayload(ms),
	}
	inputJSON, err := json.Marshal(payloadObj)
	if err != nil {
		r.logger.Warn("entities review: marshal payload failed", "record_id", recordID, "entity_index", index, "error", err)
		return nil
	}

	out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
		ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, string(inputJSON),
		"review_entities", "MID-CWB-REVIEW-ENTITIES"))
	if err != nil {
		r.logger.Warn("entities review entity failed; skipping",
			"record_id", recordID, "entity_index", index, "error", err)
		return nil
	}

	findings := normalizeFindingsJSON(out)
	loc := strings.Join(de.spans, ",")
	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "entities"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		if findings[i].Location == "" {
			findings[i].Location = loc
		}
	}

	r.logger.Info("entities review entity done",
		"record_id", recordID,
		"entity_index", index,
		"entity_id", de.view.EntityID,
		"matches", len(ms),
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}

func matchedEntitiesPayload(ms []matchedEntity) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"entity":           m.view,
			"source_record_id": m.recordID,
			"source_filename":  m.filename,
			"match_via":        m.via,
			"confidence":       m.confidence,
		})
	}
	return out
}

// entityNameKeys returns the set of normalized (lowercased, trimmed) names by which an
// entity can be matched to a same-named entity in another document: its name, English
// name, and aliases.
func entityNameKeys(v entityView) map[string]struct{} {
	keys := make(map[string]struct{})
	add := func(s string) {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			keys[s] = struct{}{}
		}
	}
	add(v.Name)
	add(v.NameEN)
	for _, a := range v.Aliases {
		add(a)
	}
	return keys
}

// buildMatches loads the artifact-graph inputs and assembles, per doc-entity index, the
// deduped & capped list of matching entities across the two branches (ADR DR1).
func (r *entitiesReviewer) buildMatches(
	ctx context.Context,
	recordID int64,
	docEntities []docEntity,
) (map[int][]matchedEntity, error) {
	// Branch A: semantically-similar entities discovered LIVE (no materialized edges). A
	// single hybrid search per doc entity finds close entities regardless of when the other
	// document was indexed, so no inbound/outbound edge bookkeeping is needed.
	hybridMatches := make(map[int][]docprocessing.OnTheFlySemanticMatch, len(docEntities))
	for i, de := range docEntities {
		if de.view.EntityID == "" {
			continue
		}
		hits, err := docprocessing.FindSimilarArtifactsOnTheFly(ctx, r.db, "entity", de.view.EntityID, "entity", r.maxMatches)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			hybridMatches[i] = hits
		}
	}

	idSet := make(map[string]struct{})
	for _, hits := range hybridMatches {
		for _, h := range hits {
			if h.ArtifactID != "" {
				idSet[h.ArtifactID] = struct{}{}
			}
		}
	}
	resolved, err := r.loadEntitiesByEntityID(ctx, idSet)
	if err != nil {
		return nil, err
	}

	// Branch B: corpus-wide name siblings (same name in other documents).
	nameKeySet := make(map[string]struct{})
	for _, de := range docEntities {
		for k := range entityNameKeys(de.view) {
			nameKeySet[k] = struct{}{}
		}
	}
	var siblings []resolvedEntity
	if len(nameKeySet) > 0 {
		names := make([]string, 0, len(nameKeySet))
		for k := range nameKeySet {
			names = append(names, k)
		}
		if siblings, err = r.loadNameSiblings(ctx, recordID, names); err != nil {
			return nil, err
		}
	}

	return assembleEntityMatches(recordID, docEntities, hybridMatches, resolved, siblings, r.maxMatches), nil
}

// assembleEntityMatches is the pure (DB-free) match-assembly used by buildMatches. It maps
// each doc-entity index to its deduped, cross-document, capped matches.
func assembleEntityMatches(
	recordID int64,
	docEntities []docEntity,
	hybridMatches map[int][]docprocessing.OnTheFlySemanticMatch,
	resolved map[string]resolvedEntity,
	siblings []resolvedEntity,
	maxMatches int,
) map[int][]matchedEntity {
	matches := make(map[int][]matchedEntity)
	dedup := make(map[int]map[string]struct{}) // docIdx -> set of matched entity_ids
	add := func(docIdx int, m matchedEntity) {
		if m.view.EntityID == "" || m.recordID == recordID {
			return // strictly cross-document
		}
		if dedup[docIdx] == nil {
			dedup[docIdx] = make(map[string]struct{})
		}
		if _, seen := dedup[docIdx][m.view.EntityID]; seen {
			return
		}
		dedup[docIdx][m.view.EntityID] = struct{}{}
		matches[docIdx] = append(matches[docIdx], m)
	}

	// Branch A: semantically-similar entities from the live hybrid search, keyed by doc
	// entity index. The add() filter drops same-document and duplicate hits.
	for docIdx, hits := range hybridMatches {
		for _, h := range hits {
			te, ok := resolved[h.ArtifactID]
			if !ok {
				continue
			}
			add(docIdx, matchedEntity{view: te.view, recordID: te.recordID, filename: te.filename, via: "hybrid_search", confidence: h.RRFScore})
		}
	}

	// Branch B: name siblings, attached to every doc entity sharing a name key.
	for _, sib := range siblings {
		sibKeys := entityNameKeys(sib.view)
		for i, de := range docEntities {
			shared := false
			for k := range entityNameKeys(de.view) {
				if _, ok := sibKeys[k]; ok {
					shared = true
					break
				}
			}
			if shared {
				add(i, matchedEntity{view: sib.view, recordID: sib.recordID, filename: sib.filename, via: "name"})
			}
		}
	}

	// Cap each list to maxMatches, highest-confidence first.
	for idx, list := range matches {
		sort.SliceStable(list, func(a, b int) bool { return list[a].confidence > list[b].confidence })
		if maxMatches > 0 && len(list) > maxMatches {
			list = list[:maxMatches]
		}
		matches[idx] = list
	}
	return matches
}

func (r *entitiesReviewer) loadRecordEntities(ctx context.Context, recordID int64) ([]docEntity, error) {
	const q = `
SELECT COALESCE(entity_id, ''), COALESCE(entity, ''), COALESCE(entity_en, ''),
       COALESCE(entity_type, ''), COALESCE(entity_type_en, ''),
       COALESCE(aliases, '[]'::jsonb), COALESCE(aliases_en, '[]'::jsonb),
       COALESCE(desc_text, ''), COALESCE(desc_text_en, ''),
       COALESCE(categories, '[]'::jsonb), COALESCE(line_spans, '[]'::jsonb)
FROM kb.entities
WHERE input_record_id = $1
ORDER BY id`
	rows, err := r.db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []docEntity
	for rows.Next() {
		var de docEntity
		if err := scanEntityRow(rows, &de.view, &de.spans); err != nil {
			return nil, err
		}
		out = append(out, de)
	}
	return out, rows.Err()
}

// resolvedEntity is an entity loaded for match resolution.
type resolvedEntity struct {
	view     entityView
	recordID int64
	filename string
}

func (r *entitiesReviewer) loadEntitiesByEntityID(ctx context.Context, idSet map[string]struct{}) (map[string]resolvedEntity, error) {
	out := make(map[string]resolvedEntity)
	if len(idSet) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	const q = `
SELECT COALESCE(e.entity_id, ''), COALESCE(e.entity, ''), COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc_text, ''), COALESCE(e.desc_text_en, ''),
       COALESCE(e.categories, '[]'::jsonb), e.input_record_id, COALESCE(i.staging_filename, '')
FROM kb.entities e
LEFT JOIN kb.inputs i ON i.id = e.input_record_id
WHERE e.entity_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var re resolvedEntity
		if err := scanResolvedEntityRow(rows, &re); err != nil {
			return nil, err
		}
		out[re.view.EntityID] = re
	}
	return out, rows.Err()
}

// entityNameSiblingLimit bounds the corpus-wide name-sibling scan.
const entityNameSiblingLimit = 500

func (r *entitiesReviewer) loadNameSiblings(ctx context.Context, recordID int64, names []string) ([]resolvedEntity, error) {
	const q = `
SELECT COALESCE(e.entity_id, ''), COALESCE(e.entity, ''), COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc_text, ''), COALESCE(e.desc_text_en, ''),
       COALESCE(e.categories, '[]'::jsonb), e.input_record_id, COALESCE(i.staging_filename, '')
FROM kb.entities e
LEFT JOIN kb.inputs i ON i.id = e.input_record_id
WHERE e.input_record_id <> $1
  AND (lower(e.entity) = ANY($2) OR lower(e.entity_en) = ANY($2))
ORDER BY e.id
LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, recordID, pq.Array(names), entityNameSiblingLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []resolvedEntity
	for rows.Next() {
		var re resolvedEntity
		if err := scanResolvedEntityRow(rows, &re); err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, rows.Err()
}

// scanEntityRow scans the shared entity column projection into a view + spans. Column
// order must match the SELECT lists above (entity fields, then categories, then spans).
func scanEntityRow(rows interface{ Scan(...any) error }, v *entityView, spans *[]string) error {
	var (
		aliasesJSON   []byte
		aliasesENJSON []byte
		catsJSON      []byte
		spansJSON     []byte
	)
	if err := rows.Scan(&v.EntityID, &v.Name, &v.NameEN, &v.Type, &v.TypeEN,
		&aliasesJSON, &aliasesENJSON, &v.Desc, &v.DescEN, &catsJSON, &spansJSON); err != nil {
		return err
	}
	v.Aliases = mergeStringArrays(parseJSONStringArray(aliasesJSON), parseJSONStringArray(aliasesENJSON))
	v.Categories = parseJSONStringArray(catsJSON)
	*spans = parseJSONStringArray(spansJSON)
	return nil
}

// scanResolvedEntityRow scans the resolution projection (entity fields, then
// input_record_id + staging_filename) into a resolvedEntity.
func scanResolvedEntityRow(rows interface{ Scan(...any) error }, re *resolvedEntity) error {
	var (
		aliasesJSON   []byte
		aliasesENJSON []byte
		catsJSON      []byte
	)
	if err := rows.Scan(&re.view.EntityID, &re.view.Name, &re.view.NameEN, &re.view.Type, &re.view.TypeEN,
		&aliasesJSON, &aliasesENJSON, &re.view.Desc, &re.view.DescEN, &catsJSON,
		&re.recordID, &re.filename); err != nil {
		return err
	}
	re.view.Aliases = mergeStringArrays(parseJSONStringArray(aliasesJSON), parseJSONStringArray(aliasesENJSON))
	re.view.Categories = parseJSONStringArray(catsJSON)
	return nil
}

// mergeStringArrays concatenates two string slices, dropping empties and duplicates
// (case-insensitive), preserving first-seen order.
func mergeStringArrays(a, b []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range append(a, b...) {
		key := strings.ToLower(strings.TrimSpace(s))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(s))
	}
	return out
}
