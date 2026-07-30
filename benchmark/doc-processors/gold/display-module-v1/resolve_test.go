package gold

import "testing"

func TestResolveProducesEveryExpectedRow(t *testing.T) {
	f := loadFixture(t)
	resolved, err := Resolve(f)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.Rows) != len(f.ExpectedVerdict) {
		t.Fatalf("got %d rows, want %d (one per expected_verdict row)", len(resolved.Rows), len(f.ExpectedVerdict))
	}
	// One subject constraint per metric_definition except the near-miss
	// distractor (vent:dynamic_contrast_ratio), which by design has no clause
	// on the subject document -- it must never be comparable to anything.
	if want := len(f.MetricDefinition) - 1; len(resolved.SubjectByMetric) != want {
		t.Fatalf("got %d subject constraints, want %d (all metrics except the distractor)", len(resolved.SubjectByMetric), want)
	}
}

func TestResolveSimulatedActualMatchesGoldOnAPerfectFixture(t *testing.T) {
	f := loadFixture(t)
	resolved, err := Resolve(f)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	actual, err := resolved.SimulatedActual()
	if err != nil {
		t.Fatalf("SimulatedActual: %v", err)
	}
	if len(actual) != len(resolved.Rows) {
		t.Fatalf("got %d simulated rows, want %d", len(actual), len(resolved.Rows))
	}
	for i, want := range resolved.Rows {
		got := actual[i]
		if got.Metric != want.Metric || got.Family != want.Family || got.Object != want.Object {
			t.Fatalf("row %d key = %+v, want key from %+v", i, got, want)
		}
		if got.Verdict != want.Verdict {
			t.Fatalf("row %d (%s vs %s): SimulatedActual = %q, gold expects %q", i, want.Metric, want.Family, got.Verdict, want.Verdict)
		}
	}
}

func TestResolveRejectsUnknownMetricReference(t *testing.T) {
	f := File{
		Clause:            []Clause{{ID: "c1", Document: "d1", Metric: "does-not-exist", Form: "qualitative"}},
		AuthorityDocument: []Document{{ID: "d1", Family: "cn_national"}},
	}
	if _, err := Resolve(f); err == nil {
		t.Fatal("expected an error for a clause referencing an unknown metric, got nil")
	}
}

func TestResolveRejectsUnknownDocumentReference(t *testing.T) {
	f := File{
		MetricDefinition: []MetricDefinition{{ID: "m1", QuantityKind: "Time"}},
		Clause:           []Clause{{ID: "c1", Document: "does-not-exist", Metric: "m1", Form: "qualitative"}},
	}
	if _, err := Resolve(f); err == nil {
		t.Fatal("expected an error for a clause referencing an unknown document, got nil")
	}
}

func TestResolveRejectsAmbiguousReferenceClause(t *testing.T) {
	f := File{
		MetricDefinition: []MetricDefinition{{ID: "m1", QuantityKind: "Time"}},
		AuthorityDocument: []Document{
			{ID: "d1", Family: "cn_national"},
			{ID: "d2", Family: "cn_national"}, // same family, two candidate documents for m1
		},
		Clause: []Clause{
			{ID: "c1", Document: "d1", Metric: "m1", Form: "qualitative"},
			{ID: "c2", Document: "d2", Metric: "m1", Form: "qualitative"},
		},
	}
	if _, err := Resolve(f); err == nil {
		t.Fatal("expected an error for two candidate reference clauses in the same (metric, family), got nil")
	}
}
