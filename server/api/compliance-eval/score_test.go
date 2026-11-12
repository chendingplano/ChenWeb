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
		{Record: 1, LineStart: 10, LineEnd: 12, Governs: set(100, 101)}, // covered: reviewer reports 100 only
		{Record: 1, LineStart: 20, LineEnd: 22, Governs: set()},         // uncovered: reviewer reports candidates
		{Record: 1, LineStart: 30, LineEnd: 31, Governs: set(200)},      // reviewer says no grounding (missed by search)
		{Record: 1, LineStart: 40, LineEnd: 41, Governs: set()},         // reviewer says no grounding (correct)
		{Record: 1, LineStart: 50, LineEnd: 51, Governs: set(300)},      // reviewer found nothing at all
		{Record: 1, LineStart: 1, LineEnd: 1, DocumentLevel: true, Governs: set(400), Ambiguous: 2},
	}
	rows := []ReviewRow{
		{Record: 1, LineStart: 10, LineEnd: 12, Candidates: set(100, 555)},
		{Record: 1, LineStart: 21, LineEnd: 22, Candidates: set(777)},
		{Record: 1, LineStart: 30, LineEnd: 31, NoGrounding: true},
		{Record: 1, LineStart: 40, LineEnd: 41, NoGrounding: true},
		{Record: 1, LineStart: 90, LineEnd: 91, Candidates: set(1)}, // spurious
		{Record: 1, LineStart: 1, LineEnd: 1, DocumentLevel: true, Candidates: set(400)},
	}
	m := Score(gold, rows)
	if m.MatchedActivities != 5 || m.SupportedRows != 5 || m.ActivityPrecision != (Ratio{5, 6}) || m.ActivityRecall != (Ratio{5, 6}) {
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

func TestScoreUndefinedRatiosAreNotZero(t *testing.T) {
	m := Score(nil, nil)
	for name, r := range map[string]Ratio{"p": m.ActivityPrecision, "r": m.ActivityRecall, "g": m.GroundingRecall,
		"n": m.NoGroundingPrecision, "f": m.FalseGrounding, "h": m.Headline} {
		if r.Defined() || !strings.Contains(r.String(), "n/a") {
			t.Errorf("%s = %v, want undefined", name, r)
		}
	}
}

func TestScoreDocumentLevelOnlyMatchesDocumentLevel(t *testing.T) {
	m := Score([]GoldAct{{Record: 1, LineStart: 1, LineEnd: 3, DocumentLevel: true}},
		[]ReviewRow{{Record: 1, LineStart: 1, LineEnd: 3}})
	if m.MatchedActivities != 0 {
		t.Fatalf("matched = %d", m.MatchedActivities)
	}
}

func TestScoreMatchesPerRecord(t *testing.T) {
	m := Score([]GoldAct{{Record: 1, LineStart: 1, LineEnd: 3}}, []ReviewRow{{Record: 2, LineStart: 1, LineEnd: 3}})
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
		{Record: 1, LineStart: 49, LineEnd: 49, Governs: set(100)},
		{Record: 1, LineStart: 50, LineEnd: 50, Governs: set(100)},
		{Record: 1, LineStart: 51, LineEnd: 51, Governs: set()},
	}
	m := Score(gold, []ReviewRow{{Record: 1, LineStart: 49, LineEnd: 52, Candidates: set(100)}})
	if m.ActivityRecall != (Ratio{3, 3}) || m.ActivityPrecision != (Ratio{1, 1}) {
		t.Fatalf("recall=%v precision=%v", m.ActivityRecall, m.ActivityPrecision)
	}
	if m.GroundingRecall != (Ratio{2, 2}) || m.FalseGrounding != (Ratio{1, 1}) {
		t.Errorf("grounding recall=%v false grounding=%v", m.GroundingRecall, m.FalseGrounding)
	}
}

func TestScoreNoGroundingNeedsEveryCoveringRowToAgree(t *testing.T) {
	gold := []GoldAct{{Record: 1, LineStart: 5, LineEnd: 9, Governs: set(7)}}
	rows := []ReviewRow{
		{Record: 1, LineStart: 5, LineEnd: 6, NoGrounding: true},
		{Record: 1, LineStart: 7, LineEnd: 9, Candidates: set(7)},
	}
	m := Score(gold, rows)
	if m.NoGroundingPrecision.Defined() || m.GroundingRecall != (Ratio{1, 1}) {
		t.Fatalf("no-grounding=%v recall=%v", m.NoGroundingPrecision, m.GroundingRecall)
	}
}
