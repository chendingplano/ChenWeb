package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

// metricsCompletenessReviewer implements ADR 2026070201 AR6: object-anchored
// missing-metric detection. It runs as a separate pass from the metrics
// conflict reviewer, comparing the metrics a document attaches to each object
// against what peer documents attach to the same canonical object (or
// comparable objects sharing the same type and normalized names).
//
// Each object generates one LLM call; calls carry the AR2 source window as a
// cacheable prefix and run under the AR4 tool-use loop (search_metrics is
// available to verify absence before claiming it).
type metricsCompletenessReviewer struct {
	client       LLMJSONExtractor
	toolClient   LLMChatClient
	toolRegistry map[string]ReviewTool
	logger       ApiTypes.JimoLogger
	db           *sql.DB
	maxTasks     int
	maxObjects   int // cap on objects reviewed; 0 = no cap
}

func (r *metricsCompletenessReviewer) Name() string             { return "metrics_completeness" }
func (r *metricsCompletenessReviewer) Group() string            { return "P5" }
func (r *metricsCompletenessReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// ── Per-object roster ──────────────────────────────────────────────────────

// peerDocMetrics is the metric set one peer document attaches to an object.
type peerDocMetrics struct {
	recordID  int64
	filename  string
	title     string
	docNo     string
	authority string       // docAuthorityClass
	metrics   []metricView // metrics this peer attaches to the object
}

// objectMetricRoster is the review unit for one object: what the doc under
// review says vs. what peer documents say about the same (or comparable)
// object.
type objectMetricRoster struct {
	objectID    string
	objectName  string
	objectType  string
	description string
	// AR2 window mapping (first window that covers any of the doc metrics).
	windowIdx  int
	windowJSON string
	truncated  bool
	// docMetrics are the metrics the document under review attaches to this
	// object. They may be empty (the doc mentions the object but lacks
	// metrics for it — a strong signal in itself).
	docMetrics []metricView
	docSpan    []string
	// peerDocs are peer documents that attach metrics to this (or comparable)
	// objects, grouped by document.
	peerDocs         []peerDocMetrics
	totalPeerDocs    int
	totalPeerMetrics int
}

// ── ReviewDocument entry point ─────────────────────────────────────────────

func (r *metricsCompletenessReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	if r.db == nil {
		return nil, fmt.Errorf("(MID_26070301) metrics_completeness reviewer: nil db handle")
	}

	rosters, err := r.buildRosters(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26070302) build object metric rosters for record %d: %w", recordID, err)
	}
	if len(rosters) == 0 {
		r.logger.Info("metrics_completeness review skipped: no objects with peer metrics", "record_id", recordID)
		return nil, nil
	}

	// AR2: load the canonical scheduler windows.
	windows, err := loadArtifactReviewWindows(ctx, recordID)
	if err != nil {
		r.logger.Warn("metrics_completeness: windows unavailable; reviewing without source context",
			"record_id", recordID, "error", err)
	}

	r.logger.Info("metrics_completeness review running",
		"record_id", recordID,
		"objects", len(rosters),
		"windows", len(windows),
	)

	// Resolve window idx and JSON per roster.
	for i := range rosters {
		rosters[i].windowIdx = windowIndexForSpans(rosters[i].docSpan, windows)
		if rosters[i].windowIdx >= 0 {
			rosters[i].windowJSON = windows[rosters[i].windowIdx].inputJSON
			rosters[i].truncated = spansTruncatedByWindow(rosters[i].docSpan, windows[rosters[i].windowIdx])
		}
	}

	// AR3: build units and run window-grouped.
	execUnits := make([]artifactReviewUnit, len(rosters))
	for i := range rosters {
		i := i
		ro := rosters[i]
		execUnits[i] = artifactReviewUnit{
			windowIdx: ro.windowIdx,
			run: func(workerCtx context.Context) []ReviewFinding {
				return r.reviewObject(workerCtx, recordID, i, cfg, ro)
			},
		}
	}
	return runArtifactUnitsWindowGrouped(ctx, r.maxTasks, execUnits, cfg, r.Name(), r.logger, recordID, cfg.OnProgress)
}

// reviewObject runs one LLM call for a single object, comparing what the doc
// under review has against what peers have.
func (r *metricsCompletenessReviewer) reviewObject(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	ro objectMetricRoster,
) []ReviewFinding {
	start := time.Now()

	payloadObj := map[string]any{
		"object": map[string]any{
			"object_id":   ro.objectID,
			"object_name": ro.objectName,
			"object_type": ro.objectType,
			"description": ro.description,
		},
		"doc_metrics":         ro.docMetrics,
		"peer_docs":           ro.peerDocsForPayload(),
		"total_peer_docs":     ro.totalPeerDocs,
		"total_peer_metrics":  ro.totalPeerMetrics,
		"artifact_line_spans": ro.docSpan,
	}
	if ro.truncated {
		payloadObj["context_truncated"] = true
	}
	payloadJSON := marshalArtifactPayload(payloadObj)
	if payloadJSON == "" {
		r.logger.Warn("metrics_completeness: marshal payload failed", "record_id", recordID, "object_index", index)
		return nil
	}

	var findings []ReviewFinding
	var cacheHitTokens, cacheMissTokens int
	if cfg.MaxToolTurns > 0 && r.toolClient != nil {
		tools := selectTools(r.toolRegistry, cfg.Tools)
		userCtx := artifactReviewToolUserContext(ro.windowJSON, artifactReviewTaskText(cfg.PromptText, payloadJSON))
		callInfo := docReviewCallInfo(ctx, map[string]any{"object_id": ro.objectID})
		loopFindings, loopUsage, loopErr := runToolUseReview(
			ctx, r.toolClient, cfg.ModelName, cfg, cfg.PromptText,
			userCtx, tools, recordID, r.logger, "review_metrics_completeness", callInfo, "MID-20260706-010",
		)
		if loopUsage != nil {
			cacheHitTokens = loopUsage.PromptCacheHitTokens
			cacheMissTokens = loopUsage.PromptCacheMissTokens
		}
		if loopErr != nil {
			r.logger.Warn("metrics_completeness tool-use loop failed; no findings for object",
				"record_id", recordID, "object_index", index, "error", loopErr)
		}
		findings = loopFindings
	} else {
		promptText, inputText := cfg.PromptText, payloadJSON
		if ro.windowJSON != "" {
			promptText, inputText = artifactReviewTaskText(cfg.PromptText, payloadJSON), ro.windowJSON
		}
		out, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(
			ctx, cfg.PromptRef, promptText, cfg.ModelName, inputText,
			"review_metrics_completeness", "MID-CWB-REVIEW-METRICS-COMPLETENESS"))
		if err != nil {
			r.logger.Warn("metrics_completeness object failed; skipping",
				"record_id", recordID, "object_index", index, "error", err)
			return nil
		}
		findings = normalizeFindingsJSON(out, cfg.ModelName)
		cacheHitTokens = reviewLLMCacheHitTokens(r.client)
		cacheMissTokens = reviewLLMCacheMissTokens(r.client)
	}

	for i := range findings {
		findings[i].Pass = "P5"
		findings[i].Aspect = "metrics_completeness"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "missing_metric"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "medium"
		}
		if findings[i].Location == "" {
			findings[i].Location = strings.Join(ro.docSpan, ",")
		}
	}

	r.logger.Info("metrics_completeness object done",
		"record_id", recordID,
		"object_index", index,
		"object_id", ro.objectID,
		"object_name", ro.objectName,
		"doc_metrics", len(ro.docMetrics),
		"peer_docs", ro.totalPeerDocs,
		"peer_metrics", ro.totalPeerMetrics,
		"findings", len(findings),
		"ms_used", time.Since(start).Milliseconds(),
		"cache_hit_tokens", cacheHitTokens,
		"cache_miss_tokens", cacheMissTokens,
	)
	return findings
}

func (ro objectMetricRoster) peerDocsForPayload() []map[string]any {
	out := make([]map[string]any, 0, len(ro.peerDocs))
	for _, pd := range ro.peerDocs {
		out = append(out, map[string]any{
			"source_record_id":     pd.recordID,
			"source_filename":      pd.filename,
			"source_doc_authority": pd.authority,
			"metrics":              pd.metrics,
		})
	}
	return out
}

// ── Roster building (the object-anchored algorithm) ────────────────────────

// buildRosters implements the algorithm at ADR 2026070201 AR6:
//
//	For each metric in the doc under review:
//	  Retrieve its artifact_id (metric_id)
//	  Retrieve object_id from kb.artifact_objects WHERE artifact_id = metric_id
//	  Retrieve all peer artifact_ids from kb.artifact_connections
//	    WHERE source_type = 'metric' AND target_id = object_id
//
// Metrics without a reconciled object (NULL object_id) are skipped.
// Objects with no peer metrics are filtered out (no expectation to check).
func (r *metricsCompletenessReviewer) buildRosters(
	ctx context.Context,
	recordID int64,
) ([]objectMetricRoster, error) {
	// Step 1a: load doc metrics from kb.metrics.
	docMetrics, err := r.loadDocMetrics(ctx, recordID)
	if err != nil {
		return nil, err
	}
	if len(docMetrics) == 0 {
		return nil, nil
	}

	// Step 1b: for each metric, find its object_id(s) via kb.artifact_objects.
	metricIDs := make([]string, 0, len(docMetrics))
	for _, dm := range docMetrics {
		if dm.view.MetricID != "" {
			metricIDs = append(metricIDs, dm.view.MetricID)
		}
	}
	if len(metricIDs) == 0 {
		return nil, nil
	}

	objLinks, err := r.loadObjectLinks(ctx, recordID, metricIDs)
	if err != nil {
		return nil, err
	}

	// Group doc metrics by object_id. A metric linked to multiple objects
	// appears under each object.
	type objGroup struct {
		objectName  string
		objectType  string
		description string
		metrics     []docMetric // doc's metrics for this object
	}
	byObject := make(map[string]*objGroup)
	for _, ol := range objLinks {
		grp, ok := byObject[ol.objectID]
		if !ok {
			grp = &objGroup{
				objectName:  ol.objectName,
				objectType:  ol.objectType,
				description: ol.description,
			}
			byObject[ol.objectID] = grp
		}
		if dm, ok := docMetrics[ol.metricID]; ok {
			grp.metrics = append(grp.metrics, dm)
		}
	}

	if r.maxObjects > 0 && len(byObject) > r.maxObjects {
		// Trim to maxObjects, keeping objects with the most doc metrics.
		type objCount struct {
			id    string
			count int
		}
		counts := make([]objCount, 0, len(byObject))
		for id, grp := range byObject {
			counts = append(counts, objCount{id: id, count: len(grp.metrics)})
		}
		sort.SliceStable(counts, func(a, b int) bool { return counts[a].count > counts[b].count })
		for _, oc := range counts[r.maxObjects:] {
			delete(byObject, oc.id)
		}
	}

	// Step 1c: for each object, find peer artifacts from artifact_connections.
	// Also find comparable objects (same type, overlapping normalized names).
	objectIDs := make([]string, 0, len(byObject))
	for id := range byObject {
		objectIDs = append(objectIDs, id)
	}
	peerEdges, compObjects, err := r.loadPeerObjectEdges(ctx, objectIDs)
	if err != nil {
		return nil, err
	}

	// Resolve peer artifact_ids to actual metric data.
	allPeerMetrics, err := r.loadResolvedMetrics(ctx, peerEdges, compObjects)
	if err != nil {
		return nil, err
	}

	// Build rosters.
	var rosters []objectMetricRoster
	for objID, grp := range byObject {
		ro := objectMetricRoster{
			objectID:    objID,
			objectName:  grp.objectName,
			objectType:  grp.objectType,
			description: grp.description,
		}
		// Collect doc's metrics as metricViews and the earliest span.
		for _, dm := range grp.metrics {
			ro.docMetrics = append(ro.docMetrics, dm.view)
			ro.docSpan = mergeSpans(ro.docSpan, dm.spans)
		}
		// Collect peer metrics grouped by document.
		peerByDoc := make(map[int64]*peerDocMetrics)
		addPeer := func(rm resolvedObjectMetric) {
			if rm.recordID == recordID {
				return // exclude own document
			}
			pd, ok := peerByDoc[rm.recordID]
			if !ok {
				pd = &peerDocMetrics{
					recordID:  rm.recordID,
					filename:  rm.filename,
					title:     rm.title,
					docNo:     rm.docNo,
					authority: docAuthorityClass(rm.docNo, rm.title, rm.filename),
				}
				peerByDoc[rm.recordID] = pd
			}
			pd.metrics = append(pd.metrics, rm.view)
		}
		for _, rm := range allPeerMetrics[objID] {
			addPeer(rm)
		}
		// Include metrics from comparable objects too (deduped by metric_id per peer doc).
		for _, compObjID := range compObjects[objID] {
			for _, rm := range allPeerMetrics[compObjID] {
				addPeer(rm)
			}
		}
		// Sort peer docs for deterministic output.
		var peerList []peerDocMetrics
		for _, pd := range peerByDoc {
			peerList = append(peerList, *pd)
		}
		sort.SliceStable(peerList, func(a, b int) bool {
			return peerList[a].recordID < peerList[b].recordID
		})
		ro.peerDocs = peerList
		ro.totalPeerDocs = len(peerList)
		for _, pd := range peerList {
			ro.totalPeerMetrics += len(pd.metrics)
		}
		if ro.totalPeerDocs > 0 {
			rosters = append(rosters, ro)
		}
	}

	// Stable order for testability.
	sort.SliceStable(rosters, func(a, b int) bool { return rosters[a].objectID < rosters[b].objectID })
	return rosters, nil
}

// mergeSpans union-merges b into a, keeping the earliest/latest line numbers.
func mergeSpans(a, b []string) []string {
	if len(a) == 0 {
		return append([]string(nil), b...)
	}
	return append(a, b...)
}

// ── DB queries ─────────────────────────────────────────────────────────────

// docMetricByID indexes doc metrics by their metric_id for fast lookup.
type docMetricByID map[string]docMetric

func (r *metricsCompletenessReviewer) loadDocMetrics(ctx context.Context, recordID int64) (docMetricByID, error) {
	metrics, err := (&metricsReviewer{db: r.db}).loadRecordMetrics(ctx, recordID)
	if err != nil {
		return nil, err
	}
	out := make(docMetricByID, len(metrics))
	for _, dm := range metrics {
		if dm.view.MetricID != "" {
			out[dm.view.MetricID] = dm
		}
	}
	return out, nil
}

// objectLink is one (metric_id → object_id) mapping with the object node's
// display fields for the payload.
type objectLink struct {
	metricID    string
	objectID    string
	objectName  string
	objectType  string
	description string
}

func (r *metricsCompletenessReviewer) loadObjectLinks(
	ctx context.Context,
	recordID int64,
	metricIDs []string,
) ([]objectLink, error) {
	const q = `
SELECT ao.artifact_id,
       COALESCE(ao.object_id, ''),
       COALESCE(onode.canonical_name,
                NULLIF(ao.object_name_en, ''),
                ao.object_name, 'unknown'),
       COALESCE(onode.object_type, ao.object_type, 'other'),
       COALESCE(onode.description, ao.description, '')
FROM kb.artifact_objects ao
LEFT JOIN kb.object_nodes onode
  ON onode.object_id = ao.object_id
 AND onode.reconcile_status <> 'rejected'
WHERE ao.source_record_id = $1
  AND ao.artifact_type = 'metric'
  AND ao.artifact_id = ANY($2)
  AND COALESCE(ao.object_id, '') <> ''
ORDER BY ao.artifact_id`

	rows, err := r.db.QueryContext(ctx, q, recordID, pq.Array(metricIDs))
	if err != nil {
		return nil, fmt.Errorf("(MID_26070303) load object links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []objectLink
	for rows.Next() {
		var ol objectLink
		if err := rows.Scan(&ol.metricID, &ol.objectID, &ol.objectName,
			&ol.objectType, &ol.description); err != nil {
			return nil, fmt.Errorf("(MID_26070304) scan object link: %w", err)
		}
		out = append(out, ol)
	}
	return out, rows.Err()
}

// peerEdge is one row from kb.artifact_connections for an object's metric
// edges, carrying the source_record_id (which document) and the artifact_ids
// that document attaches to this object.
type peerEdge struct {
	sourceRecordID int64
	artifactIDs    []string
	objectID       string
}

// comparableObjectsMax bounds the number of comparable objects accepted per
// primary object to keep payloads manageable.
const comparableObjectsMax = 5

func (r *metricsCompletenessReviewer) loadPeerObjectEdges(
	ctx context.Context,
	objectIDs []string,
) (edges map[string][]peerEdge, compObjects map[string][]string, err error) {
	edges = make(map[string][]peerEdge)
	compObjects = make(map[string][]string)

	if len(objectIDs) == 0 {
		return
	}

	// Query 1: exact-match edges from artifact_connections.
	const edgeQ = `
SELECT target_id, source_record_id, COALESCE(extra_info->'artifact_ids', '[]'::jsonb)
FROM kb.artifact_connections
WHERE relation_method = 'object_id'
  AND relation_name = 'belong_to'
  AND source_type = 'metric'
  AND target_id = ANY($1)
ORDER BY target_id, source_record_id`

	rows, err := r.db.QueryContext(ctx, edgeQ, pq.Array(objectIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26070305) load peer edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			objID       string
			recordID    int64
			artifactIDs []byte
		)
		if err := rows.Scan(&objID, &recordID, &artifactIDs); err != nil {
			return nil, nil, fmt.Errorf("(MID_26070306) scan peer edge: %w", err)
		}
		ids := parseJSONStringArray(artifactIDs)
		if len(ids) == 0 {
			continue
		}
		edges[objID] = append(edges[objID], peerEdge{
			sourceRecordID: recordID,
			artifactIDs:    ids,
			objectID:       objID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Query 2: comparable objects — objects of the same type with overlapping
	// normalized names but different object_ids. This catches metrics attached
	// to objects that resolved to different canonical nodes but are clearly
	// the same real-world object.
	type objTypeName struct {
		id      string
		objType string
		names   []string
	}
	primaries := make(map[string]objTypeName)
	for _, objID := range objectIDs {
		// At this point we have edge data; the object node info was loaded
		// in loadObjectLinks. Re-query for the comparable-object search.
		//
		// We batch-load the needed fields for all primary objects in one
		// query, then find comparable nodes.
		primaries[objID] = objTypeName{id: objID}
	}
	compObjects = r.loadComparableObjects(ctx, objectIDs)
	return edges, compObjects, nil
}

func (r *metricsCompletenessReviewer) loadComparableObjects(
	ctx context.Context,
	primaryIDs []string,
) map[string][]string {
	if len(primaryIDs) == 0 {
		return nil
	}

	// Load type + normalized_names for the primary objects.
	const primaryQ = `
SELECT object_id, COALESCE(object_type, 'other'), COALESCE(normalized_names, '[]'::jsonb)
FROM kb.object_nodes
WHERE object_id = ANY($1) AND reconcile_status <> 'rejected'`

	rows, err := r.db.QueryContext(ctx, primaryQ, pq.Array(primaryIDs))
	if err != nil {
		r.logger.Warn("metrics_completeness: comparable objects query failed", "error", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	type primaryInfo struct {
		objType string
		names   []string
	}
	info := make(map[string]primaryInfo)
	for rows.Next() {
		var id, objType string
		var namesRaw []byte
		if err := rows.Scan(&id, &objType, &namesRaw); err != nil {
			continue
		}
		info[id] = primaryInfo{objType: objType, names: parseJSONStringArray(namesRaw)}
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	if len(info) == 0 {
		return nil
	}

	// For each primary, find comparable nodes.
	out := make(map[string][]string)
	for primaryID, pi := range info {
		if len(pi.names) == 0 {
			continue
		}
		const compQ = `
SELECT object_id FROM kb.object_nodes
WHERE reconcile_status <> 'rejected'
  AND object_type = $1
  AND object_id <> $2
  AND normalized_names ?| $3
LIMIT $4`
		cRows, err := r.db.QueryContext(ctx, compQ, pi.objType, primaryID, pq.Array(pi.names), comparableObjectsMax)
		if err != nil {
			continue
		}
		var comps []string
		for cRows.Next() {
			var cid string
			if err := cRows.Scan(&cid); err == nil {
				comps = append(comps, cid)
			}
		}
		_ = cRows.Close()
		if len(comps) > 0 {
			out[primaryID] = comps
		}
	}
	return out
}

// resolvedObjectMetric is one metric row resolved from kb.metrics with its
// source document info.
type resolvedObjectMetric struct {
	view     metricView
	recordID int64
	filename string
	title    string
	docNo    string
}

func (r *metricsCompletenessReviewer) loadResolvedMetrics(
	ctx context.Context,
	edges map[string][]peerEdge,
	compObjects map[string][]string,
) (map[string][]resolvedObjectMetric, error) {
	// Collect all metric_ids and the object they belong to.
	byObj := make(map[string][]string) // objectID -> metricIDs
	seen := make(map[string]bool)
	for objID, es := range edges {
		for _, e := range es {
			for _, mid := range e.artifactIDs {
				if !seen[mid] {
					seen[mid] = true
					byObj[objID] = append(byObj[objID], mid)
				}
			}
		}
	}
	// Also collect metric_ids from comparable objects' edges.
	for primaryID, compIDs := range compObjects {
		for _, compID := range compIDs {
			for _, e := range edges[compID] {
				for _, mid := range e.artifactIDs {
					if !seen[mid] {
						seen[mid] = true
						byObj[primaryID] = append(byObj[primaryID], mid)
					}
				}
			}
		}
	}
	// Also load edges for comparable objects that weren't in the primary set.
	var missingCompIDs []string
	for _, compIDs := range compObjects {
		for _, compID := range compIDs {
			if _, ok := edges[compID]; !ok {
				missingCompIDs = append(missingCompIDs, compID)
			}
		}
	}
	if len(missingCompIDs) > 0 {
		const edgeQ = `
SELECT target_id, source_record_id, COALESCE(extra_info->'artifact_ids', '[]'::jsonb)
FROM kb.artifact_connections
WHERE relation_method = 'object_id'
  AND relation_name = 'belong_to'
  AND source_type = 'metric'
  AND target_id = ANY($1)
ORDER BY target_id, source_record_id`
		eRows, err := r.db.QueryContext(ctx, edgeQ, pq.Array(missingCompIDs))
		if err != nil {
			return nil, fmt.Errorf("(MID_26070307) load comparable object edges: %w", err)
		}
		defer func() { _ = eRows.Close() }()
		for eRows.Next() {
			var objID string
			var recordID int64
			var idsRaw []byte
			if err := eRows.Scan(&objID, &recordID, &idsRaw); err != nil {
				return nil, fmt.Errorf("(MID_26070308) scan comparable object edge: %w", err)
			}
			ids := parseJSONStringArray(idsRaw)
			for _, mid := range ids {
				if !seen[mid] {
					seen[mid] = true
					for primaryID := range compObjects {
						for _, compID := range compObjects[primaryID] {
							if compID == objID {
								byObj[primaryID] = append(byObj[primaryID], mid)
							}
						}
					}
					// Also add to the comparable object's own entry so we
					// can resolve its metrics.
					byObj[objID] = append(byObj[objID], mid)
				}
			}
		}
		if err := eRows.Err(); err != nil {
			return nil, err
		}
	}

	if len(byObj) == 0 {
		return nil, nil
	}

	// Dedup and flatten all metric_ids for a single bulk-load query.
	allIDs := make([]string, 0, len(seen))
	for id := range seen {
		allIDs = append(allIDs, id)
	}

	const metricQ = `
SELECT COALESCE(m.metric_id, ''), m.input_record_id, COALESCE(m.metric_name, ''),
       COALESCE(m.metric_subject, ''), COALESCE(m.metric_value, ''), COALESCE(m.metric_unit, ''),
       COALESCE(m.value_class, ''), COALESCE(m.metric_categories, ''), COALESCE(i.staging_filename, ''),
       COALESCE(i.title, ''), COALESCE(i.doc_metadata->>'doc_no', '')
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.metric_id = ANY($1)`
	mRows, err := r.db.QueryContext(ctx, metricQ, pq.Array(allIDs))
	if err != nil {
		return nil, fmt.Errorf("(MID_26070309) load resolved metrics: %w", err)
	}
	defer func() { _ = mRows.Close() }()

	resolved := make(map[string]resolvedObjectMetric)
	for mRows.Next() {
		var rm resolvedObjectMetric
		var catsText string
		if err := mRows.Scan(&rm.view.MetricID, &rm.recordID, &rm.view.MetricName, &rm.view.Subject,
			&rm.view.Value, &rm.view.Unit, &rm.view.ValueClass, &catsText, &rm.filename,
			&rm.title, &rm.docNo); err != nil {
			return nil, fmt.Errorf("(MID_26070310) scan resolved metric: %w", err)
		}
		rm.view.Categories = parseJSONStringArray([]byte(catsText))
		resolved[rm.view.MetricID] = rm
	}
	if err := mRows.Err(); err != nil {
		return nil, err
	}

	// Rebuild byObj with resolved metrics.
	out := make(map[string][]resolvedObjectMetric)
	for objID, mids := range byObj {
		dedup := make(map[string]bool)
		for _, mid := range mids {
			if rm, ok := resolved[mid]; ok && !dedup[mid] {
				dedup[mid] = true
				out[objID] = append(out[objID], rm)
			}
		}
	}
	return out, nil
}
