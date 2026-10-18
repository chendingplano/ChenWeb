package kbhandler

// Production and benchmark dropped candidates for the two pages that show them (openspec change
// metric-row-soft-drop-decision-model, spec metric-row-soft-drop): Knowledge System → Metrics
// and System Admin → LLM → Metrics → Benchmark. Every other reader uses kb.metrics only.

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const droppedMetricsQuery = `
SELECT d.id, d.input_record_id, d.drop_id, d.event_id, COALESCE(i.staging_filename, ''),
       d.drop_stage, d.drop_reason, d.decision, d.row_data,
       NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), ''),
       NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), ''),
       COALESCE(to_char(d.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM kb.metrics_dropped d
LEFT JOIN kb.inputs i ON i.id = d.input_record_id
WHERE d.input_record_id = $1
ORDER BY d.id ASC`

const testbedDroppedMetricsQuery = `
SELECT d.id, d.input_record_id, d.candidate_id, d.drop_reason, d.candidate_data,
       COALESCE(i.staging_filename, ''),
       NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), ''),
       NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), ''),
       COALESCE(to_char(d.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM testbed.metrics_dropped d
LEFT JOIN kb.inputs i ON i.id = d.input_record_id
WHERE d.input_record_id = $1
  AND (d.skill_name, d.skill_version, d.model_name, d.run_id) = ($2, $3, $4, $5)
ORDER BY d.id ASC`

func listBenchmarkDroppedMetrics(ctx context.Context, db *sql.DB, inputID int64, run benchmarkRunKey) ([]metricRecord, error) {
	rows, err := db.QueryContext(ctx, testbedDroppedMetricsQuery, run.queryArgs(inputID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]metricRecord, 0)
	for rows.Next() {
		var id, recordID int64
		var candidateID, reason, filename, createdAt string
		var candidate []byte
		var title, docNo sql.NullString
		if err := rows.Scan(&id, &recordID, &candidateID, &reason, &candidate, &filename, &title, &docNo, &createdAt); err != nil {
			return nil, err
		}
		r, err := benchmarkDroppedMetricRecord(id, recordID, candidateID, reason, candidate)
		if err != nil {
			return nil, err
		}
		r.InputFilename, r.CreatedAt = filename, createdAt
		r.DocumentTitle, r.DocumentDocNo = nullStringPtr(title), nullStringPtr(docNo)
		r.RunID, r.SkillName, r.SkillVersion = run.RunID, run.SkillName, run.SkillVersion
		r.ModelName = &run.ModelName
		out = append(out, r)
	}
	return out, rows.Err()
}

// Benchmark candidates need not have enriched metric fields. Preserve any that
// were supplied; use the excerpt and audit rationale for older, minimal ledgers.
func benchmarkDroppedMetricRecord(id, recordID int64, candidateID, reason string, candidate []byte) (metricRecord, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(candidate, &data); err != nil {
		return metricRecord{}, err
	}
	for storage, enrichment := range map[string]string{
		"metric_subject": "subject", "metric_subject_en": "subject_en",
		"metric_desc": "desc", "metric_desc_en": "desc_en",
		"metric_context": "context", "metric_context_en": "context_en",
		"metric_keywords": "keywords", "metric_keywords_en": "keywords_en",
		"metric_unit": "unit", "metric_unit_en": "unit_en",
	} {
		if value, ok := data[storage]; ok {
			data[enrichment] = value
		}
	}
	defaultText := func(key string, fallbacks ...string) {
		for _, candidateKey := range append([]string{key}, fallbacks...) {
			var value string
			if json.Unmarshal(data[candidateKey], &value) == nil && strings.TrimSpace(value) != "" {
				data[key] = data[candidateKey]
				return
			}
		}
	}
	defaultText("metric_name", "name", "evidence_quote", "reason", "candidate_id")
	defaultText("desc", "reason")
	rowData, err := json.Marshal(data)
	if err != nil {
		return metricRecord{}, err
	}
	return droppedMetricRecord(id, recordID, candidateID, "benchmark_rule", reason, candidate, rowData)
}

func listDroppedMetrics(ctx context.Context, db *sql.DB, inputID int64) ([]metricRecord, error) {
	rows, err := db.QueryContext(ctx, droppedMetricsQuery, inputID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]metricRecord, 0)
	for rows.Next() {
		var (
			id                  int64
			recordID            int64
			dropID, stage, rsn  string
			eventID             sql.NullString
			filename, createdAt string
			decision, rowData   []byte
			title, docNo        sql.NullString
		)
		if err := rows.Scan(&id, &recordID, &dropID, &eventID, &filename, &stage, &rsn, &decision, &rowData,
			&title, &docNo, &createdAt); err != nil {
			return nil, err
		}
		r, err := droppedMetricRecord(id, recordID, dropID, stage, rsn, decision, rowData)
		if err != nil {
			return nil, err
		}
		r.InputFilename = filename
		r.EventID = nullStringPtr(eventID)
		r.DocumentTitle = nullStringPtr(title)
		r.DocumentDocNo = nullStringPtr(docNo)
		r.CreatedAt = createdAt
		out = append(out, r)
	}
	return out, rows.Err()
}

// droppedMetricRecord maps a kb.metrics_dropped row (row_data holds the enriched row with
// extraction field names) onto the metricRecord the metrics pages already render.
func droppedMetricRecord(id, recordID int64, dropID, stage, reason string, decision, rowData []byte) (metricRecord, error) {
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rowData, &row); err != nil {
		return metricRecord{}, err
	}
	str := func(key string) *string {
		var v any
		if raw, ok := row[key]; !ok || json.Unmarshal(raw, &v) != nil || v == nil {
			return nil
		}
		var s string
		switch t := v.(type) {
		case string:
			s = t
		default:
			b, _ := json.Marshal(t)
			s = string(b)
		}
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return &s
	}
	raw := func(key string) json.RawMessage {
		if v, ok := row[key]; ok && string(v) != "null" {
			return v
		}
		return nil
	}
	r := metricRecord{
		ID:                  -id,
		InputRecordID:       recordID,
		MetricID:            &dropID,
		MetricName:          str("metric_name"),
		MetricNameEn:        str("metric_name_en"),
		SourceLineSpans:     raw("source_line_spans"),
		MetricSubject:       str("subject"),
		MetricSubjectEn:     str("subject_en"),
		MetricDesc:          str("desc"),
		MetricDescEn:        str("desc_en"),
		MetricContext:       str("context"),
		MetricContextEn:     str("context_en"),
		MetricKeywords:      raw("keywords"),
		MetricKeywordsEn:    raw("keywords_en"),
		LocationType:        str("location_type"),
		MetricUnit:          str("unit"),
		MetricUnitEn:        str("unit_en"),
		MetricValue:         str("metric_value"),
		ValueStructure:      raw("value_structure"),
		ValueDataType:       str("value_data_type"),
		ValueRangeType:      str("value_range_type"),
		ValueClass:          str("value_class"),
		ValueClassEn:        str("value_class_en"),
		FormulaOrDefinition: str("formula_or_definition"),
		ThresholdOrTarget:   str("threshold_or_target"),
		Condition:           str("condition"),
		ConditionEn:         str("condition_en"),
		Frame:               raw("frame"),
		MeasurementFreq:     str("measurement_frequency"),
		TableNameOrSection:  str("table_name_or_section"),
		ReasoningTags:       raw("reasoning_tags"),
		Dropped:             true,
		DropStage:           &stage,
		DropReason:          &reason,
	}
	// Expose the same row-reference shape as gold metrics, including older
	// extraction rows that stored citations such as "116#r2".
	if refs := docprocessing.ParseTableRowRefs(string(raw("source_table_rows"))); len(refs) > 0 {
		r.SourceTableRows, _ = json.Marshal(refs)
	}
	var conf float64
	if v := raw("confidence"); v != nil && json.Unmarshal(v, &conf) == nil {
		r.Confidence = &conf
	}
	var explicit bool
	if v := raw("is_explicit_metric"); v != nil && json.Unmarshal(v, &explicit) == nil {
		r.IsExplicitMetric = &explicit
	}
	if len(decision) > 0 && string(decision) != "null" {
		r.DropDecision = json.RawMessage(decision)
	}
	return r, nil
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
