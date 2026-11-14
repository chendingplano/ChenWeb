package complianceeval

import (
	"strings"
	"testing"
)

func set(ids ...int64) map[int64]bool {
	m := map[int64]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestScoreMatchesRecallAndNoGrounding(t *testing.T) {
	gold := []GoldAct{
		{Record: 1, LineStart: 10, LineEnd: 12, Governs: set(100, 101)},                                               // covered: reviewer reports 100 only
		{Record: 1, LineStart: 20, LineEnd: 22, Governs: set(), Labels: map[string]string{"chunk/k777": "unrelated"}}, // uncovered: reviewer reports a judged passage
		{Record: 1, LineStart: 30, LineEnd: 31, Governs: set(200)},                                                    // reviewer says no grounding (missed by search)
		{Record: 1, LineStart: 40, LineEnd: 41, Governs: set()},                                                       // reviewer says no grounding (correct)
		{Record: 1, LineStart: 50, LineEnd: 51, Governs: set(300)},                                                    // reviewer found nothing at all
		{Record: 1, LineStart: 1, LineEnd: 1, DocumentLevel: true, Governs: set(400), Ambiguous: 2},
	}
	rows := []ReviewRow{
		{Record: 1, LineStart: 10, LineEnd: 12, Candidates: set(100, 555)},
		reported(1, 21, 22, art("chunk/k777", 777)),
		{Record: 1, LineStart: 30, LineEnd: 31, NoGrounding: true},
		{Record: 1, LineStart: 40, LineEnd: 41, NoGrounding: true},
		{Record: 1, LineStart: 90, LineEnd: 91, Candidates: set(1)}, // spurious
		{Record: 1, LineStart: 1, LineEnd: 1, DocumentLevel: true, Candidates: set(400)},
	}
	m := Score(gold, rows, nil)
	if m.GoldWithGoverns != 4 || m.MatchedActivities != 5 || m.SupportedRows != 5 || m.ActivityPrecision != (Ratio{5, 6}) || m.ActivityRecall != (Ratio{5, 6}) {
		t.Fatalf("matching: %+v", m)
	}
	// governing records: 100,101 (reported 100), 200 (row says no grounding), 400 (reported) -> 2 of 4
	if m.GroundingRecall != (Ratio{2, 4}) {
		t.Errorf("grounding recall = %v", m.GroundingRecall)
	}
	if m.NoGroundingPrecision != (Ratio{1, 2}) || m.NoGroundingMissedBySearch != (Ratio{1, 2}) {
		t.Errorf("no grounding = %v / %v", m.NoGroundingPrecision, m.NoGroundingMissedBySearch)
	}
	if m.FalseGrounding != (Ratio{1, 2}) { // activities 20-22 (reported) and 40-41 (no-grounding row, correct)
		t.Errorf("false grounding = %v", m.FalseGrounding)
	}
	if m.AmbiguousJudgements != 2 {
		t.Errorf("ambiguous = %d", m.AmbiguousJudgements)
	}
	if !m.Headline.Defined() {
		t.Error("headline must be defined")
	}
}

// art is a reported passage; reported builds a row from its passages.
func art(key string, rec int64) ReviewCandidate { return ReviewCandidate{Key: key, Record: rec} }

func reported(rec int64, s, e int, arts ...ReviewCandidate) ReviewRow {
	r := ReviewRow{Record: rec, LineStart: s, LineEnd: e, Candidates: map[int64]bool{}, Artifacts: arts}
	for _, a := range arts {
		r.Candidates[a.Record] = true
	}
	return r
}

func TestScoreCandidatePrecisionAndPassagesOutsideThePool(t *testing.T) {
	gold := []GoldAct{
		{Record: 1, LineStart: 10, LineEnd: 10, Governs: set(5), Labels: map[string]string{"chunk/g": "governs", "chunk/t": "topical", "chunk/a": "ambiguous"}},
		{Record: 1, LineStart: 20, LineEnd: 20, Governs: set(), Labels: map[string]string{"chunk/j": "unrelated"}}, // nothing expected
	}
	rows := []ReviewRow{
		reported(1, 10, 10, art("chunk/g", 5), art("chunk/t", 5), art("chunk/a", 6), art("chunk/u", 7)),
		reported(1, 20, 20, art("chunk/v", 7)), // a snapshot passage the gold never judged
	}
	m := Score(gold, rows, set(5, 6, 7))
	if m.CandidatePrecision != (Ratio{1, 2}) || m.CandidateLabels["ambiguous"] != 1 || m.CandidatesNotInPool != 2 {
		t.Fatalf("candidate precision %v, labels %v, not in pool %d", m.CandidatePrecision, m.CandidateLabels, m.CandidatesNotInPool)
	}
	// Unjudged is never wrong: the activity that expects nothing is unjudged, not a false grounding.
	if m.FalseGrounding.Defined() || m.UnjudgedActivities != 1 {
		t.Errorf("false grounding %v, unjudged activities %d", m.FalseGrounding, m.UnjudgedActivities)
	}
}

func TestScoreCountsPassagesOnTheDocumentsOwnCopiesAsUnrelated(t *testing.T) {
	own := set(416, 198)
	gold := []GoldAct{{Record: 416, LineStart: 10, LineEnd: 10, Governs: set(), Labels: map[string]string{}, OwnCopies: own}}
	m := Score(gold, []ReviewRow{reported(416, 10, 10, art("chunk/198_chk_1", 198))}, set(5, 198))
	if m.CandidatePrecision != (Ratio{0, 1}) || m.CandidatesOnOwnCopies != 1 || m.CandidatesNotInPool != 0 {
		t.Fatalf("precision %v, own copies %d, not in pool %d", m.CandidatePrecision, m.CandidatesOnOwnCopies, m.CandidatesNotInPool)
	}
	if m.FalseGrounding != (Ratio{1, 1}) { // the reviewer grounded the activity on its own copy
		t.Errorf("false grounding = %v", m.FalseGrounding)
	}
}

func TestScoreUndefinedRatiosAreNotZero(t *testing.T) {
	m := Score(nil, nil, nil)
	for name, r := range map[string]Ratio{"p": m.ActivityPrecision, "r": m.ActivityRecall, "g": m.GroundingRecall,
		"n": m.NoGroundingPrecision, "f": m.FalseGrounding, "h": m.Headline} {
		if r.Defined() || !strings.Contains(r.String(), "n/a") {
			t.Errorf("%s = %v, want undefined", name, r)
		}
	}
}

func TestScoreDocumentLevelOnlyMatchesDocumentLevel(t *testing.T) {
	m := Score([]GoldAct{{Record: 1, LineStart: 1, LineEnd: 3, DocumentLevel: true}},
		[]ReviewRow{{Record: 1, LineStart: 1, LineEnd: 3}}, nil)
	if m.MatchedActivities != 0 {
		t.Fatalf("matched = %d", m.MatchedActivities)
	}
}

func TestScoreMatchesPerRecord(t *testing.T) {
	m := Score([]GoldAct{{Record: 1, LineStart: 1, LineEnd: 3}}, []ReviewRow{{Record: 2, LineStart: 1, LineEnd: 3}}, nil)
	if m.MatchedActivities != 0 {
		t.Fatalf("matched across records: %d", m.MatchedActivities)
	}
}

func TestParseReviewRow(t *testing.T) {
	desc := "Activity: x\n\nRetrieved 2 candidate artifact(s).\n- [chunk 198_chk_1] (cosine 0.812) record 198 a.pdf: text\n- [provision p1] record 77 b: more\n"
	r, err := ParseReviewRow(416, "compliance_grounding_candidates", "Grounding candidates found: document: Waste", "1-169", desc)
	if err != nil || !r.DocumentLevel || r.NoGrounding || r.LineStart != 1 || r.LineEnd != 169 || !r.Candidates[198] || !r.Candidates[77] || len(r.Candidates) != 2 {
		t.Fatalf("row = %+v err=%v", r, err)
	}
	if len(r.Artifacts) != 2 || r.Artifacts[0].Key != "chunk/198_chk_1" || r.Artifacts[0].Record != 198 || *r.Artifacts[0].Cosine != 0.812 ||
		r.Artifacts[1].Key != "provision/p1" || r.Artifacts[1].Cosine != nil {
		t.Fatalf("artifacts = %+v", r.Artifacts)
	}
	n, _ := ParseReviewRow(416, "compliance_no_grounding", "Activity with no grounding candidates found: log_x", "5-9", "text")
	if !n.NoGrounding || n.DocumentLevel || len(n.Candidates) != 0 {
		t.Fatalf("row = %+v", n)
	}
	if _, err := ParseReviewRow(1, "x", "t", "garbage", ""); err == nil {
		t.Fatal("bad location must be an error")
	}
}

func TestActivityListsEqual(t *testing.T) {
	a := []ReviewRow{{LineStart: 1, LineEnd: 2}, {LineStart: 5, LineEnd: 6, DocumentLevel: true}}
	b := []ReviewRow{{LineStart: 5, LineEnd: 6, DocumentLevel: true}, {LineStart: 1, LineEnd: 2}}
	if ok, d := ActivityListsEqual(a, b); !ok || len(d) != 0 {
		t.Fatalf("equal lists reported different: %v", d)
	}
	ok, d := ActivityListsEqual(a, append(b, ReviewRow{LineStart: 9, LineEnd: 9}))
	if ok || len(d) != 1 || !strings.Contains(d[0], "9-9") {
		t.Fatalf("diffs = %v", d)
	}
}

func TestScoreOneMergedRowCoversManyFineGoldActivities(t *testing.T) {
	gold := []GoldAct{
		{Record: 1, LineStart: 49, LineEnd: 49, Governs: set(100), Labels: map[string]string{"chunk/g": "governs"}},
		{Record: 1, LineStart: 50, LineEnd: 50, Governs: set(100), Labels: map[string]string{"chunk/g": "governs"}},
		{Record: 1, LineStart: 51, LineEnd: 51, Governs: set(), Labels: map[string]string{"chunk/g": "unrelated"}},
	}
	m := Score(gold, []ReviewRow{reported(1, 49, 52, art("chunk/g", 100))}, nil)
	if m.ActivityRecall != (Ratio{3, 3}) || m.ActivityPrecision != (Ratio{1, 1}) {
		t.Fatalf("recall=%v precision=%v", m.ActivityRecall, m.ActivityPrecision)
	}
	if m.GroundingRecall != (Ratio{2, 2}) || m.FalseGrounding != (Ratio{1, 1}) {
		t.Errorf("grounding recall=%v false grounding=%v", m.GroundingRecall, m.FalseGrounding)
	}
	// The row covers all three: the passage governs two of them, so it counts as governs once.
	if m.CandidatePrecision != (Ratio{1, 1}) {
		t.Errorf("candidate precision = %v", m.CandidatePrecision)
	}
}

func TestScoreNoGroundingNeedsEveryCoveringRowToAgree(t *testing.T) {
	gold := []GoldAct{{Record: 1, LineStart: 5, LineEnd: 9, Governs: set(7)}}
	rows := []ReviewRow{
		{Record: 1, LineStart: 5, LineEnd: 6, NoGrounding: true},
		{Record: 1, LineStart: 7, LineEnd: 9, Candidates: set(7)},
	}
	m := Score(gold, rows, nil)
	if m.NoGroundingPrecision.Defined() || m.GroundingRecall != (Ratio{1, 1}) {
		t.Fatalf("no-grounding=%v recall=%v", m.NoGroundingPrecision, m.GroundingRecall)
	}
}

func TestScoreHitsOnRecordsAddedAfterSnapshotAreUnjudged(t *testing.T) {
	gold := []GoldAct{
		{Record: 1, LineStart: 10, LineEnd: 10, Governs: set()},                                                   // reviewer hit only a record added later
		{Record: 1, LineStart: 20, LineEnd: 20, Governs: set(), Labels: map[string]string{"chunk/s6": "topical"}}, // reviewer hit a judged snapshot passage
		{Record: 1, LineStart: 30, LineEnd: 30, Governs: set(5), Labels: map[string]string{"chunk/s5": "governs"}},
	}
	rows := []ReviewRow{
		reported(1, 10, 10, art("chunk/n900", 900)),
		reported(1, 20, 20, art("chunk/s6", 6), art("chunk/n901", 901)),
		reported(1, 30, 30, art("chunk/s5", 5)),
	}
	m := Score(gold, rows, set(5, 6))
	if m.FalseGrounding != (Ratio{1, 1}) || m.UnjudgedActivities != 1 || m.UnjudgedCandidates != 2 || m.CandidatesOnNewRecords != 2 {
		t.Fatalf("false grounding %v, unjudged activities %d, unjudged candidates %d, on new records %d",
			m.FalseGrounding, m.UnjudgedActivities, m.UnjudgedCandidates, m.CandidatesOnNewRecords)
	}
	if m.CandidatePrecision != (Ratio{1, 2}) { // s5 governs, s6 topical
		t.Errorf("candidate precision = %v", m.CandidatePrecision)
	}
	if m.GroundingRecall != (Ratio{1, 1}) {
		t.Errorf("grounding recall = %v", m.GroundingRecall)
	}
}

func TestMetricCandidateParsingAndScoring(t *testing.T) {
	row, err := ParseReviewRow(1, "compliance_candidates_found", "Activity", "2-3", "Activity: inspect\n\n- [metric m42] (cosine 0.012) record 9 Standard: limit 10 mg/L\n")
	if err != nil || len(row.Artifacts) != 1 || row.Artifacts[0].Key != "metric/m42" || *row.Artifacts[0].Cosine != 0.012 {
		t.Fatalf("metric row=%+v err=%v", row, err)
	}
	g := []GoldAct{{Record: 1, LineStart: 2, LineEnd: 3, Governs: map[int64]bool{9: true}, Labels: map[string]string{"metric/m42": "governs"}}}
	m := Score(g, []ReviewRow{row}, map[int64]bool{9: true})
	if m.CandidatePrecision.Num != 1 || m.CandidatePrecision.Den != 1 || m.GroundingRecall.Num != 1 {
		t.Fatalf("metric score=%+v", m)
	}
	if m.CandidatesByArtifactType["metric"].Labels["governs"] != 1 {
		t.Fatalf("metric statistics=%+v", m.CandidatesByArtifactType)
	}
}
