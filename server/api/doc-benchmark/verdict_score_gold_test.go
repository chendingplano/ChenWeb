package docbenchmark

// This file wires ScoreVerdictMatrix into the same checked-in synthetic gold
// fixture the comparison package tests against
// (benchmark/doc-processors/gold/display-module-v1/gold.toml, ADR 2026072901
// DR12/DR21/DR22), so the scorer's key-matching and aggregation logic is
// exercised at realistic scale (36 cells across 8 metrics and 5 families),
// not only on the small hand-built cases in verdict_score_test.go.
//
// The TOML-loading structs here intentionally duplicate a subset of
// comparison/gold_fixture_test.go's loader rather than importing it: that
// file lives in package comparison's own test binary and is not something a
// different package can import, and a production-code export solely to
// share it with this test would be a bigger, unrequested surface just to
// avoid ~50 lines of duplication in a test file.

import (
	"os"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/shopspring/decimal"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
)

const verdictGoldFixturePath = "../../../benchmark/doc-processors/gold/display-module-v1/gold.toml"

type verdictGoldFile struct {
	MetricDefinition  []verdictGoldMetricDefinition `toml:"metric_definition"`
	AuthorityDocument []verdictGoldDocument         `toml:"authority_document"`
	Clause            []verdictGoldClause           `toml:"clause"`
	ClosedDimension   []verdictGoldClosedDimension  `toml:"closed_dimension"`
	ExpectedVerdict   []verdictGoldExpectedVerdict  `toml:"expected_verdict"`
}

type verdictGoldMetricDefinition struct {
	ID           string `toml:"id"`
	QuantityKind string `toml:"quantity_kind"`
}

type verdictGoldDocument struct {
	ID     string `toml:"id"`
	Family string `toml:"family"`
}

type verdictGoldClause struct {
	ID         string   `toml:"id"`
	Document   string   `toml:"document"`
	Metric     string   `toml:"metric"`
	Form       string   `toml:"form"`
	Value      *float64 `toml:"value"`
	Unit       *string  `toml:"unit"`
	LowerValue *float64 `toml:"lower_value"`
	UpperValue *float64 `toml:"upper_value"`
}

type verdictGoldClosedDimension struct {
	Metric string `toml:"metric"`
	Family string `toml:"family"`
	Closed bool   `toml:"closed"`
}

type verdictGoldExpectedVerdict struct {
	Metric   string `toml:"metric"`
	VsFamily string `toml:"vs_family"`
	Verdict  string `toml:"verdict"`
	Object   string `toml:"object"`
}

func loadVerdictGoldFile(t *testing.T) verdictGoldFile {
	t.Helper()
	raw, err := os.ReadFile(verdictGoldFixturePath)
	if err != nil {
		t.Fatalf("reading gold fixture at %s: %v", verdictGoldFixturePath, err)
	}
	var g verdictGoldFile
	if err := toml.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parsing gold fixture: %v", err)
	}
	return g
}

func verdictGoldClauseConstraint(c verdictGoldClause, quantityKind string) comparison.Constraint {
	con := comparison.Constraint{QuantityKind: quantityKind, Form: comparison.ValueForm(c.Form)}
	if c.Unit != nil {
		con.Unit = *c.Unit
	}
	if c.Value != nil {
		con.Value = decimal.NewFromFloat(*c.Value)
	}
	if c.LowerValue != nil {
		con.LowerValue = decimal.NewFromFloat(*c.LowerValue)
	}
	if c.UpperValue != nil {
		con.UpperValue = decimal.NewFromFloat(*c.UpperValue)
	}
	return con
}

// buildGoldVerdictCells reproduces every expected_verdict row as a
// VerdictCell (the "expected" input to ScoreVerdictMatrix) and, by actually
// calling comparison.EvaluateFamily against the fixture's own clause data,
// an "actual" matrix as if a perfect pipeline had produced it. This does not
// re-test EvaluateFamily's per-row correctness (comparison's own
// gold_fixture_test.go already does that); it tests whether ScoreVerdictMatrix
// correctly matches and aggregates 36 realistically-keyed cells.
func buildGoldVerdictCells(t *testing.T) (expected, actual []VerdictCell) {
	t.Helper()
	g := loadVerdictGoldFile(t)

	quantityKindByMetric := map[string]string{}
	for _, m := range g.MetricDefinition {
		quantityKindByMetric[m.ID] = m.QuantityKind
	}
	familyByDocument := map[string]string{}
	for _, d := range g.AuthorityDocument {
		familyByDocument[d.ID] = d.Family
	}

	const subjectDocument = "doc:ent-q-syn-001-2026"
	subjectByMetric := map[string]comparison.Constraint{}
	type key struct{ metric, family string }
	referenceByMetricFamily := map[key]comparison.Constraint{}

	for _, c := range g.Clause {
		family, ok := familyByDocument[c.Document]
		if !ok {
			t.Fatalf("clause %s references unknown document %s", c.ID, c.Document)
		}
		qk, ok := quantityKindByMetric[c.Metric]
		if !ok {
			t.Fatalf("clause %s references unknown metric %s", c.ID, c.Metric)
		}
		if c.Document == subjectDocument {
			subjectByMetric[c.Metric] = verdictGoldClauseConstraint(c, qk)
			continue
		}
		if family == "enterprise" {
			continue
		}
		referenceByMetricFamily[key{c.Metric, family}] = verdictGoldClauseConstraint(c, qk)
	}

	closedByMetricFamily := map[key]bool{}
	for _, cd := range g.ClosedDimension {
		closedByMetricFamily[key{cd.Metric, cd.Family}] = cd.Closed
	}
	dimensionClosed := func(metric, family string) bool {
		if v, ok := closedByMetricFamily[key{metric, family}]; ok {
			return v
		}
		return true
	}

	for _, ev := range g.ExpectedVerdict {
		k := VerdictCellKey{Metric: ev.Metric, Family: ev.VsFamily, Object: ev.Object}
		expected = append(expected, VerdictCell{VerdictCellKey: k, Verdict: comparison.Verdict(ev.Verdict)})

		subject, ok := subjectByMetric[ev.Metric]
		if !ok {
			t.Fatalf("no subject clause found for metric %s", ev.Metric)
		}
		evidence := comparison.FamilyEvidence{Applicable: ev.Object == ""}
		if evidence.Applicable {
			if ref, ok := referenceByMetricFamily[key{ev.Metric, ev.VsFamily}]; ok {
				evidence.Representative = &ref
			} else {
				evidence.DimensionClosed = dimensionClosed(ev.Metric, ev.VsFamily)
			}
		}
		got, _, err := comparison.EvaluateFamily(subject, evidence)
		if err != nil {
			t.Fatalf("EvaluateFamily(%s vs %s): %v", ev.Metric, ev.VsFamily, err)
		}
		actual = append(actual, VerdictCell{VerdictCellKey: k, Verdict: got})
	}
	return expected, actual
}

func TestScoreVerdictMatrixAgainstGoldFixturePerfectRun(t *testing.T) {
	expected, actual := buildGoldVerdictCells(t)
	if len(expected) != 36 {
		t.Fatalf("gold fixture has %d expected_verdict rows, want 36 (update this test if the fixture changed intentionally)", len(expected))
	}

	got := ScoreVerdictMatrix(expected, actual)
	if got.TotalCells != 36 || got.MatchedCells != 36 || got.Accuracy != 1.0 {
		t.Fatalf("a perfect run over the gold fixture scored %+v, want 36/36", got)
	}
	if len(got.Mismatches) != 0 || len(got.MissingActual) != 0 || len(got.UnexpectedActual) != 0 {
		t.Fatalf("a perfect run reported diagnostics it shouldn't: mismatches=%v missing=%v unexpected=%v",
			got.Mismatches, got.MissingActual, got.UnexpectedActual)
	}
	if wantKinds := 11; len(got.ByVerdict) != wantKinds {
		t.Fatalf("got %d distinct verdict kinds in the fixture, want %d (DR21's full vocabulary)", len(got.ByVerdict), wantKinds)
	}
}

func TestScoreVerdictMatrixAgainstGoldFixtureDetectsAnInjectedRegression(t *testing.T) {
	expected, actual := buildGoldVerdictCells(t)

	// Simulate a regression: one cell's actual verdict silently flips.
	target := VerdictCellKey{Metric: "vent:alarm_response_time", Family: "us"} // gold expects `conflict`
	found := false
	for i := range actual {
		if actual[i].VerdictCellKey == target {
			if actual[i].Verdict != comparison.Conflict {
				t.Fatalf("test fixture assumption broken: expected %v to be conflict before injection, got %v", target, actual[i].Verdict)
			}
			actual[i].Verdict = comparison.Incomparable
			found = true
		}
	}
	if !found {
		t.Fatalf("could not find target cell %+v to inject a regression into", target)
	}

	got := ScoreVerdictMatrix(expected, actual)
	if got.MatchedCells != 35 || got.Accuracy >= 1.0 {
		t.Fatalf("got %+v, want exactly one cell to fail after the injected regression", got)
	}
	if len(got.Mismatches) != 1 || got.Mismatches[0].VerdictCellKey != target {
		t.Fatalf("got Mismatches=%+v, want exactly the injected cell %+v", got.Mismatches, target)
	}
}
