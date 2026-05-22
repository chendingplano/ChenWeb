package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
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
	cfg := appconfig.GetMetricSearchConfig()
	return registrySearchConfig{
		dictionary:      sanitizeTSConfig(cfg.Dictionary),
		defaultPageSize: cfg.DefaultPageSize,
		maxPageSize:     cfg.MaxPageSize,
		previewMaxWords: cfg.PreviewMaxWords,
		phraseFriendly:  cfg.PhraseFriendly,
		minRank:         cfg.MinRank,
	}
}

func countRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, cfg registrySearchConfig) (int64, error) {
	whereSQL, args := buildRegistrySearchWhereClause(artifactType, queryText, filters, cfg)
	sqlText := fmt.Sprintf(`SELECT COUNT(*) FROM kb.search_artifacts sa WHERE %s`, whereSQL)
	var total int64
	err := db.QueryRow(sqlText, args...).Scan(&total)
	return total, err
}

func queryRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
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

	results := make([]artifactSearchResult, 0, pageSize)
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

	clauses := []string{ftsClause}
	args := []any{queryText}
	nextArg := 2

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

	return strings.Join(clauses, " AND "), args
}
