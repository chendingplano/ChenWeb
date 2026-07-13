package docbenchmark

import (
	"reflect"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func scoreFixture() ChunkScoreInput {
	lines := []docprocessing.Line{
		{LineNo: 10, PageNo: 1, LineType: "paragraph", Content: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{LineNo: 30, PageNo: 1, LineType: "paragraph", Content: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{LineNo: 40, PageNo: 1, LineType: "paragraph", Content: "cccccccccccccccccccccccccccccc"},
	}
	return ChunkScoreInput{
		SourceLines: lines,
		ExpectedChunks: []ExpectedChunk{
			{Sequence: 1, NormalLines: []int{10, 30}},
			{Sequence: 2, OverlapLines: []int{30}, NormalLines: []int{40}},
		},
		ActualChunks: []ScoredChunk{
			{Sequence: 1, NormalLines: []int{10, 30}},
			{Sequence: 2, OverlapLines: []int{30}, NormalLines: []int{40}},
		},
		ResolvedChunkSize: 100, DesiredOverlapPercent: 90,
		ArtifactHashes: map[string]string{"input": "abc", "expected": "gold", "actual": "def"},
	}
}

func TestScoreChunksExactAndDeterministic(t *testing.T) {
	in := scoreFixture()
	got := ScoreChunks(in)
	if value(got.ExactSequenceMatch) != 1 || value(got.ExactCasePass) != 1 {
		t.Fatalf("exact fixture did not pass: %#v", got)
	}
	for name, metric := range map[string]ScoreMetric{
		"boundary": got.BoundaryF1, "coverage": got.NormalCoverage, "overlap": got.OverlapF1,
	} {
		if value(metric) != 1 {
			t.Errorf("%s = %v, want 1", name, metric.Value)
		}
	}
	if got.HardViolationCount != 0 || len(got.Diagnostics) != 0 {
		t.Fatalf("unexpected violations/diagnostics: %#v", got)
	}
	if !reflect.DeepEqual(got, ScoreChunks(in)) {
		t.Fatal("scoring is not deterministic")
	}
}

func TestScoreChunksPrimaryVectorAndAdditiveRows(t *testing.T) {
	s := ScoreChunks(scoreFixture())
	assertMetric(t, "exact sequence", s.ExactSequenceMatch, 1, 1, 1)
	assertMetric(t, "exact pass", s.ExactCasePass, 1, 1, 1)
	assertSetMetric(t, "boundary precision", s.BoundaryPrecision, 1, 1, 1, 1, 0, 0)
	assertSetMetric(t, "boundary recall", s.BoundaryRecall, 1, 1, 1, 1, 0, 0)
	assertSetMetric(t, "boundary f1", s.BoundaryF1, 1, 0, 0, 1, 0, 0)
	assertMetric(t, "coverage", s.NormalCoverage, 1, 3, 3)
	assertMetric(t, "missing", s.MissingRate, 0, 0, 3)
	assertMetric(t, "extra", s.ExtraRate, 0, 0, 3)
	assertMetric(t, "duplicate", s.DuplicateRate, 0, 0, 3)
	assertMetric(t, "reordered", s.ReorderedRate, 0, 0, 2)
	assertSetMetric(t, "overlap precision", s.OverlapPrecision, 1, 1, 1, 1, 0, 0)
	assertSetMetric(t, "overlap recall", s.OverlapRecall, 1, 1, 1, 1, 0, 0)
	assertSetMetric(t, "overlap f1", s.OverlapF1, 1, 0, 0, 1, 0, 0)
}

func TestScoreChunksMutationsDetectOnlyRelevantQualitySignals(t *testing.T) {
	base := scoreFixture()
	tests := []struct {
		name   string
		mutate func(*ChunkScoreInput)
		check  func(*testing.T, ChunkScore)
	}{
		{"shift boundary", func(in *ChunkScoreInput) {
			in.ActualChunks = []ScoredChunk{{Sequence: 1, NormalLines: []int{10}}, {Sequence: 2, OverlapLines: []int{10}, NormalLines: []int{30, 40}}}
		}, func(t *testing.T, s ChunkScore) {
			if value(s.BoundaryF1) == 1 || value(s.OverlapF1) == 1 || value(s.NormalCoverage) != 1 || value(s.DuplicateRate) != 0 || value(s.ExtraRate) != 0 {
				t.Fatalf("wrong shifted scores: %#v", s)
			}
		}},
		{"remove", func(in *ChunkScoreInput) { in.ActualChunks[0].NormalLines = []int{10} }, func(t *testing.T, s ChunkScore) {
			if value(s.MissingRate) <= 0 || s.RuleCounts[RuleEligibleExactlyOnce] == 0 || value(s.ExtraRate) != 0 || value(s.DuplicateRate) != 0 || value(s.OverlapF1) != 1 {
				t.Fatalf("wrong missing scores: %#v", s)
			}
		}},
		{"add ineligible", func(in *ChunkScoreInput) {
			in.ActualChunks[0].NormalLines = append(in.ActualChunks[0].NormalLines, 999)
		}, func(t *testing.T, s ChunkScore) {
			if value(s.ExtraRate) <= 0 || s.RuleCounts[RuleIneligibleNormal] == 0 || value(s.NormalCoverage) != 1 || value(s.BoundaryF1) != 1 || value(s.OverlapF1) != 1 {
				t.Fatalf("wrong extra scores: %#v", s)
			}
		}},
		{"duplicate", func(in *ChunkScoreInput) { in.ActualChunks[1].NormalLines = append(in.ActualChunks[1].NormalLines, 40) }, func(t *testing.T, s ChunkScore) {
			if value(s.DuplicateRate) <= 0 || s.RuleCounts[RuleEligibleExactlyOnce] == 0 || value(s.ReorderedRate) != 0 || value(s.BoundaryF1) != 1 || value(s.OverlapF1) != 1 {
				t.Fatalf("wrong duplicate scores: %#v", s)
			}
		}},
		{"reorder", func(in *ChunkScoreInput) { in.ActualChunks[0].NormalLines = []int{30, 10} }, func(t *testing.T, s ChunkScore) {
			if value(s.ReorderedRate) <= 0 || s.RuleCounts[RuleSourceOrder] == 0 || value(s.BoundaryF1) != 1 || value(s.ExactSequenceMatch) != 1 || value(s.NormalCoverage) != 1 || value(s.OverlapF1) != 1 {
				t.Fatalf("wrong reorder scores: %#v", s)
			}
		}},
		{"corrupt overlap", func(in *ChunkScoreInput) { in.ActualChunks[1].OverlapLines = []int{10} }, func(t *testing.T, s ChunkScore) {
			if value(s.OverlapF1) == 1 || value(s.NormalCoverage) != 1 || value(s.BoundaryF1) != 1 || value(s.DuplicateRate) != 0 || value(s.ReorderedRate) != 0 {
				t.Fatalf("wrong overlap scores: %#v", s)
			}
		}},
		{"sequence gap", func(in *ChunkScoreInput) { in.ActualChunks[1].Sequence = 3 }, func(t *testing.T, s ChunkScore) {
			if s.RuleCounts[RuleChunkSequence] == 0 || value(s.NormalCoverage) != 1 || value(s.BoundaryF1) != 1 || value(s.DuplicateRate) != 0 || value(s.ReorderedRate) != 0 {
				t.Fatalf("wrong gap scores: %#v", s)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			in.ActualChunks = cloneChunks(base.ActualChunks)
			tt.mutate(&in)
			tt.check(t, ScoreChunks(in))
		})
	}
}

func TestScoreChunksEmptySetRules(t *testing.T) {
	tests := []struct {
		name                                          string
		in                                            ChunkScoreInput
		exact, coverage, missing                      float64
		boundaryPrecision, boundaryRecall, boundaryF1 float64
		overlapPrecision, overlapRecall, overlapF1    float64
	}{
		{"all empty", ChunkScoreInput{}, 1, 1, 0, 1, 1, 1, 1, 1, 1},
		{"eligible actual empty", ChunkScoreInput{SourceLines: []docprocessing.Line{{LineNo: 1, LineType: "paragraph"}}, ExpectedChunks: []ExpectedChunk{{Sequence: 1, NormalLines: []int{1}}}, ResolvedChunkSize: 10}, 0, 0, 1, 1, 1, 1, 1, 1, 1},
		{"predicted boundary only", ChunkScoreInput{SourceLines: []docprocessing.Line{{LineNo: 1, LineType: "paragraph"}, {LineNo: 2, LineType: "paragraph"}}, ExpectedChunks: []ExpectedChunk{{Sequence: 1, NormalLines: []int{1, 2}}}, ActualChunks: []ScoredChunk{{Sequence: 1, NormalLines: []int{1}}, {Sequence: 2, NormalLines: []int{2}}}}, 0, 1, 0, 0, 1, 0, 1, 1, 1},
		{"expected boundary only", ChunkScoreInput{SourceLines: []docprocessing.Line{{LineNo: 1, LineType: "paragraph"}}, ExpectedChunks: []ExpectedChunk{{Sequence: 1, NormalLines: []int{1}}, {Sequence: 2}}, ActualChunks: []ScoredChunk{{Sequence: 1, NormalLines: []int{1}}}}, 0, 1, 0, 0, 0, 0, 1, 1, 1},
		{"predicted overlap only", ChunkScoreInput{ActualChunks: []ScoredChunk{{Sequence: 1}, {Sequence: 2, OverlapLines: []int{1}}}}, 0, 1, 0, 1, 1, 1, 0, 1, 0},
		{"expected overlap only", ChunkScoreInput{SourceLines: []docprocessing.Line{{LineNo: 1, LineType: "paragraph"}, {LineNo: 2, LineType: "paragraph"}}, ExpectedChunks: []ExpectedChunk{{Sequence: 1, NormalLines: []int{1}}, {Sequence: 2, OverlapLines: []int{1}, NormalLines: []int{2}}}, ActualChunks: []ScoredChunk{{Sequence: 1, NormalLines: []int{1}}, {Sequence: 2, NormalLines: []int{2}}}}, 0, 1, 0, 1, 1, 1, 0, 0, 0},
		{"no eligible with normal output", ChunkScoreInput{SourceLines: []docprocessing.Line{{LineNo: 5, LineType: "TOC"}}, ActualChunks: []ScoredChunk{{Sequence: 1, NormalLines: []int{5}}}}, 0, 0, 1, 1, 1, 1, 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScoreChunks(tt.in)
			if value(s.ExactCasePass) != tt.exact || value(s.NormalCoverage) != tt.coverage || value(s.MissingRate) != tt.missing ||
				value(s.BoundaryPrecision) != tt.boundaryPrecision || value(s.BoundaryRecall) != tt.boundaryRecall || value(s.BoundaryF1) != tt.boundaryF1 ||
				value(s.OverlapPrecision) != tt.overlapPrecision || value(s.OverlapRecall) != tt.overlapRecall || value(s.OverlapF1) != tt.overlapF1 {
				t.Fatalf("scores %#v", s)
			}
			if tt.name == "no eligible with normal output" && (s.RuleCounts[RuleIneligibleNormal] != 1 || s.CasesWithAnyHardViolation != 1) {
				t.Fatalf("missing no-eligible invariant: %#v", s)
			}
		})
	}
}

func TestScoreChunksHardRulesAndProtectedGroups(t *testing.T) {
	in := scoreFixture()
	in.SourceLines = append([]docprocessing.Line{{LineNo: 5, LineType: "ToC"}, {LineNo: 6, LineType: "UNKNOWN"}}, in.SourceLines...)
	in.ExpectedChunks[0].NormalLines = append([]int{6}, in.ExpectedChunks[0].NormalLines...)
	in.ActualChunks[0].NormalLines = append([]int{5, 6}, in.ActualChunks[0].NormalLines...)
	in.ActualChunks[0].OverlapLines = []int{10}
	in.ProtectedGroups = []ProtectedGroup{{GroupID: "list-never", SplitPolicy: "never", Lines: []int{10, 30}}, {GroupID: "list-expected", SplitPolicy: "expected", Lines: []int{30, 40}}}
	in.ActualChunks[0].NormalLines = append(in.ActualChunks[0].NormalLines, 40)
	in.ActualChunks[1].NormalLines = nil
	s := ScoreChunks(in)
	if s.RuleCounts[RuleIneligibleNormal] == 0 || s.RuleCounts[RuleOverlapFirstChunk] == 0 || s.RuleCounts[RuleProtectedNever] != 0 || s.RuleCounts[RuleProtectedExpected] == 0 {
		t.Fatalf("rule counts: %#v", s.RuleCounts)
	}
}

func TestScoreChunksOverlapOwnershipMinimumBytesAndFixedCap(t *testing.T) {
	in := scoreFixture()
	in.ResolvedChunkSize = 200
	in.ActualChunks[1].OverlapLines = []int{10, 30}
	s := ScoreChunks(in)
	if s.RuleCounts[RuleOverlapPreviousChunk] != 0 {
		t.Fatalf("valid previous ownership rejected: %#v", s.RuleCounts)
	}
	if s.RuleCounts[RuleOverlapSafetyCap] == 0 || s.RuleCounts[RuleMinimumPayloadBytes] == 0 {
		t.Fatalf("fixed rules absent: %#v", s.RuleCounts)
	}
	in.DesiredOverlapPercent = 0
	if ScoreChunks(in).RuleCounts[RuleOverlapSafetyCap] == 0 {
		t.Fatal("safety cap incorrectly depends on desired overlap")
	}
	in.ActualChunks[1].OverlapLines = []int{999}
	if ScoreChunks(in).RuleCounts[RuleOverlapPreviousChunk] == 0 {
		t.Fatal("illegal overlap ownership not detected")
	}
}

func TestScoreChunksAdjacentDuplicateIsNotReordering(t *testing.T) {
	in := scoreFixture()
	in.ActualChunks[0].NormalLines = []int{10, 30, 30}
	s := ScoreChunks(in)
	if value(s.DuplicateRate) == 0 || value(s.ReorderedRate) != 0 {
		t.Fatalf("duplicate and reorder metrics conflated: %#v", s)
	}
}

func TestScoreChunksProtectedPolicySeesDuplicateCrossChunkAssignment(t *testing.T) {
	in := scoreFixture()
	in.ProtectedGroups = []ProtectedGroup{{GroupID: "list", SplitPolicy: "never", Lines: []int{10, 30}}}
	in.ActualChunks[1].NormalLines = append(in.ActualChunks[1].NormalLines, 30)
	if got := ScoreChunks(in).RuleCounts[RuleProtectedNever]; got == 0 {
		t.Fatal("cross-chunk protected assignment was not detected")
	}
}

func TestScoreChunksProtectedListSplitMutation(t *testing.T) {
	in := scoreFixture()
	in.ExpectedChunks[1].OverlapLines = nil
	in.ActualChunks[1].OverlapLines = nil
	in.ProtectedGroups = []ProtectedGroup{{GroupID: "long-list", Kind: "list", SplitPolicy: "never", Lines: []int{10, 30}}}
	if exact := ScoreChunks(in); value(exact.ExactCasePass) != 1 || exact.RuleCounts[RuleProtectedNever] != 0 {
		t.Fatalf("valid protected fixture is not exact: %#v", exact)
	}
	in.ActualChunks[0].NormalLines = []int{10}
	in.ActualChunks[1].NormalLines = []int{30, 40}
	s := ScoreChunks(in)
	if s.RuleCounts[RuleProtectedNever] != 1 || value(s.BoundaryF1) == 1 {
		t.Fatalf("true protected-list split not detected: %#v", s)
	}
	if value(s.NormalCoverage) != 1 || value(s.MissingRate) != 0 || value(s.ExtraRate) != 0 || value(s.DuplicateRate) != 0 || value(s.ReorderedRate) != 0 || value(s.OverlapF1) != 1 {
		t.Fatalf("protected-list mutation changed unrelated metrics: %#v", s)
	}
}

func TestScoreChunksMismatchDiagnosticsContainReproductionFields(t *testing.T) {
	in := scoreFixture()
	in.ActualChunks = []ScoredChunk{
		{Sequence: 1, NormalLines: []int{10, 999}},
		{Sequence: 3, OverlapLines: []int{10}, NormalLines: []int{40, 10}},
	}
	s := ScoreChunks(in)
	if len(s.Diagnostics) != 1 {
		t.Fatalf("diagnostics count = %d, want 1", len(s.Diagnostics))
	}
	d := s.Diagnostics[0]
	if !reflect.DeepEqual(d.ExpectedSequence, []int{1, 2}) || !reflect.DeepEqual(d.ActualSequence, []int{1, 3}) ||
		!reflect.DeepEqual(d.ExpectedBoundary, []int{30}) || !reflect.DeepEqual(d.ActualBoundary, []int{10}) ||
		!reflect.DeepEqual(d.MissingLines, []int{30}) || !reflect.DeepEqual(d.ExtraLines, []int{999}) ||
		!reflect.DeepEqual(d.DuplicateLines, []int{10}) || !reflect.DeepEqual(d.ReorderedPairs, [][2]int{{40, 10}}) {
		t.Fatalf("sequence/boundary/line diagnostics incomplete: %#v", d)
	}
	if !reflect.DeepEqual(d.MissingOverlap, []ChunkLineAssignment{{Sequence: 2, LineNumber: 30}}) ||
		!reflect.DeepEqual(d.ExtraOverlap, []ChunkLineAssignment{{Sequence: 3, LineNumber: 10}}) {
		t.Fatalf("overlap diagnostics incomplete: %#v", d)
	}
	if !containsString(d.RuleIDs, RuleChunkSequence) || !containsString(d.RuleIDs, RuleEligibleExactlyOnce) || !containsString(d.RuleIDs, RuleIneligibleNormal) {
		t.Fatalf("rule diagnostics incomplete: %#v", d.RuleIDs)
	}
	wantHashes := []ArtifactHash{{Name: "actual", Hash: "def"}, {Name: "expected", Hash: "gold"}, {Name: "input", Hash: "abc"}}
	if !reflect.DeepEqual(d.ArtifactHashes, wantHashes) {
		t.Fatalf("artifact hashes = %#v, want %#v", d.ArtifactHashes, wantHashes)
	}
}

func TestScoreChunksOneChunkAndNonASCIIProductionBytes(t *testing.T) {
	line := docprocessing.Line{LineNo: 7, PageNo: 1, LineType: "UNKNOWN", Content: "中文α"}
	in := ChunkScoreInput{
		SourceLines:       []docprocessing.Line{line},
		ExpectedChunks:    []ExpectedChunk{{Sequence: 1, NormalLines: []int{7}}},
		ActualChunks:      []ScoredChunk{{Sequence: 1, NormalLines: []int{7}}},
		ResolvedChunkSize: 1,
	}
	s := ScoreChunks(in)
	if value(s.ExactCasePass) != 1 || value(s.BoundaryF1) != 1 || s.RuleCounts[RuleMinimumPayloadBytes] != 0 {
		t.Fatalf("one final unicode chunk scored incorrectly: %#v (bytes=%d)", s, docprocessing.ChunkLineRawByteSize(line))
	}
}

func value(m ScoreMetric) float64 {
	if m.Value == nil {
		return -1
	}
	return *m.Value
}
func cloneChunks(in []ScoredChunk) []ScoredChunk {
	out := make([]ScoredChunk, len(in))
	for i, c := range in {
		out[i] = ScoredChunk{Sequence: c.Sequence, NormalLines: append([]int(nil), c.NormalLines...), OverlapLines: append([]int(nil), c.OverlapLines...)}
	}
	return out
}

func assertMetric(t *testing.T, name string, got ScoreMetric, wantValue float64, wantNumerator, wantDenominator int) {
	t.Helper()
	if value(got) != wantValue || got.Numerator != wantNumerator || got.Denominator != wantDenominator {
		t.Errorf("%s = %#v, want value=%v numerator=%d denominator=%d", name, got, wantValue, wantNumerator, wantDenominator)
	}
}

func assertSetMetric(t *testing.T, name string, got ScoreMetric, wantValue float64, wantNumerator, wantDenominator, wantTP, wantFP, wantFN int) {
	t.Helper()
	if value(got) != wantValue || got.Numerator != wantNumerator || got.Denominator != wantDenominator || got.TP != wantTP || got.FP != wantFP || got.FN != wantFN {
		t.Errorf("%s = %#v, want value=%v numerator=%d denominator=%d TP/FP/FN=%d/%d/%d", name, got, wantValue, wantNumerator, wantDenominator, wantTP, wantFP, wantFN)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
