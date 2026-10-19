package kbhandler

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/metricvalue"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricRecord struct {
	ID                  int64           `json:"id"`
	InputRecordID       int64           `json:"input_record_id"`
	MetricID            *string         `json:"metric_id,omitempty"`
	EventID             *string         `json:"event_id,omitempty"`
	InputFilename       string          `json:"input_filename"`
	MetricName          *string         `json:"metric_name,omitempty"`
	MetricNameEn        *string         `json:"metric_name_en,omitempty"`
	SourceLineSpans     json.RawMessage `json:"source_line_spans,omitempty"`
	MetricSubject       *string         `json:"metric_subject,omitempty"`
	MetricSubjectEn     *string         `json:"metric_subject_en,omitempty"`
	MetricDesc          *string         `json:"metric_desc,omitempty"`
	MetricDescEn        *string         `json:"metric_desc_en,omitempty"`
	MetricContext       *string         `json:"metric_context,omitempty"`
	MetricContextEn     *string         `json:"metric_context_en,omitempty"`
	MetricKeywords      json.RawMessage `json:"metric_keywords,omitempty"`
	MetricKeywordsEn    json.RawMessage `json:"metric_keywords_en,omitempty"`
	SearchDocument      *string         `json:"search_document,omitempty"`
	ModelName           *string         `json:"model_name,omitempty"`
	LocationType        *string         `json:"location_type,omitempty"`
	MetricUnit          *string         `json:"metric_unit,omitempty"`
	MetricUnitEn        *string         `json:"metric_unit_en,omitempty"`
	ValueStructure      json.RawMessage `json:"value_structure,omitempty"`
	MetricValue         *string         `json:"metric_value,omitempty"`
	ValueDataType       *string         `json:"value_data_type,omitempty"`
	ValueRangeType      *string         `json:"value_range_type,omitempty"`
	ValueClass          *string         `json:"value_class,omitempty"`
	ValueClassEn        *string         `json:"value_class_en,omitempty"`
	FormulaOrDefinition *string         `json:"formula_or_definition,omitempty"`
	ThresholdOrTarget   *string         `json:"threshold_or_target,omitempty"`
	MeasurementFreq     *string         `json:"measurement_frequency,omitempty"`
	Confidence          *float64        `json:"confidence,omitempty"`
	IsExplicitMetric    *bool           `json:"is_explicit_metric,omitempty"`
	DocumentTitle       *string         `json:"document_title,omitempty"`
	DocumentDocNo       *string         `json:"document_doc_no,omitempty"`
	ObjectName          *string         `json:"object_name,omitempty"`
	TableNameOrSection  *string         `json:"table_name_or_section,omitempty"`
	ReasoningTags       json.RawMessage `json:"reasoning_tags,omitempty"`
	CreatedAt           string          `json:"created_at,omitempty"`
	// ObjectNodeCanonicalName is kb.object_nodes.canonical_name for the object
	// this metric is linked to via kb.artifact_objects, when reconciled.
	// Distinct from ObjectName (the raw extraction-time name).
	ObjectNodeCanonicalName *string `json:"object_node_canonical_name,omitempty"`
	// ValueRangeTypeError is set by extract_metrics (ADR 2026081401 DR6) when
	// ValueRangeType has no approved kb.metric_value_range_type_map mapping.
	ValueRangeTypeError *string `json:"value_range_type_error,omitempty"`
	// KeywordConceptID and MetricDefinitionTermID are governed identifiers from
	// spec 2026080403 §19 step 12.
	KeywordConceptID       *string `json:"keyword_concept_id,omitempty"`
	MetricDefinitionTermID *string `json:"metric_definition_term_id,omitempty"`
	// SourceTableRows is kb.metrics.source_table_rows: the table rows this metric
	// came from (openspec change table-row-context).
	SourceTableRows json.RawMessage `json:"source_table_rows,omitempty"`
	// ProvisionID is kb.metrics.provision_id: the provision clause this row is a
	// criterion of (ADR 2026100603 DR2). Always serialized; null until linked.
	ProvisionID *int64 `json:"provision_id"`
	// Ref* are set only on a resolved reference row <rec>_rfm_<n>: a metric another document
	// defines, which this document requires through a reference (openspec change
	// extract-references-processor). RefMetricID is the original metric in RefInputRecordID.
	RefMentionID     *int64  `json:"ref_mention_id,omitempty"`
	RefInputRecordID *int64  `json:"ref_input_record_id,omitempty"`
	RefMetricID      *string `json:"ref_metric_id,omitempty"`
	// Condition / ConditionEn: when, where, for whom or how the value holds
	// (openspec change extract-metrics-conditions).
	Condition   *string `json:"condition,omitempty"`
	ConditionEn *string `json:"condition_en,omitempty"`
	// Frame is kb.metrics.frame: the resources the value uses, its method and its place in a
	// procedure (openspec change metric-frame). Always serialized; null when the row has none.
	Frame json.RawMessage `json:"frame"`
	// TableContext is built at read time from SourceTableRows: header + matched rows
	// + one neighbor data row either side. Never stored.
	TableContext []docprocessing.TableContextWindow `json:"table_context,omitempty"`
	// Dropped rows come from kb.metrics_dropped or testbed.metrics_dropped and are
	// returned only with include_dropped=true. MetricID is the production drop_id
	// or benchmark candidate_id; ID is the negated ID within the selected table.
	Dropped      bool            `json:"dropped,omitempty"`
	RunID        string          `json:"run_id,omitempty"`
	SkillName    string          `json:"skill_name,omitempty"`
	SkillVersion string          `json:"skill_version,omitempty"`
	DropStage    *string         `json:"drop_stage,omitempty"`
	DropReason   *string         `json:"drop_reason,omitempty"`
	DropDecision json.RawMessage `json:"drop_decision,omitempty"`
}

type listMetricsResponse struct {
	Status  bool           `json:"status"`
	Results []metricRecord `json:"results"`
	Total   int            `json:"total"`
	RunID   string         `json:"run_id,omitempty"`
}

type metricDetailResponse struct {
	Status bool         `json:"status"`
	Record metricRecord `json:"record"`
}

func firstMetricSourceLine(spans json.RawMessage) int {
	if len(spans) == 0 {
		return int(^uint(0) >> 1)
	}
	var raw []any
	if err := json.Unmarshal(spans, &raw); err != nil || len(raw) == 0 {
		return int(^uint(0) >> 1)
	}
	first := raw[0]
	switch v := first.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		var digits strings.Builder
		for _, r := range strings.TrimSpace(v) {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
				continue
			}
			break
		}
		if digits.Len() > 0 {
			if n, err := strconv.Atoi(digits.String()); err == nil && n > 0 {
				return n
			}
		}
	case map[string]any:
		for _, key := range []string{"line_number", "line", "line_no", "lineNo"} {
			if val, ok := v[key]; ok {
				switch vv := val.(type) {
				case float64:
					if vv > 0 {
						return int(vv)
					}
				case string:
					if n, err := strconv.Atoi(strings.TrimSpace(vv)); err == nil && n > 0 {
						return n
					}
				}
			}
		}
	}
	return int(^uint(0) >> 1)
}

// Resolve once, then read both accepted and dropped rows using this full identity.
// A new benchmark saved between the two reads cannot mix runs in the response.
const selectBenchmarkRunQuery = `
SELECT skill_name, skill_version, model_name, benchmark_run_id
FROM testbed.metric_benchmark_runs
WHERE input_record_id = $1 AND skill_version = $2 AND ($3 = '' OR model_name = $3)
  AND ($4 = '' OR benchmark_run_id = $4)
ORDER BY created_at DESC, benchmark_run_id DESC, skill_name ASC, model_name ASC
LIMIT 1`

type benchmarkRunKey struct {
	SkillName, SkillVersion, ModelName, RunID string
}

func (run benchmarkRunKey) queryArgs(inputID int64) []any {
	return []any{inputID, run.SkillName, run.SkillVersion, run.ModelName, run.RunID}
}

// testbedMetricsQuery reads an explicitly resolved run, in the same column order
// as the kb.metrics query in ListMetrics; columns testbed lacks are NULL.
const testbedMetricsQuery = `
SELECT
    m.id, m.input_record_id, m.metric_id, NULL::text AS event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, NULL::text AS search_document, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    NULL::text AS object_name,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at,
    NULL::text AS keyword_concept_id, NULL::text AS metric_definition_term_id, NULL::text AS value_range_type_error,
    m.source_table_rows, NULL::bigint AS provision_id,
    NULL::bigint AS ref_mention_id, NULL::bigint AS ref_input_record_id, NULL::text AS ref_metric_id,
    m.condition, m.condition_en, m.frame, m.value_structure
FROM testbed.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.input_record_id = $1
  AND (m.skill_name, m.skill_version, m.model_name, m.run_id) = ($2, $3, $4, $5)
ORDER BY m.id ASC
`

// ListMetrics handles GET /api/v1/kb/metrics?input_record_id=N[&source=testbed].
// source=testbed reads gold metrics from testbed.metrics instead of kb.metrics,
// narrowed by optional skill_version (default: the newest version) and model_name
// (default: any model).
func ListMetrics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	if idStr == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "input_record_id is required (CWB_KB_M_010)",
		})
	}
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_M_011)",
		})
	}

	source := strings.TrimSpace(c.QueryParam("source"))
	if source != "" && source != "kb" && source != "testbed" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid source, expected kb or testbed (CWB_KB_M_012)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	query := `
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.search_document, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    ao.object_name,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at,
    m.keyword_concept_id, m.metric_definition_term_id, m.value_range_type_error,
    m.source_table_rows, m.provision_id,
    m.ref_mention_id, m.ref_input_record_id, m.ref_metric_id,
    m.condition, m.condition_en, m.frame, m.value_structure
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
    SELECT NULLIF(BTRIM(artifact.object_name), '') AS object_name
    FROM kb.artifact_objects artifact
    WHERE artifact.artifact_type = 'metric'
      AND artifact.artifact_id = m.metric_id
      AND NULLIF(BTRIM(artifact.object_name), '') IS NOT NULL
    ORDER BY
      CASE WHEN COALESCE(artifact.object_role, '') IN ('measured_object', 'self') THEN 0 ELSE 1 END,
      artifact.id
    LIMIT 1
) ao ON TRUE
WHERE m.input_record_id = $1
ORDER BY m.id ASC
`
	args := []any{inputID}
	var benchmarkRun benchmarkRunKey
	if source == "testbed" {
		skillVersion := strings.TrimSpace(c.QueryParam("skill_version"))
		modelName := strings.TrimSpace(c.QueryParam("model_name"))
		runID := strings.TrimSpace(c.QueryParam("run_id"))
		if skillVersion == "" {
			skillVersion, err = newestGoldSkillVersion(db)
			if err != nil {
				logger.Error("resolve newest gold skill version failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{
					Status:   false,
					ErrorMsg: "failed to resolve gold metrics skill version (CWB_KB_M_023)",
				})
			}
		}
		logger.Info("list gold metrics", "input_record_id", inputID, "skill_version", skillVersion, "model_name", modelName, "run_id", runID)
		err = db.QueryRowContext(c.Request().Context(), selectBenchmarkRunQuery, inputID, skillVersion, modelName, runID).
			Scan(&benchmarkRun.SkillName, &benchmarkRun.SkillVersion, &benchmarkRun.ModelName, &benchmarkRun.RunID)
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, listMetricsResponse{Status: true, Results: []metricRecord{}})
		}
		if err != nil {
			logger.Error("resolve benchmark run failed", "input_record_id", inputID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve benchmark run (CWB_KB_M_025)"})
		}
		query = testbedMetricsQuery
		args = benchmarkRun.queryArgs(inputID)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("query metrics failed", "source", source, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to retrieve kb metrics (CWB_KB_M_020)",
		})
	}
	defer rows.Close()

	out := make([]metricRecord, 0)
	for rows.Next() {
		var (
			r                   metricRecord
			valueStructureBytes []byte
			spansBytes          []byte
			keywordsBytes       []byte
			keywordsEnBytes     []byte
			reasoningBytes      []byte
			tableRowsBytes      []byte
			frameBytes          []byte
			confidence          sql.NullFloat64
			isExplicit          sql.NullBool
		)
		if err := rows.Scan(
			&r.ID, &r.InputRecordID, &r.MetricID, &r.EventID, &r.InputFilename,
			&r.MetricName, &r.MetricNameEn, &spansBytes, &r.MetricSubject, &r.MetricSubjectEn,
			&r.MetricDesc, &r.MetricDescEn, &r.MetricContext, &r.MetricContextEn,
			&keywordsBytes, &keywordsEnBytes, &r.SearchDocument, &r.ModelName, &r.LocationType, &r.MetricUnit, &r.MetricUnitEn,
			&r.MetricValue, &r.ValueDataType, &r.ValueRangeType, &r.ValueClass, &r.ValueClassEn,
			&r.FormulaOrDefinition, &r.ThresholdOrTarget, &r.MeasurementFreq,
			&confidence, &isExplicit, &r.DocumentTitle, &r.DocumentDocNo, &r.ObjectName,
			&r.TableNameOrSection, &reasoningBytes, &r.CreatedAt,
			&r.KeywordConceptID, &r.MetricDefinitionTermID, &r.ValueRangeTypeError,
			&tableRowsBytes, &r.ProvisionID,
			&r.RefMentionID, &r.RefInputRecordID, &r.RefMetricID,
			&r.Condition, &r.ConditionEn, &frameBytes, &valueStructureBytes,
		); err != nil {
			logger.Error("scan kb.metrics row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to scan kb metrics (CWB_KB_M_021)",
			})
		}
		r.ValueStructure = json.RawMessage(valueStructureBytes)
		if len(spansBytes) > 0 {
			r.SourceLineSpans = json.RawMessage(spansBytes)
		}
		if len(keywordsBytes) > 0 {
			r.MetricKeywords = json.RawMessage(keywordsBytes)
		}
		if len(keywordsEnBytes) > 0 {
			r.MetricKeywordsEn = json.RawMessage(keywordsEnBytes)
		}
		if len(reasoningBytes) > 0 {
			r.ReasoningTags = json.RawMessage(reasoningBytes)
		}
		if len(frameBytes) > 0 {
			r.Frame = json.RawMessage(frameBytes)
		}
		if len(tableRowsBytes) > 0 {
			r.SourceTableRows = json.RawMessage(tableRowsBytes)
		}
		if confidence.Valid {
			v := confidence.Float64
			r.Confidence = &v
		}
		if isExplicit.Valid {
			v := isExplicit.Bool
			r.IsExplicitMetric = &v
		}
		if source == "testbed" {
			r.RunID, r.SkillName, r.SkillVersion = benchmarkRun.RunID, benchmarkRun.SkillName, benchmarkRun.SkillVersion
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.metrics failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to iterate kb metrics (CWB_KB_M_022)",
		})
	}
	if c.QueryParam("include_dropped") == "true" {
		var dropped []metricRecord
		if source == "testbed" {
			dropped, err = listBenchmarkDroppedMetrics(c.Request().Context(), db, inputID, benchmarkRun)
		} else {
			dropped, err = listDroppedMetrics(c.Request().Context(), db, inputID)
		}
		if err != nil {
			logger.Error("query dropped metrics failed", "source", source, "input_record_id", inputID, "run_id", benchmarkRun.RunID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to retrieve dropped metrics (CWB_KB_M_024)",
			})
		}
		out = append(out, dropped...)
	}
	attachMetricTableContexts(c.Request().Context(), db, inputID, out, logger)
	sort.SliceStable(out, func(i, j int) bool {
		left := firstMetricSourceLine(out[i].SourceLineSpans)
		right := firstMetricSourceLine(out[j].SourceLineSpans)
		if left != right {
			return left < right
		}
		return out[i].ID < out[j].ID
	})

	return c.JSON(http.StatusOK, listMetricsResponse{
		Status:  true,
		Results: out,
		Total:   len(out),
		RunID:   benchmarkRun.RunID,
	})
}

// attachMetricTableContexts fills TableContext for metrics that carry table-row
// references. The record's line file is read once, and only when at least one metric
// needs it; a failure is logged and leaves TableContext empty.
func attachMetricTableContexts(ctx context.Context, db *sql.DB, inputID int64, metrics []metricRecord, logger ApiTypes.JimoLogger) {
	need := false
	for i := range metrics {
		if len(metrics[i].SourceTableRows) > 0 {
			need = true
			break
		}
	}
	if !need {
		return
	}
	lines, err := docprocessing.LoadRecordLinesByID(ctx, db, inputID)
	if err != nil {
		logger.Warn("table context: record lines unavailable", "input_record_id", inputID, "err", err)
		return
	}
	for i := range metrics {
		if len(metrics[i].SourceTableRows) == 0 {
			continue
		}
		refs := docprocessing.ParseTableRowRefs(string(metrics[i].SourceTableRows))
		metrics[i].TableContext = docprocessing.TableContextWindows(lines, refs)
	}
}

func fetchMetricByID(db *sql.DB, id int64) (metricRecord, error) {
	const query = `
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.search_document, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    ao.object_name,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at,
    m.keyword_concept_id, m.metric_definition_term_id, m.value_range_type_error,
    m.source_table_rows, m.provision_id,
    m.ref_mention_id, m.ref_input_record_id, m.ref_metric_id,
    m.condition, m.condition_en, m.frame, m.value_structure
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
    SELECT NULLIF(BTRIM(artifact.object_name), '') AS object_name
    FROM kb.artifact_objects artifact
    WHERE artifact.artifact_type = 'metric'
      AND artifact.artifact_id = m.metric_id
      AND NULLIF(BTRIM(artifact.object_name), '') IS NOT NULL
    ORDER BY
      CASE WHEN COALESCE(artifact.object_role, '') IN ('measured_object', 'self') THEN 0 ELSE 1 END,
      artifact.id
    LIMIT 1
) ao ON TRUE
WHERE m.id = $1
`
	var (
		r                   metricRecord
		valueStructureBytes []byte
		spansBytes          []byte
		keywordsBytes       []byte
		keywordsEnBytes     []byte
		reasoningBytes      []byte
		tableRowsBytes      []byte
		frameBytes          []byte
		confidence          sql.NullFloat64
		isExplicit          sql.NullBool
	)
	err := db.QueryRow(query, id).Scan(
		&r.ID, &r.InputRecordID, &r.MetricID, &r.EventID, &r.InputFilename,
		&r.MetricName, &r.MetricNameEn, &spansBytes, &r.MetricSubject, &r.MetricSubjectEn,
		&r.MetricDesc, &r.MetricDescEn, &r.MetricContext, &r.MetricContextEn,
		&keywordsBytes, &keywordsEnBytes, &r.SearchDocument, &r.ModelName, &r.LocationType, &r.MetricUnit, &r.MetricUnitEn,
		&r.MetricValue, &r.ValueDataType, &r.ValueRangeType, &r.ValueClass, &r.ValueClassEn,
		&r.FormulaOrDefinition, &r.ThresholdOrTarget, &r.MeasurementFreq,
		&confidence, &isExplicit, &r.DocumentTitle, &r.DocumentDocNo, &r.ObjectName,
		&r.TableNameOrSection, &reasoningBytes, &r.CreatedAt,
		&r.KeywordConceptID, &r.MetricDefinitionTermID, &r.ValueRangeTypeError,
		&tableRowsBytes, &r.ProvisionID,
		&r.RefMentionID, &r.RefInputRecordID, &r.RefMetricID,
		&r.Condition, &r.ConditionEn, &frameBytes, &valueStructureBytes,
	)
	if err != nil {
		return metricRecord{}, err
	}
	if len(frameBytes) > 0 {
		r.Frame = json.RawMessage(frameBytes)
	}
	if len(tableRowsBytes) > 0 {
		r.SourceTableRows = json.RawMessage(tableRowsBytes)
	}
	r.ValueStructure = json.RawMessage(valueStructureBytes)
	if len(spansBytes) > 0 {
		r.SourceLineSpans = json.RawMessage(spansBytes)
	}
	if len(keywordsBytes) > 0 {
		r.MetricKeywords = json.RawMessage(keywordsBytes)
	}
	if len(keywordsEnBytes) > 0 {
		r.MetricKeywordsEn = json.RawMessage(keywordsEnBytes)
	}
	if len(reasoningBytes) > 0 {
		r.ReasoningTags = json.RawMessage(reasoningBytes)
	}
	if confidence.Valid {
		v := confidence.Float64
		r.Confidence = &v
	}
	if isExplicit.Valid {
		v := isExplicit.Bool
		r.IsExplicitMetric = &v
	}
	return r, nil
}

type inputDetailResponse struct {
	Status bool        `json:"status"`
	Record inputRecord `json:"record"`
}

func fetchInputRecordByID(db *sql.DB, inputTable string, id int64) (inputRecord, error) {
	nameColumnExpr, err := resolveNameColumnExpr(db, inputTable)
	if err != nil {
		return inputRecord{}, err
	}

	parserNameColumnExpr, err := resolveParserNameColumnExpr(db, inputTable)
	if err != nil {
		return inputRecord{}, err
	}

	query := fmt.Sprintf(`
SELECT
    i.id, %s AS name, %s AS parser_name, i.type, i.user_id, i.ks_store_id, i.title, i.doc_no, i.ks_desc, i.source,
    i.file_name, i.backup_filename, i.result_filename, i.publish_date,
    i.authors, i.owner, COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time, i.modify_time, i.public_info, i.private_info, i.doc_metadata::text,
    i.notes, i.error_msg
FROM %s i
WHERE i.id = $1
`, nameColumnExpr, parserNameColumnExpr, inputTable)

	row := db.QueryRow(query, id)
	var (
		record              inputRecord
		statusBytes         []byte
		publishDate         sql.NullTime
		publicInfoNullable  sql.NullString
		privateInfoNullable sql.NullString
		docMetadataNullable sql.NullString
	)
	if err := row.Scan(
		&record.ID, &record.Name, &record.ParserName, &record.Type, &record.UserID, &record.KSStoreID, &record.Title, &record.DocNo, &record.KSDesc, &record.Source,
		&record.FileName, &record.BackupFileName, &record.ResultFileName, &publishDate,
		&record.Authors, &record.Owner, &statusBytes,
		&record.CreateTime, &record.ModifyTime, &publicInfoNullable, &privateInfoNullable, &docMetadataNullable,
		&record.Notes, &record.ErrorMsg,
	); err != nil {
		return inputRecord{}, err
	}
	if publishDate.Valid {
		ts := publishDate.Time
		record.PublishDate = &ts
	}
	record.Status = json.RawMessage(statusBytes)
	if publicInfoNullable.Valid && strings.TrimSpace(publicInfoNullable.String) != "" {
		record.PublicInfo = json.RawMessage([]byte(publicInfoNullable.String))
	}
	if privateInfoNullable.Valid && strings.TrimSpace(privateInfoNullable.String) != "" {
		record.PrivateInfo = json.RawMessage([]byte(privateInfoNullable.String))
	}
	if docMetadataNullable.Valid {
		docMetadataText := strings.TrimSpace(docMetadataNullable.String)
		if docMetadataText != "" && docMetadataText != "null" {
			record.DocMetadata = json.RawMessage([]byte(docMetadataText))
		}
	}
	return record, nil
}

// GetInput handles GET /api/v1/kb/inputs/:id
func GetInput(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_100")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid id (CWB_KB_M_110)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_M_120)",
		})
	}
	record, err := fetchInputRecordByID(db, inputTable, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status:   false,
				ErrorMsg: "record not found (CWB_KB_M_130)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to retrieve kb input (CWB_KB_M_131)",
		})
	}

	return c.JSON(http.StatusOK, inputDetailResponse{Status: true, Record: record})
}

func decodeStringValue(raw json.RawMessage, trim bool) (*string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("missing value")
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("must be a string or null")
	}
	if trim {
		s = strings.TrimSpace(s)
	}
	return &s, nil
}

func compactJSONRaw(raw json.RawMessage) (string, error) {
	if !json.Valid(raw) {
		return "", fmt.Errorf("must be valid json")
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("must be valid json")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("failed to encode json")
	}
	return string(encoded), nil
}

func decodeOwnerValue(raw json.RawMessage) (*int64, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return &numeric, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("owner must be integer text")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("owner must be integer or string")
}

func decodeInt64Value(raw json.RawMessage) (*int64, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return &numeric, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("must be integer text")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("must be integer or string")
}

func decodeFloat64Value(raw json.RawMessage) (*float64, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var numeric float64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return &numeric, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("must be numeric text")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("must be number or string")
}

func decodeBoolValue(raw json.RawMessage) (*bool, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return &boolean, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "":
			return nil, nil
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
	return nil, fmt.Errorf("must be boolean or string")
}

func decodeAuthorsValue(raw json.RawMessage) (*string, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}

	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		clean := make([]string, 0, len(arr))
		for _, entry := range arr {
			v := strings.TrimSpace(entry)
			if v == "" {
				continue
			}
			clean = append(clean, v)
		}
		encoded, err := json.Marshal(clean)
		if err != nil {
			return nil, fmt.Errorf("failed to encode authors")
		}
		s := string(encoded)
		return &s, nil
	}

	var asText string
	if err := json.Unmarshal(raw, &asText); err == nil {
		asText = strings.TrimSpace(asText)
		if asText == "" {
			return nil, nil
		}
		return &asText, nil
	}

	return nil, fmt.Errorf("authors must be string[] or string")
}

// UpdateInput handles PUT /api/v1/kb/inputs/:id
func UpdateInput(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_140")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid id (CWB_KB_M_141)",
		})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid request body (CWB_KB_M_142)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_M_143)",
		})
	}

	sets := make([]string, 0, len(payload)+1)
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	sets = append(sets, "modify_time = NOW()")

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "title", "doc_no", "source", "user_id":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_144)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "ks_desc":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_145A)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "notes", "error_msg":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_145)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "publish_date":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid publish_date: %v (CWB_KB_M_146)", err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
				break
			}
			ts, err := parseTimeQuery(*value)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid publish_date format: %v (CWB_KB_M_147)", err),
				})
			}
			if ts == nil {
				addSet(field, nil)
			} else {
				addSet(field, *ts)
			}

		case "authors":
			value, err := decodeAuthorsValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid authors: %v (CWB_KB_M_148)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "owner":
			value, err := decodeOwnerValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid owner: %v (CWB_KB_M_149)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "ks_store_id":
			value, err := decodeInt64Value(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_M_149A)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "public_info", "private_info", "doc_metadata":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, nil)
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_150)", field, err),
				})
			}
			addSet(field, compact)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "no editable fields in request (CWB_KB_M_151)",
		})
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", inputTable, strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update kb input failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to update kb input (CWB_KB_M_152)",
		})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected kb input failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to verify kb input update (CWB_KB_M_153)",
		})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "record not found (CWB_KB_M_154)",
		})
	}

	record, err := fetchInputRecordByID(db, inputTable, id)
	if err != nil {
		logger.Error("reload kb input after update failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to reload kb input (CWB_KB_M_155)",
		})
	}

	return c.JSON(http.StatusOK, inputDetailResponse{Status: true, Record: record})
}

// UpdateMetric handles PUT /api/v1/kb/metrics/:id
func UpdateMetric(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_160")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid id (CWB_KB_M_161)",
		})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid request body (CWB_KB_M_162)",
		})
	}

	// A partial edit must validate against the retained counterpart, so changing
	// display text cannot silently leave a contradictory collection behind.
	if rawStructure, hasStructure := payload["value_structure"]; hasStructure || payload["metric_value"] != nil {
		var structure any
		display := ""
		rawDisplay, hasDisplay := payload["metric_value"]
		if hasStructure {
			if err := json.Unmarshal(rawStructure, &structure); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid value_structure JSON"})
			}
			if _, err := metricvalue.NormalizeStructure(structure); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid value_structure: " + err.Error()})
			}
		}
		if hasDisplay {
			decoded, err := decodeStringValue(rawDisplay, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid metric_value"})
			}
			if decoded != nil {
				display = *decoded
			}
		}
		if !hasStructure || !hasDisplay {
			var retainedDisplay sql.NullString
			var retainedStructure []byte
			err := ApiTypes.ProjectDBHandle.QueryRow("SELECT metric_value, value_structure FROM kb.metrics WHERE id = $1", id).Scan(&retainedDisplay, &retainedStructure)
			if err == sql.ErrNoRows {
				return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "metric not found"})
			}
			if err != nil {
				logger.Error("read metric value before update failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read metric value"})
			}
			if !hasDisplay {
				display = retainedDisplay.String
			}
			if !hasStructure && len(retainedStructure) > 0 {
				structure = json.RawMessage(retainedStructure)
			}
		}
		if err := metricvalue.ValidateForValue(structure, display); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid value_structure: " + err.Error()})
		}
	}

	db := ApiTypes.ProjectDBHandle
	sets := make([]string, 0, len(payload))
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "metric_name", "metric_name_en", "metric_subject", "metric_subject_en",
			"metric_desc", "metric_desc_en", "metric_context", "metric_context_en",
			"model_name", "location_type", "metric_unit", "metric_unit_en",
			"metric_value", "value_data_type", "value_range_type", "value_class", "value_class_en",
			"formula_or_definition", "threshold_or_target", "measurement_frequency", "table_name_or_section",
			"condition", "condition_en":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_163)", field, err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "value_structure":
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid value_structure JSON"})
			}
			normalized, err := metricvalue.SQLValue(value)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid value_structure: " + err.Error()})
			}
			addSet(field, normalized)

		case "source_line_spans", "metric_keywords", "metric_keywords_en", "reasoning_tags":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, nil)
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_M_164)", field, err),
				})
			}
			addSet(field, compact)

		case "confidence":
			value, err := decodeFloat64Value(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid confidence: %v (CWB_KB_M_165)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "is_explicit_metric":
			value, err := decodeBoolValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status:   false,
					ErrorMsg: fmt.Sprintf("invalid is_explicit_metric: %v (CWB_KB_M_166)", err),
				})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}
		}
	}

	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "no editable fields in request (CWB_KB_M_167)",
		})
	}

	query := fmt.Sprintf("UPDATE kb.metrics SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update kb metric failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to update kb metric (CWB_KB_M_168)",
		})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected kb metric failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to verify kb metric update (CWB_KB_M_169)",
		})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "record not found (CWB_KB_M_170)",
		})
	}

	record, err := fetchMetricByID(db, id)
	if err != nil {
		logger.Error("reload kb metric after update failed", "id", id, "err", err)
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status:   false,
				ErrorMsg: "record not found (CWB_KB_M_171)",
			})
		}
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to reload kb metric (CWB_KB_M_172)",
		})
	}

	return c.JSON(http.StatusOK, metricDetailResponse{Status: true, Record: record})
}

type rawLine struct {
	LineNumber int       `json:"line_number"`
	PageNumber int       `json:"page_number"`
	LineType   string    `json:"line_type"`
	Content    string    `json:"content"`
	Coords     []float64 `json:"coords"`
}

type rawLinesResponse struct {
	Status   bool      `json:"status"`
	InputID  int64     `json:"input_id"`
	FileName string    `json:"file_name,omitempty"`
	Lines    []rawLine `json:"lines"`
	Pages    int       `json:"pages"`
}

type inputDeleteAssets struct {
	FileName       string
	BackupFileName string
	ResultFileName string
}

// GetRawLines handles GET /api/v1/kb/raw-lines?input_record_id=N
func GetRawLines(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_M_210)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_M_220)",
		})
	}
	q := fmt.Sprintf(`SELECT i.result_filename, i.file_name FROM %s i WHERE i.id = $1`, inputTable)
	var resultFile sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(q, id).Scan(&resultFile, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_M_230)",
			})
		}
		logger.Error("query result_filename failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_M_231)",
		})
	}
	if !resultFile.Valid || strings.TrimSpace(resultFile.String) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "result_filename is empty (CWB_KB_M_232)",
		})
	}

	rawPath := rawLinePathFor(resultFile.String)
	f, err := os.Open(rawPath)
	if err != nil {
		logger.Error("open raw_line file failed", "path", rawPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("raw_line file not found: %s (CWB_KB_M_240)", filepath.Base(rawPath)),
		})
	}
	defer f.Close()

	lines := make([]rawLine, 0, 256)
	pages := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		ln, ok := parseRawLine(scanner.Text())
		if !ok {
			continue
		}
		if ln.PageNumber > pages {
			pages = ln.PageNumber
		}
		lines = append(lines, ln)
	}
	if err := scanner.Err(); err != nil {
		logger.Error("scan raw_line file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to read raw_line file (CWB_KB_M_241)",
		})
	}

	resp := rawLinesResponse{Status: true, InputID: id, Lines: lines, Pages: pages}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

// GetInputFile handles GET /api/v1/kb/inputs/:id/file
// Streams the original document referenced by kb.inputs.file_name. Falls back
// to backup_filename, then result_filename, if earlier columns are empty.
func GetInputFile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid id (CWB_KB_M_310)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_M_320)",
		})
	}

	q := fmt.Sprintf(
		`SELECT i.file_name, i.backup_filename, i.result_filename, i.type FROM %s i WHERE i.id = $1`,
		inputTable,
	)
	var fileName, backupName, resultName, docType sql.NullString
	if err := db.QueryRow(q, id).Scan(&fileName, &backupName, &resultName, &docType); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_M_330)",
			})
		}
		logger.Error("query kb input file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_M_331)",
		})
	}

	pickFirstNonEmpty := func(vals ...sql.NullString) string {
		if len(vals) > 0 && vals[0].Valid && strings.TrimSpace(vals[0].String) != "" {
			return pathutil.ResolveDataHomePath(vals[0].String)
		}
		if len(vals) > 1 && vals[1].Valid && strings.TrimSpace(vals[1].String) != "" {
			return pathutil.ResolveBackupPath(vals[1].String)
		}
		if len(vals) > 2 && vals[2].Valid && strings.TrimSpace(vals[2].String) != "" {
			return pathutil.ResolveDataHomePath(vals[2].String)
		}
		return ""
	}
	path := pickFirstNonEmpty(fileName, backupName, resultName)
	if path == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "no file path on record (CWB_KB_M_340)",
		})
	}

	if _, err := os.Stat(path); err != nil {
		logger.Error("stat input file failed", "path", path, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("file not found: %s (CWB_KB_M_341)", filepath.Base(path)),
		})
	}

	contentType := contentTypeFor(docType.String, path)
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(path)))
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.File(path)
}

func contentTypeFor(docType, path string) string {
	switch strings.ToLower(strings.TrimSpace(docType)) {
	case "pdf":
		return "application/pdf"
	case "json":
		return "application/json; charset=utf-8"
	case "xml":
		return "application/xml; charset=utf-8"
	case "markdown", "md":
		return "text/markdown; charset=utf-8"
	case "text", "txt", "typst":
		return "text/plain; charset=utf-8"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt", ".typ":
		return "text/plain; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	return "application/octet-stream"
}

func rawLinePathFor(resultFilename string) string {
	resolved := pathutil.ResolveDataHomePath(resultFilename)
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	ext := filepath.Ext(base)
	root := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, root+".txt")
}

// parseRawLine parses canonical line file records:
// "<line>\t<page>\t<type>\t<font>\t<font_size>\t[x1,y1,x2,y2]\t<content...>"
func parseRawLine(s string) (rawLine, bool) {
	s = strings.TrimRight(s, "\r\n")
	if strings.TrimSpace(s) == "" {
		return rawLine{}, false
	}
	fields := strings.Split(s, "\t")
	if len(fields) != 7 {
		return rawLine{}, false
	}
	lineNum, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
	pageNum, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err1 != nil || err2 != nil {
		return rawLine{}, false
	}
	lineType := strings.TrimSpace(fields[2])
	coordRaw := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if lineNum < 1 || pageNum < 1 || lineType == "" || coordRaw == "" {
		return rawLine{}, false
	}
	if !strings.HasPrefix(coordRaw, "[") || !strings.HasSuffix(coordRaw, "]") {
		return rawLine{}, false
	}
	coordRaw = strings.TrimSpace(coordRaw[1 : len(coordRaw)-1])

	coords := make([]float64, 0, 4)
	for _, tok := range strings.Split(coordRaw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			continue
		}
		coords = append(coords, v)
	}

	return rawLine{
		LineNumber: lineNum,
		PageNumber: pageNum,
		LineType:   lineType,
		Content:    content,
		Coords:     coords,
	}, true
}

func loadInputDeleteAssets(tx *sql.Tx, inputTable string, id int64) (inputDeleteAssets, bool, error) {
	query := fmt.Sprintf(`SELECT COALESCE(file_name, ''), COALESCE(backup_filename, ''), COALESCE(result_filename, '') FROM %s WHERE id = $1`, inputTable)
	var assets inputDeleteAssets
	if err := tx.QueryRow(query, id).Scan(&assets.FileName, &assets.BackupFileName, &assets.ResultFileName); err != nil {
		if err == sql.ErrNoRows {
			return inputDeleteAssets{}, false, nil
		}
		return inputDeleteAssets{}, false, err
	}
	return assets, true, nil
}

type inputRelatedDeleteSpec struct {
	Table  string
	Column string
}

var inputRelatedDeleteSpecs = []inputRelatedDeleteSpec{
	{Table: "kb.doc_review_provision_analyses", Column: "input_record_id"},
	{Table: "kb.doc_review_logs", Column: "input_record_id"},
	{Table: "kb.doc_review_findings", Column: "input_record_id"},
	{Table: "kb.doc_review_activities", Column: "input_record_id"},
	{Table: "kb.doc_review_status", Column: "input_record_id"},
	{Table: "kb.doc_review_reports", Column: "input_record_id"},
	{Table: "kb.doc_review_runs", Column: "input_record_id"},
	{Table: "kb.doc_review_requests", Column: "input_record_id"},
	{Table: "kb.doc_proc_logs", Column: "record_id"},
	{Table: "kb.doc_process_runs", Column: "record_id"},
	{Table: "kb.input_proc_status", Column: "record_id"},
	{Table: "kb.artifact_connections", Column: "source_record_id"},
	{Table: "kb.artifact_connections", Column: "target_record_id"},
	{Table: "kb.artifact_connections", Column: "input_record_id"},
	{Table: "kb.artifact_objects", Column: "source_record_id"},
	{Table: "kb.artifact_objects", Column: "input_record_id"},
	{Table: "kb.inventory_item_duplicates", Column: "input_record_id"},
	{Table: "kb.inventory_items", Column: "input_record_id"},
	{Table: "kb.relations", Column: "input_record_id"},
	{Table: "kb.entities", Column: "input_record_id"},
	{Table: "kb.knowledges", Column: "input_record_id"},
	{Table: "kb.semantic_projections", Column: "input_record_id"},
	{Table: "kb.scene_objects", Column: "input_record_id"},
	{Table: "kb.products", Column: "input_record_id"},
	{Table: "kb.provisions", Column: "input_record_id"},
	{Table: "kb.metrics", Column: "input_record_id"},
	{Table: "kb.topics", Column: "input_record_id"},
	{Table: "kb.summaries", Column: "input_record_id"},
	{Table: "kb.chunk_ranges", Column: "input_record_id"},
	{Table: "kb.search_artifacts", Column: "input_record_id"},
	{Table: "kb.chunks", Column: "source_record_id"},
}

const inputRelatedColumnExistsQuery = `
SELECT EXISTS (
	SELECT 1
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
	WHERE n.nspname = $1
	  AND c.relname = $2
	  AND a.attname = $3
	  AND a.attnum > 0
	  AND NOT a.attisdropped
)`

func inputRelatedColumnExists(tx *sql.Tx, relation string, column string) (bool, error) {
	schema, table, ok := strings.Cut(relation, ".")
	if !ok || strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" || strings.TrimSpace(column) == "" {
		return false, fmt.Errorf("invalid relation cleanup target: %s.%s", relation, column)
	}

	var exists bool
	if err := tx.QueryRow(inputRelatedColumnExistsQuery, schema, table, column).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func deleteDocProcLogsForInputRuns(tx *sql.Tx, id int64) error {
	required := []inputRelatedDeleteSpec{
		{Table: "kb.doc_proc_logs", Column: "run_id"},
		{Table: "kb.doc_process_runs", Column: "id"},
		{Table: "kb.doc_process_runs", Column: "record_id"},
	}
	for _, spec := range required {
		exists, err := inputRelatedColumnExists(tx, spec.Table, spec.Column)
		if err != nil {
			return fmt.Errorf("inspect doc process run log cleanup %s.%s: %w", spec.Table, spec.Column, err)
		}
		if !exists {
			return nil
		}
	}

	stmt := `DELETE FROM kb.doc_proc_logs WHERE run_id IN (SELECT id FROM kb.doc_process_runs WHERE record_id = $1)`
	if _, err := tx.Exec(stmt, id); err != nil {
		return fmt.Errorf("delete kb.doc_proc_logs for doc process runs: %w", err)
	}
	return nil
}

func deleteInputRelatedRows(tx *sql.Tx, id int64) error {
	if err := deleteDocProcLogsForInputRuns(tx, id); err != nil {
		return err
	}
	if err := deleteInputReferenceRows(tx, id); err != nil {
		return err
	}
	for _, spec := range inputRelatedDeleteSpecs {
		exists, err := inputRelatedColumnExists(tx, spec.Table, spec.Column)
		if err != nil {
			return fmt.Errorf("inspect %s.%s: %w", spec.Table, spec.Column, err)
		}
		if !exists {
			continue
		}

		stmt := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", spec.Table, spec.Column)
		if spec.Table == "kb.doc_proc_logs" {
			stmt += " AND entry_type <> 'delete_input'"
		}
		if _, err := tx.Exec(stmt, id); err != nil {
			return fmt.Errorf("delete %s where %s: %w", spec.Table, spec.Column, err)
		}
	}
	return nil
}

// deleteInputReferenceRows applies the reference layer's deletion contract (openspec change
// extract-references-processor): the record's own mentions go (their links and resolved rows
// follow by cascade), its attachment to a catalog resource goes, and other records' references
// into it lose their holding: resolved copies of its metrics are removed, links become stale and
// the mentions wait for a target again.
func deleteInputReferenceRows(tx *sql.Tx, id int64) error {
	exists, err := inputRelatedColumnExists(tx, "kb.ref_mentions", "input_record_id")
	if err != nil || !exists {
		return err
	}
	stmts := []string{
		`DELETE FROM kb.metrics WHERE ref_input_record_id = $1`,
		`UPDATE kb.reference_artifact_links SET status = 'stale', updated_at = NOW() WHERE target_input_id = $1 AND citing_input_id <> $1`,
		`UPDATE kb.ref_mentions SET target_input_id = NULL, r2_state = 'not_held',
		        metric_state = CASE WHEN metric_state IS NULL THEN NULL ELSE 'pending_target' END
		  WHERE target_input_id = $1 AND input_record_id <> $1`,
		`DELETE FROM kb.ref_mentions WHERE input_record_id = $1`,
		`DELETE FROM kb.ref_resource_identifiers WHERE kind = 'kb_input' AND value = $1::text`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt, id); err != nil {
			return fmt.Errorf("delete reference rows for input %d: %w", id, err)
		}
	}
	return nil
}

type artifactWebCleanupStats struct {
	FilesScanned int      `json:"files_scanned"`
	FilesChanged int      `json:"files_changed"`
	LinesRemoved int      `json:"lines_removed"`
	DeletedFiles []string `json:"deleted_files"`
}

const deleteInputDocProcLogLoc = "CWB_KB_M_416"

var artifactWebIndexFilenames = map[string]struct{}{
	"chunks.txt":               {},
	"entities.txt":             {},
	"inventory_items.txt":      {},
	"metrics.txt":              {},
	"products.txt":             {},
	"provisions.txt":           {},
	"relations.txt":            {},
	"scenes.txt":               {},
	"semantic_projections.txt": {},
	"summaries.txt":            {},
	"topics.txt":               {},
}

func removeArtifactWebTreeRecord(treeRootDir string, recordID int64) (artifactWebCleanupStats, error) {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	stats := artifactWebCleanupStats{}
	err := filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := artifactWebIndexFilenames[d.Name()]; !ok {
			return nil
		}
		stats.FilesScanned++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		removed := 0
		for row := range strings.SplitSeq(string(body), "\n") {
			row = strings.TrimSpace(row)
			if row == "" {
				continue
			}
			if artifactWebIndexRowBelongsToRecord(row, prefix) {
				removed++
				continue
			}
			rows = append(rows, row)
		}
		if removed == 0 {
			return nil
		}
		stats.FilesChanged++
		stats.LinesRemoved += removed
		if len(rows) == 0 {
			if err := os.Remove(path); err == nil {
				stats.DeletedFiles = append(stats.DeletedFiles, path)
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		sort.Strings(rows)
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
	return stats, err
}

func artifactWebIndexRowBelongsToRecord(row string, recordIDPrefix string) bool {
	firstField := strings.Fields(strings.TrimSpace(row))
	if len(firstField) == 0 {
		return false
	}
	return strings.HasPrefix(firstField[0], recordIDPrefix)
}

/*
func removeSemanticProjectionTreeRecord(treeRootDir string, recordID int64) error {
	stats, err := removeArtifactWebTreeRecord(treeRootDir, recordID)
	_ = stats
	return err
}
*/

func pruneMetadataOnlyArtifactWebDirs(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil
	}
	report := newInputDeleteReport()
	_, err := pruneMetadataOnlyDir(root, root, &report)
	return err
}

func pruneMetadataOnlyDir(path string, root string, report *inputDeleteReport) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	keep := false
	for _, entry := range entries {
		if !entry.IsDir() {
			if entry.Name() != "metadata.txt" {
				keep = true
			}
			continue
		}
		removed, err := pruneMetadataOnlyDir(filepath.Join(path, entry.Name()), root, report)
		if err != nil {
			return false, err
		}
		if !removed {
			keep = true
		}
	}
	if keep || path == root {
		return false, nil
	}
	metadata := filepath.Join(path, "metadata.txt")
	if err := os.Remove(metadata); err == nil {
		report.DeletedFiles = append(report.DeletedFiles, metadata)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func artifactWebCleanupRoots() []string {
	artifactWebDir := filepath.Clean(strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")))
	if artifactWebDir == "" || artifactWebDir == "." {
		return nil
	}
	if filepath.Base(artifactWebDir) == "ArtifactWeb" {
		return []string{artifactWebDir}
	}
	child := filepath.Join(artifactWebDir, "ArtifactWeb")
	if info, err := os.Stat(child); err == nil && info.IsDir() {
		return []string{child}
	}
	return []string{artifactWebDir}
}

func cleanupInputArtifactWeb(recordID int64) (artifactWebCleanupStats, []string) {
	var total artifactWebCleanupStats
	var failures []string
	for _, root := range artifactWebCleanupRoots() {
		stats, err := removeArtifactWebTreeRecord(root, recordID)
		total.FilesScanned += stats.FilesScanned
		total.FilesChanged += stats.FilesChanged
		total.LinesRemoved += stats.LinesRemoved
		total.DeletedFiles = append(total.DeletedFiles, stats.DeletedFiles...)
		if err != nil {
			failures = append(failures, fmt.Sprintf("artifact web cleanup %s: %v", root, err))
			continue
		}
		report := newInputDeleteReport()
		if _, err := pruneMetadataOnlyDir(root, root, &report); err != nil {
			failures = append(failures, fmt.Sprintf("prune artifact web dirs %s: %v", root, err))
		}
		total.DeletedFiles = append(total.DeletedFiles, report.DeletedFiles...)
	}
	return total, failures
}

func shouldDeleteInputFiles(c echo.Context) bool {
	return strings.EqualFold(strings.TrimSpace(c.QueryParam("delete_file")), "true")
}

// DeleteInput handles DELETE /api/v1/kb/inputs/:id
//
// With delete_file=true the uploaded PDF is archived to ARCHIVE_DIR, and the record's backup,
// result file and artifact folder are removed. Every planned location is checked before anything
// changes. If the database delete cannot finish, the PDF is moved back. Any file that could not
// be removed makes the response an error, and the audit log in kb.doc_proc_logs lists what was done.
func DeleteInput(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_400")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid id (CWB_KB_M_401)",
		})
	}

	deleteFiles := shouldDeleteInputFiles(c)
	archiveRoot := strings.TrimSpace(os.Getenv("ARCHIVE_DIR"))
	if deleteFiles && archiveRoot == "" {
		logger.Error("delete kb input refused: ARCHIVE_DIR is not set", "id", id)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "ARCHIVE_DIR is not set, so the PDF cannot be archived; nothing was deleted (CWB_KB_M_421)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve kb input table (CWB_KB_M_402)",
		})
	}

	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin delete kb input transaction failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to delete kb input (CWB_KB_M_403)",
		})
	}
	defer tx.Rollback()

	assets, found, err := loadInputDeleteAssets(tx, inputTable, id)
	if err != nil {
		logger.Error("load kb input delete assets failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to verify kb input delete (CWB_KB_M_404)",
		})
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "record not found (CWB_KB_M_405)",
		})
	}

	report := newInputDeleteReport()
	var plan inputDeletePlan
	if deleteFiles {
		plan, err = planInputDeletePaths(id, assets)
		if err != nil {
			logger.Error("plan kb input file delete failed", "id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: fmt.Sprintf("refused to delete kb input: %v; nothing was deleted (CWB_KB_M_420)", err),
			})
		}
		archived, missing, err := archiveInputPDF(archiveRoot, id, plan.PDFPath)
		if err != nil {
			logger.Error("archive kb input pdf failed", "id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: fmt.Sprintf("failed to archive the PDF: %v; nothing was deleted (CWB_KB_M_422)", err),
			})
		}
		report.ArchivedFile = archived
		if missing {
			report.MissingFiles = append(report.MissingFiles, plan.PDFPath)
		}
	}

	// abort ends the request with an error and moves an archived PDF back, so the record is
	// left as it was when the database delete does not finish.
	abort := func(status int, errorMsg string, cause error) error {
		logger.Error("delete kb input failed", "id", id, "err", cause)
		if report.ArchivedFile != "" {
			if restoreErr := restoreArchivedPDF(report.ArchivedFile, plan.PDFPath); restoreErr != nil {
				logger.Error("restore archived kb input pdf failed", "id", id, "archived", report.ArchivedFile, "path", plan.PDFPath, "err", restoreErr)
				errorMsg += fmt.Sprintf("; the PDF could not be restored from %s: %v", report.ArchivedFile, restoreErr)
			} else {
				errorMsg += "; the PDF was restored"
			}
		}
		return c.JSON(status, errorResponse{Status: false, ErrorMsg: errorMsg})
	}

	if err := deleteInputRelatedRows(tx, id); err != nil {
		return abort(http.StatusInternalServerError, "failed to delete kb input (CWB_KB_M_403)", err)
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", inputTable)
	result, err := tx.Exec(query, id)
	if err != nil {
		return abort(http.StatusInternalServerError, "failed to delete kb input (CWB_KB_M_403)", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return abort(http.StatusInternalServerError, "failed to verify kb input delete (CWB_KB_M_404)", err)
	}
	if affected == 0 {
		return abort(http.StatusNotFound, "record not found (CWB_KB_M_405)", nil)
	}
	deleted, err := readInputDeletedTables(tx)
	if err != nil {
		return abort(http.StatusInternalServerError, "failed to audit deleted tables (CWB_KB_M_426)", err)
	}
	auditID, err := startInputDeleteAudit(tx, id, deleteFiles, plan, report, deleted)
	if err != nil {
		return abort(http.StatusInternalServerError, "failed to create delete audit; nothing was deleted (CWB_KB_M_427)", err)
	}
	if err := tx.Commit(); err != nil {
		return abort(http.StatusInternalServerError, "failed to delete kb input (CWB_KB_M_403)", err)
	}

	if deleteFiles {
		deleteInputFiles(plan.Files, &report)
		deleteInputArtifactDirs(plan.Dirs, &report)
	}
	webStats, webFailures := cleanupInputArtifactWeb(id)
	report.Failures = append(report.Failures, webFailures...)
	report.DeletedFiles = append(report.DeletedFiles, webStats.DeletedFiles...)
	if err := finishInputDeleteAudit(db, auditID, deleteFiles, report, deleted, webStats); err != nil {
		report.Failures = append(report.Failures, fmt.Sprintf("finalize audit log %d (cleanup remains pending): %v", auditID, err))
		logger.Error("finalize input delete audit failed", "audit_id", auditID, "report", report, "deleted_tables", deleted, "err", err)
	}
	if len(report.Failures) > 0 {
		logger.Error("delete kb input left files behind", "id", id, "failures", report.Failures)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false,
			ErrorMsg: fmt.Sprintf("kb input %d was removed from the database, but not everything was deleted: %s (CWB_KB_M_423)",
				id, strings.Join(report.Failures, "; ")),
		})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

type cleanInputArtifactsRequest struct {
	RecordID int64 `json:"record_id"`
}

// CleanInputArtifacts handles POST /api/v1/admin/db/kb-input-artifacts/clean.
// It performs the same cleanup as DeleteInput, but tolerates an already-missing
// kb.inputs row so orphaned ArtifactWeb index entries can be cleaned repeatedly.
// It does not archive the PDF; a failed removal is returned as an error.
func CleanInputArtifacts(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_410")
	defer rc.Close()
	logger := rc.GetLogger()

	var req cleanInputArtifactsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid request body (CWB_KB_M_411)",
		})
	}
	id := req.RecordID
	if id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid record_id (CWB_KB_M_412)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve kb input table (CWB_KB_M_413)",
		})
	}

	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin clean kb input artifacts transaction failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to clean kb input artifacts (CWB_KB_M_414)",
		})
	}
	defer tx.Rollback()

	assets, found, err := loadInputDeleteAssets(tx, inputTable, id)
	if err != nil {
		logger.Error("load kb input clean assets failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to verify kb input artifacts clean (CWB_KB_M_415)",
		})
	}

	plan, err := planInputDeletePaths(id, assets)
	if err != nil {
		logger.Error("plan clean kb input artifacts failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: fmt.Sprintf("refused to clean kb input artifacts: %v; nothing was deleted (CWB_KB_M_424)", err),
		})
	}

	if err := deleteInputRelatedRows(tx, id); err != nil {
		logger.Error("clean kb input related rows failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to clean kb input artifacts (CWB_KB_M_414)",
		})
	}

	if found {
		query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", inputTable)
		result, err := tx.Exec(query, id)
		if err != nil {
			logger.Error("clean delete kb input failed", "id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to clean kb input artifacts (CWB_KB_M_414)",
			})
		}
		affected, err := result.RowsAffected()
		if err != nil {
			logger.Error("rows affected clean delete kb input failed", "id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to verify kb input artifacts clean (CWB_KB_M_415)",
			})
		}
		found = affected > 0
	}

	if err := tx.Commit(); err != nil {
		logger.Error("commit clean kb input artifacts failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to clean kb input artifacts (CWB_KB_M_414)",
		})
	}

	report := newInputDeleteReport()
	if found {
		deleteInputFiles(plan.filesWithPDF(), &report)
	}
	deleteInputArtifactDirs(plan.Dirs, &report)
	webStats, webFailures := cleanupInputArtifactWeb(id)
	report.Failures = append(report.Failures, webFailures...)
	if len(report.Failures) > 0 {
		logger.Error("clean kb input artifacts left files behind", "id", id, "failures", report.Failures)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false,
			ErrorMsg: fmt.Sprintf("kb input artifacts for record %d were not fully removed: %s (CWB_KB_M_425)",
				id, strings.Join(report.Failures, "; ")),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":               true,
		"record_id":            id,
		"record_found":         found,
		"artifact_web_cleanup": webStats,
	})
}
