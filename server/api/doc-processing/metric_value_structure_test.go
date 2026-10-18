package docprocessing

import (
	"context"
	"testing"
)

func structuredPoints(kind string) map[string]any {
	return map[string]any{"kind": kind, "values": []any{"10", "20", "30"}}
}

func valueDecision(action string) []any {
	return []any{map[string]any{"candidate_id": "7_12", "action": action, "reason": "Source states one property with permitted alternatives, not separate conditions."}}
}

func TestMultiValueRecoveryAcceptsSemanticKeep(t *testing.T) {
	original := timingPointRow("7_12", "10、20、30", "mm")
	original["condition"], original["condition_en"] = "适用人群：成年人", "Population: adults"
	original["frame"] = &metricFrame{Uses: []metricFrameResource{{Role: "apparatus", Name: "量块"}}}
	kept := timingPointRow("7_12", "10、20、30", "mm")
	kept["value_structure"] = structuredPoints("alternatives")
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{kept}, "value_review_decisions": valueDecision("keep")}}
	p := makeTestMetricsProcessor(ext, 1)
	got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source",
		[]metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{original})
	if len(got) != 1 || got[0]["value_structure"] == nil || ext.structuredCalledCount != 1 {
		t.Fatalf("semantic keep not accepted: %v", got)
	}
	if len(multiValueMetricRows(got)) != 0 {
		t.Fatal("valid collection reported as extraction error")
	}
	if !metricConditionsEqual(original, got[0]) || !metricFramesEqual(original, got[0]) {
		t.Fatalf("keep lost applicability or frame: %v", got)
	}
}

func TestMultiValueRecoveryDoesNotRetryValidCollection(t *testing.T) {
	for _, kind := range []string{"alternatives", "required_points", "composite", "expression"} {
		m := timingPointRow("7_12", "10、20、30", "mm")
		m["value_structure"] = structuredPoints(kind)
		ext := &fakeJSONExtractor{}
		p := makeTestMetricsProcessor(ext, 1)
		got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source", []metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{m})
		if len(got) != 1 || ext.structuredCalledCount != 0 || len(multiValueMetricRows(got)) != 0 {
			t.Fatalf("%s incorrectly retried or warned: %v", kind, got)
		}
	}
}

func TestMultiValueRecoveryRequiresExplicitDecision(t *testing.T) {
	original := timingPointRow("7_12", "10、20、30", "mm")
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{timingPointRow("7_12", "10", "mm"), timingPointRow("7_12", "20", "mm"), timingPointRow("7_12", "30", "mm")}}}
	p := makeTestMetricsProcessor(ext, 1)
	got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source", []metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{original})
	if len(got) != 1 || got[0]["metric_value"] != "10、20、30" || got[0]["value_structure"] == nil {
		t.Fatalf("blind split accepted or unresolved structure lost: %v", got)
	}
}

func TestNormalizeMetricListPreservesStructure(t *testing.T) {
	m := timingPointRow("7_12", "10、20、30", "mm")
	m["value_structure"] = structuredPoints("alternatives")
	got := normalizeMetricList([]any{m})
	if len(got) != 1 || got[0]["value_structure"] == nil {
		t.Fatalf("lost value relationship: %v", got)
	}
}

func TestMetricMergePreservesValueRelationship(t *testing.T) {
	a := timingPointRow("7_12", "10、20、30", "mm")
	b := timingPointRow("7_12", "10、20、30", "mm")
	a["keyword_concept_id"], b["keyword_concept_id"] = "same", "same"
	a["value_structure"], b["value_structure"] = structuredPoints("alternatives"), structuredPoints("required_points")
	if metricsIdentical(a, b) || metricStaticMatch(a, b) || metricContentEqual(a, b) || len(dedupeFinalMetricRows([]map[string]any{a, b})) != 2 {
		t.Fatal("different relationships merged")
	}
}

func TestMultiValueRecoverySplitsSourceConditions(t *testing.T) {
	original := timingPointRow("7_12", "男300 mm、女250 mm", "mm")
	male, female := timingPointRow("7_12", "300", "mm"), timingPointRow("7_12", "250", "mm")
	male["condition"], female["condition"] = "性别：男", "性别：女"
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{male, female}, "value_review_decisions": []any{map[string]any{"candidate_id": "7_12", "action": "split", "reason": "Source assigns separate heights to male and female populations."}}}}
	p := makeTestMetricsProcessor(ext, 1)
	got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source", []metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{original})
	if len(got) != 2 || got[0]["condition"] != "性别：男" || got[1]["condition"] != "性别：女" {
		t.Fatalf("lost value-to-condition mapping: %v", got)
	}
}

func TestMultiValueRecoveryRejectsContradictoryCollection(t *testing.T) {
	original := timingPointRow("7_12", "10、20、30", "mm")
	kept := timingPointRow("7_12", "10、20、30", "mm")
	kept["value_structure"] = map[string]any{"kind": "alternatives", "values": []string{"10", "20"}}
	ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{kept}, "value_review_decisions": valueDecision("keep")}}
	p := makeTestMetricsProcessor(ext, 1)
	got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source", []metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{original})
	if len(got) != 1 || !metricRowNeedsValueReview(got[0]) {
		t.Fatalf("accepted incomplete structure: %v", got)
	}
}

func TestStructuredCollectionCannotAcquireIntervalEndpoints(t *testing.T) {
	m := timingPointRow("7_12", "10、20、30", "mm")
	m["value_structure"], m["value_range_type"], m["value_min"], m["value_max"] = structuredPoints("alternatives"), "range", float64(10), float64(30)
	normalizeMetricValueNotation([]map[string]any{m})
	if m["value_min"] != nil || m["value_max"] != nil {
		t.Fatalf("set turned into interval: %v", m)
	}
}
