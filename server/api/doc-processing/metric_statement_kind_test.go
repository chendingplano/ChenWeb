package docprocessing

import "testing"

// Scenarios mirror web/src/lib/metric-statement-kind.test.ts.
func TestMetricStatementKind(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]any
		want string
	}{
		{"qualitative requirement is an inspection requirement", map[string]any{"value_class": "requirement", "value_range_type": "qualitative"}, statementKindInspectionRequirement},
		{"numeric requirement keeps its criterion", map[string]any{"value_class": "requirement", "value_range_type": "upper_bound"}, statementKindRequirementWithCriterion},
		{"target counts as a requirement", map[string]any{"value_class": "target", "value_range_type": "exact"}, statementKindRequirementWithCriterion},
		{"limit_absent leaves the value open", map[string]any{"value_class": "requirement", "value_range_type": "limit_absent"}, statementKindRequirementValueOpen},
		{"gold metric with no value", map[string]any{"value_class": "metric-with-no-value", "value_range_type": "limit_absent"}, statementKindRequirementValueOpen},
		{"delegated requirement", map[string]any{"value_class": "reference", "value_range_type": "qualitative", "reasoning_tags": []string{"cited_doc:CJJ 52"}}, statementKindDelegatedRequirement},
		{"citation tag alone delegates", map[string]any{"value_class": "requirement", "value_range_type": "qualitative", "reasoning_tags": []any{"external_reference"}}, statementKindDelegatedRequirement},
		{"test setting wins over requirement", map[string]any{"value_class": "requirement", "value_range_type": "exact", "reasoning_tags": []string{"test_condition"}}, statementKindTestParameter},
		{"formula definition", map[string]any{"value_class": "definition", "formula_or_definition": "GI = a / b"}, statementKindMetricDefinition},
		{"term definition", map[string]any{"value_class": "definition", "formula_or_definition": "  "}, statementKindDefinition},
		{"observation value", map[string]any{"value_class": "observation", "value_range_type": "exact"}, statementKindMetricValue},
		{"qualitative design capability", map[string]any{"value_class": "design_capability", "value_range_type": "qualitative"}, statementKindObservation},
		{"empty row", map[string]any{}, statementKindUnclassified},
		{"unknown class", map[string]any{"value_class": "whatever", "value_range_type": "exact"}, statementKindUnclassified},
		{"tags as JSON string", map[string]any{"value_class": "requirement", "value_range_type": "exact", "reasoning_tags": `["test_condition"]`}, statementKindTestParameter},
		{"malformed tag string", map[string]any{"value_class": "requirement", "value_range_type": "exact", "reasoning_tags": "not json"}, statementKindRequirementWithCriterion},
		{"trimmed and case-insensitive", map[string]any{"value_class": " Requirement ", "value_range_type": "QUALITATIVE"}, statementKindInspectionRequirement},
	}
	for _, c := range cases {
		if got := metricStatementKind(c.row); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// Record 416 gold benchmark (skill 2.0.0, gpt-6.1-sol, run 20261006_121911), the same
// 69 rows as the TS test: value_class, value_range_type, tags, has formula.
func TestMetricStatementKindRecord416(t *testing.T) {
	rows := []struct {
		valueClass, rangeType string
		tags                  []string
		hasFormula            bool
	}{
		{"requirement", "qualitative", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 27"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 27", "cited_doc:GB 16889"}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "lower_bound", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "exact", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 184"}, false},
		{"requirement", "exact", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 52"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 52"}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:NY/T 90"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:NY/T 2371"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB 50869"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB 16889"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:CJJ 90"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB 18485"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB/T 31962"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB 16889"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:GB 14554"}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "qualitative", []string{}, false},
		{"requirement", "limit_absent", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:NY 1109"}, false},
		{"requirement", "lower_bound", []string{}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:NY884"}, false},
		{"reference", "qualitative", []string{"external_reference", "cited_doc:NY884"}, false},
		{"requirement", "lower_bound", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "range", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "upper_bound", []string{}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "lower_bound", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "lower_bound", []string{"test_condition"}, false},
		{"requirement", "lower_bound", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "range", []string{"test_condition"}, false},
		{"requirement", "upper_bound", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"requirement", "exact", []string{"test_condition"}, false},
		{"definition", "qualitative", []string{}, true},
		{"reference", "upper_bound", []string{"strict_bound"}, false},
		{"reference", "lower_bound", []string{"strict_bound"}, false},
	}
	counts := map[string]int{}
	pure := 0
	for _, r := range rows {
		formula := ""
		if r.hasFormula {
			formula = "formula"
		}
		kind := metricStatementKind(map[string]any{
			"value_class": r.valueClass, "value_range_type": r.rangeType,
			"reasoning_tags": r.tags, "formula_or_definition": formula,
		})
		counts[kind]++
		if isPureRequirementKind(kind) {
			pure++
		}
	}
	want := map[string]int{
		statementKindInspectionRequirement:    15,
		statementKindDelegatedRequirement:     17,
		statementKindRequirementWithCriterion: 12,
		statementKindRequirementValueOpen:     7,
		statementKindTestParameter:            15,
		statementKindMetricDefinition:         1,
		statementKindMetricValue:              2,
	}
	if len(counts) != len(want) {
		t.Fatalf("kinds = %v, want %v", counts, want)
	}
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("%s: got %d, want %d", k, counts[k], n)
		}
	}
	if pure != 32 {
		t.Errorf("pure requirements = %d, want 32", pure)
	}
}
