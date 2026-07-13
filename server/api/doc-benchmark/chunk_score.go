package docbenchmark

import (
	"sort"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const (
	RuleChunkSequence        = "chunk.sequence_contiguous"
	RuleIneligibleNormal     = "chunk.normal.ineligible"
	RuleEligibleExactlyOnce  = "chunk.normal.exactly_once"
	RuleSourceOrder          = "chunk.normal.source_order"
	RuleOverlapFirstChunk    = "chunk.overlap.first_chunk"
	RuleOverlapPreviousChunk = "chunk.overlap.previous_chunk"
	RuleMinimumPayloadBytes  = "chunk.normal.minimum_payload_bytes"
	RuleOverlapSafetyCap     = "chunk.overlap.safety_cap"
	RuleProtectedNever       = "chunk.protected.never_split"
	RuleProtectedExpected    = "chunk.protected.expected_assignment"
)

type ScoredChunk struct {
	Sequence     int
	NormalLines  []int
	OverlapLines []int
}

type ChunkScoreInput struct {
	SourceLines           []docprocessing.Line
	ExpectedChunks        []ExpectedChunk
	ProtectedGroups       []ProtectedGroup
	ActualChunks          []ScoredChunk
	ResolvedChunkSize     int
	DesiredOverlapPercent int
	ArtifactHashes        map[string]string
}

// ScoreMetric retains a nullable scalar and additive numerator/denominator and
// confusion-matrix rows. TP/FP/FN are populated for set metrics.
type ScoreMetric struct {
	Value       *float64
	Numerator   int
	Denominator int
	TP          int
	FP          int
	FN          int
}

type ArtifactHash struct{ Name, Hash string }

type ChunkDiagnostic struct {
	ExpectedSequence []int
	ActualSequence   []int
	ExpectedBoundary []int
	ActualBoundary   []int
	MissingLines     []int
	ExtraLines       []int
	DuplicateLines   []int
	ReorderedPairs   [][2]int
	MissingOverlap   []ChunkLineAssignment
	ExtraOverlap     []ChunkLineAssignment
	RuleIDs          []string
	ArtifactHashes   []ArtifactHash
}

type ChunkLineAssignment struct{ Sequence, LineNumber int }

type ChunkScore struct {
	ExactSequenceMatch        ScoreMetric
	ExactCasePass             ScoreMetric
	BoundaryPrecision         ScoreMetric
	BoundaryRecall            ScoreMetric
	BoundaryF1                ScoreMetric
	NormalCoverage            ScoreMetric
	MissingRate               ScoreMetric
	ExtraRate                 ScoreMetric
	DuplicateRate             ScoreMetric
	ReorderedRate             ScoreMetric
	OverlapPrecision          ScoreMetric
	OverlapRecall             ScoreMetric
	OverlapF1                 ScoreMetric
	CasesWithAnyHardViolation int
	HardViolationCount        int
	RuleCounts                map[string]int
	Diagnostics               []ChunkDiagnostic
}

func ScoreChunks(in ChunkScoreInput) ChunkScore {
	s := ChunkScore{RuleCounts: make(map[string]int)}
	eligible, sourcePos, lineByNo := eligibleLines(in.SourceLines)
	expected := canonicalExpected(in.ExpectedChunks)

	exact := chunksEqual(expected, in.ActualChunks)
	s.ExactSequenceMatch = ratioMetric(boolInt(exact), 1)

	expectedBoundaries := chunkBoundariesExpected(in.ExpectedChunks, sourcePos)
	actualBoundaries := chunkBoundariesActual(in.ActualChunks, sourcePos)
	s.BoundaryPrecision, s.BoundaryRecall, s.BoundaryF1 = setScores(expectedBoundaries, actualBoundaries)

	expectedOverlap := overlapExpected(in.ExpectedChunks)
	actualOverlap := overlapActual(in.ActualChunks)
	s.OverlapPrecision, s.OverlapRecall, s.OverlapF1 = assignmentScores(expectedOverlap, actualOverlap)

	actualNormal := flattenNormal(in.ActualChunks)
	counts := make(map[int]int)
	for _, line := range actualNormal {
		counts[line]++
	}
	present := 0
	missing := make([]int, 0)
	duplicate := make([]int, 0)
	for _, line := range eligible {
		if counts[line] > 0 {
			present++
		} else {
			missing = append(missing, line)
		}
		if counts[line] > 1 {
			duplicate = append(duplicate, line)
		}
		if counts[line] != 1 {
			s.RuleCounts[RuleEligibleExactlyOnce]++
		}
	}
	if len(eligible) == 0 {
		if len(actualNormal) == 0 {
			s.NormalCoverage = ratioMetric(1, 1)
			s.MissingRate = ratioMetric(0, 1)
		} else {
			s.NormalCoverage = ratioMetric(0, 1)
			s.MissingRate = ratioMetric(1, 1)
		}
	} else {
		s.NormalCoverage = ratioMetric(present, len(eligible))
		s.MissingRate = ratioMetric(len(eligible)-present, len(eligible))
	}

	extra := make([]int, 0)
	eligibleAssignments, excess := 0, 0
	for _, line := range actualNormal {
		if _, ok := sourcePos[line]; !ok {
			extra = append(extra, line)
			s.RuleCounts[RuleIneligibleNormal]++
		} else {
			eligibleAssignments++
		}
	}
	for line, n := range counts {
		if _, ok := sourcePos[line]; ok && n > 1 {
			excess += n - 1
		}
	}
	s.ExtraRate = ratioMetric(len(extra), len(actualNormal))
	s.DuplicateRate = ratioMetric(excess, eligibleAssignments)

	reordered := make([][2]int, 0)
	for i := 1; i < len(actualNormal); i++ {
		left, lok := sourcePos[actualNormal[i-1]]
		right, rok := sourcePos[actualNormal[i]]
		if lok && rok && left > right {
			reordered = append(reordered, [2]int{actualNormal[i-1], actualNormal[i]})
			s.RuleCounts[RuleSourceOrder]++
		}
	}
	s.ReorderedRate = ratioMetric(len(reordered), maxInt(0, len(actualNormal)-1))

	for i, chunk := range in.ActualChunks {
		if chunk.Sequence != i+1 {
			s.RuleCounts[RuleChunkSequence]++
		}
		if i == 0 && len(chunk.OverlapLines) > 0 {
			s.RuleCounts[RuleOverlapFirstChunk] += len(chunk.OverlapLines)
		}
		if i > 0 {
			owned := intSet(in.ActualChunks[i-1].NormalLines)
			for _, line := range chunk.OverlapLines {
				if _, ok := owned[line]; !ok {
					s.RuleCounts[RuleOverlapPreviousChunk]++
				}
			}
		}
		if i < len(in.ActualChunks)-1 && normalBytes(chunk.NormalLines, lineByNo) < minPayloadBytes(in.ResolvedChunkSize) {
			s.RuleCounts[RuleMinimumPayloadBytes]++
		}
		if len(chunk.OverlapLines) > 1 && normalBytes(chunk.OverlapLines, lineByNo) > in.ResolvedChunkSize*20/100 {
			s.RuleCounts[RuleOverlapSafetyCap]++
		}
	}
	checkProtected(in, &s)

	ruleIDs := sortedRuleIDs(s.RuleCounts)
	for _, count := range s.RuleCounts {
		s.HardViolationCount += count
	}
	if s.HardViolationCount > 0 {
		s.CasesWithAnyHardViolation = 1
	}
	pass := exact && s.HardViolationCount == 0
	s.ExactCasePass = ratioMetric(boolInt(pass), 1)
	if !pass {
		d := ChunkDiagnostic{
			ExpectedSequence: expectedSequences(in.ExpectedChunks), ActualSequence: actualSequences(in.ActualChunks),
			ExpectedBoundary: sortedInts(expectedBoundaries), ActualBoundary: sortedInts(actualBoundaries),
			MissingLines: missing, ExtraLines: extra, DuplicateLines: duplicate, ReorderedPairs: reordered,
			MissingOverlap: assignmentDifference(expectedOverlap, actualOverlap), ExtraOverlap: assignmentDifference(actualOverlap, expectedOverlap),
			RuleIDs: ruleIDs, ArtifactHashes: sortedHashes(in.ArtifactHashes),
		}
		s.Diagnostics = []ChunkDiagnostic{d}
	}
	return s
}

func eligibleLines(lines []docprocessing.Line) ([]int, map[int]int, map[int]docprocessing.Line) {
	ids := make([]int, 0, len(lines))
	pos := make(map[int]int)
	byNo := make(map[int]docprocessing.Line)
	for _, line := range lines {
		byNo[line.LineNo] = line
		if strings.EqualFold(strings.TrimSpace(line.LineType), "toc") {
			continue
		}
		pos[line.LineNo] = len(ids)
		ids = append(ids, line.LineNo)
	}
	return ids, pos, byNo
}

func canonicalExpected(in []ExpectedChunk) []ScoredChunk {
	out := make([]ScoredChunk, len(in))
	for i, c := range in {
		out[i] = ScoredChunk{Sequence: c.Sequence, NormalLines: c.NormalLines, OverlapLines: c.OverlapLines}
	}
	return out
}
func chunksEqual(a, b []ScoredChunk) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Sequence != b[i].Sequence || !intSetsEqual(a[i].NormalLines, b[i].NormalLines) || !intSetsEqual(a[i].OverlapLines, b[i].OverlapLines) {
			return false
		}
	}
	return true
}
func intSetsEqual(a, b []int) bool {
	aSet, bSet := intSet(a), intSet(b)
	if len(aSet) != len(bSet) {
		return false
	}
	for line := range aSet {
		if _, ok := bSet[line]; !ok {
			return false
		}
	}
	return true
}
func flattenNormal(chunks []ScoredChunk) []int {
	var out []int
	for _, c := range chunks {
		out = append(out, c.NormalLines...)
	}
	return out
}
func chunkBoundariesExpected(chunks []ExpectedChunk, sourcePos map[int]int) map[int]struct{} {
	out := map[int]struct{}{}
	for i := 0; i < len(chunks)-1; i++ {
		if line, ok := finalSourceLine(chunks[i].NormalLines, sourcePos); ok {
			out[line] = struct{}{}
		}
	}
	return out
}
func chunkBoundariesActual(chunks []ScoredChunk, sourcePos map[int]int) map[int]struct{} {
	out := map[int]struct{}{}
	for i := 0; i < len(chunks)-1; i++ {
		if line, ok := finalSourceLine(chunks[i].NormalLines, sourcePos); ok {
			out[line] = struct{}{}
		}
	}
	return out
}
func finalSourceLine(lines []int, sourcePos map[int]int) (int, bool) {
	line, maxPos, found := 0, -1, false
	for _, candidate := range lines {
		if pos, ok := sourcePos[candidate]; ok && (!found || pos > maxPos) {
			line, maxPos, found = candidate, pos, true
		}
	}
	return line, found
}
func overlapExpected(chunks []ExpectedChunk) map[ChunkLineAssignment]struct{} {
	out := map[ChunkLineAssignment]struct{}{}
	for _, c := range chunks {
		for _, l := range c.OverlapLines {
			out[ChunkLineAssignment{c.Sequence, l}] = struct{}{}
		}
	}
	return out
}
func overlapActual(chunks []ScoredChunk) map[ChunkLineAssignment]struct{} {
	out := map[ChunkLineAssignment]struct{}{}
	for _, c := range chunks {
		for _, l := range c.OverlapLines {
			out[ChunkLineAssignment{c.Sequence, l}] = struct{}{}
		}
	}
	return out
}

func ratioMetric(num, den int) ScoreMetric {
	v := 0.0
	if den > 0 {
		v = float64(num) / float64(den)
	}
	return ScoreMetric{Value: &v, Numerator: num, Denominator: den}
}
func setScores(expected, actual map[int]struct{}) (ScoreMetric, ScoreMetric, ScoreMetric) {
	tp := 0
	for x := range actual {
		if _, ok := expected[x]; ok {
			tp++
		}
	}
	return confusionScores(tp, len(actual)-tp, len(expected)-tp)
}
func assignmentScores(expected, actual map[ChunkLineAssignment]struct{}) (ScoreMetric, ScoreMetric, ScoreMetric) {
	tp := 0
	for x := range actual {
		if _, ok := expected[x]; ok {
			tp++
		}
	}
	return confusionScores(tp, len(actual)-tp, len(expected)-tp)
}
func confusionScores(tp, fp, fn int) (ScoreMetric, ScoreMetric, ScoreMetric) {
	p, r := 0.0, 0.0
	switch {
	case tp+fp == 0 && tp+fn == 0:
		p, r = 1, 1
	case tp+fp == 0:
		p, r = 0, 0
	case tp+fn == 0:
		p, r = 0, 1
	default:
		p = float64(tp) / float64(tp+fp)
		r = float64(tp) / float64(tp+fn)
	}
	f := 0.0
	if p+r > 0 {
		f = 2 * p * r / (p + r)
	}
	pm := ScoreMetric{Value: &p, Numerator: tp, Denominator: tp + fp, TP: tp, FP: fp, FN: fn}
	rm := ScoreMetric{Value: &r, Numerator: tp, Denominator: tp + fn, TP: tp, FP: fp, FN: fn}
	fm := ScoreMetric{Value: &f, TP: tp, FP: fp, FN: fn}
	return pm, rm, fm
}

func checkProtected(in ChunkScoreInput, s *ChunkScore) {
	expectedAssignment := map[int]int{}
	for _, c := range in.ExpectedChunks {
		for _, l := range c.NormalLines {
			expectedAssignment[l] = c.Sequence
		}
	}
	actualAssignments := map[int][]int{}
	for _, c := range in.ActualChunks {
		for _, l := range c.NormalLines {
			actualAssignments[l] = append(actualAssignments[l], c.Sequence)
		}
	}
	for _, g := range in.ProtectedGroups {
		switch g.SplitPolicy {
		case "never":
			seq := -1
			bad := false
			for _, l := range g.Lines {
				assignments := actualAssignments[l]
				if len(assignments) != 1 {
					bad = true
					if len(assignments) == 0 {
						continue
					}
				}
				for _, a := range assignments {
					if seq < 0 {
						seq = a
					} else if a != seq {
						bad = true
					}
				}
			}
			if bad {
				s.RuleCounts[RuleProtectedNever]++
			}
		case "expected":
			bad := false
			for _, l := range g.Lines {
				assignments := actualAssignments[l]
				if len(assignments) != 1 || assignments[0] != expectedAssignment[l] {
					bad = true
				}
			}
			if bad {
				s.RuleCounts[RuleProtectedExpected]++
			}
		}
	}
}

func normalBytes(ids []int, lines map[int]docprocessing.Line) int {
	n := 0
	for _, id := range ids {
		if l, ok := lines[id]; ok {
			n += docprocessing.ChunkLineRawByteSize(l)
		}
	}
	return n
}
func minPayloadBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return (n*80 + 99) / 100
}
func intSet(in []int) map[int]struct{} {
	out := map[int]struct{}{}
	for _, x := range in {
		out[x] = struct{}{}
	}
	return out
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func sortedRuleIDs(m map[string]int) []string {
	out := []string{}
	for id, n := range m {
		if n > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func sortedInts(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Ints(out)
	return out
}
func expectedSequences(c []ExpectedChunk) []int {
	out := make([]int, len(c))
	for i, x := range c {
		out[i] = x.Sequence
	}
	return out
}
func actualSequences(c []ScoredChunk) []int {
	out := make([]int, len(c))
	for i, x := range c {
		out[i] = x.Sequence
	}
	return out
}
func assignmentDifference(a, b map[ChunkLineAssignment]struct{}) []ChunkLineAssignment {
	out := []ChunkLineAssignment{}
	for x := range a {
		if _, ok := b[x]; !ok {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].LineNumber < out[j].LineNumber
	})
	return out
}
func sortedHashes(m map[string]string) []ArtifactHash {
	out := make([]ArtifactHash, 0, len(m))
	for n, h := range m {
		out = append(out, ArtifactHash{n, h})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
