package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MetricNormalizer is the seam-5 instantiation for the metric artifact
// family (ADR DR12: metrics is the pilot family). It reads kb.metrics rows
// for an input record and proposes one 'assertion' decision candidate per
// metric row -- never writing kb.semantic_assertions directly.
//
// extract_metrics today emits threshold_or_target as free text (the ADR's
// own "single highest-leverage change" note); rather than requiring that
// processor to change its output schema first, this normalizer performs the
// text-to-structured parsing itself (DR8's stated job for
// normalize_assertions). A value that cannot be parsed still produces a
// candidate with value_form='unparsed' and the untouched raw_text -- an
// artifact is never silently dropped for being hard to parse (spec §10.5:
// missing context produces deferred, not an invented target).
type MetricNormalizer struct{}

// FamilyName implements Normalizer.
func (MetricNormalizer) FamilyName() string { return "metric" }

// init registers the metric normalizer with the default registry (DR11 seam
// 5): adding this family required only this file, no edit to the registry
// mechanism or to normalize_assertions.go.
func init() {
	if err := RegisterNormalizer(MetricNormalizer{}); err != nil {
		panic(err)
	}
}

type metricRow struct {
	ID                   int64
	MetricID             string
	MetricName           sql.NullString
	MetricUnit           sql.NullString
	FormulaOrDefinition  sql.NullString
	ThresholdOrTarget    sql.NullString
	MeasurementFrequency sql.NullString
	Confidence           sql.NullFloat64
	SourceLineSpans      json.RawMessage
	SubjectObjectID      sql.NullString
}

// Normalize implements Normalizer.
func (n MetricNormalizer) Normalize(ctx context.Context, db *sql.DB, inputRecordID int64) (NormalizeReport, error) {
	report := NormalizeReport{ArtifactFamily: n.FamilyName()}
	if db == nil {
		return report, fmt.Errorf("db is nil")
	}

	const stmt = `
SELECT m.id, m.metric_id, m.metric_name, m.metric_unit, m.formula_or_definition,
       m.threshold_or_target, m.measurement_frequency, m.confidence, m.source_line_spans,
       ao.object_id
FROM kb.metrics m
LEFT JOIN kb.artifact_objects ao
  ON ao.artifact_type = 'metric' AND ao.artifact_id = m.metric_id AND ao.input_record_id = m.input_record_id
WHERE m.input_record_id = $1`
	rows, err := db.QueryContext(ctx, stmt, inputRecordID)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	dcStore := DecisionCandidateStore{DB: db}
	for rows.Next() {
		var r metricRow
		if err := rows.Scan(&r.ID, &r.MetricID, &r.MetricName, &r.MetricUnit, &r.FormulaOrDefinition,
			&r.ThresholdOrTarget, &r.MeasurementFrequency, &r.Confidence, &r.SourceLineSpans, &r.SubjectObjectID); err != nil {
			return report, err
		}
		report.Examined++

		metricID := strings.TrimSpace(r.MetricID)
		if metricID == "" {
			report.Skipped++
			continue
		}
		parsed := parseThresholdOrTarget(r.ThresholdOrTarget.String)

		payload := map[string]any{
			"metric_id":      metricID,
			"metric_name":    r.MetricName.String,
			"unit":           r.MetricUnit.String,
			"raw_text":       r.ThresholdOrTarget.String,
			"value_form":     parsed.ValueForm,
			"comparator":     parsed.Comparator,
			"assertion_kind": parsed.AssertionKind,
		}
		if r.SubjectObjectID.Valid && r.SubjectObjectID.String != "" {
			payload["subject_object_id"] = r.SubjectObjectID.String
		}
		if parsed.NumericValue != nil {
			payload["numeric_value"] = *parsed.NumericValue
		}
		if parsed.LowerValue != nil {
			payload["lower_value"] = *parsed.LowerValue
		}
		if parsed.UpperValue != nil {
			payload["upper_value"] = *parsed.UpperValue
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return report, err
		}

		candidate := DecisionCandidate{
			LogicalIdentityKey: fmt.Sprintf("metric:%d:%s", inputRecordID, metricID),
			CandidateKind:      "assertion",
			ProposedPayload:    payloadJSON,
			Method:             "explicit_structured",
			SourceArtifactType: "metric",
			SourceArtifactID:   metricID,
			SourceLineSpans:    r.SourceLineSpans,
		}
		if r.Confidence.Valid {
			v := r.Confidence.Float64
			candidate.Confidence = &v
		}

		result, err := dcStore.Propose(ctx, candidate)
		if err != nil {
			return report, fmt.Errorf("propose candidate for metric %s: %w", metricID, err)
		}
		if result.Reused {
			report.Reused++
		} else {
			report.Proposed++
		}
	}
	return report, rows.Err()
}

// The pilot corpus (ADR OD1: 呼吸机/医疗器械) is predominantly Chinese-language
// standard text, so the comparator vocabulary covers both languages rather
// than only English -- 不低于/不小于/至少 ("not less than"/"at least") and
// 不超过/不大于/不高于 ("not more than"/"not exceeding") are the common forms
// observed in the gold corpus's threshold_or_target values.
var (
	reLowerBound = regexp.MustCompile(`(?i)(at least|no less than|not less than|minimum of|>=|≥|不低于|不小于|至少)`)
	reUpperBound = regexp.MustCompile(`(?i)(at most|no more than|not more than|maximum of|<=|≤|不超过|不大于|不高于)`)
	reRange      = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)\s*(?:to|~|-|―|–|—|至)\s*(-?\d+(?:\.\d+)?)`)
	reNumber     = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	// reRatioDenominatorOne matches the "N:1" contrast/ratio notation.
	reRatioDenominatorOne = regexp.MustCompile(`(\d+(?:\.\d+)?):1\b`)
)

type parsedThreshold struct {
	ValueForm     string
	Comparator    string
	AssertionKind string
	NumericValue  *float64
	LowerValue    *float64
	UpperValue    *float64
}

// parseThresholdOrTarget best-effort parses extract_metrics' free-text
// threshold_or_target into a value form, comparator, and assertion kind. It
// never fabricates a value: a string with no recognizable number produces
// value_form='unparsed' with every numeric field left nil, and the original
// text is preserved unchanged by the caller regardless of parse outcome.
func parseThresholdOrTarget(raw string) parsedThreshold {
	text := strings.TrimSpace(raw)
	if text == "" {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	// Contrast-ratio notation ("500:1") is common in the pilot corpus's
	// display metrics; without this, the ":1" denominator is mistaken for a
	// second range endpoint (matching "1" instead of "500"). Collapsing it
	// to the numerator is the parseable reading -- raw_text still preserves
	// the untouched "500:1" for anyone who needs the literal ratio.
	text = reRatioDenominatorOne.ReplaceAllString(text, "$1")

	if m := reRange.FindStringSubmatch(text); m != nil {
		lo, errLo := strconv.ParseFloat(m[1], 64)
		hi, errHi := strconv.ParseFloat(m[2], 64)
		if errLo == nil && errHi == nil {
			return parsedThreshold{
				ValueForm: "range", Comparator: "between", AssertionKind: "interval_requirement",
				LowerValue: &lo, UpperValue: &hi,
			}
		}
	}

	numMatch := reNumber.FindString(text)
	if numMatch == "" {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	value, err := strconv.ParseFloat(numMatch, 64)
	if err != nil {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}

	switch {
	case reLowerBound.MatchString(text):
		return parsedThreshold{ValueForm: "single", Comparator: ">=", AssertionKind: "lower_bound_requirement", NumericValue: &value}
	case reUpperBound.MatchString(text):
		return parsedThreshold{ValueForm: "single", Comparator: "<=", AssertionKind: "upper_bound_requirement", NumericValue: &value}
	default:
		// A bare number with no comparator language is recorded as an
		// observed value, not a requirement -- spec §16.3 item 8's
		// distinction between requirement and observation assertion kinds.
		return parsedThreshold{ValueForm: "single", Comparator: "=", AssertionKind: "observed_value", NumericValue: &value}
	}
}
