package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// relation_name / relation_method for metric -> artifact semantic similarity edges.
const (
	RelationMethodHybridSearch  = "hybrid_search"
	RelationSemanticallyRelated = "semantically_related"
)

// Hybrid-connect tuning. These mirror the search handler's RRF constants so the
// connection ranking matches /api/v1/kb/metrics/search. See
// KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md. Inventory-item
// connect (spec 3.3.5) reuses these same constants per "same as 3.1.5".
const (
	rrfKMetricConnect                 = 60
	hybridCandidateLimitMetricConnect = 200
	defaultMetricConnectMinCosine     = 0.75
	defaultMetricConnectMaxLinks      = 10
)

// metricConnectMinCosine is the minimum embedding cosine similarity for the semantic
// acceptance channel (env ARTIFACT_CONNECT_MIN_COSINE, default 0.75).
func metricConnectMinCosine() float64 {
	if v := strings.TrimSpace(os.Getenv("ARTIFACT_CONNECT_MIN_COSINE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultMetricConnectMinCosine
}

// metricConnectMaxLinks caps accepted semantic links per metric (env
// ARTIFACT_CONNECT_MAX_LINKS, default 10).
func metricConnectMaxLinks() int {
	if v := strings.TrimSpace(os.Getenv("ARTIFACT_CONNECT_MAX_LINKS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMetricConnectMaxLinks
}

// metricIndexConfig binds the shared artifact-indexing engine (artifact_indexing.go) to
// the metric family (spec 3.1).
var metricIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactMetric,
	CategoryType:         "metric",
	InstanceSource:       "extract_metrics",
	Table:                "kb.metrics",
	IDColumn:             "metric_id",
	CategoryTreeFilename: "metrics.txt",
	LogPrefix:            "metrics indexing",
}

// indexedMetric is the persisted view of one metric used by the post-save indexing
// steps. It is loaded from kb.metrics so indexing works off the stored source of truth.
type indexedMetric struct {
	MetricID       string
	SourceSpans    []string
	Categories     []string
	SearchDocument string
}

// IndexMetricsForRecord runs the post-save metrics indexing workflow (outputs 2-5 of
// spec 3.1): connected_artifacts JSON, category_name edges, category-path metrics.txt
// entries, and hybrid_search semantic links. Output 1 (kb.search_artifacts) is handled
// separately by ReindexMetricSearchForRecord, which the caller must run first. Each step
// is best-effort and logged; a failure in one step does not abort the others.
func IndexMetricsForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("metrics indexing skipped: nil project db handle", "record_id", recordID)
		}
		return
	}
	metrics, err := loadIndexedMetricsForRecord(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("metrics indexing: load metrics failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(metrics) == 0 {
		return
	}
	if logger != nil {
		logger.Info("metrics indexing start", "record_id", recordID, "metrics", len(metrics))
	}

	// Persist canonical fixed-size chunk line ranges for on-demand kb.connected_artifacts()
	// (Option A). Idempotent; skipped when chunks failed to load so we never wipe the set.
	if len(inputChunks) > 0 {
		if err := replaceChunkRangesForRecord(ctx, db, recordID, inputChunks); err != nil && logger != nil {
			logger.Warn("metrics indexing: replace chunk_ranges failed", "record_id", recordID, "error", err.Error())
		}
	}

	artifacts := metricsToIndexedArtifacts(metrics)

	// Enrich artifacts that the LLM gave no categories by deriving category keys from
	// overlapping semantic projections, so that category membership edges can still be
	// written for them.
	semProjs, semProjErr := loadSemanticProjectionsForCategoryPaths(ctx, db, recordID)
	if semProjErr != nil && logger != nil {
		logger.Warn("metrics indexing: load semantic projections for category enrichment failed",
			"record_id", recordID, "error", semProjErr)
	}
	enrichArtifactCategoriesFromSemProjs(artifacts, semProjs)

	resolver := newMetricCategoryResolver(db, logger)
	categoryConnections := upsertArtifactCategoryConnections(ctx, db, recordID, artifacts, metricIndexConfig, resolver, logger)
	categoryPathMetrics := indexArtifactsByCategoryPaths(ctx, db, recordID, artifacts, metricIndexConfig, logger)
	semanticLinks := connectArtifactsBySearch(ctx, db, recordID, artifacts, metricIndexConfig, logger)

	if logger != nil {
		logger.Info("metrics indexing result",
			"record_id", recordID,
			"category_connections", categoryConnections,
			"category_path_metrics", categoryPathMetrics,
			"semantic_links", semanticLinks,
		)
	}
}

func loadIndexedMetricsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedMetric, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT metric_id,
		        COALESCE(source_line_spans, '[]'::jsonb),
		        COALESCE(metric_categories, ''),
		        COALESCE(search_document, '')
		 FROM kb.metrics
		 WHERE input_record_id = $1
		 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []indexedMetric
	for rows.Next() {
		var (
			metricID  string
			spansRaw  []byte
			catsText  string
			searchDoc string
		)
		if err := rows.Scan(&metricID, &spansRaw, &catsText, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedMetric{
			MetricID:       strings.TrimSpace(metricID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			Categories:     parseMetricCategoriesText(catsText),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

// parseMetricCategoriesText parses the kb.metrics.metric_categories TEXT column. It is
// stored as a JSON array string by SaveMetrics; a comma-separated fallback is accepted
// for resilience.
func parseMetricCategoriesText(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return uniqueStrings(arr)
	}
	return uniqueStrings(strings.Split(s, ","))
}

// upsertMetricCategoryInstances is the metric-family entry point retained for tests; it
// delegates to the shared engine.
/*
func upsertMetricCategoryInstances(ctx context.Context, db *sql.DB, recordID int64, metrics []indexedMetric, resolver categoryBatchResolver, logger ApiTypes.JimoLogger) int {
	return upsertArtifactCategoryConnections(ctx, db, recordID, metricsToIndexedArtifacts(metrics), metricIndexConfig, resolver, logger)
}
*/

// upsertMetricToLeafDir / removeMetricTreeRecord are metric-named shims over the shared
// category-tree helpers, retained for tests.
func upsertMetricToLeafDir(leafDir, metricID string) error {
	return upsertArtifactToLeafDir(leafDir, "metrics.txt", metricID)
}

func removeMetricTreeRecord(treeRootDir string, recordID int64) error {
	return removeArtifactTreeRecord(treeRootDir, recordID, "metrics.txt")
}

// semProjForIndex carries the semantic projection fields needed to derive an artifact's
// category paths via line overlap.
type semProjForIndex struct {
	spans           []string
	categoryPaths   any
	categoryPathsEn any
}

func loadSemanticProjectionsForCategoryPaths(ctx context.Context, db *sql.DB, recordID int64) ([]semProjForIndex, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT COALESCE(line_spans, '[]'::jsonb),
		        COALESCE(category_paths, '[]'::jsonb),
		        COALESCE(category_paths_en, '[]'::jsonb)
		 FROM kb.semantic_projections
		 WHERE input_record_id = $1`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []semProjForIndex
	for rows.Next() {
		var spansRaw, cpRaw, cpEnRaw []byte
		if err := rows.Scan(&spansRaw, &cpRaw, &cpEnRaw); err != nil {
			return nil, err
		}
		var spansAny, cp, cpEn any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		if len(cpRaw) > 0 {
			_ = json.Unmarshal(cpRaw, &cp)
		}
		if len(cpEnRaw) > 0 {
			_ = json.Unmarshal(cpEnRaw, &cpEn)
		}
		out = append(out, semProjForIndex{
			spans:           normalizeSourceLineSpans(spansAny),
			categoryPaths:   cp,
			categoryPathsEn: cpEn,
		})
	}
	return out, rows.Err()
}

// sanitizeTSDictionary guards the FTS config identifier (interpolated into SQL) against
// injection by allowing only letters/underscore; anything else falls back to 'simple'.
func sanitizeTSDictionary(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return "simple"
	}
	for _, r := range d {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			return "simple"
		}
	}
	return d
}

// enrichArtifactCategoriesFromSemProjs fills in Categories for artifacts that have none,
// deriving category keys from overlapping semantic projections. This covers the case where
// the LLM did not assign metric_categories at extraction time but the metric's source lines
// fall inside a semantic projection that does have category paths.
func enrichArtifactCategoriesFromSemProjs(artifacts []indexedArtifact, projs []semProjForIndex) {
	for i := range artifacts {
		if len(artifacts[i].Categories) > 0 {
			continue
		}
		artifactLines := lineSetFromSpans(artifacts[i].SourceSpans)
		for _, pj := range projs {
			if !spansOverlapLineSet(pj.spans, artifactLines) {
				continue
			}
			// Prefer English category paths; fall back to original-language paths.
			for _, raw := range []any{pj.categoryPathsEn, pj.categoryPaths} {
				for _, entry := range parseCategoryPathsAny(raw) {
					for _, name := range categoryPathNames(entry.Nodes) {
						if key := normalizeCategorySegment(name); key != "" {
							artifacts[i].Categories = appendUniqueString(artifacts[i].Categories, key)
						}
					}
				}
				if len(artifacts[i].Categories) > 0 {
					break
				}
			}
		}
	}
}

func lineSetFromSpans(spans []string) map[int]struct{} {
	set := make(map[int]struct{})
	for _, sp := range spans {
		start, end, ok := parseMetricLineSpan(sp)
		if !ok {
			continue
		}
		for n := start; n <= end; n++ {
			if n > 0 {
				set[n] = struct{}{}
			}
		}
	}
	return set
}

func chunkOverlapsLineSet(ch Block, set map[int]struct{}) bool {
	if len(set) == 0 {
		return false
	}
	for _, l := range ch.Lines {
		if l.LineNumber > 0 {
			if _, ok := set[l.LineNumber]; ok {
				return true
			}
		}
	}
	return false
}

func spansOverlapLineSet(spans []string, set map[int]struct{}) bool {
	if len(set) == 0 {
		return false
	}
	for _, sp := range spans {
		start, end, ok := parseMetricLineSpan(sp)
		if !ok {
			continue
		}
		for n := start; n <= end; n++ {
			if _, ok := set[n]; ok {
				return true
			}
		}
	}
	return false
}
