package docbenchmark

import "github.com/chendingplano/deepdoc/server/api/ontology/comparison"

// VerdictCellKey identifies one cell in a comparison-matrix run (ADR
// 2026072901 DR22): a metric definition compared against one authority
// family, optionally scoped to a non-default target object (DR20
// applicability exceptions, e.g. an excluded product variant).
type VerdictCellKey struct {
	Metric string
	Family string
	Object string // empty for the default target object
}

// VerdictCell pairs a key with a verdict -- either the gold-expected verdict
// or the verdict a real comparison run actually produced.
type VerdictCell struct {
	VerdictCellKey
	Verdict comparison.Verdict
}

// VerdictCellMismatch is one cell where the actual run disagreed with gold.
type VerdictCellMismatch struct {
	VerdictCellKey
	Expected, Actual comparison.Verdict
}

// VerdictKindScore breaks accuracy down by expected verdict kind, so a
// regression confined to one kind (e.g. every `conflict` cell starts
// scoring as `incomparable`) stays visible even when overall accuracy looks
// fine.
type VerdictKindScore struct {
	Expected int // cells gold expects this verdict for
	Matched  int // of those, how many the actual run matched exactly
}

// VerdictMatrixScore is the outcome score for one comparison run (ADR
// 2026072901 P3, the outcome scorer the benchmark ADR §3.4 flags as not yet
// defined).
type VerdictMatrixScore struct {
	TotalCells       int
	MatchedCells     int
	Accuracy         float64
	ByVerdict        map[comparison.Verdict]VerdictKindScore
	Mismatches       []VerdictCellMismatch
	MissingActual    []VerdictCellKey // gold expects a cell; the run produced none
	UnexpectedActual []VerdictCellKey // the run produced a cell gold never expected
}

// ScoreVerdictMatrix compares an actual verdict matrix against the gold
// expected matrix, matching cells by (metric, family, object). It is a pure
// function -- no I/O, no pipeline dependency -- so a future corpus-level
// benchmark case can call it unchanged once `actual` comes from real
// pipeline output instead of from a fixture.
func ScoreVerdictMatrix(expected, actual []VerdictCell) VerdictMatrixScore {
	actualByKey := make(map[VerdictCellKey]comparison.Verdict, len(actual))
	for _, c := range actual {
		actualByKey[c.VerdictCellKey] = c.Verdict
	}
	seen := make(map[VerdictCellKey]bool, len(expected))

	score := VerdictMatrixScore{ByVerdict: map[comparison.Verdict]VerdictKindScore{}}
	for _, exp := range expected {
		seen[exp.VerdictCellKey] = true
		score.TotalCells++
		kind := score.ByVerdict[exp.Verdict]
		kind.Expected++

		got, ok := actualByKey[exp.VerdictCellKey]
		if !ok {
			score.MissingActual = append(score.MissingActual, exp.VerdictCellKey)
			score.ByVerdict[exp.Verdict] = kind
			continue
		}
		if got == exp.Verdict {
			score.MatchedCells++
			kind.Matched++
		} else {
			score.Mismatches = append(score.Mismatches, VerdictCellMismatch{
				VerdictCellKey: exp.VerdictCellKey,
				Expected:       exp.Verdict,
				Actual:         got,
			})
		}
		score.ByVerdict[exp.Verdict] = kind
	}
	for _, act := range actual {
		if !seen[act.VerdictCellKey] {
			score.UnexpectedActual = append(score.UnexpectedActual, act.VerdictCellKey)
		}
	}
	if score.TotalCells > 0 {
		score.Accuracy = float64(score.MatchedCells) / float64(score.TotalCells)
	}
	return score
}
