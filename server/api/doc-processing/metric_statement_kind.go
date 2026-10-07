package docprocessing

import (
	"encoding/json"
	"strings"
)

// Statement kinds of a metric row (ADR 2026100603 DR3, spec metric-statement-kind).
// The same rules exist in web/src/lib/metric-statement-kind.ts (display labels) and
// .agents/skills/extract-metrics-benchmark/scripts/benchmark_io.py (gold kind tags);
// change all three together.
const (
	statementKindRequirementWithCriterion = "requirement_with_criterion"
	statementKindRequirementValueOpen     = "requirement_value_open"
	statementKindInspectionRequirement    = "inspection_requirement"
	statementKindDelegatedRequirement     = "delegated_requirement"
	statementKindTestParameter            = "test_parameter"
	statementKindMetricDefinition         = "metric_definition"
	statementKindDefinition               = "definition"
	statementKindMetricValue              = "metric_value"
	statementKindObservation              = "observation"
	statementKindUnclassified             = "unclassified"
)

var statementNumericRangeTypes = map[string]bool{
	"lower_bound": true, "upper_bound": true, "exact": true, "range": true,
}

// metricStatementKind classifies one metric row from value_class,
// value_range_type, formula_or_definition and reasoning_tags.
func metricStatementKind(row map[string]any) string {
	valueClass := strings.ToLower(strings.TrimSpace(asString(row["value_class"])))
	// "metric-with-no-value" exists only in gold rows (testbed.metrics, benchmark rules 4.1.0+):
	// the same assertion extract_metrics stores as requirement + limit_absent.
	if valueClass == "metric-with-no-value" {
		valueClass = "requirement"
	}
	rangeType := strings.ToLower(strings.TrimSpace(asString(row["value_range_type"])))
	numeric := statementNumericRangeTypes[rangeType]
	tags := statementTags(row["reasoning_tags"])

	hasTag := func(match func(string) bool) bool {
		for _, t := range tags {
			if match(t) {
				return true
			}
		}
		return false
	}

	if hasTag(func(t string) bool { return t == "test_condition" }) {
		return statementKindTestParameter
	}
	if valueClass == "definition" {
		if strings.TrimSpace(asString(row["formula_or_definition"])) != "" {
			return statementKindMetricDefinition
		}
		return statementKindDefinition
	}
	cites := hasTag(func(t string) bool { return t == "external_reference" || strings.HasPrefix(t, "cited_doc:") })
	if valueClass == "reference" || cites {
		if numeric {
			return statementKindMetricValue
		}
		return statementKindDelegatedRequirement
	}
	if valueClass == "requirement" || valueClass == "target" {
		if numeric {
			return statementKindRequirementWithCriterion
		}
		if rangeType == "limit_absent" {
			return statementKindRequirementValueOpen
		}
		return statementKindInspectionRequirement
	}
	if valueClass == "observation" || valueClass == "design_capability" {
		if numeric {
			return statementKindMetricValue
		}
		return statementKindObservation
	}
	return statementKindUnclassified
}

// isPureRequirementKind reports whether a kind is a requirement with nothing
// to measure. extract_metrics does not store these (spec
// metric-pure-requirement-exclusion).
func isPureRequirementKind(kind string) bool {
	return kind == statementKindInspectionRequirement || kind == statementKindDelegatedRequirement
}

// statementTags accepts tags as []string, []any or a JSON array string; anything
// else yields no tags, as in the TS classifier.
func statementTags(raw any) []string {
	var items []string
	switch x := raw.(type) {
	case []string:
		items = x
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok {
				items = append(items, s)
			}
		}
	case string:
		if err := json.Unmarshal([]byte(x), &items); err != nil {
			return nil
		}
	}
	out := make([]string, 0, len(items))
	for _, s := range items {
		out = append(out, strings.ToLower(strings.TrimSpace(s)))
	}
	return out
}
