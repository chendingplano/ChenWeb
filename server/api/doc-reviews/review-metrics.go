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
//
// Per ADR 2026070201 AR2/AR3 each per-metric call carries the canonical
// scheduler window containing the metric's span start as a cacheable prefix,
// and units are executed window-grouped (seed → stagger → remainder). With
// max_tool_turns > 0 the call runs the DR10b tool loop (AR4) instead of the
// one-shot JSON extraction.
type metricsReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	db           *sql.DB
	runID        int64
	logStore     ReviewLogsStore
	maxTasks     int
	maxMatches   int // cap on matching metrics per doc metric (METRIC_REVIEW_MAX_MATCHES)
	maxMetrics   int // cap on doc metrics reviewed; 0 = no cap (METRIC_REVIEW_MAX_METRICS)
}

func (r *metricsReviewer) Name() string             { return "metrics" }
func (r *metricsReviewer) Group() string            { return "P5" }
func (r *metricsReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// metricView is the JSON-serializable subset of a metric sent to the LLM.
type metricView struct {
	MetricID          string   `json:"metric_id,omitempty"`
	MetricName        string   `json:"metric_name,omitempty"`
	MetricNameEn      string   `json:"metric_name_en,omitempty"`
	Subject           string   `json:"metric_subject,omitempty"`
	SubjectEn         string   `json:"metric_subject_en,omitempty"`
	Description       string   `json:"metric_desc,omitempty"`
	DescriptionEn     string   `json:"metric_desc_en,omitempty"`
	Context           string   `json:"metric_context,omitempty"`
	ContextEn         string   `json:"metric_context_en,omitempty"`
	Value             string   `json:"metric_value,omitempty"`
	Unit              string   `json:"metric_unit,omitempty"`
	UnitEn            string   `json:"metric_unit_en,omitempty"`
	ValueDataType     string   `json:"value_data_type,omitempty"`
	ValueRangeType    string   `json:"value_range_type,omitempty"`
	ValueClass        string   `json:"value_class,omitempty"`
	ValueClassEn      string   `json:"value_class_en,omitempty"`
	FormulaDefinition string   `json:"formula_or_definition,omitempty"`
	ThresholdTarget   string   `json:"threshold_or_target,omitempty"`
	Frequency         string   `json:"measurement_frequency,omitempty"`
	LocationType      string   `json:"location_type,omitempty"`
	TableSection      string   `json:"table_name_or_section,omitempty"`
	Categories        []string `json:"metric_categories,omitempty"`
	SourceLineSpans   []string `json:"source_line_spans,omitempty"`
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
	title      string // source document title (authority classification, AR5 §3)
	docNo      string // source document number from kb.inputs.doc_metadata
	via        string // "hybrid_search" | "metric_category" | "entity"
	confidence float64
	context    []map[string]any
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
	r.hydrateMatchedMetricContexts(ctx, matches)

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

	// AR2: load the canonical scheduler windows so each call carries the
	// metric's extraction context as a cacheable prefix. A load failure only
	// disables the window layout (units fall back to the payload-only input).
	windows, err := loadArtifactReviewWindows(ctx, recordID)
	if err != nil {
		r.logger.Warn("metrics review: windows unavailable; reviewing without source context",
			"record_id", recordID, "error", err)
	}

	r.logger.Info("metrics review running",
		"record_id", recordID,
		"metrics", len(docMetrics),
		"reviewed_metrics", len(units),
		"windows", len(windows),
	)

	// AR3: group units by source window and run seed → stagger → remainder.
	execUnits := make([]artifactReviewUnit, len(units))
	for i := range units {
		i := i
		u := units[i]
		wIdx := windowIndexForSpans(u.dm.spans, windows)
		windowJSON := ""
		truncated := false
		if wIdx >= 0 {
			windowJSON = windows[wIdx].inputJSON
			truncated = spansTruncatedByWindow(u.dm.spans, windows[wIdx])
		}
		execUnits[i] = artifactReviewUnit{
			windowIdx: wIdx,
			run: func(workerCtx context.Context) []ReviewFinding {
				return r.reviewMetric(workerCtx, recordID, i, cfg, u.dm, u.matches, windowJSON, truncated)
			},
		}
	}
	return runArtifactUnitsWindowGrouped(ctx, r.maxTasks, execUnits, cfg.OnProgress)
}

// reviewMetric runs one LLM comparison for a single doc metric and its matches.
// windowJSON is the canonical scheduler window containing the metric's span
// start (AR2); empty when no window was resolvable.
func (r *metricsReviewer) reviewMetric(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	dm docMetric,
	ms []matchedMetric,
	windowJSON string,
	truncated bool,
) []ReviewFinding {
	start := time.Now()
	logEntry := ReviewLogEntry{
		InputRecordID: recordID,
		RunID:         r.runID,
		Pass:          "P5",
		Aspect:        "metrics",
		UnitType:      "metric",
		UnitKey:       metricLogUnitKey(dm),
		UnitLocation:  map[string]any{"line_spans": append([]string(nil), dm.spans...)},
		MatchedUnits:  matchedMetricsPayload(ms),
		Detail: map[string]any{
			"metric_index": index,
			"match_count":  len(ms),
			"model_name":   cfg.ModelName,
			"prompt_ref":   cfg.PromptRef,
		},
	}

	payloadObj := map[string]any{
		"metric_under_review": dm.view,
		"artifact_line_spans": dm.spans,
		"matching_metrics":    matchedMetricsPayload(ms),
	}
	if truncated {
		// AR2: the metric's spans extend past the included window; tell the
		// model so it does not misread the cut-off as an extraction error.
		payloadObj["context_truncated"] = true
	}
	payloadJSON := marshalArtifactPayload(payloadObj)
	if payloadJSON == "" {
		r.logger.Warn("metrics review: marshal payload failed", "record_id", recordID, "metric_index", index)
		logEntry.Outcome = "error"
		logEntry.Detail["error"] = "marshal payload failed"
		r.saveReviewLog(ctx, logEntry)
		return nil
	}

	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int
	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		// r.logger.Info("prompt", "promptName", cfg.PromptRef)
		userCtx := artifactReviewToolUserContext(windowJSON, artifactReviewTaskText(cfg.PromptText, payloadJSON))
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger,
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("metrics review tool-use loop failed; no findings for metric",
				"record_id", recordID, "metric_index", index, "error", loopErr)
			logEntry.Detail["error"] = loopErr.Error()
		}
		findings = loopFindings
	} else {
		// AR2 window-first layout: the window is the document input; the rubric
		// plus the per-metric payload form the task tail. Without a window the
		// payload itself is the document input (pre-AR2 layout).
		promptText, inputText := cfg.PromptText, payloadJSON
		if windowJSON != "" {
			promptText, inputText = artifactReviewTaskText(cfg.PromptText, payloadJSON), windowJSON
		}
		out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
			ctx, cfg.PromptRef, promptText, cfg.ModelName, inputText,
			"review_metrics", "MID-CWB-REVIEW-METRICS"))
		if err != nil {
			r.logger.Warn("metrics review metric failed; skipping",
				"record_id", recordID, "metric_index", index, "error", err)
			logEntry.Outcome = "error"
			logEntry.Detail["error"] = err.Error()
			r.saveReviewLog(ctx, logEntry)
			return nil
		}
		findings = normalizeFindingsJSON(out)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}
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
	logEntry.Findings = append([]ReviewFinding(nil), findings...)
	logEntry.Detail["cache_hit_tokens"] = cacheHitTokens
	logEntry.Detail["cache_miss_tokens"] = cacheMissTokens
	logEntry.Detail["ms_used"] = time.Since(start).Milliseconds()
	if truncated {
		logEntry.Detail["context_truncated"] = true
	}
	if len(findings) > 0 {
		logEntry.Outcome = "findings_emitted"
	} else if logEntry.Outcome == "" {
		logEntry.Outcome = "no_issue"
	}
	r.saveReviewLog(ctx, logEntry)

	r.logger.Info("metrics review metric done",
		"record_id", recordID,
		"metric_index", index,
		"metric_id", dm.view.MetricID,
		"matches", len(ms),
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}

func (r *metricsReviewer) saveReviewLog(ctx context.Context, entry ReviewLogEntry) {
	if r.logStore == nil || r.runID == 0 {
		return
	}
	if _, err := r.logStore.SaveLogs(ctx, []ReviewLogEntry{entry}); err != nil && r.logger != nil {
		r.logger.Warn("metrics review: save review log failed",
			"run_id", r.runID,
			"unit_key", entry.UnitKey,
			"error", err,
		)
	}
}

func metricLogUnitKey(dm docMetric) string {
	if id := strings.TrimSpace(dm.view.MetricID); id != "" {
		if dm.id > 0 {
			return fmt.Sprintf("%s#%d", id, dm.id)
		}
		return id
	}
	return fmt.Sprintf("%d", dm.id)
}

// matchedMetricsPayload serializes the matched candidates. Raw RRF scores are
// replaced by 1-based rank (AR5 §4: tiny hybrid-search scores read as
// "unrelated"), and each match carries its source document's authority class
// (AR5 §3) so severity can weight conflicts with governing standards.
func matchedMetricsPayload(ms []matchedMetric) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for i, m := range ms {
		out = append(out, map[string]any{
			"metric":               m.view,
			"source_record_id":     m.recordID,
			"source_filename":      m.filename,
			"source_doc_authority": docAuthorityClass(m.docNo, m.title, m.filename),
			"match_via":            m.via,
			"match_rank":           i + 1,
			"source_context":       m.context,
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
			add(docIdx, matchedMetric{view: tm.view, recordID: tm.recordID, filename: tm.filename, title: tm.title, docNo: tm.docNo, via: "hybrid_search", confidence: h.RRFScore})
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
				add(i, matchedMetric{view: sib.view, recordID: sib.recordID, filename: sib.filename, title: sib.title, docNo: sib.docNo, via: "metric_category"})
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
				add(i, matchedMetric{view: tm.view, recordID: tm.recordID, filename: tm.filename, title: tm.title, docNo: tm.docNo, via: "entity", confidence: e.Confidence})
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
       COALESCE(metric_name_en, ''), COALESCE(metric_subject_en, ''), COALESCE(metric_desc, ''),
       COALESCE(metric_desc_en, ''), COALESCE(metric_context, ''), COALESCE(metric_context_en, ''),
       COALESCE(metric_value, ''), COALESCE(metric_unit, ''), COALESCE(metric_unit_en, ''),
       COALESCE(value_data_type, ''), COALESCE(value_range_type, ''), COALESCE(value_class, ''),
       COALESCE(value_class_en, ''), COALESCE(formula_or_definition, ''), COALESCE(threshold_or_target, ''),
       COALESCE(measurement_frequency, ''), COALESCE(location_type, ''), COALESCE(table_name_or_section, ''),
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
			&dm.view.MetricNameEn, &dm.view.SubjectEn, &dm.view.Description, &dm.view.DescriptionEn,
			&dm.view.Context, &dm.view.ContextEn, &dm.view.Value, &dm.view.Unit, &dm.view.UnitEn,
			&dm.view.ValueDataType, &dm.view.ValueRangeType, &dm.view.ValueClass, &dm.view.ValueClassEn,
			&dm.view.FormulaDefinition, &dm.view.ThresholdTarget, &dm.view.Frequency, &dm.view.LocationType,
			&dm.view.TableSection, &catsText, &spansJSON); err != nil {
			return nil, err
		}
		dm.view.Categories = parseJSONStringArray([]byte(catsText))
		dm.spans = parseJSONStringArray(spansJSON)
		dm.view.SourceLineSpans = append([]string(nil), dm.spans...)
		out = append(out, dm)
	}
	return out, rows.Err()
}

// resolvedMetric is a metric loaded for match resolution.
type resolvedMetric struct {
	view     metricView
	recordID int64
	filename string
	title    string
	docNo    string
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
       COALESCE(m.metric_subject, ''), COALESCE(m.metric_name_en, ''), COALESCE(m.metric_subject_en, ''),
       COALESCE(m.metric_desc, ''), COALESCE(m.metric_desc_en, ''), COALESCE(m.metric_context, ''),
       COALESCE(m.metric_context_en, ''), COALESCE(m.metric_value, ''), COALESCE(m.metric_unit, ''),
       COALESCE(m.metric_unit_en, ''), COALESCE(m.value_data_type, ''), COALESCE(m.value_range_type, ''),
       COALESCE(m.value_class, ''), COALESCE(m.value_class_en, ''), COALESCE(m.formula_or_definition, ''),
       COALESCE(m.threshold_or_target, ''), COALESCE(m.measurement_frequency, ''), COALESCE(m.location_type, ''),
       COALESCE(m.table_name_or_section, ''), COALESCE(m.metric_categories, ''), COALESCE(m.source_line_spans, '[]'::jsonb),
       COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
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
			rm        resolvedMetric
			catsText  string
			spansJSON []byte
		)
		if err := rows.Scan(&rm.view.MetricID, &rm.recordID, &rm.view.MetricName, &rm.view.Subject,
			&rm.view.MetricNameEn, &rm.view.SubjectEn, &rm.view.Description, &rm.view.DescriptionEn,
			&rm.view.Context, &rm.view.ContextEn, &rm.view.Value, &rm.view.Unit, &rm.view.UnitEn,
			&rm.view.ValueDataType, &rm.view.ValueRangeType, &rm.view.ValueClass, &rm.view.ValueClassEn,
			&rm.view.FormulaDefinition, &rm.view.ThresholdTarget, &rm.view.Frequency, &rm.view.LocationType,
			&rm.view.TableSection, &catsText, &spansJSON, &rm.filename, &rm.title, &rm.docNo); err != nil {
			return nil, err
		}
		rm.view.Categories = parseJSONStringArray([]byte(catsText))
		rm.view.SourceLineSpans = parseJSONStringArray(spansJSON)
		out[rm.view.MetricID] = rm
	}
	return out, rows.Err()
}

// metricCategorySiblingLimit bounds the corpus-wide category-sibling scan.
const metricCategorySiblingLimit = 500

func (r *metricsReviewer) loadCategorySiblings(ctx context.Context, recordID int64, cats []string) ([]resolvedMetric, error) {
	const q = `
SELECT COALESCE(m.metric_id, ''), m.input_record_id, COALESCE(m.metric_name, ''),
       COALESCE(m.metric_subject, ''), COALESCE(m.metric_name_en, ''), COALESCE(m.metric_subject_en, ''),
       COALESCE(m.metric_desc, ''), COALESCE(m.metric_desc_en, ''), COALESCE(m.metric_context, ''),
       COALESCE(m.metric_context_en, ''), COALESCE(m.metric_value, ''), COALESCE(m.metric_unit, ''),
       COALESCE(m.metric_unit_en, ''), COALESCE(m.value_data_type, ''), COALESCE(m.value_range_type, ''),
       COALESCE(m.value_class, ''), COALESCE(m.value_class_en, ''), COALESCE(m.formula_or_definition, ''),
       COALESCE(m.threshold_or_target, ''), COALESCE(m.measurement_frequency, ''), COALESCE(m.location_type, ''),
       COALESCE(m.table_name_or_section, ''), COALESCE(m.metric_categories, ''), COALESCE(m.source_line_spans, '[]'::jsonb),
       COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
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
			rm        resolvedMetric
			catsText  string
			spansJSON []byte
		)
		if err := rows.Scan(&rm.view.MetricID, &rm.recordID, &rm.view.MetricName, &rm.view.Subject,
			&rm.view.MetricNameEn, &rm.view.SubjectEn, &rm.view.Description, &rm.view.DescriptionEn,
			&rm.view.Context, &rm.view.ContextEn, &rm.view.Value, &rm.view.Unit, &rm.view.UnitEn,
			&rm.view.ValueDataType, &rm.view.ValueRangeType, &rm.view.ValueClass, &rm.view.ValueClassEn,
			&rm.view.FormulaDefinition, &rm.view.ThresholdTarget, &rm.view.Frequency, &rm.view.LocationType,
			&rm.view.TableSection, &catsText, &spansJSON, &rm.filename, &rm.title, &rm.docNo); err != nil {
			return nil, err
		}
		rm.view.Categories = parseJSONStringArray([]byte(catsText))
		rm.view.SourceLineSpans = parseJSONStringArray(spansJSON)
		out = append(out, rm)
	}
	return out, rows.Err()
}

// artifactSourceContextRadius is the prompt payload context requested for each
// matched artifact: 10 lines before source_line_spans[0], the artifact's actual
// span lines, and 10 lines after source_line_spans[0].
const artifactSourceContextRadius = 10

func (r *metricsReviewer) hydrateMatchedMetricContexts(ctx context.Context, matches map[int][]matchedMetric) {
	if len(matches) == 0 {
		return
	}
	linesByRecord := make(map[int64][]Line)
	failedRecords := make(map[int64]bool)
	for idx, list := range matches {
		for i := range list {
			spans := list[i].view.SourceLineSpans
			if len(spans) == 0 || list[i].recordID <= 0 {
				continue
			}
			lines, ok := linesByRecord[list[i].recordID]
			if !ok {
				if failedRecords[list[i].recordID] {
					continue
				}
				var err error
				lines, err = loadRecordLines(ctx, list[i].recordID)
				if err != nil {
					failedRecords[list[i].recordID] = true
					if r.logger != nil {
						r.logger.Warn("metrics review: matched metric context unavailable",
							"source_record_id", list[i].recordID,
							"metric_id", list[i].view.MetricID,
							"error", err,
						)
					}
					continue
				}
				linesByRecord[list[i].recordID] = lines
			}
			list[i].context = artifactSourceContextLines(lines, spans)
		}
		matches[idx] = list
	}
}

func artifactSourceContextLines(lines []Line, spans []string) []map[string]any {
	if len(lines) == 0 || len(spans) == 0 {
		return nil
	}
	anchorStart := 0
	for _, s := range spans {
		start, _ := parseArtifactSpan(s)
		if start > 0 {
			anchorStart = start
			break
		}
	}
	if anchorStart == 0 {
		return nil
	}
	include := make(map[int]bool)
	for n := anchorStart - artifactSourceContextRadius; n <= anchorStart+artifactSourceContextRadius; n++ {
		if n > 0 {
			include[n] = true
		}
	}
	for _, s := range spans {
		start, end := parseArtifactSpan(s)
		if start == 0 {
			continue
		}
		for n := start; n <= end; n++ {
			include[n] = true
		}
	}
	out := make([]map[string]any, 0, len(include))
	for _, ln := range lines {
		if include[ln.LineNo] {
			out = append(out, map[string]any{
				"line_number": ln.LineNo,
				"content":     ln.Content,
			})
		}
	}
	return out
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
