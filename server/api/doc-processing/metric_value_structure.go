package docprocessing

import (
	"encoding/json"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/metricvalue"
)

func normalizeMetricValueStructure(raw any) map[string]any {
	s, err := metricvalue.NormalizeStructure(raw)
	if err != nil {
		return map[string]any{"kind": "unresolved"}
	}
	return s
}

func metricValueStructureKey(m map[string]any) string {
	s := normalizeMetricValueStructure(m["value_structure"])
	if s != nil && (s["kind"] == "alternatives" || s["kind"] == "required_points") {
		values := append([]string(nil), s["values"].([]string)...)
		sort.Strings(values)
		s["values"] = values
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func metricRowNeedsValueReview(m map[string]any) bool {
	if metricvalue.ValidateForValue(m["value_structure"], asString(m["metric_value"])) != nil {
		return true
	}
	s, err := metricvalue.NormalizeStructure(m["value_structure"])
	if err != nil {
		return true
	}
	if s != nil {
		return s["kind"] == "unresolved"
	}
	return metricRowHoldsSeveralValues(m)
}

func markMetricValueUnresolved(m map[string]any) {
	m["value_structure"] = map[string]any{"kind": "unresolved"}
	addReasoningTag(m, "value_review_unresolved")
}

func normalizeMetricValueStructureForValue(raw any, display string) map[string]any {
	if metricvalue.ValidateForValue(raw, display) != nil {
		return map[string]any{"kind": "unresolved"}
	}
	return normalizeMetricValueStructure(raw)
}

func metricValueStructuresEqual(a, b map[string]any) bool {
	if !metricvalue.StructureEqual(a["value_structure"], b["value_structure"]) {
		return false
	}
	s := normalizeMetricValueStructure(a["value_structure"])
	if s != nil && (s["kind"] == "expression" || s["kind"] == "unresolved") {
		return compactMetricValue(asString(a["metric_value"])) == compactMetricValue(asString(b["metric_value"]))
	}
	return true
}
