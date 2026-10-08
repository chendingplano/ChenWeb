package kbhandler

// Dropped metric rows for the two pages that may show them (openspec change
// metric-row-soft-drop-decision-model, spec metric-row-soft-drop): Knowledge System → Metrics
// and System Admin → LLM → Metrics → Benchmark. Every other reader uses kb.metrics only.

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
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
		ValueDataType:       str("value_data_type"),
		ValueRangeType:      str("value_range_type"),
		ValueClass:          str("value_class"),
		ValueClassEn:        str("value_class_en"),
		FormulaOrDefinition: str("formula_or_definition"),
		ThresholdOrTarget:   str("threshold_or_target"),
		MeasurementFreq:     str("measurement_frequency"),
		TableNameOrSection:  str("table_name_or_section"),
		ReasoningTags:       raw("reasoning_tags"),
		Dropped:             true,
		DropStage:           &stage,
		DropReason:          &reason,
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
