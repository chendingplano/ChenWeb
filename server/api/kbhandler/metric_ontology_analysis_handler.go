package kbhandler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricOntologyAnalysisResponse struct {
	Status         bool                             `json:"status"`
	GeneratedAt    string                           `json:"generated_at"`
	Coverage       metricOntologyCoverage           `json:"coverage"`
	KPI            metricOntologyKPI                `json:"kpi"`
	ErrorFacts     []metricOntologyCount            `json:"error_presence"`
	CoverageStates []metricOntologyCount            `json:"coverage_states"`
	Mappings       []metricOntologyMapping          `json:"mappings"`
	Errors         []metricOntologyError            `json:"errors_by_type"`
	Recent         []metricOntologyRecentOccurrence `json:"recent_occurrences"`
}

type metricOntologyCoverage struct {
	Writer      string `json:"writer"`
	Scope       string `json:"scope"`
	ReadModel   string `json:"read_model"`
	Rules       string `json:"rules"`
	Conformance string `json:"conformance"`
	Projection  string `json:"projection"`
	Resolution  string `json:"resolution"`
	Denominator int64  `json:"denominator"`
}

type metricOntologyKPI struct {
	Occurrences      int64 `json:"occurrences"`
	CurrentInstances int64 `json:"current_instances"`
	OntologyMetrics  int64 `json:"ontology_metrics"`
	MetricClasses    int64 `json:"metric_classes"`
	WithErrors       int64 `json:"with_errors"`
	WithoutErrors    int64 `json:"without_errors"`
}

type metricOntologyCount struct {
	Label   string `json:"label"`
	Value   int64  `json:"value"`
	Percent int64  `json:"percent"`
}

type metricOntologyMapping struct {
	Term        string `json:"term"`
	Module      string `json:"module"`
	Occurrences int64  `json:"occurrences"`
	Instances   int64  `json:"instances"`
	State       string `json:"state"`
}

type metricOntologyError struct {
	Label    string `json:"label"`
	Value    int64  `json:"value"`
	Percent  int64  `json:"percent"`
	Severity string `json:"severity"`
}

type metricOntologyRecentOccurrence struct {
	ID         string `json:"id"`
	Document   string `json:"document"`
	Metric     string `json:"metric"`
	Value      string `json:"value"`
	Definition string `json:"definition"`
	Status     string `json:"status"`
	Updated    string `json:"updated"`
}

// GetMetricOntologyAnalysis handles the composed, read-only dashboard query.
// The browser consumes this shape directly; it must not join the source
// endpoints itself because occurrence, instance, term, and finding grains are
// intentionally different.
func GetMetricOntologyAnalysis(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MOA_001")
	defer rc.Close()
	logger := rc.GetLogger()

	storeID, err := optionalPositiveInt64(c.QueryParam("ks_store_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid ks_store_id (CWB_KB_MOA_010)"})
	}

	db := ApiTypes.ProjectDBHandle
	where := ""
	args := make([]any, 0, 1)
	if storeID != nil {
		where = " WHERE i.ks_store_id = $1 "
		args = append(args, *storeID)
	}

	var out metricOntologyAnalysisResponse
	out.Status = true
	out.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	// Keep collection fields as JSON arrays even when the selected corpus has
	// no rows. The dashboard can then render an explicit empty state instead of
	// failing while mapping a null response field.
	out.ErrorFacts = make([]metricOntologyCount, 0)
	out.CoverageStates = make([]metricOntologyCount, 0)
	out.Mappings = make([]metricOntologyMapping, 0)
	out.Errors = make([]metricOntologyError, 0)
	out.Recent = make([]metricOntologyRecentOccurrence, 0)
	out.Coverage = metricOntologyCoverage{
		Writer: "semantic writer / current", Scope: "authorized corpus", ReadModel: "metric ontology analysis v1",
		Rules: "diagnostic rules v1.4", Conformance: "passed", Projection: "current", Resolution: "mixed",
	}

	metricQuery := `
SELECT
  COUNT(*) AS occurrences,
  COUNT(DISTINCT COALESCE(NULLIF(o.assertion_id::text, ''), m.input_record_id::text || ':' || COALESCE(m.metric_id, m.id::text))) AS instances,
  COUNT(*) FILTER (WHERE COALESCE(o.finding_count, 0) > 0 OR o.execution_status = 'failed' OR m.value_range_type_error IS NOT NULL) AS with_errors,
  COUNT(*) FILTER (WHERE COALESCE(o.finding_count, 0) = 0 AND COALESCE(o.execution_status, 'completed') <> 'failed' AND m.value_range_type_error IS NULL) AS without_errors
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
  SELECT so.assertion_id, so.finding_count, so.execution_status
  FROM kb.semantic_processing_outcomes so
  WHERE so.input_record_id = m.input_record_id
    AND so.artifact_type = 'metric'
    AND so.artifact_id = COALESCE(m.metric_id, m.id::text)
    AND so.active = true
  ORDER BY so.id DESC
  LIMIT 1
) o ON true` + where
	if err := db.QueryRow(metricQuery, args...).Scan(&out.KPI.Occurrences, &out.KPI.CurrentInstances, &out.KPI.WithErrors, &out.KPI.WithoutErrors); err != nil {
		logger.Error("metric ontology dashboard aggregate failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to aggregate metric ontology analysis (CWB_KB_MOA_020)"})
	}
	out.Coverage.Denominator = out.KPI.Occurrences

	termQuery := `
SELECT COUNT(*) FROM (
  SELECT DISTINCT ON (term_id) term_id
  FROM kb.ontology_terms
  WHERE term_kind = 'metric_definition'
  ORDER BY term_id, version DESC
) current_terms`
	if err := db.QueryRow(termQuery).Scan(&out.KPI.OntologyMetrics); err != nil {
		logger.Error("metric ontology term aggregate failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to aggregate ontology metrics (CWB_KB_MOA_021)"})
	}

	classQuery := `SELECT COUNT(DISTINCT COALESCE(m.value_class, '')) FILTER (WHERE NULLIF(BTRIM(m.value_class), '') IS NOT NULL) FROM kb.metrics m LEFT JOIN kb.inputs i ON i.id = m.input_record_id` + where
	if err := db.QueryRow(classQuery, args...).Scan(&out.KPI.MetricClasses); err != nil {
		logger.Error("metric class aggregate failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to aggregate metric classes (CWB_KB_MOA_022)"})
	}

	denom := out.KPI.Occurrences
	out.ErrorFacts = []metricOntologyCount{
		{Label: "Without detected errors", Value: out.KPI.WithoutErrors, Percent: percentOf(out.KPI.WithoutErrors, denom)},
		{Label: "With error facts", Value: out.KPI.WithErrors, Percent: percentOf(out.KPI.WithErrors, denom)},
	}

	coverageQuery := `
SELECT CASE
  WHEN o.execution_status = 'failed' THEN 'Execution failed'
  WHEN o.finding_count > 0 AND EXISTS (SELECT 1 FROM kb.semantic_processing_findings f WHERE f.outcome_id = o.id AND f.active = true AND f.finding_term_id LIKE '%blocked%') THEN 'Blocked claim'
  WHEN o.finding_count > 0 THEN 'Completed with findings'
  WHEN o.id IS NULL THEN 'Unknown coverage'
  ELSE 'Complete'
END AS label, COUNT(*)
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
  SELECT so.* FROM kb.semantic_processing_outcomes so
  WHERE so.input_record_id = m.input_record_id AND so.artifact_type = 'metric' AND so.artifact_id = COALESCE(m.metric_id, m.id::text) AND so.active = true
  ORDER BY so.id DESC LIMIT 1
) o ON true` + where + ` GROUP BY 1 ORDER BY 1`
	rows, err := db.Query(coverageQuery, args...)
	if err != nil {
		logger.Error("metric ontology coverage aggregate failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to aggregate coverage (CWB_KB_MOA_023)"})
	}
	for rows.Next() {
		var row metricOntologyCount
		if err := rows.Scan(&row.Label, &row.Value); err != nil {
			rows.Close()
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan coverage (CWB_KB_MOA_024)"})
		}
		row.Percent = percentOf(row.Value, denom)
		out.CoverageStates = append(out.CoverageStates, row)
	}
	rows.Close()

	mapQuery := `
SELECT COALESCE(NULLIF(l.label, ''), t.term_id), t.module_id,
       COUNT(m.id), COUNT(DISTINCT COALESCE(m.metric_id, m.id::text)),
       CASE WHEN COUNT(m.id) FILTER (WHERE m.value_range_type_error IS NOT NULL) > 0 THEN 'Warning' ELSE 'Current' END
FROM (SELECT DISTINCT ON (term_id) * FROM kb.ontology_terms WHERE term_kind = 'metric_definition' ORDER BY term_id, version DESC) t
LEFT JOIN LATERAL (SELECT label FROM kb.ontology_term_labels WHERE term_id = t.term_id AND version = t.version AND label_role = 'prefLabel' ORDER BY CASE WHEN lang = 'en' THEN 0 ELSE 1 END LIMIT 1) l ON true
LEFT JOIN kb.metrics m ON m.metric_definition_term_id = t.term_id
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
GROUP BY t.term_id, t.module_id, l.label
ORDER BY COUNT(m.id) DESC, 1
LIMIT 5`
	rows, err = db.Query(mapQuery)
	if err != nil {
		logger.Error("metric ontology mappings query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to query metric mappings (CWB_KB_MOA_025)"})
	}
	for rows.Next() {
		var row metricOntologyMapping
		if err := rows.Scan(&row.Term, &row.Module, &row.Occurrences, &row.Instances, &row.State); err != nil {
			rows.Close()
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan metric mappings (CWB_KB_MOA_026)"})
		}
		out.Mappings = append(out.Mappings, row)
	}
	rows.Close()

	errorQuery := `
SELECT COALESCE(NULLIF(f.error_code, ''), f.finding_term_id), COUNT(*),
       CASE WHEN BOOL_OR(f.severity_term_id LIKE '%error%' OR f.severity_term_id LIKE '%fail%') THEN 'Error' ELSE 'Warning' END
FROM kb.semantic_processing_findings f
JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id AND o.active = true
WHERE f.active = true
GROUP BY 1 ORDER BY COUNT(*) DESC LIMIT 6`
	rows, err = db.Query(errorQuery)
	if err != nil {
		logger.Error("metric ontology errors query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to query metric errors (CWB_KB_MOA_027)"})
	}
	for rows.Next() {
		var row metricOntologyError
		if err := rows.Scan(&row.Label, &row.Value, &row.Severity); err != nil {
			rows.Close()
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan metric errors (CWB_KB_MOA_028)"})
		}
		row.Percent = percentOf(row.Value, out.KPI.WithErrors)
		out.Errors = append(out.Errors, row)
	}
	rows.Close()

	recentQuery := `
SELECT COALESCE(m.metric_id, m.id::text), COALESCE(NULLIF(COALESCE(i.doc_metadata->>'title', i.title, i.staging_filename), ''), 'Untitled document'),
       COALESCE(m.metric_name, ''), CONCAT_WS(' ', m.metric_value, m.metric_unit), COALESCE(m.metric_definition_term_id, 'Not governed'),
       CASE WHEN o.execution_status = 'failed' THEN 'failed' WHEN o.finding_count > 0 THEN 'findings' WHEN o.id IS NULL THEN 'historical' ELSE 'complete' END,
       COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (SELECT so.* FROM kb.semantic_processing_outcomes so WHERE so.input_record_id = m.input_record_id AND so.artifact_type = 'metric' AND so.artifact_id = COALESCE(m.metric_id, m.id::text) AND so.active = true ORDER BY so.id DESC LIMIT 1) o ON true` + where + ` ORDER BY m.created_at DESC, m.id DESC LIMIT 12`
	rows, err = db.Query(recentQuery, args...)
	if err != nil {
		logger.Error("metric ontology recent query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to query recent metrics (CWB_KB_MOA_029)"})
	}
	for rows.Next() {
		var row metricOntologyRecentOccurrence
		if err := rows.Scan(&row.ID, &row.Document, &row.Metric, &row.Value, &row.Definition, &row.Status, &row.Updated); err != nil {
			rows.Close()
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan recent metrics (CWB_KB_MOA_030)"})
		}
		out.Recent = append(out.Recent, row)
	}
	rows.Close()

	return c.JSON(http.StatusOK, out)
}

func optionalPositiveInt64(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return nil, err
	}
	return &n, nil
}

func percentOf(value, denominator int64) int64 {
	if denominator <= 0 {
		return 0
	}
	return (value*100 + denominator/2) / denominator
}
