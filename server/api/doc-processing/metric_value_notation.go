package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/width"
)

// Deterministic Pass 2 value normalization and condition identity (openspec change
// extract-metrics-conditions, design D2/D3). Notation only: nothing here needs judgement about
// the document, so the rules hold for every domain and language.

// metricConditionKey normalizes a row's condition for identity comparisons: full-width forms
// folded, case and whitespace ignored.
func metricConditionKey(m map[string]any) string {
	c := width.Fold.String(strings.TrimSpace(asString(m["condition"])))
	c = strings.ToLower(strings.Join(strings.Fields(c), ""))
	return strings.ReplaceAll(c, ";", "；")
}

// metricConditionLabelRe matches the `<kind>：` label of one condition qualifier.
var metricConditionLabelRe = regexp.MustCompile(`^[^：:]{1,12}[：:]`)

// metricConditionCompact folds width and drops whitespace, for containment checks.
func metricConditionCompact(s string) string {
	return strings.Join(strings.Fields(width.Fold.String(s)), "")
}

// dropConditionPropertyRepeats removes condition qualifiers that only repeat the row's property:
// "测量部位：座板高度" on 座板高度分度值, "测量项目：体重" on 体脂率测试仪体重分度值 (371). The property is
// already in metric_name, so the qualifier adds nothing, and because condition is part of the
// metric's identity it would keep the row from merging with the same metric stated elsewhere.
// condition_en loses the same qualifier when both have the same number of qualifiers. A test
// apparatus or method qualifier is kept even when the name repeats it: on the apparatus's own
// error row (电子秒表误差, 试验装置：电子秒表) it still says which clause's setting this is.
func dropConditionPropertyRepeats(metrics []map[string]any) {
	for _, m := range metrics {
		cond := strings.TrimSpace(asString(m["condition"]))
		name := metricConditionCompact(asString(m["metric_name"]))
		if cond == "" || name == "" {
			continue
		}
		parts := strings.Split(strings.ReplaceAll(cond, ";", "；"), "；")
		enParts := strings.Split(asString(m["condition_en"]), ";")
		keep, keepEn := make([]string, 0, len(parts)), make([]string, 0, len(enParts))
		for i, q := range parts {
			label := metricConditionLabelRe.FindString(strings.TrimSpace(q))
			v := metricConditionCompact(strings.TrimPrefix(strings.TrimSpace(q), label))
			if v != "" && strings.Contains(name, v) && !metricConditionApparatusLabel(label) {
				continue
			}
			if strings.TrimSpace(q) != "" {
				keep = append(keep, strings.TrimSpace(q))
			}
			if len(enParts) == len(parts) && strings.TrimSpace(enParts[i]) != "" {
				keepEn = append(keepEn, strings.TrimSpace(enParts[i]))
			}
		}
		if len(keep) == len(parts) {
			continue
		}
		m["condition"] = strings.Join(keep, "；")
		if len(enParts) == len(parts) {
			m["condition_en"] = strings.Join(keepEn, "; ")
		}
	}
}

// metricConditionApparatusLabel reports whether a qualifier label names a test apparatus or method.
func metricConditionApparatusLabel(label string) bool {
	l := strings.ToLower(width.Fold.String(label))
	for _, k := range []string{"试验装置", "试验方法", "试验方式", "apparatus", "method"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// metricConditionsEqual reports whether two rows have the same normalized condition. An empty
// condition differs from a non-empty one.
func metricConditionsEqual(a, b map[string]any) bool {
	return metricConditionKey(a) == metricConditionKey(b)
}

// toOptionalFloat returns a float64 for a numeric JSON value, or nil when the value is absent or
// not a number. Range endpoints must stay NULL rather than become 0.
func toOptionalFloat(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f
		}
	}
	return nil
}

// nullFloatValue maps a scanned nullable column to a float64 or nil.
func nullFloatValue(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// metricValueBoundSQLValue binds value_min / value_max: a number, or NULL.
func metricValueBoundSQLValue(v any) any {
	return toOptionalFloat(v)
}

const metricNumberPattern = `-?\d+(?:\.\d+)?`

var (
	// A value that is a single number, a symmetric tolerance, an interval or a centre±deviation.
	metricPlainValueRe   = regexp.MustCompile(`^(?:±?` + metricNumberPattern + `)(?:(?:~|-|±)` + metricNumberPattern + `)?$`)
	metricToleranceRe    = regexp.MustCompile(`^±(` + metricNumberPattern + `)$`)
	metricIntervalRe     = regexp.MustCompile(`^(` + metricNumberPattern + `)~(` + metricNumberPattern + `)$`)
	metricHyphenRe       = regexp.MustCompile(`^(\d+(?:\.\d+)?)-(\d+(?:\.\d+)?)$`)
	metricCentreRe       = regexp.MustCompile(`^(` + metricNumberPattern + `)±(\d+(?:\.\d+)?)$`)
	metricNumberRe       = regexp.MustCompile(`\d+(?:\.\d+)?`)
	metricSingleNumberRe = regexp.MustCompile(`^` + metricNumberPattern + `$`)
	metricFractionRe     = regexp.MustCompile(`^\d+/\d+$`)
	metricListSepRe      = regexp.MustCompile(`[、；;，]`) // not "/": it is part of units such as 次/min
)

// metricLeadingComparators maps a leading comparator to its range type and strictness. Longer
// words come first so "不小于" is not read as "小于".
var metricLeadingComparators = []struct {
	text      string
	rangeType string
	strict    bool
}{
	{"不小于", "lower_bound", false}, {"不低于", "lower_bound", false}, {"不少于", "lower_bound", false},
	{"不大于", "upper_bound", false}, {"不高于", "upper_bound", false}, {"不超过", "upper_bound", false},
	{"不多于", "upper_bound", false}, {"至少", "lower_bound", false}, {"至多", "upper_bound", false},
	{"大于", "lower_bound", true}, {"高于", "lower_bound", true}, {"超过", "lower_bound", true},
	{"小于", "upper_bound", true}, {"低于", "upper_bound", true},
	{">=", "lower_bound", false}, {"<=", "upper_bound", false},
	{"≥", "lower_bound", false}, {"⩾", "lower_bound", false}, {"≧", "lower_bound", false},
	{"≤", "upper_bound", false}, {"⩽", "upper_bound", false}, {"≦", "upper_bound", false},
	{">", "lower_bound", true}, {"<", "upper_bound", true},
}

// compactMetricValue folds full-width forms, removes all whitespace and writes interval
// separators as "~", so "600 mm～1 600 mm" style values can be checked against the plain forms.
func compactMetricValue(v string) string {
	v = width.Fold.String(v)
	v = strings.Join(strings.Fields(v), "")
	for _, sep := range []string{"〜", "～", "—", "–", "至", "到"} {
		v = strings.ReplaceAll(v, sep, "~")
	}
	return strings.ReplaceAll(v, "+/-", "±")
}

// stripUnitFromMetricValue removes copies of unit written right after a number. A unit followed
// by a letter or "/" is part of a longer unit (次 in 次/min) and is kept. The result is used only
// when it is a plain value.
func stripUnitFromMetricValue(value, unit string) (string, bool) {
	unit = strings.TrimSpace(width.Fold.String(unit))
	if unit == "" {
		return value, false
	}
	re, err := regexp.Compile(`(\d)\s*` + regexp.QuoteMeta(unit) + `([^\p{L}/²³·]|$)`)
	if err != nil {
		return value, false
	}
	folded := width.Fold.String(value)
	stripped := folded
	for {
		next := re.ReplaceAllString(stripped, "$1$2")
		if next == stripped {
			break
		}
		stripped = next
	}
	if stripped == folded {
		return value, false
	}
	compact := compactMetricValue(stripped)
	if !metricPlainValueRe.MatchString(trimLeadingComparator(compact)) {
		return value, false
	}
	return compact, true
}

// trimLeadingComparator drops one leading comparator, if any.
func trimLeadingComparator(v string) string {
	for _, c := range metricLeadingComparators {
		if strings.HasPrefix(v, c.text) {
			return strings.TrimPrefix(v, c.text)
		}
	}
	return v
}

// stripLeadingComparator removes one leading comparator from a single-number value.
func stripLeadingComparator(value string) (rest, rangeType string, strict, ok bool) {
	v := compactMetricValue(value)
	for _, c := range metricLeadingComparators {
		if strings.HasPrefix(v, c.text) {
			rest = strings.TrimPrefix(v, c.text)
			if metricSingleNumberRe.MatchString(rest) {
				return rest, c.rangeType, c.strict, true
			}
			return "", "", false, false
		}
	}
	return "", "", false, false
}

func addReasoningTag(m map[string]any, tag string) {
	tags := toStringSlice(m["reasoning_tags"])
	for _, t := range tags {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return
		}
	}
	m["reasoning_tags"] = append(tags, tag)
}

// metricRowUnit returns the row's unit under either the Pass 2 or the kb.metrics key.
func metricRowUnit(m map[string]any) string {
	return firstNonEmptyTrimmed(asString(m["unit"]), asString(m["metric_unit"]))
}

// normalizeMetricValueNotation applies design D3 to rows whose range types are already
// canonical: unit and comparator out of metric_value, ±x as upper_bound, range endpoints.
func normalizeMetricValueNotation(metrics []map[string]any) {
	dropConditionPropertyRepeats(metrics)
	for _, m := range metrics {
		value := strings.TrimSpace(asString(m["metric_value"]))
		if value == "" {
			continue
		}
		rangeType := strings.TrimSpace(asString(m["value_range_type"]))

		if stripped, ok := stripUnitFromMetricValue(value, metricRowUnit(m)); ok {
			value = stripped
		}
		if rest, cmpRange, strict, ok := stripLeadingComparator(value); ok {
			value = rest
			if rangeType == "" || rangeType == "exact" {
				rangeType = cmpRange
			}
			if strict {
				addReasoningTag(m, "strict_bound")
			}
		}
		compact := compactMetricValue(value)
		if metricToleranceRe.MatchString(compact) && (rangeType == "exact" || rangeType == "range") {
			rangeType = "upper_bound"
			m["value_min"], m["value_max"] = nil, nil
		}
		if rangeType == "range" && (toOptionalFloat(m["value_min"]) == nil || toOptionalFloat(m["value_max"]) == nil) {
			if lo, hi, ok := metricRangeEndpoints(compact); ok {
				m["value_min"], m["value_max"] = lo, hi
			}
		}
		m["metric_value"] = value
		if rangeType != "" {
			m["value_range_type"] = rangeType
		}
	}
}

// metricRangeEndpoints reads the endpoints of a plain interval (a~b, a-b) or of a±b.
func metricRangeEndpoints(compact string) (float64, float64, bool) {
	for _, re := range []*regexp.Regexp{metricIntervalRe, metricHyphenRe} {
		if g := re.FindStringSubmatch(compact); g != nil {
			lo, err1 := strconv.ParseFloat(g[1], 64)
			hi, err2 := strconv.ParseFloat(g[2], 64)
			if err1 == nil && err2 == nil {
				return lo, hi, true
			}
		}
	}
	if g := metricCentreRe.FindStringSubmatch(compact); g != nil {
		c, err1 := strconv.ParseFloat(g[1], 64)
		d, err2 := strconv.ParseFloat(g[2], 64)
		if err1 == nil && err2 == nil {
			return c - d, c + d, true
		}
	}
	return 0, 0, false
}

// metricValueHoldsSeveralValues reports a metric_value that still holds more than one number
// joined by a list separator or by sex labels (design D3 step 5).
func metricValueHoldsSeveralValues(value string) bool {
	v := compactMetricValue(value)
	if len(metricNumberRe.FindAllString(v, -1)) < 2 || metricFractionRe.MatchString(v) {
		return false
	}
	return metricListSepRe.MatchString(v) || (strings.Contains(v, "男") && strings.Contains(v, "女"))
}

// multiValueMetricRows lists the rows whose metric_value holds several values.
func multiValueMetricRows(metrics []map[string]any) []map[string]any {
	var out []map[string]any
	for _, m := range metrics {
		if metricValueHoldsSeveralValues(asString(m["metric_value"])) {
			out = append(out, map[string]any{
				"candidate_id":      m["candidate_id"],
				"metric_name":       m["metric_name"],
				"metric_value":      m["metric_value"],
				"source_line_spans": m["source_line_spans"],
			})
		}
	}
	return out
}

// logMultiValueMetricRows writes one extract_metrics log entry (activity multi_value_rows)
// listing the run's rows that still hold several values, so later runs can measure the
// one-value-per-row prompt rule.
func (p *MetricsProcessor) logMultiValueMetricRows(ctx context.Context, recordID int64, metrics []map[string]any) {
	rows := multiValueMetricRows(metrics)
	if len(rows) == 0 {
		return
	}
	p.Logger.Warn("metric rows hold several values", "record_id", recordID, "num_rows", len(rows))
	artifactBytes, _ := json.Marshal(map[string]any{"multi_value_rows": rows})
	artifactStr := string(artifactBytes)
	extraBytes, _ := json.Marshal(map[string]any{"num_rows": len(rows)})
	extraStr := string(extraBytes)
	activityName := "multi_value_rows"
	rec := DocProcLogRecord{
		CallReason:    p.Name(),
		DocProcName:   p.Name(),
		ModelNames:    compactNonEmptyStrings([]string{p.RelationModelName}),
		PromptName:    p.RelationPromptRef,
		RecordID:      int64Ptr(recordID),
		ActivityName:  &activityName,
		ArtifactJSON:  &artifactStr,
		ExtraInfoJSON: &extraStr,
	}
	if err := p.ProcLogger.LogExtractMetrics(ctx, rec, "MID-26101601"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		p.Logger.Warn("failed to write multi_value_rows log", "record_id", recordID, "error", err)
	}
}
