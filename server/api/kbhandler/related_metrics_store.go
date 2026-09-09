package kbhandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/classcontractsearch"
	"github.com/lib/pq"
)

// Tuning for GET /api/v1/kb/metrics/:metric_id/related-metrics
// (openspec change analysis-node-related-metrics, design D2/D4/D5).
const (
	relatedMetricsDefaultSimilar   = 20
	relatedMetricsDefaultSameClass = 200
	relatedMetricsMaxLimit         = 200
	// similarClassCandidateK: matched class contracts pulled before expanding
	// to their instances (design D5).
	similarClassCandidateK = 30
)

// errRelatedMetricNotFound is returned when no kb.metrics row has the id; the
// handler maps it to HTTP 404.
var errRelatedMetricNotFound = errors.New("metric not found")

// relatedMetricRow is one row of the related-metrics response (design D2).
type relatedMetricRow struct {
	MetricID           string  `json:"metric_id"`
	MetricName         string  `json:"metric_name"`
	MetricNameEn       string  `json:"metric_name_en"`
	InputRecordID      int64   `json:"input_record_id"`
	ClassTermID        string  `json:"class_term_id"`
	ClassLabel         string  `json:"class_label"`
	MetricValue        string  `json:"metric_value,omitempty"`
	MetricUnit         string  `json:"metric_unit,omitempty"`
	SourceFilename     string  `json:"source_filename,omitempty"`
	Score              float64 `json:"score,omitempty"`
	MatchedClassTermID string  `json:"matched_class_term_id,omitempty"`
}

type relatedMetricsStore struct {
	DB *sql.DB
}

func (s relatedMetricsStore) metricExists(ctx context.Context, metricID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM kb.metrics WHERE metric_id = $1)`, metricID).Scan(&ok)
	return ok, err
}

// resolveClassTermID returns the governed class term the metric's resulting
// assertion(s) resolved to, or "" when the metric has none (deferred candidate,
// no assertion, Phase D not completed) — an expected state, not an error.
func (s relatedMetricsStore) resolveClassTermID(ctx context.Context, metricID string) (string, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT DISTINCT a.instance_of_term_id
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
WHERE dc.source_artifact_id = $1
  AND COALESCE(a.instance_of_term_id, '') <> ''
ORDER BY a.instance_of_term_id`, metricID)
	if err != nil {
		return "", fmt.Errorf("resolve class term for %s: %w", metricID, err)
	}
	defer rows.Close()
	var first string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return "", err
		}
		if first == "" {
			first = t
		}
	}
	return first, rows.Err()
}

// sameClass returns every other metric whose resolved class term equals
// classTermID (design D4). Deterministic order, deduped, capped at limit.
func (s relatedMetricsStore) sameClass(ctx context.Context, classTermID, excludeMetricID string, limit int) ([]relatedMetricRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT m.metric_id,
       COALESCE(m.metric_name, ''),
       COALESCE(m.metric_name_en, ''),
       m.input_record_id,
       COALESCE(m.metric_value, ''),
       COALESCE(m.metric_unit, ''),
       COALESCE(i.staging_filename, '')
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE a.instance_of_term_id = $1
  AND m.metric_id <> $2
GROUP BY m.metric_id, m.metric_name, m.metric_name_en, m.input_record_id,
         m.metric_value, m.metric_unit, i.staging_filename
ORDER BY m.input_record_id, m.metric_id
LIMIT $3`, classTermID, excludeMetricID, limit)
	if err != nil {
		return nil, fmt.Errorf("same-class query for %s: %w", classTermID, err)
	}
	defer rows.Close()
	var out []relatedMetricRow
	for rows.Next() {
		var r relatedMetricRow
		if err := rows.Scan(&r.MetricID, &r.MetricName, &r.MetricNameEn, &r.InputRecordID,
			&r.MetricValue, &r.MetricUnit, &r.SourceFilename); err != nil {
			return nil, err
		}
		r.ClassTermID = classTermID
		r.ClassLabel = classTermID
		out = append(out, r)
	}
	return out, rows.Err()
}

// similarClass hybrid-matches class contracts similar to classTermID, expands to
// their instances, ranks those metrics by their matched class's score, dedupes,
// drops the selected metric's own class, and returns the top limit (design D5).
func (s relatedMetricsStore) similarClass(ctx context.Context, classTermID, excludeMetricID string, limit int, embed classcontractsearch.EmbedFunc) ([]relatedMetricRow, error) {
	matches, err := (classcontractsearch.Store{DB: s.DB}).MatchSimilar(ctx, classTermID, similarClassCandidateK, embed)
	if err != nil {
		return nil, err
	}
	scoreByClass := make(map[string]float64, len(matches))
	classIDs := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.ClassTermID == "" || m.ClassTermID == classTermID {
			continue
		}
		if _, seen := scoreByClass[m.ClassTermID]; seen {
			continue
		}
		scoreByClass[m.ClassTermID] = m.Score
		classIDs = append(classIDs, m.ClassTermID)
	}
	if len(classIDs) == 0 {
		return nil, nil
	}

	rows, err := s.DB.QueryContext(ctx, `
SELECT cls, metric_id, metric_name, metric_name_en, input_record_id, metric_value, metric_unit, source_filename
FROM (
    SELECT d.*, ROW_NUMBER() OVER (PARTITION BY d.cls ORDER BY d.input_record_id, d.metric_id) AS rn
    FROM (
        SELECT DISTINCT
            a.instance_of_term_id AS cls,
            m.metric_id,
            COALESCE(m.metric_name, '')    AS metric_name,
            COALESCE(m.metric_name_en, '') AS metric_name_en,
            m.input_record_id,
            COALESCE(m.metric_value, '')   AS metric_value,
            COALESCE(m.metric_unit, '')    AS metric_unit,
            COALESCE(i.staging_filename, '') AS source_filename
        FROM kb.semantic_assertions a
        JOIN kb.semantic_decision_candidates dc
             ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
        JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
        LEFT JOIN kb.inputs i ON i.id = m.input_record_id
        WHERE a.instance_of_term_id = ANY($1)
          AND a.instance_of_term_id <> $2
          AND m.metric_id <> $3
    ) d
) w
WHERE rn <= $4
ORDER BY cls, input_record_id, metric_id`,
		pq.Array(classIDs), classTermID, excludeMetricID, limit)
	if err != nil {
		return nil, fmt.Errorf("similar-class instance query for %s: %w", classTermID, err)
	}
	defer rows.Close()

	best := make(map[string]relatedMetricRow)
	for rows.Next() {
		var cls string
		var r relatedMetricRow
		if err := rows.Scan(&cls, &r.MetricID, &r.MetricName, &r.MetricNameEn, &r.InputRecordID,
			&r.MetricValue, &r.MetricUnit, &r.SourceFilename); err != nil {
			return nil, err
		}
		r.ClassTermID = cls
		r.ClassLabel = cls
		r.MatchedClassTermID = cls
		r.Score = scoreByClass[cls]
		if prev, ok := best[r.MetricID]; !ok || r.Score > prev.Score {
			best[r.MetricID] = r
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]relatedMetricRow, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].InputRecordID != out[j].InputRecordID {
			return out[i].InputRecordID < out[j].InputRecordID
		}
		return out[i].MetricID < out[j].MetricID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// parseRelatedMetricsScope validates the required scope query parameter.
func parseRelatedMetricsScope(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case "same_class":
		return "same_class", true
	case "similar_class":
		return "similar_class", true
	default:
		return "", false
	}
}

// relatedMetricsLimit clamps ?limit to 1..relatedMetricsMaxLimit, defaulting per
// scope when absent or invalid.
func relatedMetricsLimit(raw, scope string) int {
	def := relatedMetricsDefaultSimilar
	if scope == "same_class" {
		def = relatedMetricsDefaultSameClass
	}
	n := parsePositiveInt(strings.TrimSpace(raw), def)
	if n > relatedMetricsMaxLimit {
		return relatedMetricsMaxLimit
	}
	if n < 1 {
		return def
	}
	return n
}
