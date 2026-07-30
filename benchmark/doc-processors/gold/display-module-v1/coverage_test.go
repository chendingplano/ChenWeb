package gold

import (
	"reflect"
	"testing"
)

func TestParseLineSpansSingleAndRange(t *testing.T) {
	got := ParseLineSpans([]string{"3", "5:7", "not-a-number"})
	want := map[int]bool{3: true, 5: true, 6: true, 7: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseLineSpans = %v, want %v", got, want)
	}
}

func TestScoreCoverageDistinguishesRequiredFromBestEffort(t *testing.T) {
	f := loadFixture(t)

	// CN document: line 5 (resolution, limit_absent) is required; line 6
	// (viewing angle, qualitative) is best_effort. Cover neither.
	got := ScoreCoverage(f, "doc:cn-gb-syn-9706-1-2020", map[int]bool{})

	byLine := map[int]ClauseCoverage{}
	for _, c := range got {
		byLine[c.Line] = c
	}

	if j := byLine[5].Judgement; j != MissingRequired {
		t.Fatalf("line 5 (resolution, limit_absent) = %q, want %q", j, MissingRequired)
	}
	if byLine[5].Expectation != "required" {
		t.Fatalf("line 5 expectation = %q, want %q (default, unmarked)", byLine[5].Expectation, "required")
	}
	if j := byLine[6].Judgement; j != MissingBestEffort {
		t.Fatalf("line 6 (viewing angle, qualitative) = %q, want %q", j, MissingBestEffort)
	}
	if byLine[6].Expectation != "best_effort" {
		t.Fatalf("line 6 expectation = %q, want %q", byLine[6].Expectation, "best_effort")
	}
}

func TestScoreCoverageCapturedOverridesExpectation(t *testing.T) {
	f := loadFixture(t)
	// Cover every line: everything must report Captured regardless of
	// required/best_effort.
	all := map[int]bool{}
	for i := 1; i <= 8; i++ {
		all[i] = true
	}
	got := ScoreCoverage(f, "doc:cn-gb-syn-9706-1-2020", all)
	for _, c := range got {
		if c.Judgement != Captured {
			t.Fatalf("clause %s (line %d, expectation %s) = %q, want %q", c.ClauseID, c.Line, c.Expectation, c.Judgement, Captured)
		}
	}
}

// TestScoreCoverageAgainstBugInvestigationRuns replays the exact per-line
// presence pattern observed across the four real pipeline runs analyzed in
// bug 2026073001 (records 13, 22, 23, 24 -- see that bug's evidence table)
// and confirms the new best_effort marking changes the CN document's
// headline result: line 5 (resolution) is the only judgement that stays
// MissingRequired -- a real defect -- in every run, while the qualitative
// clauses that used to look like "missing" now correctly report
// MissingBestEffort when absent.
func TestScoreCoverageAgainstBugInvestigationRuns(t *testing.T) {
	f := loadFixture(t)
	const doc = "doc:cn-gb-syn-9706-1-2020"

	runs := map[string]map[int]bool{
		"record_13": {2: true, 6: true, 7: true, 8: true},
		"record_22": {1: true, 2: true, 3: true, 4: true, 6: true},
		"record_23": {1: true, 2: true, 3: true, 4: true, 6: true, 7: true, 8: true},
		"record_24": {1: true, 6: true, 7: true, 8: true},
	}

	for run, covered := range runs {
		t.Run(run, func(t *testing.T) {
			got := ScoreCoverage(f, doc, covered)
			for _, c := range got {
				if c.Line == 5 && c.Judgement != MissingRequired {
					t.Errorf("%s: line 5 (resolution) = %q, want %q in every run (bug 2026073001 finding F1)", run, c.Judgement, MissingRequired)
				}
			}
		})
	}

	// Cross-run check: line 5 must be MissingRequired in all four runs --
	// the bug's central, most reproducible finding, now expressed as a
	// judgement rather than a manually-read table.
	for run, covered := range runs {
		got := ScoreCoverage(f, doc, covered)
		for _, c := range got {
			if c.Line == 5 && c.Judgement == Captured {
				t.Fatalf("%s: line 5 unexpectedly captured; the bug's dataset says resolution was never extracted in any of the four runs", run)
			}
		}
	}
}
