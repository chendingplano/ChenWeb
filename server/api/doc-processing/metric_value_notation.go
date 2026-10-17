package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/width"
)

// Deterministic Pass 2 value normalization and condition identity (openspec change
// extract-metrics-conditions, design D2/D3). Notation only: nothing here needs judgement about
// the document, so the rules hold for every domain and language.

// metricConditionKey normalizes a row's condition for identity comparisons: full-width forms
// folded, case and whitespace ignored. Qualifier order and repetition do not change identity;
// labels stay in the key so unrelated qualifiers with the same value do not become equal.
func metricConditionKey(m map[string]any) string {
	c := width.Fold.String(strings.TrimSpace(asString(m["condition"])))
	c = strings.ToLower(strings.Join(strings.Fields(c), ""))
	parts := compactNonEmptyStrings(strings.Split(strings.ReplaceAll(c, ";", "；"), "；"))
	slices.Sort(parts)
	return strings.Join(parts, "；")
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

// metricClauseNumberRe matches the clause number that starts a numbered clause or heading line:
// "6.5.7 肺活量测试仪(计)", "6.5.4.2 对于…", "B.2.3 横梁弹性检验…".
var metricClauseNumberRe = regexp.MustCompile(`^(?:[A-Z]\.\d+(?:\.\d+)*|\d+(?:\.\d+)+)(?:\s|$)`)

// metricInheritedConditionLabels are the clause-wide qualifiers inheritClauseConditions copies.
var metricInheritedConditionLabels = []string{"试验装置", "试验方法", "试验介质"}

// inheritClauseConditions copies a clause's apparatus, method and medium qualifiers to the rows of
// the same clause that lack them. A clause names its apparatus once ("应采用…气体流量标准装置按下列条件进行
// 检验", then a) 环境温度 … ), and the model often writes it only on the rows of the line that names it,
// or only in some of the clause's enrich batches (371, gold 20261016_162743). A row's clause is the
// nearest numbered line at or above its first source line; rows from table lines are left alone,
// because one table line holds rows with different methods. A label is copied only when the
// clause's rows agree on one value for it, so two methods in one clause stay as the model wrote them.
// The value must also appear in the clause's lead (its numbered line, or the line after a numbered
// heading): a qualifier stated for the whole clause is stated there, while one written only in a
// later list item belongs to that item (371 5.4.3 lists several kinds of equipment, each with its
// own load and method). It returns the number of rows changed.
func inheritClauseConditions(metrics []map[string]any, chunks []Chunk) int {
	lines := map[int]Line{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			lines[ml.Line.LineNo] = ml.Line
		}
	}
	nums := make([]int, 0, len(lines))
	for n := range lines {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	// A clause's lead is its numbered line, plus the next line when the numbered line is a heading
	// ("6.5.7 肺活量测试仪(计)" then "应采用…气体流量标准装置按下列条件进行检验").
	clauseOf, lead, current, headingOpen := make(map[int]string, len(nums)), map[string]string{}, "", false
	for _, n := range nums {
		l := lines[n]
		if l.LineType != "table" {
			if num := metricClauseNumberRe.FindString(strings.TrimSpace(l.Content)); num != "" {
				current = strings.TrimSpace(num)
				lead[current] = metricConditionCompact(l.Content)
				headingOpen = strings.HasPrefix(l.LineType, "heading")
			} else if headingOpen && current != "" {
				lead[current] += metricConditionCompact(l.Content)
				headingOpen = false
			}
		}
		clauseOf[n] = current
	}

	groups := map[string][]int{}
	for i, m := range metrics {
		if key := metricRowClause(m, lines, clauseOf); key != "" {
			groups[key] = append(groups[key], i)
		}
	}
	changed := map[int]bool{}
	for clause, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		for _, label := range metricInheritedConditionLabels {
			var part, partEn string
			values := map[string]bool{}
			for _, i := range idx {
				if q, en, ok := metricConditionPart(metrics[i], label); ok {
					values[metricConditionCompact(q)] = true
					part, partEn = q, en
				}
			}
			value := metricConditionCompact(strings.TrimPrefix(part, metricConditionLabelRe.FindString(part)))
			if len(values) != 1 || value == "" || !strings.Contains(lead[clause], value) {
				continue
			}
			for _, i := range idx {
				if _, _, ok := metricConditionPart(metrics[i], label); ok || metricConditionOverlaps(metrics[i], part) {
					continue
				}
				m := metrics[i]
				cond := strings.TrimSpace(asString(m["condition"]))
				condEn := strings.TrimSpace(asString(m["condition_en"]))
				aligned := metricConditionCount(cond, "；") == metricConditionCount(condEn, ";")
				m["condition"] = strings.TrimPrefix(cond+"；"+part, "；")
				if partEn != "" && aligned {
					m["condition_en"] = strings.TrimPrefix(condEn+"; "+partEn, "; ")
				}
				changed[i] = true
			}
		}
	}
	return len(changed)
}

// metricServedQuantitySuffixes name the rows that serve another quantity of the same cell: its
// permitted error and its resolution.
var metricServedQuantitySuffixes = []string{"允许误差", "分度值"}

// inheritServedQuantityConditions copies to a permitted-error or resolution row ("底座尺寸允许误差")
// the qualifiers it lacks from the row of the quantity it serves ("底座尺寸"): the same subject, the
// name without the suffix, and an overlapping source line. A table cell "底座尺寸为长300 mm×宽 300 mm
// 的方形或直径为 300 mm 的圆形 | — | ±5 mm" gives 底座尺寸 with 形状：方形底座, and the ±5 mm holds for
// that square base; the model wrote the shape on the size row but dropped it from the tolerance
// row in two of three runs (371, 2026-10-17). A qualifier is copied only when exactly one served
// row matches and the row has no qualifier with that label or an overlapping value. It returns the
// number of rows changed.
func inheritServedQuantityConditions(metrics []map[string]any) int {
	changed := 0
	for _, m := range metrics {
		name := metricConditionCompact(asString(m["metric_name"]))
		base := ""
		for _, suffix := range metricServedQuantitySuffixes {
			if strings.HasSuffix(name, suffix) {
				base = strings.TrimSuffix(strings.TrimSuffix(name, suffix), "的")
				break
			}
		}
		if base == "" {
			continue
		}
		subject := metricRowSubject(m)
		var served map[string]any
		matches := 0
		for _, o := range metrics {
			if metricConditionCompact(asString(o["metric_name"])) != base ||
				metricRowSubject(o) != subject || !metricLineSpansOverlap(m, o) {
				continue
			}
			served = o
			matches++
		}
		if matches != 1 {
			continue
		}
		parts := strings.Split(strings.ReplaceAll(asString(served["condition"]), ";", "；"), "；")
		enParts := strings.Split(asString(served["condition_en"]), ";")
		rowChanged := false
		for i, q := range parts {
			q = strings.TrimSpace(q)
			if q == "" {
				continue
			}
			label := strings.TrimRight(width.Fold.String(metricConditionLabelRe.FindString(q)), ":")
			if label != "" {
				if _, _, ok := metricConditionPart(m, label); ok {
					continue
				}
			}
			if metricConditionOverlaps(m, q) {
				continue
			}
			cond := strings.TrimSpace(asString(m["condition"]))
			condEn := strings.TrimSpace(asString(m["condition_en"]))
			aligned := metricConditionCount(cond, "；") == metricConditionCount(condEn, ";")
			m["condition"] = strings.TrimPrefix(cond+"；"+q, "；")
			if len(enParts) == len(parts) && aligned {
				m["condition_en"] = strings.TrimPrefix(condEn+"; "+strings.TrimSpace(enParts[i]), "; ")
			}
			rowChanged = true
		}
		if rowChanged {
			changed++
		}
	}
	return changed
}

// metricRowSubject returns the row's compacted subject under either the Pass 2 or the kb.metrics key.
func metricRowSubject(m map[string]any) string {
	return metricConditionCompact(firstNonEmptyTrimmed(asString(m["metric_subject"]), asString(m["subject"])))
}

// metricRowClause returns the clause of a row's source lines, or "" when a line is a table line,
// unknown, or the lines belong to different clauses.
func metricRowClause(m map[string]any, lines map[int]Line, clauseOf map[int]string) string {
	clause := ""
	for _, span := range toStringSlice(m["source_line_spans"]) {
		start, end := parseLineSpanRange(span)
		if start == 0 {
			return ""
		}
		for n := start; n <= end; n++ {
			l, ok := lines[n]
			if !ok || l.LineType == "table" || clauseOf[n] == "" {
				return ""
			}
			if clause != "" && clause != clauseOf[n] {
				return ""
			}
			clause = clauseOf[n]
		}
	}
	return clause
}

// metricConditionPart returns the row's qualifier with the given label and, when condition_en
// has the same number of qualifiers, its English counterpart.
func metricConditionPart(m map[string]any, label string) (string, string, bool) {
	cond := strings.TrimSpace(asString(m["condition"]))
	if cond == "" {
		return "", "", false
	}
	parts := strings.Split(strings.ReplaceAll(cond, ";", "；"), "；")
	enParts := strings.Split(asString(m["condition_en"]), ";")
	for i, q := range parts {
		q = strings.TrimSpace(q)
		l := metricConditionLabelRe.FindString(q)
		if strings.TrimRight(width.Fold.String(l), ":") != label || strings.TrimSpace(strings.TrimPrefix(q, l)) == "" {
			continue
		}
		if len(enParts) == len(parts) {
			return q, strings.TrimSpace(enParts[i]), true
		}
		return q, "", true
	}
	return "", "", false
}

// metricConditionOverlaps reports whether the row already has a qualifier whose value contains, or
// is contained in, the value of part: the same apparatus under another label (试验装置：脉冲信号发生器 vs
// 试验方法：脉冲信号发生器法).
func metricConditionOverlaps(m map[string]any, part string) bool {
	v := metricConditionCompact(strings.TrimPrefix(part, metricConditionLabelRe.FindString(part)))
	if v == "" {
		return false
	}
	for _, q := range strings.Split(strings.ReplaceAll(asString(m["condition"]), ";", "；"), "；") {
		q = strings.TrimSpace(q)
		w := metricConditionCompact(strings.TrimPrefix(q, metricConditionLabelRe.FindString(q)))
		if w != "" && (strings.Contains(w, v) || strings.Contains(v, w)) {
			return true
		}
	}
	return false
}

// metricConditionCount counts the non-empty qualifiers of a condition string.
func metricConditionCount(s, sep string) int {
	return len(compactNonEmptyStrings(strings.Split(strings.ReplaceAll(s, ";", sep), sep)))
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
		// A centre with a deviation states both endpoints, unlike a bare ±x error limit.
		// Repair a numeric comparator supplied by the model before deriving the endpoints.
		if metricCentreRe.MatchString(compact) && (rangeType == "exact" || rangeType == "lower_bound" || rangeType == "upper_bound") {
			rangeType = "range"
			m["value_min"], m["value_max"] = nil, nil
		}
		if rangeType == "range" && (toOptionalFloat(m["value_min"]) == nil || toOptionalFloat(m["value_max"]) == nil) {
			if lo, hi, ok := metricRangeEndpoints(compact); ok {
				m["value_min"], m["value_max"] = lo, hi
			}
		}
		if rangeType != "range" {
			m["value_min"], m["value_max"] = nil, nil
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
