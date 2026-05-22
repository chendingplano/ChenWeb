package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricSearchFilters struct {
	InputRecordID    *int64 `json:"input_record_id,omitempty"`
	IsExplicitMetric *bool  `json:"is_explicit_metric,omitempty"`
	ValueClass       string `json:"value_class,omitempty"`
	ValueDataType    string `json:"value_data_type,omitempty"`
	MetricUnit       string `json:"metric_unit,omitempty"`
}

type metricSearchResult struct {
	ArtifactID         string          `json:"artifact_id"`
	ID                 int64           `json:"id"`
	MetricID           string          `json:"metric_id,omitempty"`
	InputRecordID      int64           `json:"input_record_id"`
	InputFilename      string          `json:"input_filename,omitempty"`
	MetricName         string          `json:"metric_name,omitempty"`
	MetricNameEn       string          `json:"metric_name_en,omitempty"`
	MetricSubject      string          `json:"metric_subject,omitempty"`
	MetricSubjectEn    string          `json:"metric_subject_en,omitempty"`
	MetricValue        string          `json:"metric_value,omitempty"`
	MetricUnit         string          `json:"metric_unit,omitempty"`
	MetricUnitEn       string          `json:"metric_unit_en,omitempty"`
	ValueClass         string          `json:"value_class,omitempty"`
	ValueClassEn       string          `json:"value_class_en,omitempty"`
	ValueDataType      string          `json:"value_data_type,omitempty"`
	IsExplicitMetric   *bool           `json:"is_explicit_metric,omitempty"`
	TableNameOrSection string          `json:"table_name_or_section,omitempty"`
	MetricKeywords     json.RawMessage `json:"metric_keywords,omitempty"`
	MetricKeywordsEn   json.RawMessage `json:"metric_keywords_en,omitempty"`
	SourceLineSpans    json.RawMessage `json:"source_line_spans,omitempty"`
	Score              float64         `json:"score"`
	Snippet            string          `json:"snippet"`
	PrimaryLabel       string          `json:"primary_label"`
}

type metricSearchResponse struct {
	Status         bool                 `json:"status"`
	Query          string               `json:"query"`
	ArtifactType   string               `json:"artifact_type"`
	Page           int                  `json:"page"`
	PageSize       int                  `json:"page_size"`
	Total          int64                `json:"total"`
	AppliedFilters metricSearchFilters  `json:"applied_filters"`
	Results        []metricSearchResult `json:"results"`
}

type metricSearchConfig struct {
	dictionary      string
	defaultPageSize int
	maxPageSize     int
	previewMaxWords int
	phraseFriendly  bool
	minRank         float64
	weights         appconfig.MetricSearchWeightsConfig
}

func loadMetricSearchConfig() metricSearchConfig {
	cfg := appconfig.GetMetricSearchConfig()
	return metricSearchConfig{
		dictionary:      sanitizeTSConfig(cfg.Dictionary),
		defaultPageSize: cfg.DefaultPageSize,
		maxPageSize:     cfg.MaxPageSize,
		previewMaxWords: cfg.PreviewMaxWords,
		phraseFriendly:  cfg.PhraseFriendly,
		minRank:         cfg.MinRank,
		weights:         cfg.Weights,
	}
}

func sanitizeTSConfig(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "english":
		return "english"
	case "simple":
		return "simple"
	default:
		return "simple"
	}
}

func parseOptionalBoolQuery(raw string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	switch strings.ToLower(raw) {
	case "true", "1", "yes", "y":
		v := true
		return &v, nil
	case "false", "0", "no", "n":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("must be boolean text")
	}
}

func SearchMetrics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MS_001")
	defer rc.Close()
	logger := rc.GetLogger()

	queryText := strings.TrimSpace(c.QueryParam("q"))
	if queryText == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "q is required (CWB_KB_MS_010)",
		})
	}

	cfg := loadMetricSearchConfig()
	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), cfg.defaultPageSize)
	if pageSize > cfg.maxPageSize {
		pageSize = cfg.maxPageSize
	}

	inputRecordID, err := parseOptionalPositiveInt64(c.QueryParam("input_record_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid input_record_id: %v (CWB_KB_MS_011)", err),
		})
	}
	isExplicitMetric, err := parseOptionalBoolQuery(c.QueryParam("is_explicit_metric"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid is_explicit_metric: %v (CWB_KB_MS_012)", err),
		})
	}

	filters := metricSearchFilters{
		InputRecordID:    inputRecordID,
		IsExplicitMetric: isExplicitMetric,
		ValueClass:       strings.TrimSpace(c.QueryParam("value_class")),
		ValueDataType:    strings.TrimSpace(c.QueryParam("value_data_type")),
		MetricUnit:       strings.TrimSpace(c.QueryParam("metric_unit")),
	}

	total, err := countMetricSearchResults(ApiTypes.ProjectDBHandle, queryText, filters, cfg)
	if err != nil {
		logger.Error("count metric search failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to count metric search results (CWB_KB_MS_013)",
		})
	}

	results, err := queryMetricSearchResults(ApiTypes.ProjectDBHandle, queryText, filters, page, pageSize, cfg)
	if err != nil {
		logger.Error("query metric search failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to search metrics (CWB_KB_MS_014)",
		})
	}

	return c.JSON(http.StatusOK, metricSearchResponse{
		Status:         true,
		Query:          queryText,
		ArtifactType:   "metric",
		Page:           page,
		PageSize:       pageSize,
		Total:          total,
		AppliedFilters: filters,
		Results:        results,
	})
}

func countMetricSearchResults(db *sql.DB, queryText string, filters metricSearchFilters, cfg metricSearchConfig) (int64, error) {
	whereSQL, args := buildMetricSearchWhereClause(queryText, filters, cfg)
	sqlText := fmt.Sprintf(`SELECT COUNT(*) FROM kb.metrics m WHERE %s`, whereSQL)
	var total int64
	err := db.QueryRow(sqlText, args...).Scan(&total)
	return total, err
}

func queryMetricSearchResults(db *sql.DB, queryText string, filters metricSearchFilters, page, pageSize int, cfg metricSearchConfig) ([]metricSearchResult, error) {
	whereSQL, args := buildMetricSearchWhereClause(queryText, filters, cfg)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	tsQueryExpr := "websearch_to_tsquery"
	if !cfg.phraseFriendly {
		tsQueryExpr = "plainto_tsquery"
	}
	snippetOptions := fmt.Sprintf("MaxWords=%d, MinWords=%d, ShortWord=2, HighlightAll=false, MaxFragments=1, FragmentDelimiter=' ... '", cfg.previewMaxWords, maxInt(6, cfg.previewMaxWords/2))
	escapedSnippetOptions := escapeSQLLiteral(snippetOptions)

	scoreExpr := fmt.Sprintf(`
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.metric_name, '') || ' ' || coalesce(m.metric_name_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.metric_subject, '') || ' ' || coalesce(m.metric_subject_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', kb.metric_search_jsonb_array_text(m.metric_keywords) || ' ' || kb.metric_search_jsonb_array_text(m.metric_keywords_en)), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.metric_desc, '') || ' ' || coalesce(m.metric_desc_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.metric_context, '') || ' ' || coalesce(m.metric_context_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.value_class, '') || ' ' || coalesce(m.value_class_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.metric_unit, '') || ' ' || coalesce(m.metric_unit_en, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', coalesce(m.table_name_or_section, '')), query_input.query) +
        %f * ts_rank_cd(to_tsvector('%s', kb.metric_search_jsonb_array_text(m.category_paths) || ' ' || kb.metric_search_jsonb_array_text(m.category_paths_en)), query_input.query)
    `,
		cfg.weights.MetricName, cfg.dictionary,
		cfg.weights.MetricSubject, cfg.dictionary,
		cfg.weights.MetricKeywords, cfg.dictionary,
		cfg.weights.MetricDesc, cfg.dictionary,
		cfg.weights.MetricContext, cfg.dictionary,
		cfg.weights.ValueClass, cfg.dictionary,
		cfg.weights.MetricUnit, cfg.dictionary,
		cfg.weights.TableNameOrSection, cfg.dictionary,
		cfg.weights.CategoryPaths, cfg.dictionary,
	)
	if containsCJKText(queryText) {
		scoreExpr = scoreExpr + `
        + CASE WHEN coalesce(m.metric_name, '') ILIKE '%' || $1 || '%' THEN 1.25 ELSE 0 END
        + CASE WHEN coalesce(m.metric_subject, '') ILIKE '%' || $1 || '%' THEN 0.75 ELSE 0 END
        + CASE WHEN coalesce(m.metric_desc, '') ILIKE '%' || $1 || '%' THEN 0.40 ELSE 0 END
        + CASE WHEN coalesce(m.metric_context, '') ILIKE '%' || $1 || '%' THEN 0.30 ELSE 0 END
        + CASE WHEN coalesce(m.search_document, '') ILIKE '%' || $1 || '%' THEN 0.20 ELSE 0 END
    `
	}

	minRankClause := ""
	if cfg.minRank > 0 {
		minRankClause = fmt.Sprintf("WHERE score >= %f", cfg.minRank)
	}

	sqlText := fmt.Sprintf(`
WITH query_input AS (
    SELECT %s('%s', $1) AS query
),
ranked AS (
    SELECT
        m.id,
        COALESCE(m.metric_id, '') AS metric_id,
        m.input_record_id,
        COALESCE(i.staging_filename, '') AS input_filename,
        COALESCE(m.metric_name, '') AS metric_name,
        COALESCE(m.metric_name_en, '') AS metric_name_en,
        COALESCE(m.metric_subject, '') AS metric_subject,
        COALESCE(m.metric_subject_en, '') AS metric_subject_en,
        COALESCE(m.metric_value, '') AS metric_value,
        COALESCE(m.metric_unit, '') AS metric_unit,
        COALESCE(m.metric_unit_en, '') AS metric_unit_en,
        COALESCE(m.value_class, '') AS value_class,
        COALESCE(m.value_class_en, '') AS value_class_en,
        COALESCE(m.value_data_type, '') AS value_data_type,
        m.is_explicit_metric,
        COALESCE(m.table_name_or_section, '') AS table_name_or_section,
        COALESCE(m.metric_keywords, '[]'::jsonb) AS metric_keywords,
        COALESCE(m.metric_keywords_en, '[]'::jsonb) AS metric_keywords_en,
        COALESCE(m.source_line_spans, '[]'::jsonb) AS source_line_spans,
        %s AS score,
        ts_headline(
            '%s',
            COALESCE(NULLIF(m.metric_desc, ''), NULLIF(m.metric_context, ''), NULLIF(m.search_document, ''), COALESCE(m.metric_name, '')),
            query_input.query,
            '%s'
        ) AS snippet
    FROM kb.metrics m
    LEFT JOIN kb.inputs i ON i.id = m.input_record_id
    CROSS JOIN query_input
    WHERE %s
),
scored AS (
    SELECT * FROM ranked
    %s
)
SELECT
    id, metric_id, input_record_id, input_filename, metric_name, metric_name_en,
    metric_subject, metric_subject_en, metric_value, metric_unit, metric_unit_en,
    value_class, value_class_en, value_data_type, is_explicit_metric, table_name_or_section,
    metric_keywords, metric_keywords_en, source_line_spans, score, snippet
FROM scored
ORDER BY score DESC, id ASC
LIMIT $%d OFFSET $%d
	`, tsQueryExpr, cfg.dictionary, scoreExpr, cfg.dictionary, escapedSnippetOptions, whereSQL, minRankClause, len(args)-1, len(args))

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]metricSearchResult, 0)
	for rows.Next() {
		var (
			record           metricSearchResult
			metricKeywords   []byte
			metricKeywordsEn []byte
			sourceLineSpans  []byte
			isExplicitMetric sql.NullBool
		)
		if err := rows.Scan(
			&record.ID, &record.MetricID, &record.InputRecordID, &record.InputFilename,
			&record.MetricName, &record.MetricNameEn, &record.MetricSubject, &record.MetricSubjectEn,
			&record.MetricValue, &record.MetricUnit, &record.MetricUnitEn,
			&record.ValueClass, &record.ValueClassEn, &record.ValueDataType,
			&isExplicitMetric, &record.TableNameOrSection, &metricKeywords, &metricKeywordsEn,
			&sourceLineSpans, &record.Score, &record.Snippet,
		); err != nil {
			return nil, err
		}
		if isExplicitMetric.Valid {
			v := isExplicitMetric.Bool
			record.IsExplicitMetric = &v
		}
		if len(metricKeywords) > 0 {
			record.MetricKeywords = json.RawMessage(metricKeywords)
		}
		if len(metricKeywordsEn) > 0 {
			record.MetricKeywordsEn = json.RawMessage(metricKeywordsEn)
		}
		if len(sourceLineSpans) > 0 {
			record.SourceLineSpans = json.RawMessage(sourceLineSpans)
		}
		seq := strings.TrimSpace(firstNonEmptyString(lastMetricSequence(record.MetricID), strconv.FormatInt(record.ID, 10)))
		record.ArtifactID = kbsearch.BuildArtifactID(record.InputRecordID, "metric", seq)
		record.PrimaryLabel = strings.TrimSpace(firstNonEmptyString(record.MetricName, record.MetricSubject, "Metric #"+strconv.FormatInt(record.ID, 10)))
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func buildMetricSearchWhereClause(queryText string, filters metricSearchFilters, cfg metricSearchConfig) (string, []any) {
	tsQueryExpr := "websearch_to_tsquery"
	if !cfg.phraseFriendly {
		tsQueryExpr = "plainto_tsquery"
	}

	ftsClause := fmt.Sprintf(
		"COALESCE(m.search_vector, to_tsvector('%s', COALESCE(m.search_document, ''))) @@ %s('%s', $1)",
		cfg.dictionary,
		tsQueryExpr,
		cfg.dictionary,
	)
	if containsCJKText(queryText) {
		ftsClause = fmt.Sprintf(`(%s
			OR coalesce(m.metric_name, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(m.metric_subject, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(m.metric_desc, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(m.metric_context, '') ILIKE '%%' || $1 || '%%'
			OR coalesce(m.search_document, '') ILIKE '%%' || $1 || '%%')`, ftsClause)
	}

	conditions := []string{ftsClause}
	args := []any{queryText}
	nextArg := 2

	if filters.InputRecordID != nil {
		conditions = append(conditions, fmt.Sprintf("m.input_record_id = $%d", nextArg))
		args = append(args, *filters.InputRecordID)
		nextArg++
	}
	if filters.IsExplicitMetric != nil {
		conditions = append(conditions, fmt.Sprintf("m.is_explicit_metric = $%d", nextArg))
		args = append(args, *filters.IsExplicitMetric)
		nextArg++
	}
	if filters.ValueClass != "" {
		conditions = append(conditions, fmt.Sprintf("coalesce(m.value_class, '') = $%d", nextArg))
		args = append(args, filters.ValueClass)
		nextArg++
	}
	if filters.ValueDataType != "" {
		conditions = append(conditions, fmt.Sprintf("coalesce(m.value_data_type, '') = $%d", nextArg))
		args = append(args, filters.ValueDataType)
		nextArg++
	}
	if filters.MetricUnit != "" {
		conditions = append(conditions, fmt.Sprintf("(coalesce(m.metric_unit, '') = $%d OR coalesce(m.metric_unit_en, '') = $%d)", nextArg, nextArg))
		args = append(args, filters.MetricUnit)
		nextArg++
	}

	return strings.Join(conditions, " AND "), args
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func escapeSQLLiteral(raw string) string {
	return strings.ReplaceAll(raw, "'", "''")
}

func lastMetricSequence(metricID string) string {
	metricID = strings.TrimSpace(metricID)
	if metricID == "" {
		return ""
	}
	parts := strings.Split(metricID, "_")
	return strings.TrimSpace(parts[len(parts)-1])
}
