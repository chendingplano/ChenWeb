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

// metricsReviewer is the cross-document metric consistency reviewer (P5, aspect
// "metrics"; ADR 2026063002). Unlike text reviewers it does not read the document
// body: it loads the document's extracted metrics and compares each against
// semantically-related metrics in OTHER documents. Semantic similarity is discovered
// LIVE at review time via docprocessing.FindSimilarArtifactsOnTheFly (Branch A) rather
// than from precomputed hybrid_search/semantically_related edges — on-the-fly search is
// always fresh and needs no inbound/outbound edge bookkeeping. Branch B adds metrics
// sharing a category key; Branch C adds entity->metric edges.
//
// It uses ReviewStrategy StrategyDocument and Input="artifact", so the prompt-cache
// scheduler routes it to runReviewersLegacy, which calls ReviewDocument directly.
type metricsReviewer struct {
	client     LLMJSONExtractor
	logger     ApiTypes.JimoLogger
	db         *sql.DB
	maxTasks   int
	maxMatches int // cap on matching metrics per doc metric (METRIC_REVIEW_MAX_MATCHES)
	maxMetrics int // cap on doc metrics reviewed; 0 = no cap (METRIC_REVIEW_MAX_METRICS)
}

func (r *metricsReviewer) Name() string             { return "metrics" }
func (r *metricsReviewer) Group() string            { return "P5" }
func (r *metricsReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// metricView is the JSON-serializable subset of a metric sent to the LLM.
type metricView struct {
	MetricID   string   `json:"metric_id,omitempty"`
	MetricName string   `json:"metric_name,omitempty"`
	Subject    string   `json:"metric_subject,omitempty"`
	Value      string   `json:"metric_value,omitempty"`
	Unit       string   `json:"metric_unit,omitempty"`
	ValueClass string   `json:"value_class,omitempty"`
	Categories []string `json:"metric_categories,omitempty"`
}

// docMetric is one metric extracted from the document under review.
type docMetric struct {
	id    int64
	view  metricView
	spans []string
}

// matchedMetric is a candidate match from another document, with provenance.
type matchedMetric struct {
	view       metricView
	recordID   int64
	filename   string
	via        string // "hybrid_search" | "metric_category" | "entity"
	confidence float64
}

func (r *metricsReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("(MID_26063002) metrics reviewer: nil db handle")
	}

	docMetrics, err := r.loadRecordMetrics(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063003) load metrics for record %d: %w", recordID, err)
	}
	if len(docMetrics) == 0 {
		r.logger.Info("metrics review skipped: no metrics", "record_id", recordID)
		return nil, nil
	}
	if r.maxMetrics > 0 && len(docMetrics) > r.maxMetrics {
		docMetrics = docMetrics[:r.maxMetrics]
	}

	matches, err := r.buildMatches(ctx, recordID, docMetrics)
	if err != nil {
		return nil, fmt.Errorf("(MID_26063004) build metric matches for record %d: %w", recordID, err)
	}

	// Only review metrics that have at least one cross-document match.
	type reviewUnit struct {
		dm      docMetric
		matches []matchedMetric
	}
	var units []reviewUnit
	for i, dm := range docMetrics {
		if ms := matches[i]; len(ms) > 0 {
			units = append(units, reviewUnit{dm: dm, matches: ms})
		}
	}
	if len(units) == 0 {
		r.logger.Info("metrics review: no cross-document matches", "record_id", recordID, "metrics", len(docMetrics))
		return nil, nil
	}

	r.logger.Info("metrics review running",
		"record_id", recordID,
		"metrics", len(docMetrics),
		"reviewed_metrics", len(units),
	)

	results, runErr := runReviewerConcurrent(ctx, r.maxTasks, len(units), cfg.OnProgress,
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.reviewMetric(workerCtx, recordID, i, cfg, units[i].dm, units[i].matches), nil
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

// reviewMetric runs one LLM comparison for a single doc metric and its matches.
func (r *metricsReviewer) reviewMetric(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	dm docMetric,
	ms []matchedMetric,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"metric_under_review": dm.view,
		"matching_metrics":    matchedMetricsPayload(ms),
	}
	inputJSON, err := json.Marshal(payloadObj)
	if err != nil {
		r.logger.Warn("metrics review: marshal payload failed", "record_id", recordID, "metric_index", index, "error", err)
		return nil
	}

	out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
		ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, string(inputJSON),
		"review_metrics", "MID-CWB-REVIEW-METRICS"))
	if err != nil {
		r.logger.Warn("metrics review metric failed; skipping",
			"record_id", recordID, "metric_index", index, "error", err)
		return nil
	}

	findings := normalizeFindingsJSON(out)
	loc := strings.Join(dm.spans, ",")
	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "metrics"
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

	r.logger.Info("metrics review metric done",
		"record_id", recordID,
		"metric_index", index,
		"metric_id", dm.view.MetricID,
		"matches", len(ms),
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", reviewLLMCacheHitTokens(r.client),
		"cache_miss_tokens", reviewLLMCacheMissTokens(r.client),
	)
	return findings
}

func matchedMetricsPayload(ms []matchedMetric) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"metric":           m.view,
			"source_record_id": m.recordID,
			"source_filename":  m.filename,
			"match_via":        m.via,
			"confidence":       m.confidence,
		})
	}
	return out
}

// buildMatches loads the artifact-graph inputs and assembles, per doc-metric index,
// the deduped & capped list of matching metrics across the three branches (ADR DR1).
// IO is isolated here; the pure assembly lives in assembleMatches.
func (r *metricsReviewer) buildMatches(
	ctx context.Context,
	recordID int64,
	docMetrics []docMetric,
) (map[int][]matchedMetric, error) {
	catKeySet := make(map[string]struct{})
	for _, dm := range docMetrics {
		for _, c := range dm.view.Categories {
			if c = strings.TrimSpace(c); c != "" {
				catKeySet[c] = struct{}{}
			}
		}
	}

	store := &docprocessing.ConnectionSQLStore{DB: r.db}

	// Branch A: semantically-similar metrics discovered LIVE (no materialized edges). A
	// single hybrid search per doc metric finds close metrics regardless of when the other
	// document was indexed, so no inbound/outbound edge bookkeeping is needed.
	hybridMatches := make(map[int][]docprocessing.OnTheFlySemanticMatch, len(docMetrics))
	for i, dm := range docMetrics {
		if dm.view.MetricID == "" {
			continue
		}
		hits, err := docprocessing.FindSimilarArtifactsOnTheFly(ctx, r.db, "metric", dm.view.MetricID, "metric", r.maxMatches)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			hybridMatches[i] = hits
		}
	}

	// Branch C: entity -> metric edges (any relation method).
	entEdges, err := store.LoadConnectionsBySource(ctx, recordID, "entity", "", "metric")
	if err != nil {
		return nil, err
	}

	// Collect every match-side metric_id needing resolution from kb.metrics.
	idSet := make(map[string]struct{})
	for _, hits := range hybridMatches {
		for _, h := range hits {
			if h.ArtifactID != "" {
				idSet[h.ArtifactID] = struct{}{}
			}
		}
	}
	for _, e := range entEdges {
		if e.TargetID != "" {
			idSet[e.TargetID] = struct{}{}
		}
	}
	resolved, err := r.loadMetricsByMetricID(ctx, idSet)
	if err != nil {
		return nil, err
	}

	var siblings []resolvedMetric
	if len(catKeySet) > 0 {
		cats := make([]string, 0, len(catKeySet))
		for c := range catKeySet {
			cats = append(cats, c)
		}
		if siblings, err = r.loadCategorySiblings(ctx, recordID, cats); err != nil {
			return nil, err
		}
	}

	return assembleMatches(recordID, docMetrics, hybridMatches, entEdges, resolved, siblings, r.maxMatches), nil
}

// assembleMatches is the pure (DB-free) match-assembly used by buildMatches. It maps
// each doc-metric index to its deduped, cross-document, capped matches.
func assembleMatches(
	recordID int64,
	docMetrics []docMetric,
	hybridMatches map[int][]docprocessing.OnTheFlySemanticMatch,
	entEdges []docprocessing.Connection,
	resolved map[string]resolvedMetric,
	siblings []resolvedMetric,
	maxMatches int,
) map[int][]matchedMetric {
	matches := make(map[int][]matchedMetric)
	dedup := make(map[int]map[string]struct{}) // docIdx -> set of matched metric_ids
	add := func(docIdx int, m matchedMetric) {
		if m.view.MetricID == "" || m.recordID == recordID {
			return // strictly cross-document
		}
		if dedup[docIdx] == nil {
			dedup[docIdx] = make(map[string]struct{})
		}
		if _, seen := dedup[docIdx][m.view.MetricID]; seen {
			return
		}
		dedup[docIdx][m.view.MetricID] = struct{}{}
		matches[docIdx] = append(matches[docIdx], m)
	}

	// Branch A: semantically-similar metrics from the live hybrid search, keyed by doc
	// metric index. The add() filter drops same-document and duplicate hits.
	for docIdx, hits := range hybridMatches {
		for _, h := range hits {
			tm, ok := resolved[h.ArtifactID]
			if !ok {
				continue
			}
			add(docIdx, matchedMetric{view: tm.view, recordID: tm.recordID, filename: tm.filename, via: "hybrid_search", confidence: h.RRFScore})
		}
	}

	// Branch B: metrics sharing a category key (corpus-wide).
	for _, sib := range siblings {
		sibCats := make(map[string]struct{}, len(sib.view.Categories))
		for _, c := range sib.view.Categories {
			sibCats[strings.TrimSpace(c)] = struct{}{}
		}
		for i, dm := range docMetrics {
			shared := false
			for _, c := range dm.view.Categories {
				if _, ok := sibCats[strings.TrimSpace(c)]; ok {
					shared = true
					break
				}
			}
			if shared {
				add(i, matchedMetric{view: sib.view, recordID: sib.recordID, filename: sib.filename, via: "metric_category"})
			}
		}
	}

	// Branch C: entity-connected metrics, attached to doc metrics that share a category.
	for _, e := range entEdges {
		tm, ok := resolved[e.TargetID]
		if !ok {
			continue
		}
		tmCats := make(map[string]struct{}, len(tm.view.Categories))
		for _, c := range tm.view.Categories {
			tmCats[strings.TrimSpace(c)] = struct{}{}
		}
		for i, dm := range docMetrics {
			shared := false
			for _, c := range dm.view.Categories {
				if _, ok := tmCats[strings.TrimSpace(c)]; ok {
					shared = true
					break
				}
			}
			if shared {
				add(i, matchedMetric{view: tm.view, recordID: tm.recordID, filename: tm.filename, via: "entity", confidence: e.Confidence})
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

func (r *metricsReviewer) loadRecordMetrics(ctx context.Context, recordID int64) ([]docMetric, error) {
	const q = `
SELECT id, COALESCE(metric_id, ''), COALESCE(metric_name, ''), COALESCE(metric_subject, ''),
       COALESCE(metric_value, ''), COALESCE(metric_unit, ''), COALESCE(value_class, ''),
       COALESCE(metric_categories, ''), COALESCE(source_line_spans, '[]'::jsonb)
FROM kb.metrics
WHERE input_record_id = $1
ORDER BY id`
	rows, err := r.db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []docMetric
	for rows.Next() {
		var (
			dm        docMetric
			catsText  string
			spansJSON []byte
		)
		if err := rows.Scan(&dm.id, &dm.view.MetricID, &dm.view.MetricName, &dm.view.Subject,
			&dm.view.Value, &dm.view.Unit, &dm.view.ValueClass, &catsText, &spansJSON); err != nil {
			return nil, err
		}
		dm.view.Categories = parseJSONStringArray([]byte(catsText))
		dm.spans = parseJSONStringArray(spansJSON)
		out = append(out, dm)
	}
	return out, rows.Err()
}

// resolvedMetric is a metric loaded for match resolution.
type resolvedMetric struct {
	view     metricView
	recordID int64
	filename string
}

func (r *metricsReviewer) loadMetricsByMetricID(ctx context.Context, idSet map[string]struct{}) (map[string]resolvedMetric, error) {
	out := make(map[string]resolvedMetric)
	if len(idSet) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	const q = `
SELECT COALESCE(m.metric_id, ''), m.input_record_id, COALESCE(m.metric_name, ''),
       COALESCE(m.metric_subject, ''), COALESCE(m.metric_value, ''), COALESCE(m.metric_unit, ''),
       COALESCE(m.value_class, ''), COALESCE(m.metric_categories, ''), COALESCE(i.staging_filename, '')
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.metric_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			rm       resolvedMetric
			catsText string
		)
		if err := rows.Scan(&rm.view.MetricID, &rm.recordID, &rm.view.MetricName, &rm.view.Subject,
			&rm.view.Value, &rm.view.Unit, &rm.view.ValueClass, &catsText, &rm.filename); err != nil {
			return nil, err
		}
		rm.view.Categories = parseJSONStringArray([]byte(catsText))
		out[rm.view.MetricID] = rm
	}
	return out, rows.Err()
}

// metricCategorySiblingLimit bounds the corpus-wide category-sibling scan.
const metricCategorySiblingLimit = 500

func (r *metricsReviewer) loadCategorySiblings(ctx context.Context, recordID int64, cats []string) ([]resolvedMetric, error) {
	const q = `
SELECT COALESCE(m.metric_id, ''), m.input_record_id, COALESCE(m.metric_name, ''),
       COALESCE(m.metric_subject, ''), COALESCE(m.metric_value, ''), COALESCE(m.metric_unit, ''),
       COALESCE(m.value_class, ''), COALESCE(m.metric_categories, ''), COALESCE(i.staging_filename, '')
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.input_record_id <> $1
  AND COALESCE(NULLIF(m.metric_categories, '')::jsonb, '[]'::jsonb) ?| $2
ORDER BY m.id
LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, recordID, pq.Array(cats), metricCategorySiblingLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []resolvedMetric
	for rows.Next() {
		var (
			rm       resolvedMetric
			catsText string
		)
		if err := rows.Scan(&rm.view.MetricID, &rm.recordID, &rm.view.MetricName, &rm.view.Subject,
			&rm.view.Value, &rm.view.Unit, &rm.view.ValueClass, &catsText, &rm.filename); err != nil {
			return nil, err
		}
		rm.view.Categories = parseJSONStringArray([]byte(catsText))
		out = append(out, rm)
	}
	return out, rows.Err()
}

// parseJSONStringArray parses a JSON array of strings; returns nil for empty/invalid.
func parseJSONStringArray(raw []byte) []string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "[]" || s == "null" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil
	}
	out := arr[:0]
	for _, v := range arr {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
