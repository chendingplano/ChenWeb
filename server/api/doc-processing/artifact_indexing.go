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
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// This file holds the artifact-agnostic Phase C (post-process) indexing engine shared by
// the metric (spec 3.1) and inventory-item (spec 3.3) indexers. Both families run the
// same four steps — connected_artifacts by line overlap, category_instance upserts,
// category-path tree files, and hybrid_search semantic links — so the logic lives here
// once and each family supplies only an artifactIndexConfig.

// indexedArtifact is the persisted view of one source artifact used by the shared
// indexing steps. It is loaded from the family's table (kb.metrics / kb.inventory_items)
// so indexing works off the stored source of truth.
type indexedArtifact struct {
	ID             string
	UpdateKey      any
	SourceSpans    []string
	Categories     []string
	SearchDocument string
	Embedding      []float64
}

// artifactIndexConfig parameterizes the shared indexing helpers for one artifact family.
type artifactIndexConfig struct {
	SelfType             string // kb.search_artifacts.artifact_type of the source family
	CategoryType         string // kb.artifact_categories.category_type
	InstanceSource       string // extra_info "source" tag written on category_instance
	Table                string // source SQL table, e.g. "kb.metrics"
	IDColumn             string // id column in Table, e.g. "metric_id"
	CategoryTreeFilename string // per-leaf index file, e.g. "metrics.txt"
	LogPrefix            string // log message prefix, e.g. "metrics indexing"
}

// categoryBatchResolver resolves a batch of raw category keys to
// kb.artifact_categories category_ids (creating missing categories via the LLM,
// concurrently and coalesced). It returns normalizedKey -> category_id plus per-key
// errors. *categoryResolver satisfies it; tests inject a fake.
type categoryBatchResolver interface {
	ResolveBatch(ctx context.Context, categoryType string, reqs []categoryRequest, maxConcurrency int) (map[string]int64, map[string]error)
}

// artifactConnectedFamily maps a registry artifact type to its JSON key inside a
// connected_artifacts object. The connected_artifacts written for a source family
// includes every family here except the source itself, plus "chunks".
type artifactConnectedFamily struct {
	regType string
	jsonKey string
}

var artifactConnectedFamilies = []artifactConnectedFamily{
	{searchArtifactSemanticProjection, "semantic_projects"},
	{searchArtifactSummary, "summaries"},
	{searchArtifactTopic, "topics"},
	{searchArtifactSceneBlock, "scenes"},
	{searchArtifactProvision, "provisions"},
	{searchArtifactEntity, "entities"},
	{searchArtifactRelation, "relations"},
	{searchArtifactMetric, "metrics"},
	{searchArtifactInventoryItem, "inv_items"},
}

// buildArtifactConnectedArtifacts populates Table.connected_artifacts for each artifact by
// deterministic line-overlap against the document's chunks and the registry artifacts of
// every other family. Returns the number of artifacts whose connected_artifacts was
// written.
func buildArtifactConnectedArtifacts(ctx context.Context, db *sql.DB, recordID int64, inputChunks []Block, artifacts []indexedArtifact, cfg artifactIndexConfig, logger ApiTypes.JimoLogger) int {
	start := time.Now()
	// Target families are every connected family except the source family itself.
	targets := make([]artifactConnectedFamily, 0, len(artifactConnectedFamilies))
	for _, fam := range artifactConnectedFamilies {
		if fam.regType == cfg.SelfType {
			continue
		}
		targets = append(targets, fam)
	}

	refsByType := make(map[string][]ArtifactRef, len(targets))
	totalRefs := 0
	for _, fam := range targets {
		familyStart := time.Now()
		var (
			refs []ArtifactRef
			err  error
		)
		if fam.regType == searchArtifactSemanticProjection {
			refs, err = loadSemanticProjectionRefsForConnections(ctx, db, recordID)
		} else {
			refs, err = loadArtifactRefsFromRegistry(ctx, db, recordID, fam.regType)
		}
		if err != nil {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+": load registry refs failed", "record_id", recordID, "artifact_type", fam.regType, "error", err.Error())
			}
			continue
		}
		refsByType[fam.regType] = refs
		totalRefs += len(refs)
		if logger != nil {
			logger.Info(cfg.LogPrefix+" connected-artifacts refs loaded",
				"record_id", recordID,
				"artifact_type", fam.regType,
				"ref_count", len(refs),
				"ms_used", time.Since(familyStart).Milliseconds(),
			)
		}
	}

	updateStmt := fmt.Sprintf("UPDATE %s SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND %s = $3", cfg.Table, cfg.IDColumn)

	updated := 0
	for _, a := range artifacts {
		artifactLines := lineSetFromSpans(a.SourceSpans)

		ca := map[string][]string{"chunks": {}}
		for _, fam := range targets {
			ca[fam.jsonKey] = []string{}
		}
		for _, ch := range inputChunks {
			if chunkOverlapsLineSet(ch, artifactLines) {
				ca["chunks"] = appendUniqueString(ca["chunks"], ChunkConnectionID(recordID, ch.Index))
			}
		}
		for _, fam := range targets {
			ca[fam.jsonKey] = overlappingRefIDs(refsByType[fam.regType], artifactLines)
		}

		if len(ca["chunks"]) == 0 && logger != nil {
			logger.Warn(cfg.LogPrefix+" error: empty chunks", 
				"record_id", recordID, 
				"artifact_id", a.ID,
				"sourceSpan", a.SourceSpans)
		}
		if semanticProjects, ok := ca["semantic_projects"]; ok && len(semanticProjects) == 0 && logger != nil {
			logger.Warn(cfg.LogPrefix+" error: empty semantic_projections",
				"record_id", recordID,
				"artifact_id", a.ID,
				"source_spans", a.SourceSpans,
				"line_numbers", sortedLineSetKeys(artifactLines),
				"semantic_projection_ref_count", len(refsByType[searchArtifactSemanticProjection]),
			)
		}

		payload, err := json.Marshal(ca)
		if err != nil {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+": marshal connected_artifacts failed", "record_id", recordID, "artifact_id", a.ID, "error", err.Error())
			}
			continue
		}
		updateKey := a.UpdateKey
		if updateKey == nil {
			updateKey = a.ID
		}
		if _, err := db.ExecContext(ctx, updateStmt, string(payload), recordID, updateKey); err != nil {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+": update connected_artifacts failed", "record_id", recordID, "artifact_id", a.ID, "error", err.Error())
			}
			continue
		}
		updated++
	}
	if logger != nil {
		logger.Info(cfg.LogPrefix+" connected-artifacts finished",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"target_families", len(targets),
			"total_refs", totalRefs,
			"updated", updated,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
	return updated
}

func hydrateArtifactEmbeddings(ctx context.Context, db *sql.DB, recordID int64, artifactType string, artifacts []indexedArtifact, logger ApiTypes.JimoLogger, logPrefix string) {
	if db == nil || len(artifacts) == 0 || !kbsearch.SemanticSearchEnabled() {
		return
	}
	start := time.Now()
	rows, err := db.QueryContext(ctx,
		`SELECT artifact_id, embedding::text
		 FROM kb.search_artifacts
		 WHERE artifact_type = $1 AND input_record_id = $2 AND embedding IS NOT NULL`,
		artifactType, recordID,
	)
	if err != nil {
		if logger != nil {
			logger.Warn(logPrefix+": load stored embeddings failed", "record_id", recordID, "artifact_type", artifactType, "error", err.Error())
		}
		return
	}
	defer func() { _ = rows.Close() }()

	embeddingsByID := make(map[string][]float64, len(artifacts))
	for rows.Next() {
		var artifactID string
		var raw string
		if err := rows.Scan(&artifactID, &raw); err != nil {
			if logger != nil {
				logger.Warn(logPrefix+": scan stored embedding failed", "record_id", recordID, "artifact_type", artifactType, "error", err.Error())
			}
			return
		}
		vec, err := parseVectorLiteral(raw)
		if err != nil {
			if logger != nil {
				logger.Warn(logPrefix+": parse stored embedding failed",
					"record_id", recordID,
					"artifact_type", artifactType,
					"artifact_id", artifactID,
					"error", err.Error())
			}
			continue
		}
		if len(vec) == kbsearch.EmbeddingDim {
			embeddingsByID[strings.TrimSpace(artifactID)] = vec
		}
	}
	if err := rows.Err(); err != nil {
		if logger != nil {
			logger.Warn(logPrefix+": iterate stored embeddings failed", "record_id", recordID, "artifact_type", artifactType, "error", err.Error())
		}
		return
	}

	hydrated := 0
	for i := range artifacts {
		if vec, ok := embeddingsByID[strings.TrimSpace(artifacts[i].ID)]; ok {
			artifacts[i].Embedding = vec
			hydrated++
		}
	}
	if logger != nil {
		logger.Info(logPrefix+" stored embeddings hydrated",
			"record_id", recordID,
			"artifact_type", artifactType,
			"artifacts", len(artifacts),
			"hydrated", hydrated,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
}

func parseVectorLiteral(raw string) ([]float64, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []float64{}, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("parse vector float %q: %w", part, err)
		}
		out = append(out, f)
	}
	return out, nil
}

// upsertArtifactCategoryInstances resolves every artifact's category keys in one Phase C
// batch (concurrent create + process-wide single-flight) and upserts a
// kb.category_instance row per (artifact, resolved category). Returns the number of
// category_instance rows upserted.
func upsertArtifactCategoryInstances(ctx context.Context, db *sql.DB, recordID int64, artifacts []indexedArtifact, cfg artifactIndexConfig, resolver categoryBatchResolver, logger ApiTypes.JimoLogger) int {
	start := time.Now()
	extraInfo, _ := json.Marshal(map[string]any{"artifact_type": cfg.CategoryType, "source": cfg.InstanceSource})

	// Gather every (key, evidence) across all artifacts into a single batch so the
	// resolver can dedup, cluster intra-batch synonyms, and create concurrently.
	var reqs []categoryRequest
	for _, a := range artifacts {
		if len(a.Categories) == 0 {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+" error: empty categories", "record_id", recordID, "artifact_id", a.ID)
			}
			continue
		}
		evidence := map[string]any{"artifact_kind": cfg.CategoryType, "search_document": a.SearchDocument}
		for _, key := range a.Categories {
			reqs = append(reqs, categoryRequest{RawKey: key, Evidence: evidence})
		}
	}
	if len(reqs) == 0 {
		return 0
	}

	if logger != nil {
		logger.Info(cfg.LogPrefix+" category-instances start",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"category_requests", len(reqs),
		)
	}
	ids, errs := resolver.ResolveBatch(ctx, cfg.CategoryType, reqs, categoryResolveMaxConcurrency())
	if logger != nil {
		for nk, err := range errs {
			logger.Warn(cfg.LogPrefix+": resolve artifact category failed", "record_id", recordID, "category_key", nk, "error", err.Error())
		}
	}

	count := 0
	for _, a := range artifacts {
		for _, key := range a.Categories {
			categoryID, ok := ids[normalizeCategoryKey(key)]
			if !ok {
				continue // unresolved; already surfaced via errs above
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO kb.category_instance (category_id, artifact_id, input_record_id, extra_info)
				 VALUES ($1, $2, $3, $4::jsonb)
				 ON CONFLICT (category_id, artifact_id)
				 DO UPDATE SET input_record_id = EXCLUDED.input_record_id, extra_info = EXCLUDED.extra_info`,
				categoryID, a.ID, recordID, string(extraInfo)); err != nil {
				if logger != nil {
					logger.Warn(cfg.LogPrefix+": upsert category_instance failed", "record_id", recordID, "artifact_id", a.ID, "category_id", categoryID, "error", err.Error())
				}
				continue
			}
			count++
		}
	}
	if logger != nil {
		logger.Info(cfg.LogPrefix+" category-instances finished",
			"record_id", recordID,
			"category_instances", count,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
	return count
}

// indexArtifactsByCategoryPaths writes each artifact_id into the family's per-leaf index
// file (cfg.CategoryTreeFilename) under the category paths of the semantic projections
// that overlap the artifact's source lines. Returns the number of artifacts indexed.
func indexArtifactsByCategoryPaths(ctx context.Context, db *sql.DB, recordID int64, artifacts []indexedArtifact, cfg artifactIndexConfig, logger ApiTypes.JimoLogger) int {
	dir := strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	if dir == "" {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+" error: missing ARTIFACT_WEB_DIR; skipping category-path indexing", "record_id", recordID)
		}
		return 0
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": create ARTIFACT_WEB_DIR failed", "record_id", recordID, "dir", dir, "error", err.Error())
		}
		return 0
	}
	if err := removeArtifactTreeRecord(dir, recordID, cfg.CategoryTreeFilename); err != nil && logger != nil {
		logger.Warn(cfg.LogPrefix+": remove old tree entries failed", "record_id", recordID, "error", err.Error())
	}

	projs, err := loadSemanticProjectionsForCategoryPaths(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": load semantic projections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}

	if logger != nil {
		logger.Info(cfg.LogPrefix+" category-paths start",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"semantic_projections", len(projs),
		)
	}
	start := time.Now()
	now := time.Now()
	indexed := 0
	for _, a := range artifacts {
		artifactLines := lineSetFromSpans(a.SourceSpans)
		var pairs []categoryPathPair
		for _, pj := range projs {
			if spansOverlapLineSet(pj.spans, artifactLines) {
				pairs = append(pairs, pairCategoryPathEntries(pj.categoryPaths, pj.categoryPathsEn)...)
			}
		}
		if len(pairs) == 0 {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+" error: no category paths for artifact", "record_id", recordID, cfg.IDColumn, a.ID, "source_spans", a.SourceSpans)
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
					logger.Warn(cfg.LogPrefix+": resolve category leaf dir failed", "record_id", recordID, "artifact_id", a.ID, "error", err.Error())
				}
				continue
			}
			if leafDir == "" {
				continue
			}
			if err := upsertArtifactToLeafDir(leafDir, cfg.CategoryTreeFilename, a.ID); err != nil {
				if logger != nil {
					logger.Warn(cfg.LogPrefix+": write "+cfg.CategoryTreeFilename+" failed", "record_id", recordID, "artifact_id", a.ID, "leaf_dir", leafDir, "error", err.Error())
				}
				continue
			}
			wrote = true
		}
		if wrote {
			indexed++
		}
	}
	if logger != nil {
		logger.Info(cfg.LogPrefix+" category-paths finished",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"indexed", indexed,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
	return indexed
}

func upsertArtifactToLeafDir(leafDir, filename, artifactID string) error {
	filePath := filepath.Join(leafDir, filename)
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, artifactID)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeArtifactTreeRecord(treeRootDir string, recordID int64, filename string) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != filename {
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

// hybridCandidate is one row returned by the artifact connect hybrid query.
/*
type hybridCandidate struct {
	artifactType string
	artifactID   string
	recordID     int64
	primaryLabel string
	rrfScore     float64
	cosineSim    sql.NullFloat64
	lexScore     sql.NullFloat64
}
*/

// connectArtifactsBySearch creates source-artifact -> artifact semantic-similarity edges.
// It runs the hybrid (lexical + pgvector) search per artifact, applies the acceptance
// policy (cosine >= min_cosine OR lexical >= min_rank), caps at max_links, and
// idempotently replaces the document's edges for this source family in one source-scoped
// call. Returns the total number of edges written. (Metric spec 3.1.5; inventory 3.3.5
// reuses the identical policy.)
/*
func connectArtifactsBySearch(ctx context.Context, db *sql.DB, recordID int64, artifacts []indexedArtifact, cfg artifactIndexConfig, logger ApiTypes.JimoLogger) int {
	start := time.Now()
	scfg := appconfig.GetArtifactSearchConfig()
	dict := sanitizeTSDictionary(scfg.Dictionary)
	minRank := scfg.MinRank
	minCosine := metricConnectMinCosine()
	maxLinks := metricConnectMaxLinks()

	embedder, embModel, _, embOK := newSearchEmbedder()
	semanticFeatureOn := kbsearch.SemanticSearchEnabled()

	allAccepted := make([]Connection, 0, len(artifacts))
	queriedArtifacts := 0
	reusedEmbeddings := 0
	fallbackEmbeddings := 0
	if logger != nil {
		logger.Info(cfg.LogPrefix+" semantic-linking start",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"semantic_enabled", semanticFeatureOn,
		)
	}
	for i, a := range artifacts {
		if logger != nil && i > 0 && i%50 == 0 {
			logger.Info(cfg.LogPrefix+" semantic-linking progress",
				"record_id", recordID,
				"artifact_index", i,
				"total_artifacts", len(artifacts),
				"accepted_links_so_far", len(allAccepted),
				"ms_used", time.Since(start).Milliseconds(),
			)
		}
		query := strings.TrimSpace(a.SearchDocument)
		if query == "" {
			continue
		}
		queriedArtifacts++

		var vec []float64
		useSem := false
		if semanticFeatureOn && len(a.Embedding) == kbsearch.EmbeddingDim {
			vec = a.Embedding
			useSem = true
			reusedEmbeddings++
		} else if semanticFeatureOn && embOK {
			if v, err := embedder.Embed(ctx, llmclients.EmbedInput{ModelName: embModel, InputText: truncateRunes(query, maxEmbeddingRunes)}); err == nil && len(v) == kbsearch.EmbeddingDim {
				vec = v
				useSem = true
				fallbackEmbeddings++
			}
		}

		candidates, err := queryArtifactHybridCandidates(ctx, db, dict, query, vec, useSem, cfg.SelfType, a.ID, maxLinks*5)
		if err != nil {
			if logger != nil {
				logger.Warn(cfg.LogPrefix+": hybrid candidate query failed", "record_id", recordID, "artifact_id", a.ID, "error", err.Error())
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
				SourceType:     cfg.SelfType,
				SourceID:       a.ID,
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
	// hybrid_search edges for this source family (including cross-document targets) and
	// reinserts.
	store := &ConnectionSQLStore{DB: db}
	if err := store.ReplaceConnectionsBySource(ctx, recordID, cfg.SelfType, RelationMethodHybridSearch, []string{RelationSemanticallyRelated}, allAccepted); err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": replace semantic connections failed", "record_id", recordID, "error", err.Error())
		}
		return 0
	}
	if logger != nil {
		logger.Info(cfg.LogPrefix+" semantic-linking finished",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"queried_artifacts", queriedArtifacts,
			"accepted_links", len(allAccepted),
			"semantic_search_enabled", semanticFeatureOn,
			"reused_embeddings", reusedEmbeddings,
			"fallback_embeddings", fallbackEmbeddings,
			"ms_used", time.Since(start).Milliseconds(),
		)
	}
	return len(allAccepted)
}
*/

// queryArtifactHybridCandidates runs the lexical (+ optional semantic) RRF search over
// kb.search_artifacts and returns the fused candidates with their component scores.
// Params: $1 = query text, $2 = self artifact_id (excluded), $3 = query embedding vector
// (hybrid path only). selfType is the source artifact_type excluded from results.
/*
func queryArtifactHybridCandidates(ctx context.Context, db *sql.DB, dict, query string, vec []float64, useSem bool, selfType, selfID string, limit int) ([]hybridCandidate, error) {
	if limit <= 0 {
		limit = defaultMetricConnectMaxLinks
	}
	lexVector := fmt.Sprintf("COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, '')))", dict)
	tsQuery := fmt.Sprintf("plainto_tsquery('%s', $1)", dict)
	ftsClause := fmt.Sprintf("%s @@ %s", lexVector, tsQuery)
	lexScore := fmt.Sprintf("ts_rank_cd(%s, %s)", lexVector, tsQuery)
	selfExclude := fmt.Sprintf("NOT (sa.artifact_type = '%s' AND sa.artifact_id = $2)", sanitizeArtifactType(selfType))

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
		args = []any{query, selfID}
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
		args = []any{query, selfID, kbsearch.FormatVectorLiteral(vec)}
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
*/

// sanitizeArtifactType guards an artifact_type identifier (interpolated into SQL) against
// injection by allowing only letters/underscore; anything else falls back to a value that
// matches nothing meaningful.
/*
func sanitizeArtifactType(t string) string {
	t = strings.TrimSpace(t)
	for _, r := range t {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			return ""
		}
	}
	return t
}
*/

// metricsToIndexedArtifacts adapts the metric-specific view to the generic one.
func metricsToIndexedArtifacts(metrics []indexedMetric) []indexedArtifact {
	out := make([]indexedArtifact, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, indexedArtifact{
			ID:             m.MetricID,
			SourceSpans:    m.SourceSpans,
			Categories:     m.Categories,
			SearchDocument: m.SearchDocument,
		})
	}
	return out
}
