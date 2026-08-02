package docbenchmark

import "testing"

func TestBuildRoutingRunEvidenceUsesExplicitDocumentKindAndRecallMetric(t *testing.T) {
	recall := ScoreRow{Metric: "review_recall", Numerator: 2, Denominator: 3}
	units := []ScoreUnit{
		{CaseID: "included", Repetition: 1, Tags: []string{"document_kind:Standard"}, Scores: []ScoreRow{recall}},
		{CaseID: "alias", Repetition: 2, Tags: []string{"doc_kind:standard"}, Scores: []ScoreRow{{Metric: "grounding_recall", Numerator: 1, Denominator: 1}}},
		{CaseID: "excluded", Repetition: 1, Tags: []string{"document_kind:invoice"}, Scores: []ScoreRow{recall}},
	}
	evidence := buildRoutingRunEvidence("run-1", "sha256:manifest", units, map[string]int{"processor_failed": 2}, " standard ")
	if evidence.RunID != "run-1" || evidence.ManifestChecksum != "sha256:manifest" || evidence.Repetitions != 2 {
		t.Fatalf("evidence=%+v", evidence)
	}
	if len(evidence.Cases) != 2 || evidence.Cases[0].CaseID != "included" || evidence.Cases[1].CaseID != "alias" {
		t.Fatalf("cases=%+v", evidence.Cases)
	}
	if evidence.Cases[0].RecallNumerator != 2 || evidence.Cases[0].RecallDenominator != 3 || evidence.Cases[0].GoldPositiveDenominator != 3 {
		t.Fatalf("recall=%+v", evidence.Cases[0])
	}
	if evidence.Cases[0].InfrastructureFailures != 2 {
		t.Fatalf("failures=%+v", evidence.Cases[0])
	}
}

func TestBuildRoutingRunEvidenceMarksMissingRecallAsScorerFailure(t *testing.T) {
	evidence := buildRoutingRunEvidence("run", "manifest", []ScoreUnit{{
		CaseID: "case", Repetition: 1, Tags: []string{"document_kind:standard"},
		Scores: []ScoreRow{{Metric: "precision", Numerator: 1, Denominator: 1}},
	}}, nil, "standard")
	if len(evidence.Cases) != 1 || evidence.Cases[0].ScorerFailures != 1 {
		t.Fatalf("evidence=%+v", evidence)
	}
}
