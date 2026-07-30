package comparison

// This file wires the checked-in synthetic gold fixture
// (benchmark/doc-processors/gold/display-module-v1/gold.toml, ADR 2026072901
// DR12/DR21) into a live regression test. It is a competency-question-style
// test (research §11.1/§13): the fixture is the source of truth, this loader
// only reshapes it into Constraint/FamilyEvidence values, and the assertion
// is that EvaluateFamily reproduces every hand-derived expected_verdict.
//
// The loader intentionally does none of the resolution work a real pipeline
// would do (representative selection among several matching documents, term
// alignment, applicability evaluation) — it uses the fixture's own
// `is_representative` / `object` / closed_dimension markers directly. That
// resolution logic belongs to DR22's cell-assembly service, not to this
// package; testing it against this same fixture is a follow-up, not part of
// the pure comparator.

import (
	"os"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/shopspring/decimal"
)

const goldFixturePath = "../../../../benchmark/doc-processors/gold/display-module-v1/gold.toml"

type goldFile struct {
	MetricDefinition  []goldMetricDefinition `toml:"metric_definition"`
	AuthorityDocument []goldDocument         `toml:"authority_document"`
	Clause            []goldClause           `toml:"clause"`
	ClosedDimension   []goldClosedDimension  `toml:"closed_dimension"`
	ExpectedVerdict   []goldExpectedVerdict  `toml:"expected_verdict"`
}

type goldMetricDefinition struct {
	ID           string `toml:"id"`
	QuantityKind string `toml:"quantity_kind"`
}

type goldDocument struct {
	ID     string `toml:"id"`
	Family string `toml:"family"`
}

type goldClause struct {
	ID         string   `toml:"id"`
	Document   string   `toml:"document"`
	Metric     string   `toml:"metric"`
	Form       string   `toml:"form"`
	Value      *float64 `toml:"value"`
	Unit       *string  `toml:"unit"`
	LowerValue *float64 `toml:"lower_value"`
	UpperValue *float64 `toml:"upper_value"`
}

type goldClosedDimension struct {
	Metric string `toml:"metric"`
	Family string `toml:"family"`
	Closed bool   `toml:"closed"`
}

type goldExpectedVerdict struct {
	Metric   string `toml:"metric"`
	VsFamily string `toml:"vs_family"`
	Verdict  string `toml:"verdict"`
	Object   string `toml:"object"`
}

func loadGoldFile(t *testing.T) goldFile {
	t.Helper()
	raw, err := os.ReadFile(goldFixturePath)
	if err != nil {
		t.Fatalf("reading gold fixture at %s: %v", goldFixturePath, err)
	}
	var g goldFile
	if err := toml.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parsing gold fixture: %v", err)
	}
	return g
}

// clauseConstraint builds a Constraint from a clause, using quantityKind
// looked up from the clause's metric_definition since the TOML clause table
// itself only carries unit/value (research §6.2 keeps property/quantity kind
// on the definition, not repeated per assertion).
func clauseConstraint(c goldClause, quantityKind string) Constraint {
	con := Constraint{QuantityKind: quantityKind, Form: ValueForm(c.Form)}
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

func TestGoldFixtureExpectedVerdicts(t *testing.T) {
	g := loadGoldFile(t)

	quantityKindByMetric := map[string]string{}
	for _, m := range g.MetricDefinition {
		quantityKindByMetric[m.ID] = m.QuantityKind
	}

	familyByDocument := map[string]string{}
	for _, d := range g.AuthorityDocument {
		familyByDocument[d.ID] = d.Family
	}

	// subjectByMetric holds the enterprise subject-organization's clause for
	// each metric: the one clause on doc:ent-q-syn-001-2026 (the current,
	// non-superseded, non-remainder enterprise edition every metric in this
	// fixture is authored under).
	const subjectDocument = "doc:ent-q-syn-001-2026"
	subjectByMetric := map[string]Constraint{}

	// referenceByMetricFamily holds, for each (metric, family) pair with
	// family != enterprise, the single reference clause the fixture supplies.
	// The fixture is constructed so at most one such clause exists per pair
	// (verified by the panic below rather than silently picking one).
	type key struct{ metric, family string }
	referenceByMetricFamily := map[key]Constraint{}

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
			if _, exists := subjectByMetric[c.Metric]; exists {
				t.Fatalf("metric %s has more than one clause on the subject document %s", c.Metric, subjectDocument)
			}
			subjectByMetric[c.Metric] = clauseConstraint(c, qk)
			continue
		}
		if family == "enterprise" {
			continue // supersession/alias/remainder/distractor clauses: not a comparison column
		}

		k := key{c.Metric, family}
		if _, exists := referenceByMetricFamily[k]; exists {
			t.Fatalf("metric %s family %s has more than one candidate reference clause (representative selection is out of this test's scope)", c.Metric, family)
		}
		referenceByMetricFamily[k] = clauseConstraint(c, qk)
	}

	closedByMetricFamily := map[key]bool{}
	for _, cd := range g.ClosedDimension {
		closedByMetricFamily[key{cd.Metric, cd.Family}] = cd.Closed
	}
	// Default per the fixture's own documented convention: closed unless
	// explicitly declared open.
	dimensionClosed := func(metric, family string) bool {
		if v, ok := closedByMetricFamily[key{metric, family}]; ok {
			return v
		}
		return true
	}

	if len(g.ExpectedVerdict) == 0 {
		t.Fatal("gold fixture has no expected_verdict rows -- fixture path or schema likely broken")
	}

	for _, ev := range g.ExpectedVerdict {
		name := ev.Metric + "/" + ev.VsFamily
		if ev.Object != "" {
			name += "/" + ev.Object
		}
		t.Run(name, func(t *testing.T) {
			subject, ok := subjectByMetric[ev.Metric]
			if !ok {
				t.Fatalf("no subject (enterprise) clause found for metric %s", ev.Metric)
			}

			evidence := FamilyEvidence{
				// Fixture-specific shorthand: this fixture's only inapplicable
				// rows are the auxiliary applicability-exception object, and
				// every one of them sets `object`. The general applicability
				// mechanism (DR20 applicability_condition evaluation) is not
				// what this test exercises.
				Applicable: ev.Object == "",
			}
			if evidence.Applicable {
				if ref, ok := referenceByMetricFamily[key{ev.Metric, ev.VsFamily}]; ok {
					evidence.Representative = &ref
				} else {
					evidence.DimensionClosed = dimensionClosed(ev.Metric, ev.VsFamily)
				}
			}

			got, rationale, err := EvaluateFamily(subject, evidence)
			if err != nil {
				t.Fatalf("EvaluateFamily returned error: %v", err)
			}
			if string(got) != ev.Verdict {
				t.Fatalf("EvaluateFamily(%s vs %s) = %q (%s), want %q",
					ev.Metric, ev.VsFamily, got, rationale, ev.Verdict)
			}
		})
	}
}
