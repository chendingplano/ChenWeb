package kbhandler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// metricWikiContext is the compiled, grounded input for generating a metric's
// wiki page. It is assembled from kb.metrics plus the source document's
// metadata. Source-chunk summary enrichment is a tracked follow-up.
type metricWikiContext struct {
	MetricID string            `json:"metric_id"`
	RecordID int64             `json:"record_id"`
	Metric   metricRecord      `json:"metric"`
	Document metricWikiDocMeta `json:"document"`
}

// metricWikiDocMeta is the subset of kb.inputs used to ground a page in its
// source document.
type metricWikiDocMeta struct {
	RecordID int64  `json:"record_id"`
	Title    string `json:"title"`
	FileName string `json:"file_name"`
	Type     string `json:"type"`
}

// compileMetricWikiContext gathers everything known about a metric. A missing
// metric is a hard error (CWB_KB_MWIKI_020 at the caller); missing document
// metadata degrades to empty fields so a page can still be produced.
func compileMetricWikiContext(db *sql.DB, recordID int64, metricID string) (metricWikiContext, error) {
	if db == nil {
		return metricWikiContext{}, fmt.Errorf("nil DB handle")
	}

	if rid, seqno, err := parseMetricID(metricID); err == nil {
		recordID = rid
		metricID = canonicalMetricID(rid, seqno)
	}

	metric, err := fetchMetricByMetricID(db, metricID)
	if err != nil {
		return metricWikiContext{}, err
	}

	doc := metricWikiDocMeta{RecordID: recordID}
	if meta, err := fetchWikiDocMeta(db, recordID); err == nil {
		doc = meta
	}

	return metricWikiContext{
		MetricID: metricID,
		RecordID: recordID,
		Metric:   metric,
		Document: doc,
	}, nil
}

// fetchMetricByMetricID loads a single kb.metrics row by its business key
// (metric_id). Returns sql.ErrNoRows-wrapped error when not found.
func fetchMetricByMetricID(db *sql.DB, metricID string) (metricRecord, error) {
	const query = `
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric, m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.metric_id = $1
`
	load := func(id string) (metricRecord, error) {
		var (
		r               metricRecord
		spansBytes      []byte
		keywordsBytes   []byte
		keywordsEnBytes []byte
		reasoningBytes  []byte
		confidence      sql.NullFloat64
		isExplicit      sql.NullBool
		)
		err := db.QueryRow(query, id).Scan(
		&r.ID, &r.InputRecordID, &r.MetricID, &r.EventID, &r.InputFilename,
		&r.MetricName, &r.MetricNameEn, &spansBytes, &r.MetricSubject, &r.MetricSubjectEn,
		&r.MetricDesc, &r.MetricDescEn, &r.MetricContext, &r.MetricContextEn,
		&keywordsBytes, &keywordsEnBytes, &r.ModelName, &r.LocationType, &r.MetricUnit, &r.MetricUnitEn,
		&r.MetricValue, &r.ValueDataType, &r.ValueRangeType, &r.ValueClass, &r.ValueClassEn,
		&r.FormulaOrDefinition, &r.ThresholdOrTarget, &r.MeasurementFreq,
		&confidence, &isExplicit, &r.TableNameOrSection, &reasoningBytes, &r.CreatedAt,
	)
		if err != nil {
			return metricRecord{}, err
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
		if r.MetricID != nil {
			r.ObjectNodeCanonicalName = objectNodeCanonicalName(db, r.InputRecordID, "metric", *r.MetricID)
		}
		return r, nil
	}

	r, err := load(metricID)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return metricRecord{}, fmt.Errorf("metric %q not found: %w", metricID, err)
	}

	if rid, seqno, parseErr := parseMetricID(metricID); parseErr == nil {
		legacyID := fmt.Sprintf("%d_%d", rid, seqno)
		canonicalID := canonicalMetricID(rid, seqno)
		fallbackID := legacyID
		if metricID == legacyID {
			fallbackID = canonicalID
		}
		if fallbackID != metricID {
			if r, fallbackErr := load(fallbackID); fallbackErr == nil {
				return r, nil
			}
		}
	}

	return metricRecord{}, fmt.Errorf("metric %q not found: %w", metricID, err)
}

// fetchWikiDocMeta loads grounding metadata for the source document.
func fetchWikiDocMeta(db *sql.DB, recordID int64) (metricWikiDocMeta, error) {
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return metricWikiDocMeta{RecordID: recordID}, err
	}
	q := fmt.Sprintf(`
SELECT
    COALESCE(NULLIF(TRIM(title), ''), NULLIF(TRIM(file_name), ''), '') AS title,
    COALESCE(file_name, '') AS file_name,
    COALESCE(type, '') AS type
FROM %s WHERE id = $1`, inputTable)
	var meta metricWikiDocMeta
	meta.RecordID = recordID
	if err := db.QueryRow(q, recordID).Scan(&meta.Title, &meta.FileName, &meta.Type); err != nil {
		return metricWikiDocMeta{RecordID: recordID}, err
	}
	return meta, nil
}

// metricWikiSourceHash is a stable digest of the compiled context. It is stored
// in the generated page so a future retention/invalidation policy can detect
// when the underlying data changed without re-reading the source.
func metricWikiSourceHash(ctx metricWikiContext) string {
	data, err := json.Marshal(ctx)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + fmt.Sprintf("%x", sum[:])
}

// metricDisplayTitle picks the best human title for a metric, preferring the
// original-language name and falling back to the English name or the id.
func metricDisplayTitle(m metricRecord) string {
	for _, v := range []*string{m.MetricName, m.MetricNameEn} {
		if v != nil && strings.TrimSpace(*v) != "" {
			return strings.TrimSpace(*v)
		}
	}
	if m.MetricID != nil {
		return *m.MetricID
	}
	return ""
}
