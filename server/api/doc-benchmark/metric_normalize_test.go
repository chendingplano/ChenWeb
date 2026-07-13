package docbenchmark

import (
	"reflect"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestNormalizeMetricTextTokensAndStates(t *testing.T) {
	n := NormalizeMetric(MetricFields{
		Name:    ptr("\u00a0（ＦＯＯ  Straße， 指标１２３。）\u2003"),
		Subject: ptr("!!!"), Value: ptr("  "), Unit: nil, SourceLines: []int{7, 2, 7},
	})
	if n.Name.State != FieldValue || n.Name.Text != "foo strasse, 指标123" {
		t.Fatalf("name = %#v", n.Name)
	}
	if got, want := n.Name.Tokens, []string{"foo", "strasse", "指标123"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
	if n.Subject.State != FieldValue || len(n.Subject.Tokens) != 0 || TokenJaccard(n.Subject, n.Subject) != 0 {
		t.Fatalf("punctuation-only subject = %#v", n.Subject)
	}
	if n.Value.State != FieldEmpty || n.Unit.State != FieldAbsent {
		t.Fatalf("states value=%v unit=%v", n.Value.State, n.Unit.State)
	}
	if !reflect.DeepEqual(n.SourceLines, []int{2, 7}) {
		t.Fatalf("source lines = %#v", n.SourceLines)
	}
}

func TestNormalizeMetricDecimalAndFallback(t *testing.T) {
	a := NormalizeValue(ptr("1000000000000000000000000000000.00"))
	b := NormalizeValue(ptr("1e30"))
	if !ValuesAgree(a, b) {
		t.Fatal("exact decimal/scientific values should agree")
	}
	if ValuesAgree(NormalizeValue(ptr("1")), NormalizeValue(ptr("1.0 ms"))) {
		t.Fatal("invalid numeric text agreed")
	}
	if !ValuesAgree(NormalizeValue(ptr("  N/A! ")), NormalizeValue(ptr("n/a"))) {
		t.Fatal("text fallback did not agree")
	}
	if ValuesAgree(NormalizeValue(nil), NormalizeValue(ptr(""))) {
		t.Fatal("absent and empty agreed")
	}
	if ValuesAgree(NormalizeValue(ptr("-1")), NormalizeValue(ptr("1"))) {
		t.Fatal("decimal sign was stripped as edge punctuation")
	}
}

func TestNormalizeMetricReasonCodesInvalidSourceLines(t *testing.T) {
	n := NormalizeMetric(MetricFields{SourceLines: []int{2, 0, -1, 2}})
	if !reflect.DeepEqual(n.SourceLines, []int{-1, 0, 2}) || !reflect.DeepEqual(n.InvalidSourceLines, []InvalidSourceLine{{LineNumber: -1, Reason: InvalidSourceLineNonPositive}, {LineNumber: 0, Reason: InvalidSourceLineNonPositive}}) {
		t.Fatalf("normalized spans = %#v", n)
	}
}

func TestNormalizeMetricUnitAliases(t *testing.T) {
	aliases := map[string][]string{
		"ms":    {"ms", "MSEC", "millisecond", "milliseconds", "毫秒"},
		"s":     {"s", "sec", "second", "seconds", "秒"},
		"%":     {"%", "pct", "percent", "percentage", "百分比"},
		"count": {"count", "counts", "item", "items", "次", "个"},
		"byte":  {"byte", "bytes"}, "kb": {"kb", "kilobyte", "kilobytes"},
		"mb": {"mb", "megabyte", "megabytes"}, "gb": {"gb", "gigabyte", "gigabytes"},
		"°c": {"°Ｃ", "celsius", "摄氏度"}, "°f": {"°Ｆ", "fahrenheit", "华氏度"},
	}
	for canonical, inputs := range aliases {
		for _, input := range inputs {
			if got := NormalizeUnit(ptr(input)); got.Text != canonical || got.State != FieldValue {
				t.Errorf("unit %q = %#v, want %q", input, got, canonical)
			}
		}
	}
	if !UnitsAgree(NormalizeUnit(ptr(" widgets ")), NormalizeUnit(ptr("WIDGETS"))) {
		t.Fatal("unknown units should use normalized text")
	}
	if UnitsAgree(NormalizeUnit(ptr("s")), NormalizeUnit(ptr("ms"))) {
		t.Fatal("units must not convert")
	}
	if got := NormalizeUnit(ptr(" % ")); got.Text != "%" {
		t.Fatalf("unit punctuation stripped: %#v", got)
	}
}
