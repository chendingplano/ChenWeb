package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// listMetricRangeTypeErrorsResponse is the wire shape for the Resolve Metric
// Range Types admin page's Left-panel search results.
type listMetricRangeTypeErrorsResponse struct {
	Status  bool           `json:"status"`
	Results []metricRecord `json:"results"`
	Total   int            `json:"total"`
}

// ListMetricRangeTypeErrors handles GET /api/v1/kb/metrics/range-type-errors.
// It lists kb.metrics rows with a non-empty value_range_type_error (ADR
// 2026081401 DR6), optionally filtered by input_record_id, a created_at
// date_from/date_to range, and the offending value_range_type ("error type").
// Capped at 500 rows -- see openspec/changes/resolve-metric-range-types
// design.md Risks for why that's acceptable at current scale.
func ListMetricRangeTypeErrors(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RTE_001")
	defer rc.Close()
	logger := rc.GetLogger()

	conds := []string{"m.value_range_type_error IS NOT NULL"}
	args := make([]any, 0, 4)

	if raw := strings.TrimSpace(c.QueryParam("input_record_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return c.JSON(http.StatusBadRequest, errorResponse{
				Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_RTE_010)",
			})
		}
		args = append(args, id)
		conds = append(conds, fmt.Sprintf("m.input_record_id = $%d", len(args)))
	}
	if raw := strings.TrimSpace(c.QueryParam("date_from")); raw != "" {
		args = append(args, raw)
		conds = append(conds, fmt.Sprintf("m.created_at >= $%d::timestamptz", len(args)))
	}
	if raw := strings.TrimSpace(c.QueryParam("date_to")); raw != "" {
		args = append(args, raw)
		conds = append(conds, fmt.Sprintf("m.created_at <= $%d::timestamptz", len(args)))
	}
	if raw := strings.TrimSpace(c.QueryParam("value_range_type")); raw != "" {
		args = append(args, assertions.NormalizeValueRangeTypeRaw(raw))
		conds = append(conds, fmt.Sprintf(
			"lower(regexp_replace(trim(m.value_range_type), '[- ]', '_', 'g')) = $%d", len(args)))
	}

	query := fmt.Sprintf(`
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at,
    m.value_range_type_error
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE %s
ORDER BY m.created_at DESC
LIMIT 500
`, strings.Join(conds, " AND "))

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("query kb.metrics range-type errors failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve metric range-type errors (CWB_KB_RTE_020)",
		})
	}
	defer rows.Close()

	out := make([]metricRecord, 0)
	for rows.Next() {
		var (
			r               metricRecord
			spansBytes      []byte
			keywordsBytes   []byte
			keywordsEnBytes []byte
			reasoningBytes  []byte
			confidence      sql.NullFloat64
			isExplicit      sql.NullBool
		)
		if err := rows.Scan(
			&r.ID, &r.InputRecordID, &r.MetricID, &r.EventID, &r.InputFilename,
			&r.MetricName, &r.MetricNameEn, &spansBytes, &r.MetricSubject, &r.MetricSubjectEn,
			&r.MetricDesc, &r.MetricDescEn, &r.MetricContext, &r.MetricContextEn,
			&keywordsBytes, &keywordsEnBytes, &r.ModelName, &r.LocationType, &r.MetricUnit, &r.MetricUnitEn,
			&r.MetricValue, &r.ValueDataType, &r.ValueRangeType, &r.ValueClass, &r.ValueClassEn,
			&r.FormulaOrDefinition, &r.ThresholdOrTarget, &r.MeasurementFreq,
			&confidence, &isExplicit, &r.DocumentTitle, &r.DocumentDocNo,
			&r.TableNameOrSection, &reasoningBytes, &r.CreatedAt,
			&r.ValueRangeTypeError,
		); err != nil {
			logger.Error("scan kb.metrics range-type error row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to scan metric range-type errors (CWB_KB_RTE_021)",
			})
		}
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
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.metrics range-type errors failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to iterate metric range-type errors (CWB_KB_RTE_022)",
		})
	}

	return c.JSON(http.StatusOK, listMetricRangeTypeErrorsResponse{
		Status: true, Results: out, Total: len(out),
	})
}

// valueRangeTypeMapEntryDTO is the wire shape for one
// kb.metric_value_range_type_map row in the Map Block.
type valueRangeTypeMapEntryDTO struct {
	RawValue          string  `json:"raw_value"`
	CanonicalBucket   *string `json:"canonical_bucket,omitempty"`
	Status            string  `json:"status"`
	OccurrenceCount   int64   `json:"occurrence_count"`
	FirstSeenRecordID *int64  `json:"first_seen_record_id,omitempty"`
	LastSeenRecordID  *int64  `json:"last_seen_record_id,omitempty"`
	Note              *string `json:"note,omitempty"`
	CreateTime        string  `json:"create_time,omitempty"`
	CreateBy          *string `json:"create_by,omitempty"`
	ModifyTime        string  `json:"modify_time,omitempty"`
	ModifyBy          *string `json:"modify_by,omitempty"`
}

const valueRangeTypeMapEntryColumns = `
    raw_value, canonical_bucket, status, occurrence_count, first_seen_record_id, last_seen_record_id, note,
    COALESCE(to_char(create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS create_time, create_by,
    COALESCE(to_char(modify_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS modify_time, modify_by`

func scanValueRangeTypeMapEntry(scanner interface {
	Scan(dest ...any) error
}) (valueRangeTypeMapEntryDTO, error) {
	var e valueRangeTypeMapEntryDTO
	err := scanner.Scan(
		&e.RawValue, &e.CanonicalBucket, &e.Status, &e.OccurrenceCount, &e.FirstSeenRecordID, &e.LastSeenRecordID,
		&e.Note, &e.CreateTime, &e.CreateBy, &e.ModifyTime, &e.ModifyBy,
	)
	return e, err
}

func fetchValueRangeTypeMapEntry(db *sql.DB, rawValue string) (valueRangeTypeMapEntryDTO, error) {
	query := fmt.Sprintf("SELECT%s FROM kb.metric_value_range_type_map WHERE raw_value = $1", valueRangeTypeMapEntryColumns)
	return scanValueRangeTypeMapEntry(db.QueryRow(query, rawValue))
}

// listValueRangeTypeMapEntriesResponse is the wire shape for the Map Block.
type listValueRangeTypeMapEntriesResponse struct {
	Status  bool                        `json:"status"`
	Results []valueRangeTypeMapEntryDTO `json:"results"`
	Total   int                         `json:"total"`
}

// ListValueRangeTypeMapEntries handles GET /api/v1/kb/metric-value-range-type-map.
// Returns every kb.metric_value_range_type_map row, invalid (status !=
// 'approved') entries first, matching the Map Block requirement that invalid
// entries surface for triage before already-approved ones.
func ListValueRangeTypeMapEntries(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RTE_100")
	defer rc.Close()
	logger := rc.GetLogger()

	query := fmt.Sprintf(`
SELECT%s
FROM kb.metric_value_range_type_map
ORDER BY (status <> 'approved') DESC, occurrence_count DESC, raw_value ASC`, valueRangeTypeMapEntryColumns)

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(query)
	if err != nil {
		logger.Error("query kb.metric_value_range_type_map failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve value range type map (CWB_KB_RTE_110)",
		})
	}
	defer rows.Close()

	out := make([]valueRangeTypeMapEntryDTO, 0)
	for rows.Next() {
		e, err := scanValueRangeTypeMapEntry(rows)
		if err != nil {
			logger.Error("scan kb.metric_value_range_type_map row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to scan value range type map (CWB_KB_RTE_111)",
			})
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.metric_value_range_type_map failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to iterate value range type map (CWB_KB_RTE_112)",
		})
	}

	return c.JSON(http.StatusOK, listValueRangeTypeMapEntriesResponse{
		Status: true, Results: out, Total: len(out),
	})
}

type upsertValueRangeTypeMapEntryRequest struct {
	RawValue        string  `json:"raw_value"`
	CanonicalBucket string  `json:"canonical_bucket"`
	Note            *string `json:"note"`
}

type upsertValueRangeTypeMapEntryResponse struct {
	Status         bool                      `json:"status"`
	Entry          valueRangeTypeMapEntryDTO `json:"entry"`
	CorrectedCount int64                     `json:"corrected_count"`
}

// UpsertValueRangeTypeMapEntry handles POST /api/v1/kb/metric-value-range-type-map.
// Creates a brand-new kb.metric_value_range_type_map entry, or corrects an
// existing one -- the same write either way (design D1). Always sets
// status='approved' (canonical_bucket is required), invalidates the
// in-process governed-mapping cache so the correction is visible immediately
// (design D3), and cascades: clears value_range_type_error on every
// kb.metrics row whose normalized value_range_type matches (design D2).
func UpsertValueRangeTypeMapEntry(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RTE_200")
	defer rc.Close()
	logger := rc.GetLogger()

	var req upsertValueRangeTypeMapEntryRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid request body (CWB_KB_RTE_210)",
		})
	}

	rawValue := assertions.NormalizeValueRangeTypeRaw(req.RawValue)
	if rawValue == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "raw_value is required (CWB_KB_RTE_211)",
		})
	}
	canonicalBucket := strings.TrimSpace(req.CanonicalBucket)
	if canonicalBucket == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "canonical_bucket is required (CWB_KB_RTE_212)",
		})
	}
	var note any
	if req.Note != nil && strings.TrimSpace(*req.Note) != "" {
		note = strings.TrimSpace(*req.Note)
	}

	db := ApiTypes.ProjectDBHandle

	const upsertQuery = `
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status, note, modify_time)
VALUES ($1, $2, 'approved', $3, now())
ON CONFLICT (raw_value) DO UPDATE
   SET canonical_bucket = EXCLUDED.canonical_bucket,
       status = 'approved',
       note = COALESCE(EXCLUDED.note, kb.metric_value_range_type_map.note),
       modify_time = now()`
	if _, err := db.Exec(upsertQuery, rawValue, canonicalBucket, note); err != nil {
		logger.Error("upsert kb.metric_value_range_type_map failed", "raw_value", rawValue, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to save mapping (CWB_KB_RTE_220)",
		})
	}

	// Cascade-clear: a hand-ported copy of assertions.normalizeValueRangeTypeRaw
	// (lowercase, trim, hyphen/space -> underscore) -- see design D2. Keep both
	// in sync if that function's normalization rules ever change.
	const cascadeQuery = `
UPDATE kb.metrics
   SET value_range_type_error = NULL
 WHERE value_range_type_error IS NOT NULL
   AND lower(regexp_replace(trim(value_range_type), '[- ]', '_', 'g')) = $1`
	result, err := db.Exec(cascadeQuery, rawValue)
	if err != nil {
		logger.Error("cascade-clear kb.metrics.value_range_type_error failed", "raw_value", rawValue, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "mapping saved but cascade correction failed (CWB_KB_RTE_221)",
		})
	}
	corrected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected on cascade-clear failed", "err", err)
		corrected = 0
	}

	assertions.InvalidateValueRangeTypeMapCache()

	// Task 6.8 (ADR 2026081801 Phase 3): approving a mapping is a dependency
	// change for every metric semantic outcome whose active finding recorded
	// THIS raw value as unresolved or ambiguous (DR10). Scoped by exact
	// current fingerprint (ScheduleForKeyedDependencyChange), not by "not yet
	// at the new target" (ScheduleForDependencyChange) -- the latter would
	// also match every other raw value's still-unresolved findings, since
	// none of them equal this target either, sweeping the whole corpus into
	// the retry queue on every single approval. Best-effort: the mapping
	// decision itself already committed above, and a scheduling failure here
	// is not a reason to fail the request -- the dependency-driven retry
	// worker's own sweep still reconciles the mismatch eventually.
	resolvedFingerprint := assertions.MetricMappingFindingFingerprint(rawValue, semantic.MappingResolved)
	retryQueue := semantic.RetryQueue{DB: db}
	priorMappingStatesByFindingTerm := []struct{ findingTerm, priorState string }{
		{semantic.FindingMappingUnresolved, semantic.MappingUnresolved},
		{semantic.FindingMappingAmbiguous, semantic.MappingAmbiguous},
	}
	for _, entry := range priorMappingStatesByFindingTerm {
		findingTerm, priorState := entry.findingTerm, entry.priorState
		currentFingerprint := assertions.MetricMappingFindingFingerprint(rawValue, priorState)
		if _, err := retryQueue.ScheduleForKeyedDependencyChange(context.Background(), findingTerm, currentFingerprint, resolvedFingerprint); err != nil {
			logger.Error("schedule targeted semantic retry after mapping approval failed",
				"raw_value", rawValue, "finding_term", findingTerm, "err", err)
		}
	}

	entry, err := fetchValueRangeTypeMapEntry(db, rawValue)
	if err != nil {
		logger.Error("reload kb.metric_value_range_type_map entry failed", "raw_value", rawValue, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "mapping saved but failed to reload (CWB_KB_RTE_222)",
		})
	}

	return c.JSON(http.StatusOK, upsertValueRangeTypeMapEntryResponse{
		Status: true, Entry: entry, CorrectedCount: corrected,
	})
}

type applyValueRangeTypeMapEntryRequest struct {
	RawValue string `json:"raw_value"`
}

type applyValueRangeTypeMapEntryResponse struct {
	Status       bool                      `json:"status"`
	Entry        valueRangeTypeMapEntryDTO `json:"entry"`
	AppliedCount int64                     `json:"applied_count"`
}

// ApplyValueRangeTypeMapEntry handles POST /api/v1/kb/metric-value-range-type-map/apply.
// Where UpsertValueRangeTypeMapEntry only clears the error flag, this rewrites
// the data: every kb.metrics row whose normalized value_range_type equals the
// entry's raw_value gets value_range_type replaced by the entry's approved
// canonical_bucket and value_range_type_error set to NULL. Only 'approved'
// entries may be applied -- a 'proposed' row's bucket is a machine guess and is
// never authoritative (see assertions.ValueRangeTypeMapper.Lookup), so applying
// it would write an unreviewed classification into kb.metrics.
func ApplyValueRangeTypeMapEntry(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RTE_300")
	defer rc.Close()
	logger := rc.GetLogger()

	var req applyValueRangeTypeMapEntryRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid request body (CWB_KB_RTE_310)",
		})
	}

	rawValue := assertions.NormalizeValueRangeTypeRaw(req.RawValue)
	if rawValue == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "raw_value is required (CWB_KB_RTE_311)",
		})
	}

	db := ApiTypes.ProjectDBHandle

	entry, err := fetchValueRangeTypeMapEntry(db, rawValue)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "no mapping entry for that raw_value (CWB_KB_RTE_312)",
		})
	}
	if err != nil {
		logger.Error("load kb.metric_value_range_type_map entry failed", "raw_value", rawValue, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to load mapping entry (CWB_KB_RTE_313)",
		})
	}
	if entry.Status != "approved" {
		return c.JSON(http.StatusConflict, errorResponse{
			Status: false, ErrorMsg: "only approved entries can be applied (CWB_KB_RTE_314)",
		})
	}
	bucket := ""
	if entry.CanonicalBucket != nil {
		bucket = strings.TrimSpace(*entry.CanonicalBucket)
	}
	if bucket == "" {
		return c.JSON(http.StatusConflict, errorResponse{
			Status: false, ErrorMsg: "entry has no canonical_bucket to apply (CWB_KB_RTE_315)",
		})
	}

	// Same hand-ported normalization predicate as the cascade in
	// UpsertValueRangeTypeMapEntry (design D2) -- keep all three copies in sync
	// with assertions.normalizeValueRangeTypeRaw. The trailing IS DISTINCT
	// FROM / IS NOT NULL guard keeps applied_count honest: a second Apply on an
	// already-applied entry reports 0 rather than re-counting untouched rows.
	const applyQuery = `
UPDATE kb.metrics
   SET value_range_type = $2,
       value_range_type_error = NULL
 WHERE lower(regexp_replace(trim(value_range_type), '[- ]', '_', 'g')) = $1
   AND (value_range_type IS DISTINCT FROM $2 OR value_range_type_error IS NOT NULL)`
	result, err := db.Exec(applyQuery, rawValue, bucket)
	if err != nil {
		logger.Error("apply canonical bucket to kb.metrics failed",
			"raw_value", rawValue, "canonical_bucket", bucket, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to apply mapping to metrics (CWB_KB_RTE_320)",
		})
	}
	applied, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected on apply failed", "err", err)
		applied = 0
	}
	logger.Info("applied value range type mapping to kb.metrics",
		"raw_value", rawValue, "canonical_bucket", bucket, "applied_count", applied)

	return c.JSON(http.StatusOK, applyValueRangeTypeMapEntryResponse{
		Status: true, Entry: entry, AppliedCount: applied,
	})
}
