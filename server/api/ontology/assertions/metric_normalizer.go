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
	ID                     int64
	MetricID               string
	MetricDefinitionTermID sql.NullString
	MetricName             sql.NullString
	MetricUnit             sql.NullString
	MetricUnitEn           sql.NullString
	FormulaOrDefinition    sql.NullString
	ThresholdOrTarget      sql.NullString
	MeasurementFrequency   sql.NullString
	Confidence             sql.NullFloat64
	SourceLineSpans        json.RawMessage
	SubjectObjectID        sql.NullString
	ValueRangeType         sql.NullString
	ValueClass             sql.NullString
	MetricValue            sql.NullString
	ValueMin               sql.NullFloat64
	ValueMax               sql.NullFloat64
	Condition              sql.NullString
}

// Normalize implements Normalizer.
func (n MetricNormalizer) Normalize(ctx context.Context, db *sql.DB, inputRecordID int64) (NormalizeReport, error) {
	report := NormalizeReport{ArtifactFamily: n.FamilyName()}
	if db == nil {
		return report, fmt.Errorf("db is nil")
	}

	const stmt = `
SELECT m.id, m.metric_id, m.metric_name, m.metric_unit, m.metric_unit_en, m.formula_or_definition,
       m.metric_definition_term_id, m.threshold_or_target, m.measurement_frequency, m.confidence, m.source_line_spans,
       ao.object_id,
       m.value_range_type, m.value_class, m.metric_value, m.value_min, m.value_max, m.condition
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
		if err := rows.Scan(&r.ID, &r.MetricID, &r.MetricName, &r.MetricUnit, &r.MetricUnitEn, &r.FormulaOrDefinition, &r.MetricDefinitionTermID,
			&r.ThresholdOrTarget, &r.MeasurementFrequency, &r.Confidence, &r.SourceLineSpans, &r.SubjectObjectID,
			&r.ValueRangeType, &r.ValueClass, &r.MetricValue, &r.ValueMin, &r.ValueMax, &r.Condition); err != nil {
			return report, err
		}
		report.Examined++

		metricID := strings.TrimSpace(r.MetricID)
		if metricID == "" {
			report.Skipped++
			continue
		}
		payload := metricCandidatePayloadForRow(r)
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return report, err
		}

		recordID := inputRecordID
		candidate := DecisionCandidate{
			LogicalIdentityKey: fmt.Sprintf("metric:%d:%s", inputRecordID, metricID),
			CandidateKind:      "assertion",
			ProposedPayload:    payloadJSON,
			Method:             "explicit_structured",
			SourceArtifactType: "metric",
			SourceArtifactID:   metricID,
			InputRecordID:      &recordID,
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

func metricCandidatePayloadForRow(r metricRow) map[string]any {
	unit := r.MetricUnit.String
	if unit == "" {
		unit = r.MetricUnitEn.String
	}
	parsed := resolveMetricValue(r)
	payload := map[string]any{
		"metric_id":      strings.TrimSpace(r.MetricID),
		"metric_name":    r.MetricName.String,
		"unit":           unit,
		"raw_text":       r.ThresholdOrTarget.String,
		"value_form":     parsed.ValueForm,
		"comparator":     parsed.Comparator,
		"assertion_kind": parsed.AssertionKind,
	}
	if r.MetricDefinitionTermID.Valid && strings.TrimSpace(r.MetricDefinitionTermID.String) != "" {
		payload["metric_definition_term_id"] = strings.TrimSpace(r.MetricDefinitionTermID.String)
	}
	if r.Condition.Valid && r.Condition.String != "" {
		payload["condition"] = r.Condition.String
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
	return payload
}

// The pilot corpus (ADR OD1: 呼吸机/医疗器械) is predominantly Chinese-language
// standard text, so the comparator vocabulary covers both languages rather
// than only English -- 不低于/不小于/不得低于/至少 ("not less than"/"at least")
// and 不超过/不大于/不高于/不应超过/不得超过 ("not more than"/"not exceeding")
// are the common forms observed in the gold corpus's threshold_or_target
// values.
var (
	reLowerBound = regexp.MustCompile(`(?i)(at least|no less than|not less than|minimum of|>=|≥|不低于|不小于|不得低于|至少)`)
	reUpperBound = regexp.MustCompile(`(?i)(at most|no more than|not more than|maximum of|<=|≤|不超过|不大于|不高于|不应超过|不得超过)`)
	// reRangeKeyword matches an explicit range keyword/dash between two
	// numbers, with optional surrounding whitespace.
	reRangeKeyword = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)\s*(?:to|~|～|至|―|–|—)\s*(-?\d+(?:\.\d+)?)`)
	// reRangeHyphen matches a plain ASCII hyphen as a range separator only
	// when whitespace surrounds it on both sides -- an unspaced hyphen
	// between two numbers is at least as likely to be part of a standard/
	// document identifier ("GB 9706.1-2020") as a genuine range, and
	// reRangeKeyword already covers "10-20"-style ranges via its keyword
	// forms once a space or Chinese range marker is present.
	reRangeHyphen = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+-\s+(-?\d+(?:\.\d+)?)`)
	// reWholeRange accepts only a complete, two-endpoint range. Structured
	// range rows may contain schedules or tiered classifications; extracting a
	// substring from those compound values would fabricate an interval.
	reWholeRange = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*(?:to|~|～|至|―|–|—|\s-\s)\s*(-?\d+(?:\.\d+)?)\s*$`)
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

// structuredValueAssertionKind maps a kb.metrics value_range_type + value_class
// to the governed assertion kind the normalizer emits (design D1).
var structuredValueAssertionKind = map[string]string{
	"lower_bound": "lower_bound_requirement",
	"upper_bound": "upper_bound_requirement",
	"exact":       "exact_value",
	"range":       "interval_requirement",
}

// canonicalMetricValueRangeType is the compatibility boundary between the
// extraction vocabulary and normalization's small structured vocabulary. The
// extractor has emitted these synonymous values in production; accepting them
// here lets both independently versioned stages retain a stable contract.
// CanonicalMetricValueRangeType is the extractor/normalizer range contract.
// Extraction persists this canonical form and normalization retains the same
// boundary defensively for historical rows.
func CanonicalMetricValueRangeType(raw string) string {
	vrt := strings.ToLower(strings.TrimSpace(raw))
	vrt = strings.ReplaceAll(vrt, "-", "_")
	vrt = strings.ReplaceAll(vrt, " ", "_")
	switch vrt {
	case "min", "minimum", "min_threshold", "lower_limit", "lower_threshold":
		return "lower_bound"
	case "max", "maximum", "max_threshold", "upper_limit", "upper_threshold":
		return "upper_bound"
	case "exact", "exact_count", "exact_duration", "exact_ratio", "exact_specification":
		return "exact"
	case "range", "interval", "between", "value_range":
		return "range"
	default:
		return vrt
	}
}

// valueClassAssertionKind refines the assertion kind by value_class, mirroring
// the parser's requirement-vs-observation distinction: an observed test result
// is an observed_value even when the source phrasing is a bound.
var valueClassAssertionKind = map[string]string{
	"observation":       "observed_value",
	"requirement":       "",
	"target":            "target",
	"reference":         "reference",
	"design_capability": "capability",
	"definition":        "reference",
}

// resolveMetricValue is the structured-first path (design D1). When the row
// carries a populated value_range_type it maps the enum + value_class +
// metric_value/value_min/value_max deterministically into value form,
// comparator, assertion kind, and numeric fields -- no free-text parsing. When
// the enum is empty (pre-structured-schema rows, or extraction that left it
// unset) it falls back to parseThresholdOrTarget on threshold_or_target. It
// never fabricates: a structured row with no parseable number produces an
// honest unparsed, never a value invented from unrelated text.
func resolveMetricValue(r metricRow) parsedThreshold {
	vrt := CanonicalMetricValueRangeType(r.ValueRangeType.String)
	if vrt == "" {
		// Legacy row (pre-structured-schema, or extraction left the enum
		// unset): fall back to the free-text parser unchanged.
		return parseThresholdOrTarget(r.ThresholdOrTarget.String)
	}
	// kind starts from the enum's base assertion kind (empty for
	// qualitative/limit_absent, which have no numeric-kind term) and is
	// refined by value_class.
	kind := structuredValueAssertionKind[vrt]
	if vc, ok := valueClassAssertionKind[strings.TrimSpace(r.ValueClass.String)]; ok && vc != "" {
		kind = vc
	}

	switch vrt {
	case "range":
		if r.ValueMin.Valid && r.ValueMax.Valid {
			lo, hi := r.ValueMin.Float64, r.ValueMax.Float64
			return parsedThreshold{
				ValueForm: "range", Comparator: "between", AssertionKind: kind,
				LowerValue: &lo, UpperValue: &hi,
			}
		}
		// The current extractor records production ranges in metric_value but
		// has not populated value_min/value_max. Parsing only the known range
		// shape retains a structured interval without inventing endpoints.
		rawRange := strings.NewReplacer("℃", "", "°C", "", "°", "").Replace(r.MetricValue.String)
		if match := reWholeRange.FindStringSubmatch(rawRange); match != nil {
			lo, errLo := strconv.ParseFloat(match[1], 64)
			hi, errHi := strconv.ParseFloat(match[2], 64)
			if errLo == nil && errHi == nil {
				return parsedThreshold{ValueForm: "range", Comparator: "between", AssertionKind: kind, LowerValue: &lo, UpperValue: &hi}
			}
		}
	case "lower_bound", "upper_bound":
		if v, ok := firstParseableNumber(r.MetricValue.String); ok {
			comparator := ">="
			if vrt == "upper_bound" {
				comparator = "<="
			}
			return parsedThreshold{
				ValueForm: "single", Comparator: comparator, AssertionKind: kind,
				NumericValue: &v,
			}
		}
	case "exact":
		// Ratios such as 1:10 are not scalar exact values. Their representation
		// is not yet modeled as a governed literal, so preserve them as
		// unparsed rather than silently accepting the numerator as a value.
		if strings.Contains(strings.TrimSpace(r.MetricValue.String), ":") {
			break
		}
		if v, ok := firstParseableNumber(r.MetricValue.String); ok {
			return parsedThreshold{
				ValueForm: "single", Comparator: "=", AssertionKind: kind,
				NumericValue: &v,
			}
		}
	case "qualitative", "limit_absent":
		// No numeric fields by design; the kind follows value_class (a
		// qualitative observation is an observed_value with no number).
		return parsedThreshold{ValueForm: vrt, AssertionKind: kind}
	}
	// The row claims a structured enum but either it is unrecognized or the
	// value cannot be grounded in the structured columns. Honest unparsed --
	// never a value invented from unrelated text, and never a free-text parse
	// of a row that declared structured values.
	return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
}

// firstParseableNumber returns the first number in s ("" -> no number), which
// is what a single numeric metric_value carries. For structured rows this is
// the trusted source; unlike parseThresholdOrTarget there is no comparator to
// anchor against, so a non-numeric metric_value is honest unparsed.
func firstParseableNumber(s string) (float64, bool) {
	m := reNumber.FindString(strings.TrimSpace(s))
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, false
	}
	return v, true
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

	m := reRangeKeyword.FindStringSubmatch(text)
	if m == nil {
		m = reRangeHyphen.FindStringSubmatch(text)
	}
	if m != nil {
		lo, errLo := strconv.ParseFloat(m[1], 64)
		hi, errHi := strconv.ParseFloat(m[2], 64)
		if errLo == nil && errHi == nil {
			return parsedThreshold{
				ValueForm: "range", Comparator: "between", AssertionKind: "interval_requirement",
				LowerValue: &lo, UpperValue: &hi,
			}
		}
	}

	// Anchor the numeric match to the position right after the matched
	// comparator keyword rather than the first number anywhere in the
	// string -- otherwise a leading distance, date, or standard-number
	// token ("在 1 m 距离处不低于 250", "GB 9706.1-2020 规定不低于 250") is
	// mistaken for the threshold value itself. A comparator keyword with no
	// number after it is unparsed, never fabricated from an earlier number.
	if loc := reLowerBound.FindStringIndex(text); loc != nil {
		if value, ok := firstNumberAfter(text, loc[1]); ok {
			return parsedThreshold{ValueForm: "single", Comparator: ">=", AssertionKind: "lower_bound_requirement", NumericValue: &value}
		}
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	if loc := reUpperBound.FindStringIndex(text); loc != nil {
		if value, ok := firstNumberAfter(text, loc[1]); ok {
			return parsedThreshold{ValueForm: "single", Comparator: "<=", AssertionKind: "upper_bound_requirement", NumericValue: &value}
		}
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}

	// No comparator keyword: a bare number is recorded as an observed value,
	// not a requirement -- spec §16.3 item 8's distinction between
	// requirement and observation assertion kinds.
	numMatch := reNumber.FindString(text)
	if numMatch == "" {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	value, err := strconv.ParseFloat(numMatch, 64)
	if err != nil {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	return parsedThreshold{ValueForm: "single", Comparator: "=", AssertionKind: "observed_value", NumericValue: &value}
}

// firstNumberAfter returns the first parseable number in text at or after
// byte offset from, and whether one was found.
func firstNumberAfter(text string, from int) (float64, bool) {
	if from > len(text) {
		return 0, false
	}
	numMatch := reNumber.FindString(text[from:])
	if numMatch == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(numMatch, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
