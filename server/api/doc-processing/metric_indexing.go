package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// relation_name / relation_method for metric -> artifact semantic similarity edges.
const (
	RelationMethodHybridSearch  = "hybrid_search"
	RelationSemanticallyRelated = "semantically_related"
)

// Hybrid-connect tuning. These mirror the search handler's RRF constants so the
// connection ranking matches /api/v1/kb/metrics/search. See
// KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md.
const (
	rrfKMetricConnect                 = 60
	hybridCandidateLimitMetricConnect = 200
	defaultMetricConnectMinCosine     = 0.75
	defaultMetricConnectMaxLinks      = 10
)

// metricConnectMinCosine is the minimum embedding cosine similarity for the semantic
// acceptance channel (env METRIC_CONNECT_MIN_COSINE, default 0.75).
func metricConnectMinCosine() float64 {
	if v := strings.TrimSpace(os.Getenv("METRIC_CONNECT_MIN_COSINE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultMetricConnectMinCosine
}

// metricConnectMaxLinks caps accepted semantic links per metric (env
// METRIC_CONNECT_MAX_LINKS, default 10).
func metricConnectMaxLinks() int {
	if v := strings.TrimSpace(os.Getenv("METRIC_CONNECT_MAX_LINKS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMetricConnectMaxLinks
}

// indexedMetric is the persisted view of one metric used by the post-save indexing
// steps. It is loaded from kb.metrics so indexing works off the stored source of truth.
type indexedMetric struct {
	MetricID       string
	SourceSpans    []string
	Categories     []string
	SearchDocument string
}

// connectedArtifacts is the required JSON shape stored in kb.metrics.connected_artifacts.
// Every field is a non-nil slice so it marshals to [] (never null).
type connectedArtifacts struct {
	Chunks           []string `json:"chunks"`
	SemanticProjects []string `json:"semantic_projects"`
	Topics           []string `json:"topics"`
	Scenes           []string `json:"scenes"`
	Provisions       []string `json:"provisions"`
	Entities         []string `json:"entities"`
	InvItems         []string `json:"inv_items"`
}

// IndexMetricsForRecord runs the post-save metrics indexing workflow (outputs 2-5 of
// the spec's Indexing section): connected_artifacts JSON, category_instance rows,
// category-path metrics.txt entries, and hybrid_search semantic links. Output 1
// (kb.search_artifacts) is handled separately by ReindexMetricSearchForRecord, which
// the caller must run first. Each step is best-effort and logged; a failure in one
// step does not abort the others.
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

	connectedCount := buildConnectedArtifacts(ctx, db, recordID, inputChunks, metrics, logger)
	categoryInstances := upsertMetricCategoryInstances(ctx, db, recordID, metrics, logger)
	categoryPathMetrics := indexMetricsByCategoryPaths(ctx, db, recordID, metrics, logger)
	semanticLinks := connectMetricArtifacts(ctx, db, recordID, metrics, logger)

	if logger != nil {
		logger.Info("metrics indexing result",
			"record_id", recordID,
			"connected_artifacts_metrics", connectedCount,
			"category_instances", categoryInstances,
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

// buildConnectedArtifacts populates kb.metrics.connected_artifacts for each metric by
// deterministic line-overlap against the document's chunks and registry artifacts.
// Returns the number of metrics whose connected_artifacts was written.
func buildConnectedArtifacts(ctx context.Context, db *sql.DB, recordID int64, inputChunks []Block, metrics []indexedMetric, logger ApiTypes.JimoLogger) int {
	// Registry-backed artifact families keyed by their connected_artifacts JSON field.
	registryTypes := []string{
		searchArtifactSemanticProjection,
		searchArtifactTopic,
		searchArtifactSceneBlock,
		searchArtifactProvision,
		searchArtifactEntity,
		searchArtifactInventoryItem,
	}
	refsByType := make(map[string][]ArtifactRef, len(registryTypes))
	for _, atype := range registryTypes {
		var (
			refs []ArtifactRef
			err  error
		)
		if atype == searchArtifactSemanticProjection {
			refs, err = loadSemanticProjectionRefsForConnections(ctx, db, recordID)
		} else {
			refs, err = loadArtifactRefsFromRegistry(ctx, db, recordID, atype)
		}
		if err != nil {
			if logger != nil {
				logger.Warn("metrics indexing: load registry refs failed", "record_id", recordID, "artifact_type", atype, "error", err.Error())
			}
			continue
		}
		refsByType[atype] = refs
		if logger != nil && atype == searchArtifactSemanticProjection {
			logger.Info("metrics indexing: loaded semantic projection refs",
				"record_id", recordID,
				"semantic_projection_ref_count", len(refs),
				"semantic_projection_ref_sample", summarizeArtifactRefs(refs, 8),
			)
		}
	}

	updated := 0
	for _, m := range metrics {
		metricLines := lineSetFromSpans(m.SourceSpans)

		ca := connectedArtifacts{
			Chunks:           []string{},
			SemanticProjects: []string{},
			Topics:           []string{},
			Scenes:           []string{},
			Provisions:       []string{},
			Entities:         []string{},
			InvItems:         []string{},
		}

		for _, ch := range inputChunks {
			if chunkOverlapsLineSet(ch, metricLines) {
				ca.Chunks = appendUniqueString(ca.Chunks, ChunkConnectionID(recordID, ch.Index))
			}
		}
		ca.SemanticProjects = overlappingRefIDs(refsByType[searchArtifactSemanticProjection], metricLines)
		ca.Topics = overlappingRefIDs(refsByType[searchArtifactTopic], metricLines)
		ca.Scenes = overlappingRefIDs(refsByType[searchArtifactSceneBlock], metricLines)
		ca.Provisions = overlappingRefIDs(refsByType[searchArtifactProvision], metricLines)
		ca.Entities = overlappingRefIDs(refsByType[searchArtifactEntity], metricLines)
		ca.InvItems = overlappingRefIDs(refsByType[searchArtifactInventoryItem], metricLines)

		if len(ca.Chunks) == 0 && logger != nil {
			logger.Warn("metrics indexing error: empty chunks", "record_id", recordID, "metric_id", m.MetricID)
		}
		if len(ca.SemanticProjects) == 0 && logger != nil {
			logger.Warn("metrics indexing error: empty semantic_projects",
				"record_id", recordID,
				"metric_id", m.MetricID,
				"metric_source_spans", m.SourceSpans,
				"metric_line_numbers", sortedLineSetKeys(metricLines),
				"semantic_projection_ref_count", len(refsByType[searchArtifactSemanticProjection]),
				"semantic_projection_ref_sample", summarizeArtifactRefs(refsByType[searchArtifactSemanticProjection], 8),
			)
		}

		payload, err := json.Marshal(ca)
		if err != nil {
			if logger != nil {
				logger.Warn("metrics indexing: marshal connected_artifacts failed", "record_id", recordID, "metric_id", m.MetricID, "error", err.Error())
			}
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE kb.metrics SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND metric_id = $3`,
			string(payload), recordID, m.MetricID); err != nil {
			if logger != nil {
				logger.Warn("metrics indexing: update connected_artifacts failed", "record_id", recordID, "metric_id", m.MetricID, "error", err.Error())
			}
			continue
		}
		updated++
	}
	return updated
}

func overlappingRefIDs(refs []ArtifactRef, metricLines map[int]struct{}) []string {
	out := []string{}
	for _, ref := range refs {
		if spansOverlapLineSet(ref.Spans, metricLines) {
			out = appendUniqueString(out, ref.ID)
		}
	}
	return out
}

func loadSemanticProjectionRefsForConnections(ctx context.Context, db *sql.DB, recordID int64) ([]ArtifactRef, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT semantic_proj_id, COALESCE(line_spans, '[]'::jsonb)
		 FROM kb.semantic_projections
		 WHERE input_record_id = $1
		 ORDER BY id`,
		recordID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	refs := make([]ArtifactRef, 0, 16)
	for rows.Next() {
		var (
			projID   string
			spansRaw []byte
		)
		if err := rows.Scan(&projID, &spansRaw); err != nil {
			return nil, err
		}
		var raw any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &raw)
		}
		refs = append(refs, ArtifactRef{
			Type:  searchArtifactSemanticProjection,
			ID:    strings.TrimSpace(projID),
			Spans: normalizeSourceLineSpans(raw),
		})
	}
	return refs, rows.Err()
}

func summarizeArtifactRefs(refs []ArtifactRef, limit int) []string {
	if len(refs) == 0 || limit <= 0 {
		return []string{}
	}
	if len(refs) < limit {
		limit = len(refs)
	}
	out := make([]string, 0, limit+1)
	for _, ref := range refs[:limit] {
		out = append(out, fmt.Sprintf("%s:%v", strings.TrimSpace(ref.ID), ref.Spans))
	}
	if len(refs) > limit {
		out = append(out, fmt.Sprintf("... +%d more", len(refs)-limit))
	}
	return out
}

func sortedLineSetKeys(set map[int]struct{}) []int {
	if len(set) == 0 {
		return []int{}
	}
	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// upsertMetricCategoryInstances resolves each metric's category keys to
// kb.artifact_categories.category_id, creating missing artifact categories first, and
// upserts a kb.category_instance row. Returns the number of category_instance rows
// upserted.
func upsertMetricCategoryInstances(ctx context.Context, db *sql.DB, recordID int64, metrics []indexedMetric, logger ApiTypes.JimoLogger) int {
	extraInfo, _ := json.Marshal(map[string]any{"artifact_type": "metric", "source": "extract_metrics"})
	count := 0
	for _, m := range metrics {
		if len(m.Categories) == 0 {
			if logger != nil {
				logger.Warn("metrics indexing error: empty metric_categories", "record_id", recordID, "metric_id", m.MetricID)
			}
			continue
		}
		for _, key := range m.Categories {
			categoryID, err := upsertMetricArtifactCategory(ctx, db, key)
			if err != nil {
				if logger != nil {
					logger.Warn("metrics indexing: upsert artifact category failed", "record_id", recordID, "metric_id", m.MetricID, "category_key", key, "error", err.Error())
				}
				continue
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO kb.category_instance (category_id, artifact_id, input_record_id, extra_info)
				 VALUES ($1, $2, $3, $4::jsonb)
				 ON CONFLICT (category_id, artifact_id)
				 DO UPDATE SET input_record_id = EXCLUDED.input_record_id, extra_info = EXCLUDED.extra_info`,
				categoryID, m.MetricID, recordID, string(extraInfo)); err != nil {
				if logger != nil {
					logger.Warn("metrics indexing: upsert category_instance failed", "record_id", recordID, "metric_id", m.MetricID, "category_id", categoryID, "error", err.Error())
				}
				continue
			}
			count++
		}
	}
	return count
}

func upsertMetricArtifactCategory(ctx context.Context, db *sql.DB, key string) (int64, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, fmt.Errorf("empty category key")
	}
	var categoryID int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO kb.artifact_categories (
		     category_key, category_type, display_names, search_document, seen_count
		 )
		 VALUES ($1, 'metric', to_jsonb(ARRAY[$1]::text[]), $1, 1)
		 ON CONFLICT (category_key) DO UPDATE SET
		     seen_count = kb.artifact_categories.seen_count + 1,
		     last_seen_at = NOW(),
		     display_names = CASE
		         WHEN kb.artifact_categories.display_names @> to_jsonb($1::text) THEN kb.artifact_categories.display_names
		         ELSE kb.artifact_categories.display_names || to_jsonb($1::text)
		     END,
		     search_document = CASE
		         WHEN COALESCE(kb.artifact_categories.search_document, '') = '' THEN EXCLUDED.search_document
		         ELSE kb.artifact_categories.search_document
		     END
		 RETURNING category_id`, key).Scan(&categoryID)
	if err != nil {
		return 0, err
	}
	if categoryID == 0 {
		return 0, fmt.Errorf("artifact category %q has no category_id", key)
	}
	return categoryID, nil
}

// semProjForIndex carries the semantic projection fields needed to derive a metric's
// category paths via line overlap.
type semProjForIndex struct {
	spans           []string
	categoryPaths   any
	categoryPathsEn any
}

// indexMetricsByCategoryPaths writes each metric_id into metrics.txt under the category
// paths of the semantic projections that overlap the metric's source lines (the same
// tree layout used for products/summaries). Returns the number of metrics indexed.
func indexMetricsByCategoryPaths(ctx context.Context, db *sql.DB, recordID int64, metrics []indexedMetric, logger ApiTypes.JimoLogger) int {
	dir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if dir == "" {
		if logger != nil {
			logger.Warn("metrics indexing error: missing ARTIFACT_WEB_DIR; skipping category-path indexing", "record_id", recordID)
		}
		return 0
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if logger != nil {
			logger.Warn("metrics indexing: create ARTIFACT_WEB_DIR failed", "record_id", recordID, "dir", dir, "error", err.Error())
		}
		return 0
	}
	if err := removeMetricTreeRecord(dir, recordID); err != nil && logger != nil {
		logger.Warn("metrics indexing: remove old metric tree entries failed", "record_id", recordID, "error", err.Error())
	}

	projs, err := loadSemanticProjectionsForCategoryPaths(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("metrics indexing: load semantic projections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}

	now := time.Now()
	indexed := 0
	for _, m := range metrics {
		metricLines := lineSetFromSpans(m.SourceSpans)
		var pairs []categoryPathPair
		for _, pj := range projs {
			if spansOverlapLineSet(pj.spans, metricLines) {
				pairs = append(pairs, pairCategoryPathEntries(pj.categoryPaths, pj.categoryPathsEn)...)
			}
		}
		if len(pairs) == 0 {
			if logger != nil {
				logger.Warn("metrics indexing error: no category paths for metric", "record_id", recordID, "metric_id", m.MetricID)
			}
			continue
		}

		wrote := false
		seen := make(map[string]struct{}, len(pairs))
		for _, pair := range pairs {
			indexPath := categoryPathNames(pair.Index.Nodes)
			if len(indexPath) == 0 {
				continue
			}
			norm := make([]string, 0, len(indexPath))
			for _, seg := range indexPath {
				norm = append(norm, normalizeCategorySegment(seg))
			}
			key := strings.Join(norm, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			leafDir, err := categoryTreeLeafDirForEntry(logger, dir, pair.Index, pair.Original, now)
			if err != nil {
				if logger != nil {
					logger.Warn("metrics indexing: resolve category leaf dir failed", "record_id", recordID, "metric_id", m.MetricID, "error", err.Error())
				}
				continue
			}
			if leafDir == "" {
				continue
			}
			if err := upsertMetricToLeafDir(leafDir, m.MetricID); err != nil {
				if logger != nil {
					logger.Warn("metrics indexing: write metrics.txt failed", "record_id", recordID, "metric_id", m.MetricID, "leaf_dir", leafDir, "error", err.Error())
				}
				continue
			}
			wrote = true
		}
		if wrote {
			indexed++
		}
	}
	return indexed
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

func upsertMetricToLeafDir(leafDir, metricID string) error {
	filePath := filepath.Join(leafDir, "metrics.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, metricID)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeMetricTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "metrics.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for _, row := range strings.Split(string(body), "\n") {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, prefix) {
				continue
			}
			rows = append(rows, row)
		}
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
}

// hybridCandidate is one row returned by the metric connect hybrid query.
type hybridCandidate struct {
	artifactType string
	artifactID   string
	recordID     int64
	primaryLabel string
	rrfScore     float64
	cosineSim    sql.NullFloat64
	lexScore     sql.NullFloat64
}

// connectMetricArtifacts creates metric -> artifact semantic similarity edges. It runs
// the hybrid (lexical + pgvector) search per metric, applies the acceptance policy
// (cosine >= min_cosine OR lexical >= min_rank), caps at max_links, and idempotently
// replaces the document's metric semantic edges in one source-scoped call. Returns the
// total number of edges written.
func connectMetricArtifacts(ctx context.Context, db *sql.DB, recordID int64, metrics []indexedMetric, logger ApiTypes.JimoLogger) int {
	cfg := appconfig.GetMetricSearchConfig()
	dict := sanitizeTSDictionary(cfg.Dictionary)
	minRank := cfg.MinRank
	minCosine := metricConnectMinCosine()
	maxLinks := metricConnectMaxLinks()

	embedder, embModel, embOK := newSearchEmbedder()
	semanticOn := kbsearch.SemanticSearchEnabled() && embOK

	allAccepted := make([]Connection, 0, len(metrics))
	for _, m := range metrics {
		query := strings.TrimSpace(m.SearchDocument)
		if query == "" {
			continue
		}

		var vec []float64
		useSem := false
		if semanticOn {
			if v, err := embedder.Embed(ctx, llmclients.EmbedInput{ModelName: embModel, InputText: truncateRunes(query, maxEmbeddingRunes)}); err == nil && len(v) == kbsearch.EmbeddingDim {
				vec = v
				useSem = true
			}
		}

		candidates, err := queryMetricHybridCandidates(ctx, db, dict, query, vec, useSem, m.MetricID, maxLinks*5)
		if err != nil {
			if logger != nil {
				logger.Warn("metrics indexing: hybrid candidate query failed", "record_id", recordID, "metric_id", m.MetricID, "error", err.Error())
			}
			continue
		}

		accepted := 0
		for _, c := range candidates {
			okSem := c.cosineSim.Valid && c.cosineSim.Float64 >= minCosine
			okLex := c.lexScore.Valid && c.lexScore.Float64 >= minRank
			if !okSem && !okLex {
				continue
			}
			prov := map[string]any{"rrf_score": c.rrfScore, "rrf_k": rrfKMetricConnect}
			if c.cosineSim.Valid {
				prov["cosine_sim"] = c.cosineSim.Float64
			}
			if c.lexScore.Valid {
				prov["lexical_score"] = c.lexScore.Float64
			}
			extra := map[string]any{
				"min_cosine":       minCosine,
				"min_rank":         minRank,
				"max_links":        maxLinks,
				"semantic_enabled": useSem,
			}
			allAccepted = append(allAccepted, Connection{
				SourceRecordID: recordID,
				TargetRecordID: c.recordID,
				SourceType:     searchArtifactMetric,
				SourceID:       m.MetricID,
				TargetType:     c.artifactType,
				TargetID:       c.artifactID,
				RelationName:   RelationSemanticallyRelated,
				RelationMethod: RelationMethodHybridSearch,
				Confidence:     c.rrfScore,
				Provenance:     prov,
				TargetDesc:     connectionEndpointDesc(c.artifactType, firstNonEmptyTrimmed(c.primaryLabel, c.artifactID)),
				ExtraInfo:      extra,
			})
			accepted++
			if accepted >= maxLinks {
				break
			}
		}
	}

	// One source-scoped replace for the whole document: clears all of this record's
	// metric hybrid_search edges (including cross-document targets) and reinserts.
	store := &ConnectionSQLStore{DB: db}
	if err := store.ReplaceConnectionsBySource(ctx, recordID, searchArtifactMetric, RelationMethodHybridSearch, []string{RelationSemanticallyRelated}, allAccepted); err != nil {
		if logger != nil {
			logger.Warn("metrics indexing: replace metric semantic connections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}
	return len(allAccepted)
}

// queryMetricHybridCandidates runs the lexical (+ optional semantic) RRF search over
// kb.search_artifacts and returns the fused candidates with their component scores.
// Params: $1 = query text, $2 = self metric_id (excluded), $3 = query embedding vector
// (hybrid path only).
func queryMetricHybridCandidates(ctx context.Context, db *sql.DB, dict, query string, vec []float64, useSem bool, selfMetricID string, limit int) ([]hybridCandidate, error) {
	if limit <= 0 {
		limit = defaultMetricConnectMaxLinks
	}
	lexVector := fmt.Sprintf("COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, '')))", dict)
	tsQuery := fmt.Sprintf("plainto_tsquery('%s', $1)", dict)
	ftsClause := fmt.Sprintf("%s @@ %s", lexVector, tsQuery)
	lexScore := fmt.Sprintf("ts_rank_cd(%s, %s)", lexVector, tsQuery)
	selfExclude := "NOT (sa.artifact_type = 'metric' AND sa.artifact_id = $2)"

	var sqlText string
	var args []any
	if !useSem {
		sqlText = fmt.Sprintf(`
WITH lexical AS (
	SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
		COALESCE(sa.primary_label, '') AS primary_label,
		%s AS lex_score,
		ROW_NUMBER() OVER (ORDER BY %s DESC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE %s AND %s
	ORDER BY rnk
	LIMIT %d
)
SELECT artifact_type, artifact_id, input_record_id, primary_label,
	1.0 / (%d + rnk) AS rrf_score, lex_score, NULL::float8 AS cosine_sim
FROM lexical
ORDER BY rrf_score DESC, artifact_id ASC
LIMIT %d`,
			lexScore, lexScore, ftsClause, selfExclude, hybridCandidateLimitMetricConnect,
			rrfKMetricConnect, limit)
		args = []any{query, selfMetricID}
	} else {
		sqlText = fmt.Sprintf(`
WITH lexical AS (
	SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
		COALESCE(sa.primary_label, '') AS primary_label,
		%s AS lex_score,
		ROW_NUMBER() OVER (ORDER BY %s DESC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE %s AND %s
	ORDER BY rnk
	LIMIT %d
),
semantic AS (
	SELECT sa.artifact_type, sa.artifact_id, sa.input_record_id,
		COALESCE(sa.primary_label, '') AS primary_label,
		1 - (sa.embedding <=> $3::vector) AS cosine_sim,
		ROW_NUMBER() OVER (ORDER BY sa.embedding <=> $3::vector ASC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE sa.embedding IS NOT NULL AND %s
	ORDER BY rnk
	LIMIT %d
)
SELECT
	COALESCE(l.artifact_type, s.artifact_type) AS artifact_type,
	COALESCE(l.artifact_id, s.artifact_id) AS artifact_id,
	COALESCE(l.input_record_id, s.input_record_id) AS input_record_id,
	COALESCE(NULLIF(l.primary_label, ''), s.primary_label, '') AS primary_label,
	COALESCE(1.0 / (%d + l.rnk), 0.0) + COALESCE(1.0 / (%d + s.rnk), 0.0) AS rrf_score,
	l.lex_score AS lex_score,
	s.cosine_sim AS cosine_sim
FROM lexical l
FULL OUTER JOIN semantic s ON l.artifact_type = s.artifact_type AND l.artifact_id = s.artifact_id
ORDER BY rrf_score DESC, artifact_id ASC
LIMIT %d`,
			lexScore, lexScore, ftsClause, selfExclude, hybridCandidateLimitMetricConnect,
			selfExclude, hybridCandidateLimitMetricConnect,
			rrfKMetricConnect, rrfKMetricConnect, limit)
		args = []any{query, selfMetricID, kbsearch.FormatVectorLiteral(vec)}
	}

	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []hybridCandidate
	for rows.Next() {
		var c hybridCandidate
		if err := rows.Scan(&c.artifactType, &c.artifactID, &c.recordID, &c.primaryLabel, &c.rrfScore, &c.lexScore, &c.cosineSim); err != nil {
			return nil, err
		}
		out = append(out, c)
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
