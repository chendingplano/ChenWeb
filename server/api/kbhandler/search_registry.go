package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// Hybrid-search tuning. rrfK is the standard Reciprocal Rank Fusion constant;
// hybridCandidateLimit bounds each candidate list (lexical / semantic) before fusion.
const (
	rrfK                 = 60
	hybridCandidateLimit = 200
)

type lexicalBackend string

const (
	lexicalBackendPostgres lexicalBackend = "postgres"
	lexicalBackendParadeDB lexicalBackend = "paradedb"
)

type registrySearchConfig struct {
	dictionary      string
	defaultPageSize int
	maxPageSize     int
	previewMaxWords int
	phraseFriendly  bool
	minRank         float64
}

func loadRegistrySearchConfig() registrySearchConfig {
	cfg := appconfig.GetArtifactSearchConfig()
	return registrySearchConfig{
		dictionary:      sanitizeTSConfig(cfg.Dictionary),
		defaultPageSize: cfg.DefaultPageSize,
		maxPageSize:     cfg.MaxPageSize,
		previewMaxWords: cfg.PreviewMaxWords,
		phraseFriendly:  cfg.PhraseFriendly,
		minRank:         cfg.MinRank,
	}
}

func registryLexicalBackend() lexicalBackend {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SEARCH_LEXICAL_BACKEND"))) {
	case "postgres", "pg", "tsvector":
		return lexicalBackendPostgres
	default:
		return lexicalBackendParadeDB
	}
}

// CheckSearchBackend verifies the configured search backend is available.
// When the effective backend is paradedb (the default), it confirms the
// pg_search extension is installed and returns a fatal-worthy error if not.
// Call this once at startup after migrations have run.
func CheckSearchBackend(db *sql.DB) error {
	if registryLexicalBackend() != lexicalBackendParadeDB {
		return nil
	}
	return CheckParadeDBInstalled(db)
}

// CheckParadeDBInstalled verifies that the pg_search extension is present in the
// database. Call this at startup when the lexical backend is paradedb; the
// extension must be installed before any BM25 query can run.
func CheckParadeDBInstalled(db *sql.DB) error {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_search')`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("could not query pg_extension for pg_search: %w", err)
	}
	if !exists {
		return fmt.Errorf("ParadeDB (pg_search) extension is not installed; " +
			"install pg_search on the PostgreSQL server or set SEARCH_LEXICAL_BACKEND=postgres to use tsvector")
	}
	return nil
}

func countRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, cfg registrySearchConfig) (int64, error) {
	if registryLexicalBackend() == lexicalBackendParadeDB {
		if total, err := countRegistrySearchResultsParadeDB(db, artifactType, queryText, filters); err == nil {
			return total, nil
		}
	}
	return countRegistrySearchResultsPostgres(db, artifactType, queryText, filters, cfg)
}

func countRegistrySearchResultsPostgres(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, cfg registrySearchConfig) (int64, error) {
	whereSQL, args := buildRegistrySearchWhereClause(artifactType, queryText, filters, cfg)
	sqlText := fmt.Sprintf(`SELECT COUNT(*) FROM kb.search_artifacts sa WHERE %s`, whereSQL)
	var total int64
	err := db.QueryRow(sqlText, args...).Scan(&total)
	return total, err
}

func countRegistrySearchResultsParadeDB(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters) (int64, error) {
	whereSQL, args := buildParadeDBSearchWhereClause(artifactType, queryText, filters, 2)
	sqlText := fmt.Sprintf(`SELECT COUNT(*) FROM kb.search_artifacts sa WHERE %s`, whereSQL)
	var total int64
	err := db.QueryRow(sqlText, args...).Scan(&total)
	return total, err
}

func queryRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	backend := registryLexicalBackend()

	// Hybrid path (flag-gated): fuse lexical BM25-style ranking with pgvector cosine
	// via RRF. Best-effort — if the query can't be embedded or the hybrid query
	// errors, fall through to the original lexical-only search below.
	if kbsearch.SemanticSearchEnabled() {
		if vec, ok := computeQueryEmbedding(context.Background(), queryText); ok {
			if backend == lexicalBackendParadeDB {
				if results, err := queryHybridSearchResultsParadeDB(db, artifactType, queryText, vec, filters, page, pageSize); err == nil {
					return results, nil
				}
			}
			if results, err := queryHybridSearchResults(db, artifactType, queryText, vec, filters, page, pageSize, cfg); err == nil {
				return results, nil
			}
		}
	}

	if backend == lexicalBackendParadeDB {
		if results, err := queryRegistrySearchResultsParadeDB(db, artifactType, queryText, filters, page, pageSize, cfg); err == nil {
			return results, nil
		}
	}
	return queryRegistrySearchResultsPostgres(db, artifactType, queryText, filters, page, pageSize, cfg)
}

func queryRegistrySearchResultsPostgres(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	whereSQL, args := buildRegistrySearchWhereClause(artifactType, queryText, filters, cfg)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	tsQueryExpr := "websearch_to_tsquery"
	if !cfg.phraseFriendly {
		tsQueryExpr = "plainto_tsquery"
	}
	snippetOptions := fmt.Sprintf("MaxWords=%d, MinWords=%d, ShortWord=2, HighlightAll=false, MaxFragments=1, FragmentDelimiter=' ... '", cfg.previewMaxWords, maxInt(6, cfg.previewMaxWords/2))
	escapedSnippetOptions := escapeSQLLiteral(snippetOptions)

	minRankClause := ""
	if cfg.minRank > 0 {
		minRankClause = fmt.Sprintf("WHERE score >= %f", cfg.minRank)
	}

	scoreExpr := fmt.Sprintf("ts_rank_cd(COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, ''))), query_input.query)", cfg.dictionary)
	if containsCJKText(queryText) {
		scoreExpr += `
			+ CASE WHEN coalesce(sa.primary_label, '') ILIKE '%' || $1 || '%' THEN 1.25 ELSE 0 END
			+ CASE WHEN coalesce(sa.secondary_label, '') ILIKE '%' || $1 || '%' THEN 0.50 ELSE 0 END
			+ CASE WHEN coalesce(sa.snippet_basis, '') ILIKE '%' || $1 || '%' THEN 0.40 ELSE 0 END
			+ CASE WHEN coalesce(sa.search_document, '') ILIKE '%' || $1 || '%' THEN 0.20 ELSE 0 END`
	}

	sqlText := fmt.Sprintf(`
WITH query_input AS (
	SELECT %s('%s', $1) AS query
),
ranked AS (
	SELECT
		sa.artifact_type,
		sa.artifact_id,
		sa.input_record_id,
		COALESCE(sa.primary_label, '') AS primary_label,
		COALESCE(sa.secondary_label, '') AS secondary_label,
		COALESCE(sa.source_title, '') AS source_title,
		COALESCE(sa.source_filename, '') AS source_filename,
		COALESCE(sa.source_line_spans, '[]'::jsonb) AS source_line_spans,
		COALESCE(sa.semantic_payload, '{}'::jsonb) AS semantic_payload,
		%s AS score,
		ts_headline(
			'%s',
			COALESCE(NULLIF(sa.snippet_basis, ''), NULLIF(sa.search_document, ''), COALESCE(sa.primary_label, '')),
			query_input.query,
			'%s'
		) AS snippet
	FROM kb.search_artifacts sa
	CROSS JOIN query_input
	WHERE %s
)
SELECT artifact_type, artifact_id, input_record_id, primary_label, secondary_label, source_title, source_filename, source_line_spans, semantic_payload, score, snippet
FROM ranked
%s
ORDER BY score DESC, artifact_id ASC
LIMIT $%d OFFSET $%d`,
		tsQueryExpr, cfg.dictionary,
		scoreExpr,
		cfg.dictionary,
		escapedSnippetOptions,
		whereSQL,
		minRankClause,
		len(args)-1, len(args),
	)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanArtifactSearchRows(rows, pageSize)
}

func queryRegistrySearchResultsParadeDB(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	whereSQL, args := buildParadeDBSearchWhereClause(artifactType, queryText, filters, 2)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	minRankClause := ""
	if cfg.minRank > 0 {
		minRankClause = fmt.Sprintf("WHERE score >= %f", cfg.minRank)
	}

	sqlText := fmt.Sprintf(`
WITH ranked AS (
	SELECT
		sa.artifact_type,
		sa.artifact_id,
		sa.input_record_id,
		COALESCE(sa.primary_label, '') AS primary_label,
		COALESCE(sa.secondary_label, '') AS secondary_label,
		COALESCE(sa.source_title, '') AS source_title,
		COALESCE(sa.source_filename, '') AS source_filename,
		COALESCE(sa.source_line_spans, '[]'::jsonb) AS source_line_spans,
		COALESCE(sa.semantic_payload, '{}'::jsonb) AS semantic_payload,
		pdb.score(sa.artifact_id) AS score,
		COALESCE(
			NULLIF(pdb.snippet(sa.search_document, max_num_chars => %d), ''),
			NULLIF(sa.snippet_basis, ''),
			NULLIF(sa.search_document, ''),
			COALESCE(sa.primary_label, '')
		) AS snippet
	FROM kb.search_artifacts sa
	WHERE %s
)
SELECT artifact_type, artifact_id, input_record_id, primary_label, secondary_label, source_title, source_filename, source_line_spans, semantic_payload, score, snippet
FROM ranked
%s
ORDER BY score DESC, artifact_id ASC
LIMIT $%d OFFSET $%d`,
		maxInt(80, cfg.previewMaxWords*12),
		whereSQL,
		minRankClause,
		len(args)-1, len(args),
	)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanArtifactSearchRows(rows, pageSize)
}

// scanArtifactSearchRows scans the shared 11-column result shape (used by both the
// lexical and hybrid queries) into artifactSearchResult values.
func scanArtifactSearchRows(rows *sql.Rows, capacity int) ([]artifactSearchResult, error) {
	if capacity < 0 {
		capacity = 0
	}
	results := make([]artifactSearchResult, 0, capacity)
	for rows.Next() {
		var item artifactSearchResult
		var sourceLineSpans []byte
		var semanticPayload []byte
		if err := rows.Scan(
			&item.ArtifactType,
			&item.ArtifactID,
			&item.InputRecordID,
			&item.PrimaryLabel,
			&item.SecondaryLabel,
			&item.SourceTitle,
			&item.SourceFilename,
			&sourceLineSpans,
			&semanticPayload,
			&item.Score,
			&item.Snippet,
		); err != nil {
			return nil, err
		}
		item.SourceLineSpans = jsonArrayOrEmpty(sourceLineSpans)
		if len(semanticPayload) == 0 {
			item.SemanticPayload = json.RawMessage("{}")
		} else {
			item.SemanticPayload = json.RawMessage(semanticPayload)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func buildRegistrySearchWhereClause(artifactType string, queryText string, filters artifactSearchFilters, cfg registrySearchConfig) (string, []any) {
	tsQueryExpr := "websearch_to_tsquery"
	if !cfg.phraseFriendly {
		tsQueryExpr = "plainto_tsquery"
	}

	ftsClause := fmt.Sprintf("COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, ''))) @@ %s('%s', $1)", cfg.dictionary, tsQueryExpr, cfg.dictionary)
	if containsCJKText(queryText) {
		ftsClause = fmt.Sprintf(`(%s
			OR coalesce(sa.primary_label, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.secondary_label, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.snippet_basis, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.search_document, '') ILIKE '%%' || $1 || '%%')`, ftsClause)
	}

	clauses, args, _ := appendArtifactFilterClauses([]string{ftsClause}, []any{queryText}, 2, artifactType, filters)
	return strings.Join(clauses, " AND "), args
}

func buildParadeDBSearchWhereClause(artifactType string, queryText string, filters artifactSearchFilters, startArg int) (string, []any) {
	// ParadeDB's modern SQL API uses field-level text operators. The BM25
	// migration indexes these columns with a jieba tokenizer, so this path does
	// not need the PostgreSQL FTS CJK substring fallback.
	searchClause := `(
		sa.search_document ||| $1
		OR sa.primary_label ||| $1
		OR sa.secondary_label ||| $1
		OR sa.snippet_basis ||| $1
		OR sa.source_title ||| $1
		OR sa.source_filename ||| $1
	)`
	clauses, args, _ := appendArtifactFilterClauses([]string{searchClause}, []any{queryText}, startArg, artifactType, filters)
	return strings.Join(clauses, " AND "), args
}

// appendArtifactFilterClauses appends the artifact_type selector and all structural
// filters (record id, category, semantic_payload facets) onto clauses/args using
// positional placeholders starting at nextArg, returning the next free index. The
// artifact_type selector is prepended so it keeps the lowest placeholder number
// (preserving the historical numbering asserted by the WHERE-builder tests). Shared
// by the lexical WHERE builder and the hybrid structural-filter builder.
func appendArtifactFilterClauses(clauses []string, args []any, nextArg int, artifactType string, filters artifactSearchFilters) ([]string, []any, int) {
	if strings.TrimSpace(artifactType) != "" && strings.TrimSpace(artifactType) != "all" {
		clauses = append([]string{fmt.Sprintf("sa.artifact_type = $%d", nextArg)}, clauses...)
		args = append(args, strings.TrimSpace(artifactType))
		nextArg++
	} else if len(filters.ArtifactTypes) > 0 {
		placeholders := make([]string, 0, len(filters.ArtifactTypes))
		for _, value := range filters.ArtifactTypes {
			placeholders = append(placeholders, "$"+strconv.Itoa(nextArg))
			args = append(args, value)
			nextArg++
		}
		clauses = append([]string{fmt.Sprintf("sa.artifact_type IN (%s)", strings.Join(placeholders, ", "))}, clauses...)
	}

	if filters.InputRecordID != nil {
		clauses = append(clauses, fmt.Sprintf("sa.input_record_id = $%d", nextArg))
		args = append(args, *filters.InputRecordID)
		nextArg++
	}
	if filters.CategoryPath != "" {
		clauses = append(clauses, fmt.Sprintf("sa.category_paths::text ILIKE $%d", nextArg))
		args = append(args, "%"+filters.CategoryPath+"%")
		nextArg++
	}
	if filters.TopicType != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'topic_type', '') = $%d", nextArg))
		args = append(args, filters.TopicType)
		nextArg++
	}
	if filters.SceneType != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'scene_type', '') = $%d", nextArg))
		args = append(args, filters.SceneType)
		nextArg++
	}
	if filters.ProvisionType != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'provision_type', '') = $%d", nextArg))
		args = append(args, filters.ProvisionType)
		nextArg++
	}
	if filters.ProductType != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'product_type', '') = $%d", nextArg))
		args = append(args, filters.ProductType)
		nextArg++
	}
	if filters.RelationType != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'relation_type', '') = $%d", nextArg))
		args = append(args, filters.RelationType)
		nextArg++
	}
	if filters.InventoryCategory != "" {
		// item_categories is a JSON array in the semantic payload; match membership.
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->'item_categories', '[]'::jsonb) @> to_jsonb($%d::text)", nextArg))
		args = append(args, filters.InventoryCategory)
		nextArg++
	}
	if filters.Manufacturer != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'manufacturer', '') = $%d", nextArg))
		args = append(args, filters.Manufacturer)
		nextArg++
	}
	if filters.Brand != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'brand', '') = $%d", nextArg))
		args = append(args, filters.Brand)
		nextArg++
	}
	if filters.ModelNumber != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'model_number', '') = $%d", nextArg))
		args = append(args, filters.ModelNumber)
		nextArg++
	}
	if filters.PartNumber != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'part_number', '') = $%d", nextArg))
		args = append(args, filters.PartNumber)
		nextArg++
	}
	if filters.ValidationStatus != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(sa.semantic_payload->>'validation_status', '') = $%d", nextArg))
		args = append(args, filters.ValidationStatus)
		nextArg++
	}

	return clauses, args, nextArg
}

// buildStructuralFilterClauses returns only the structural filters (no FTS clause,
// no query text) for the hybrid-search CTEs, with placeholders starting at startArg.
func buildStructuralFilterClauses(artifactType string, filters artifactSearchFilters, startArg int) ([]string, []any, int) {
	return appendArtifactFilterClauses(nil, nil, startArg, artifactType, filters)
}

// queryHybridSearchResults runs the flag-gated hybrid query: a lexical candidate
// list (tsvector ts_rank_cd, same expressions as the lexical path) and a semantic
// candidate list (pgvector cosine distance), fused with Reciprocal Rank Fusion.
//
// Parameter layout: $1 = query text, $2 = query embedding (::vector), $3.. =
// structural filters (shared by both CTEs), then page size and offset. Structural
// filters apply to BOTH lists; the FTS clause constrains only the lexical list, so
// semantically-similar artifacts that share no query terms can still surface.
//
// NOTE: the result total reported by the handler still comes from the lexical
// countRegistrySearchResults, so it can undercount semantic-only matches. Acceptable
// for the interim; revisit if exact hybrid totals are needed.
func queryHybridSearchResults(db *sql.DB, artifactType string, queryText string, vec []float64, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	offset := (page - 1) * pageSize

	tsQueryExpr := "websearch_to_tsquery"
	if !cfg.phraseFriendly {
		tsQueryExpr = "plainto_tsquery"
	}

	ftsClause := fmt.Sprintf("COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, ''))) @@ %s('%s', $1)", cfg.dictionary, tsQueryExpr, cfg.dictionary)
	lexScore := fmt.Sprintf("ts_rank_cd(COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, ''))), query_input.query)", cfg.dictionary)
	if containsCJKText(queryText) {
		ftsClause = fmt.Sprintf(`(%s
			OR coalesce(sa.primary_label, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.secondary_label, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.snippet_basis, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(sa.search_document, '') ILIKE '%%' || $1 || '%%')`, ftsClause)
		lexScore += `
			+ CASE WHEN coalesce(sa.primary_label, '') ILIKE '%' || $1 || '%' THEN 1.25 ELSE 0 END
			+ CASE WHEN coalesce(sa.secondary_label, '') ILIKE '%' || $1 || '%' THEN 0.50 ELSE 0 END
			+ CASE WHEN coalesce(sa.snippet_basis, '') ILIKE '%' || $1 || '%' THEN 0.40 ELSE 0 END
			+ CASE WHEN coalesce(sa.search_document, '') ILIKE '%' || $1 || '%' THEN 0.20 ELSE 0 END`
	}

	structuralClauses, structuralArgs, nextArg := buildStructuralFilterClauses(artifactType, filters, 3)
	structuralSQL := ""
	if len(structuralClauses) > 0 {
		structuralSQL = " AND " + strings.Join(structuralClauses, " AND ")
	}

	snippetOptions := fmt.Sprintf("MaxWords=%d, MinWords=%d, ShortWord=2, HighlightAll=false, MaxFragments=1, FragmentDelimiter=' ... '", cfg.previewMaxWords, maxInt(6, cfg.previewMaxWords/2))
	escapedSnippetOptions := escapeSQLLiteral(snippetOptions)

	pageSizeArg := nextArg
	offsetArg := nextArg + 1

	sqlText := fmt.Sprintf(`
WITH query_input AS (
	SELECT %s('%s', $1) AS query
),
lexical AS (
	SELECT sa.artifact_type AS artifact_type, sa.artifact_id AS artifact_id,
		ROW_NUMBER() OVER (ORDER BY %s DESC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	CROSS JOIN query_input
	WHERE %s%s
	ORDER BY rnk
	LIMIT %d
),
semantic AS (
	SELECT sa.artifact_type AS artifact_type, sa.artifact_id AS artifact_id,
		ROW_NUMBER() OVER (ORDER BY sa.embedding <=> $2::vector ASC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE sa.embedding IS NOT NULL%s
	ORDER BY rnk
	LIMIT %d
),
fused AS (
	SELECT
		COALESCE(l.artifact_type, s.artifact_type) AS artifact_type,
		COALESCE(l.artifact_id, s.artifact_id) AS artifact_id,
		COALESCE(1.0 / (%d + l.rnk), 0.0) + COALESCE(1.0 / (%d + s.rnk), 0.0) AS score
	FROM lexical l
	FULL OUTER JOIN semantic s
		ON l.artifact_type = s.artifact_type AND l.artifact_id = s.artifact_id
)
SELECT
	sa.artifact_type,
	sa.artifact_id,
	sa.input_record_id,
	COALESCE(sa.primary_label, '') AS primary_label,
	COALESCE(sa.secondary_label, '') AS secondary_label,
	COALESCE(sa.source_title, '') AS source_title,
	COALESCE(sa.source_filename, '') AS source_filename,
	COALESCE(sa.source_line_spans, '[]'::jsonb) AS source_line_spans,
	COALESCE(sa.semantic_payload, '{}'::jsonb) AS semantic_payload,
	f.score AS score,
	ts_headline(
		'%s',
		COALESCE(NULLIF(sa.snippet_basis, ''), NULLIF(sa.search_document, ''), COALESCE(sa.primary_label, '')),
		query_input.query,
		'%s'
	) AS snippet
FROM fused f
JOIN kb.search_artifacts sa
	ON sa.artifact_type = f.artifact_type AND sa.artifact_id = f.artifact_id
CROSS JOIN query_input
ORDER BY f.score DESC, sa.artifact_id ASC
LIMIT $%d OFFSET $%d`,
		tsQueryExpr, cfg.dictionary,
		lexScore,
		ftsClause, structuralSQL,
		hybridCandidateLimit,
		structuralSQL,
		hybridCandidateLimit,
		rrfK, rrfK,
		cfg.dictionary,
		escapedSnippetOptions,
		pageSizeArg, offsetArg,
	)

	args := make([]any, 0, len(structuralArgs)+4)
	args = append(args, queryText, kbsearch.FormatVectorLiteral(vec))
	args = append(args, structuralArgs...)
	args = append(args, pageSize, offset)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactSearchRows(rows, pageSize)
}

func queryHybridSearchResultsParadeDB(db *sql.DB, artifactType string, queryText string, vec []float64, filters artifactSearchFilters, page int, pageSize int) ([]artifactSearchResult, error) {
	offset := (page - 1) * pageSize

	lexicalWhereSQL, lexicalArgs := buildParadeDBSearchWhereClause(artifactType, queryText, filters, 3)
	structuralClauses, structuralArgs, nextArg := buildStructuralFilterClauses(artifactType, filters, 3)
	structuralSQL := ""
	if len(structuralClauses) > 0 {
		structuralSQL = " AND " + strings.Join(structuralClauses, " AND ")
	}

	pageSizeArg := nextArg
	offsetArg := nextArg + 1

	sqlText := fmt.Sprintf(`
WITH lexical AS (
	SELECT sa.artifact_type AS artifact_type, sa.artifact_id AS artifact_id,
		ROW_NUMBER() OVER (ORDER BY pdb.score(sa.artifact_id) DESC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE %s
	ORDER BY rnk
	LIMIT %d
),
semantic AS (
	SELECT sa.artifact_type AS artifact_type, sa.artifact_id AS artifact_id,
		ROW_NUMBER() OVER (ORDER BY sa.embedding <=> $2::vector ASC, sa.artifact_id ASC) AS rnk
	FROM kb.search_artifacts sa
	WHERE sa.embedding IS NOT NULL%s
	ORDER BY rnk
	LIMIT %d
),
fused AS (
	SELECT
		COALESCE(l.artifact_type, s.artifact_type) AS artifact_type,
		COALESCE(l.artifact_id, s.artifact_id) AS artifact_id,
		COALESCE(1.0 / (%d + l.rnk), 0.0) + COALESCE(1.0 / (%d + s.rnk), 0.0) AS score
	FROM lexical l
	FULL OUTER JOIN semantic s
		ON l.artifact_type = s.artifact_type AND l.artifact_id = s.artifact_id
)
SELECT
	sa.artifact_type,
	sa.artifact_id,
	sa.input_record_id,
	COALESCE(sa.primary_label, '') AS primary_label,
	COALESCE(sa.secondary_label, '') AS secondary_label,
	COALESCE(sa.source_title, '') AS source_title,
	COALESCE(sa.source_filename, '') AS source_filename,
	COALESCE(sa.source_line_spans, '[]'::jsonb) AS source_line_spans,
	COALESCE(sa.semantic_payload, '{}'::jsonb) AS semantic_payload,
	f.score AS score,
	COALESCE(NULLIF(sa.snippet_basis, ''), NULLIF(sa.search_document, ''), COALESCE(sa.primary_label, '')) AS snippet
FROM fused f
JOIN kb.search_artifacts sa
	ON sa.artifact_type = f.artifact_type AND sa.artifact_id = f.artifact_id
ORDER BY f.score DESC, sa.artifact_id ASC
LIMIT $%d OFFSET $%d`,
		lexicalWhereSQL,
		hybridCandidateLimit,
		structuralSQL,
		hybridCandidateLimit,
		rrfK, rrfK,
		pageSizeArg, offsetArg,
	)

	args := make([]any, 0, len(structuralArgs)+4)
	args = append(args, queryText, kbsearch.FormatVectorLiteral(vec))
	args = append(args, lexicalArgs[1:]...)
	args = append(args, pageSize, offset)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactSearchRows(rows, pageSize)
}
