package docprocessing

import (
	"encoding/json"
	"testing"
)

func TestBuildMetricDefinitionCandidatePreservesProvenance(t *testing.T) {
	candidate, err := buildMetricDefinitionCandidate(42, metricDefinitionMention{
		CanonicalName: "Air flow rate",
		Aliases:       []string{"flow rate"},
		Definition:    "The volume of air delivered per unit time.",
		ValueType:     "number",
		RangeType:     "minimum",
		Confidence:    0.91,
		LineNumbers:   []int{17, 18},
	})
	if err != nil {
		t.Fatalf("buildMetricDefinitionCandidate: %v", err)
	}
	if candidate.CandidateKind != "term" || candidate.ProposedModuleID != "measurement" || candidate.SourceRef != "input_record:42" {
		t.Fatalf("candidate metadata = %#v", candidate)
	}
	if got := string(candidate.SourceLineSpans); got != `[17,18]` {
		t.Fatalf("source spans = %s", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.ProposedPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["term_id"] != "measurement:air_flow_rate" || payload["term_kind"] != "metric_definition" {
		t.Fatalf("payload identity = %#v", payload)
	}
	if got := payload["aliases"].([]any); len(got) != 1 || got[0] != "flow rate" {
		t.Fatalf("aliases = %#v", got)
	}
}
