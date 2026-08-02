package docbenchmark

import "testing"

func TestRoutingClearanceApprovesWhenPairedRoutedRecallMatchesBaseline(t *testing.T) {
	got := EvaluateRoutingClearance(validRoutingClearanceEvidence())
	if !got.Approved || got.Reason != "approved" {
		t.Fatalf("decision=%+v, want approved", got)
	}
	if got.CasePairs != 3 || got.GoldPositiveDenom != 3 {
		t.Fatalf("counts=%+v, want 3 pairs and 3 gold positives", got)
	}
	if got.BaselineRecall == nil || got.RoutedRecall == nil || *got.BaselineRecall != *got.RoutedRecall {
		t.Fatalf("recall baseline=%v routed=%v", got.BaselineRecall, got.RoutedRecall)
	}
}

func TestRoutingClearanceRejectsManifestMismatch(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.ManifestChecksum = "sha256:other"
	assertRoutingClearanceRejects(t, e, "manifest_mismatch")
}

func TestRoutingClearanceRejectsRepetitionMismatch(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.Repetitions = 2
	assertRoutingClearanceRejects(t, e, "repetition_mismatch")
}

func TestRoutingClearanceRejectsIncompletePairedCases(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.Cases = e.Routed.Cases[:2]
	assertRoutingClearanceRejects(t, e, "incomplete_paired_cases")
}

func TestRoutingClearanceRejectsFewerThanThreeCases(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Baseline.Cases = e.Baseline.Cases[:2]
	e.Routed.Cases = e.Routed.Cases[:2]
	assertRoutingClearanceRejects(t, e, "minimum_three_cases")
}

func TestRoutingClearanceRejectsMissingGoldPositiveDenominator(t *testing.T) {
	e := validRoutingClearanceEvidence()
	for i := range e.Baseline.Cases {
		e.Baseline.Cases[i].GoldPositiveDenominator = 0
		e.Baseline.Cases[i].RecallDenominator = 0
		e.Routed.Cases[i].GoldPositiveDenominator = 0
		e.Routed.Cases[i].RecallDenominator = 0
	}
	assertRoutingClearanceRejects(t, e, "no_gold_positive_denominator")
}

func TestRoutingClearanceRejectsProcessorFailures(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.Cases[0].ProcessorFailures = 1
	assertRoutingClearanceRejects(t, e, "processor_failures")
}

func TestRoutingClearanceRejectsInfrastructureFailures(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Baseline.Cases[0].InfrastructureFailures = 1
	assertRoutingClearanceRejects(t, e, "infrastructure_failures")
}

func TestRoutingClearanceRejectsScorerFailures(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.Cases[0].ScorerFailures = 1
	assertRoutingClearanceRejects(t, e, "scorer_failures")
}

func TestRoutingClearanceRejectsRoutedRecallBelowBaseline(t *testing.T) {
	e := validRoutingClearanceEvidence()
	e.Routed.Cases[2].RecallNumerator = 0
	assertRoutingClearanceRejects(t, e, "routed_recall_below_baseline")
}

func assertRoutingClearanceRejects(t *testing.T, evidence RoutingClearanceEvidence, reason string) {
	t.Helper()
	got := EvaluateRoutingClearance(evidence)
	if got.Approved || got.Reason != reason {
		t.Fatalf("decision=%+v, want rejected reason %q", got, reason)
	}
}

func validRoutingClearanceEvidence() RoutingClearanceEvidence {
	cases := []RoutingClearanceCaseEvidence{
		{CaseID: "case-a", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1},
		{CaseID: "case-b", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1},
		{CaseID: "case-c", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1},
	}
	return RoutingClearanceEvidence{
		DocumentKind:     "enterprise-standard",
		PolicyChecksum:   "sha256:policy",
		BindingChecksums: []string{"sha256:binding"},
		GateChecksums:    []string{"sha256:gate"},
		CostYield:        map[string]float64{"routed_cost_delta": -0.2},
		DecisionTraces:   []string{"trace-a"},
		Baseline: RoutingClearanceRunEvidence{
			RunID:            "baseline",
			ManifestChecksum: "sha256:manifest",
			Repetitions:      1,
			Cases:            append([]RoutingClearanceCaseEvidence(nil), cases...),
		},
		Routed: RoutingClearanceRunEvidence{
			RunID:            "routed",
			ManifestChecksum: "sha256:manifest",
			Repetitions:      1,
			Cases:            append([]RoutingClearanceCaseEvidence(nil), cases...),
		},
	}
}
