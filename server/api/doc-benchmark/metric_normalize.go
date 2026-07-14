package docbenchmark

import (
	"sort"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const NormalizationVersion = "metric-normalization-v1"

type FieldState string

const (
	FieldAbsent FieldState = "absent"
	FieldEmpty  FieldState = "empty"
	FieldValue  FieldState = "value"
)

type NormalizedField struct {
	State   FieldState
	Text    string
	Tokens  []string
	Decimal *decimal.Decimal
}

type MetricFields struct {
	Name, Subject, Value, Unit *string
	SourceLines                []int
}

type NormalizedMetric struct {
	Name, Subject, Value, Unit NormalizedField
	SourceLines                []int
	InvalidSourceLines         []InvalidSourceLine
}

const InvalidSourceLineNonPositive = "metric.source_line.non_positive"

type InvalidSourceLine struct {
	LineNumber int
	Reason     string
}

var fold = cases.Fold()

var unitAliasesV1 = map[string]string{
	"ms": "ms", "msec": "ms", "millisecond": "ms", "milliseconds": "ms", "毫秒": "ms",
	"s": "s", "sec": "s", "second": "s", "seconds": "s", "秒": "s",
	"%": "%", "pct": "%", "percent": "%", "percentage": "%", "百分比": "%",
	"count": "count", "counts": "count", "item": "count", "items": "count", "次": "count", "个": "count",
	"byte": "byte", "bytes": "byte", "kb": "kb", "kilobyte": "kb", "kilobytes": "kb",
	"mb": "mb", "megabyte": "mb", "megabytes": "mb", "gb": "gb", "gigabyte": "gb", "gigabytes": "gb",
	"°c": "°c", "celsius": "°c", "摄氏度": "°c", "°f": "°f", "fahrenheit": "°f", "华氏度": "°f",
}

func NormalizeMetric(m MetricFields) NormalizedMetric {
	return NormalizedMetric{
		Name: NormalizeText(m.Name), Subject: NormalizeText(m.Subject),
		Value: NormalizeValue(m.Value), Unit: NormalizeUnit(m.Unit),
		SourceLines: canonicalLines(m.SourceLines), InvalidSourceLines: invalidSourceLines(m.SourceLines),
	}
}

func NormalizeText(raw *string) NormalizedField {
	if raw == nil {
		return NormalizedField{State: FieldAbsent}
	}
	if strings.TrimFunc(*raw, unicode.IsSpace) == "" {
		return NormalizedField{State: FieldEmpty}
	}
	text := normalizeCommon(*raw)
	text = strings.TrimFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	return NormalizedField{State: FieldValue, Text: text, Tokens: tokenize(text)}
}

func NormalizeValue(raw *string) NormalizedField {
	if raw == nil {
		return NormalizedField{State: FieldAbsent}
	}
	if strings.TrimFunc(*raw, unicode.IsSpace) == "" {
		return NormalizedField{State: FieldEmpty}
	}
	numericText := normalizeCommon(*raw)
	if d, err := decimal.NewFromString(numericText); err == nil {
		f := NormalizedField{State: FieldValue, Text: numericText, Tokens: tokenize(numericText)}
		f.Decimal = &d
		return f
	}
	return NormalizeText(raw)
}

func NormalizeUnit(raw *string) NormalizedField {
	if raw == nil {
		return NormalizedField{State: FieldAbsent}
	}
	if strings.TrimFunc(*raw, unicode.IsSpace) == "" {
		return NormalizedField{State: FieldEmpty}
	}
	text := normalizeCommon(*raw)
	if alias, ok := unitAliasesV1[text]; ok {
		text = alias
	}
	return NormalizedField{State: FieldValue, Text: text, Tokens: tokenize(text)}
}

func normalizeCommon(s string) string {
	s = fold.String(norm.NFKC.String(s))
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

func tokenize(s string) []string {
	set := make(map[string]struct{})
	start := -1
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			set[string(runes[start:i])] = struct{}{}
			start = -1
		}
	}
	if start >= 0 {
		set[string(runes[start:])] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for token := range set {
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

func canonicalLines(lines []int) []int {
	set := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		set[line] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for line := range set {
		out = append(out, line)
	}
	sort.Ints(out)
	return out
}

func invalidSourceLines(lines []int) []InvalidSourceLine {
	out := []InvalidSourceLine{}
	for _, line := range canonicalLines(lines) {
		if line <= 0 {
			out = append(out, InvalidSourceLine{line, InvalidSourceLineNonPositive})
		}
	}
	return out
}

func TokenJaccard(a, b NormalizedField) float64 {
	if len(a.Tokens) == 0 || len(b.Tokens) == 0 {
		return 0
	}
	common := sortedIntersectionCount(a.Tokens, b.Tokens)
	return float64(common) / float64(len(a.Tokens)+len(b.Tokens)-common)
}

func ValuesAgree(a, b NormalizedField) bool {
	if a.State != FieldValue || b.State != FieldValue {
		return false
	}
	if a.Decimal != nil && b.Decimal != nil {
		return a.Decimal.Equal(*b.Decimal)
	}
	if (a.Decimal == nil) != (b.Decimal == nil) {
		return false
	}
	return a.Text == b.Text
}

func UnitsAgree(a, b NormalizedField) bool { return exactPresentAgreement(a, b) }

func exactPresentAgreement(a, b NormalizedField) bool {
	return a.State == FieldValue && b.State == FieldValue && a.Text != "" && a.Text == b.Text
}

/*
func fieldsEqual(a, b NormalizedField) bool {
	if a.State != b.State {
		return false
	}
	if a.State != FieldValue {
		return true
	}
	return a.Text == b.Text
}
*/

func sortedIntersectionCount(a, b []string) int {
	i, j, n := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			n++
			i++
			j++
		}
	}
	return n
}
