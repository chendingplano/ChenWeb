package gold

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
)

// MetricFamilyKey identifies one (metric, authority family) pair -- the same
// granularity as a comparison-matrix column within a metric's row (ADR
// 2026072901 DR22).
type MetricFamilyKey struct{ Metric, Family string }

// VerdictRow pairs a (metric, family, object) cell key with a verdict. The
// same type serves two purposes depending on where it came from: a row in
// Resolved.Rows is gold's expected verdict; a row returned by
// SimulatedActual is a stand-in for what a run produced. Field name is
// deliberately neutral (Verdict, not Expected) so a slice from one context
// is never misread as the other.
type VerdictRow struct {
	Metric, Family, Object string
	Verdict                comparison.Verdict
}

// Resolved is everything a comparison run needs to reproduce and grade this
// gold file's expected verdicts: the expected rows themselves, the
// subject-organization constraint per metric, each family's reference
// constraint (where one exists), and which (metric, family) pairs are
// declared closed-world per DR21 rule 2.
//
// Resolved does not select a representative among several candidate
// documents in one family the way DR22's real precedence policy would; this
// fixture is authored so at most one candidate exists per (metric, family)
// pair (enforced by Resolve returning an error otherwise).
type Resolved struct {
	Rows                    []VerdictRow
	SubjectByMetric         map[string]comparison.Constraint
	ReferenceByMetricFamily map[MetricFamilyKey]comparison.Constraint
	ClosedByMetricFamily    map[MetricFamilyKey]bool
}

// SubjectDocument is the enterprise/subject-organization document every
// metric in this fixture is authored under (ADR 2026072901 DR22's
// subject-anchored comparison). A fixture spanning more than one subject
// organization would need this to be a parameter; this one doesn't.
const SubjectDocument = "doc:ent-q-syn-001-2026"

func clauseConstraint(c Clause, quantityKind string) comparison.Constraint {
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

// Resolve builds a Resolved from f. It errors on any clause or
// expected_verdict row that references an unknown metric or document, and on
// any (metric, family) pair with more than one non-enterprise reference
// clause -- representative selection among several candidates is DR22's real
// precedence policy, out of scope here, so an ambiguous fixture is a fixture
// bug, not something to guess through.
func Resolve(f File) (*Resolved, error) {
	quantityKindByMetric := map[string]string{}
	for _, m := range f.MetricDefinition {
		quantityKindByMetric[m.ID] = m.QuantityKind
	}
	familyByDocument := map[string]string{}
	for _, d := range f.AuthorityDocument {
		familyByDocument[d.ID] = d.Family
	}

	subjectByMetric := map[string]comparison.Constraint{}
	referenceByMetricFamily := map[MetricFamilyKey]comparison.Constraint{}

	for _, c := range f.Clause {
		family, ok := familyByDocument[c.Document]
		if !ok {
			return nil, fmt.Errorf("gold: clause %s references unknown document %s", c.ID, c.Document)
		}
		qk, ok := quantityKindByMetric[c.Metric]
		if !ok {
			return nil, fmt.Errorf("gold: clause %s references unknown metric %s", c.ID, c.Metric)
		}
		if c.Document == SubjectDocument {
			subjectByMetric[c.Metric] = clauseConstraint(c, qk)
			continue
		}
		if family == "enterprise" {
			continue // supersession/alias/remainder/distractor clauses: not a comparison column
		}
		key := MetricFamilyKey{c.Metric, family}
		if _, exists := referenceByMetricFamily[key]; exists {
			return nil, fmt.Errorf("gold: metric %s family %s has more than one candidate reference clause", c.Metric, family)
		}
		referenceByMetricFamily[key] = clauseConstraint(c, qk)
	}

	closedByMetricFamily := map[MetricFamilyKey]bool{}
	for _, cd := range f.ClosedDimension {
		closedByMetricFamily[MetricFamilyKey{cd.Metric, cd.Family}] = cd.Closed
	}

	rows := make([]VerdictRow, 0, len(f.ExpectedVerdict))
	for _, ev := range f.ExpectedVerdict {
		if _, ok := subjectByMetric[ev.Metric]; !ok {
			return nil, fmt.Errorf("gold: expected_verdict references metric %s with no subject-document clause", ev.Metric)
		}
		rows = append(rows, VerdictRow{
			Metric: ev.Metric, Family: ev.VsFamily, Object: ev.Object,
			Verdict: comparison.Verdict(ev.Verdict),
		})
	}

	return &Resolved{
		Rows:                    rows,
		SubjectByMetric:         subjectByMetric,
		ReferenceByMetricFamily: referenceByMetricFamily,
		ClosedByMetricFamily:    closedByMetricFamily,
	}, nil
}

// dimensionClosed reports whether (metric, family) is declared closed,
// defaulting to true per this fixture's own documented convention (see
// gold.toml's closed_dimension section): the corpus is authored exhaustively
// unless a row explicitly says otherwise.
func (r *Resolved) dimensionClosed(metric, family string) bool {
	if v, ok := r.ClosedByMetricFamily[MetricFamilyKey{metric, family}]; ok {
		return v
	}
	return true
}

// SimulatedActual computes, for every expected row, the verdict
// comparison.EvaluateFamily produces from this fixture's own clause data --
// i.e. what a perfect, already-normalized pipeline run would output. This is
// a stand-in for real pipeline output (ADR 2026072901 DR8/DR9): callers must
// not present it as a genuine extraction or normalization result. It exists
// so a corpus-level benchmark case has something concrete to score against
// Rows before extract_metrics/normalize_assertions exist.
func (r *Resolved) SimulatedActual() ([]VerdictRow, error) {
	actual := make([]VerdictRow, 0, len(r.Rows))
	for _, row := range r.Rows {
		subject := r.SubjectByMetric[row.Metric] // guaranteed present by Resolve

		evidence := comparison.FamilyEvidence{Applicable: row.Object == ""}
		if evidence.Applicable {
			if ref, ok := r.ReferenceByMetricFamily[MetricFamilyKey{row.Metric, row.Family}]; ok {
				evidence.Representative = &ref
			} else {
				evidence.DimensionClosed = r.dimensionClosed(row.Metric, row.Family)
			}
		}

		got, _, err := comparison.EvaluateFamily(subject, evidence)
		if err != nil {
			return nil, fmt.Errorf("gold: simulating %s vs %s: %w", row.Metric, row.Family, err)
		}
		actual = append(actual, VerdictRow{Metric: row.Metric, Family: row.Family, Object: row.Object, Verdict: got})
	}
	return actual, nil
}
