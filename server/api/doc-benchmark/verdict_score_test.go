package docbenchmark

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
)

func cell(metric, family string, v comparison.Verdict) VerdictCell {
	return VerdictCell{VerdictCellKey: VerdictCellKey{Metric: metric, Family: family}, Verdict: v}
}

func TestScoreVerdictMatrixPerfectMatch(t *testing.T) {
	expected := []VerdictCell{
		cell("m1", "cn", comparison.Identical),
		cell("m1", "us", comparison.Conflict),
	}
	actual := []VerdictCell{
		cell("m1", "cn", comparison.Identical),
		cell("m1", "us", comparison.Conflict),
	}
	got := ScoreVerdictMatrix(expected, actual)
	if got.TotalCells != 2 || got.MatchedCells != 2 || got.Accuracy != 1.0 {
		t.Fatalf("got %+v, want a perfect 2/2 match", got)
	}
	if len(got.Mismatches) != 0 || len(got.MissingActual) != 0 || len(got.UnexpectedActual) != 0 {
		t.Fatalf("got %+v, want no mismatches/missing/unexpected", got)
	}
	if got.ByVerdict[comparison.Identical] != (VerdictKindScore{Expected: 1, Matched: 1}) {
		t.Fatalf("ByVerdict[identical] = %+v, want {1,1}", got.ByVerdict[comparison.Identical])
	}
}

func TestScoreVerdictMatrixDetectsMismatch(t *testing.T) {
	expected := []VerdictCell{cell("m1", "cn", comparison.Stronger)}
	actual := []VerdictCell{cell("m1", "cn", comparison.Incomparable)}

	got := ScoreVerdictMatrix(expected, actual)
	if got.TotalCells != 1 || got.MatchedCells != 0 || got.Accuracy != 0 {
		t.Fatalf("got %+v, want 0/1 match", got)
	}
	if len(got.Mismatches) != 1 {
		t.Fatalf("got %d mismatches, want 1", len(got.Mismatches))
	}
	m := got.Mismatches[0]
	if m.Metric != "m1" || m.Family != "cn" || m.Expected != comparison.Stronger || m.Actual != comparison.Incomparable {
		t.Fatalf("mismatch = %+v, want m1/cn stronger->incomparable", m)
	}
	if kind := got.ByVerdict[comparison.Stronger]; kind.Expected != 1 || kind.Matched != 0 {
		t.Fatalf("ByVerdict[stronger] = %+v, want {1,0}", kind)
	}
}

func TestScoreVerdictMatrixMissingActual(t *testing.T) {
	expected := []VerdictCell{cell("m1", "cn", comparison.Identical)}
	got := ScoreVerdictMatrix(expected, nil)
	if got.TotalCells != 1 || got.MatchedCells != 0 {
		t.Fatalf("got %+v, want 1 expected cell with no match", got)
	}
	if len(got.MissingActual) != 1 || got.MissingActual[0].Metric != "m1" {
		t.Fatalf("got MissingActual=%+v, want one entry for m1/cn", got.MissingActual)
	}
	if len(got.Mismatches) != 0 {
		t.Fatalf("a missing cell must not also be reported as a mismatch, got %+v", got.Mismatches)
	}
}

func TestScoreVerdictMatrixUnexpectedActualDoesNotAffectAccuracy(t *testing.T) {
	expected := []VerdictCell{cell("m1", "cn", comparison.Identical)}
	actual := []VerdictCell{
		cell("m1", "cn", comparison.Identical),
		cell("m2", "us", comparison.Conflict), // gold never asked about this cell
	}
	got := ScoreVerdictMatrix(expected, actual)
	if got.TotalCells != 1 || got.MatchedCells != 1 || got.Accuracy != 1.0 {
		t.Fatalf("an extra actual cell must not change the accuracy denominator, got %+v", got)
	}
	if len(got.UnexpectedActual) != 1 || got.UnexpectedActual[0].Metric != "m2" {
		t.Fatalf("got UnexpectedActual=%+v, want one entry for m2/us", got.UnexpectedActual)
	}
}

func TestScoreVerdictMatrixObjectDistinguishesCells(t *testing.T) {
	// Same metric and family, different target object (DR20 applicability
	// exception) -- these must be scored as two independent cells, not one.
	expected := []VerdictCell{
		{VerdictCellKey: VerdictCellKey{Metric: "m1", Family: "cn"}, Verdict: comparison.QualitativeOnly},
		{VerdictCellKey: VerdictCellKey{Metric: "m1", Family: "cn", Object: "variant"}, Verdict: comparison.NotApplicable},
	}
	actual := expected // a perfect run
	got := ScoreVerdictMatrix(expected, actual)
	if got.TotalCells != 2 || got.MatchedCells != 2 {
		t.Fatalf("got %+v, want both the base and variant-object cells scored independently", got)
	}
}

func TestScoreVerdictMatrixEmptyInputsAreZeroNotNaN(t *testing.T) {
	got := ScoreVerdictMatrix(nil, nil)
	if got.TotalCells != 0 || got.Accuracy != 0 {
		t.Fatalf("got %+v, want a zero-cell score with Accuracy 0 (not NaN)", got)
	}
}
