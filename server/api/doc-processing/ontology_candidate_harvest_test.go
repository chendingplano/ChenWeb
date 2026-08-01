package docprocessing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
)

type recordingOntologyCandidateSink struct{ got []candidates.Candidate }

func (s *recordingOntologyCandidateSink) CreateCandidate(_ context.Context, c candidates.Candidate) (candidates.Candidate, error) {
	s.got = append(s.got, c)
	return c, nil
}

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

func TestHarvestMetricDefinitionsCreatesReviewCandidateForDefinedMetric(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	err := harvestMetricDefinitions(context.Background(), 42, []map[string]any{{
		"metric_name":           "Air flow rate",
		"formula_or_definition": "The volume of air delivered per unit time.",
		"value_data_type":       "number",
		"value_range_type":      "minimum",
		"source_line_spans":     []string{"17", "18"},
	}}, sink)
	if err != nil {
		t.Fatalf("harvestMetricDefinitions: %v", err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("created %d candidates, want 1", len(sink.got))
	}
	if got := string(sink.got[0].SourceLineSpans); got != `[17,18]` {
		t.Fatalf("source spans = %s", got)
	}
}

func TestMetricsProcessorHarvestsDefinitionCandidatesAfterMetricExtraction(t *testing.T) {
	sink := &recordingOntologyCandidateSink{}
	p := &MetricsProcessor{batchRecordID: 42, CandidateSink: sink}
	if err := p.harvestMetricDefinitionCandidates(context.Background(), []map[string]any{{
		"metric_name":           "Air flow rate",
		"formula_or_definition": "The volume of air delivered per unit time.",
		"source_line_spans":     []string{"17"},
	}}); err != nil {
		t.Fatalf("harvestMetricDefinitionCandidates: %v", err)
	}
	if len(sink.got) != 1 || sink.got[0].SourceRef != "input_record:42" {
		t.Fatalf("candidate sink = %#v", sink.got)
	}
}
