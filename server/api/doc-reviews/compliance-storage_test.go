package docreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func storageTestFinding() ReviewFinding {
	score := 7
	order := 0
	cos := 0.71
	return ReviewFinding{
		Pass: "P5", Aspect: "compliance", Severity: "info", FindingType: "compliance_assessment", ResultKind: "compliance_assessment",
		Title: "Compliance assessment: seal (+1 more)", Location: "3-4", LineStart: 3, LineEnd: 4, Language: "zh",
		Compliance: &complianceRecord{
			StageModels: map[string]string{"judgement": "m1"},
			Activities: []complianceActivityRecord{
				{LocalLabel: "a1", Kind: "provision", Label: "seal", LineStart: 3, LineEnd: 3, SourceType: "provision", SourceID: "7_prv_1",
					Searches: []complianceSearchRecord{{Purpose: "activity", Limit: 30, Results: []complianceTraceResult{
						{ArtifactType: "provision", ArtifactID: "2_prv_1", RecordID: 2, Rank: 1, Score: 0.03},
						{ArtifactType: "provision", ArtifactID: "3_prv_1", RecordID: 3, Rank: 2, Score: 0.02}}}},
					Candidates: []complianceCandidateRecord{
						{Type: "provision", ID: "2_prv_1", RecordID: 2, Cosine: &cos, RelevanceAnswer: "yes", RelevanceScore: &score, Kept: true,
							Status: candidateJudged, Verdict: verdictCompatible, Binding: "mandatory", Summary: "s", JudgementOrder: &order},
						{Type: "provision", ID: "3_prv_1", RecordID: 3, RelevanceAnswer: "no", Status: candidateRejected, DisplayOrder: 2}}},
				{LocalLabel: "a2", Kind: "metric", Label: "t", LineStart: 4, LineEnd: 4, SourceType: "metric", SourceID: "7_mtc_1",
					Candidates: []complianceCandidateRecord{
						{Type: "provision", ID: "2_prv_1", RecordID: 2, Kept: true, Status: candidateJudged, Verdict: verdictCompatible, Binding: "mandatory", DisplayOrder: 1}}},
			},
		},
	}
}

// A candidate judged for two activities of a group counts twice: counters are pair counts.
func TestComplianceCountsArePairCounts(t *testing.T) {
	c := storageTestFinding().Compliance.counts()
	if c.Activities != 2 || c.SearchResults != 2 || c.Selected != 3 || c.Relevant != 2 || c.Judged != 2 || c.Verdicts[verdictCompatible] != 2 {
		t.Fatalf("counts = %+v", c)
	}
}

func TestSaveComplianceFindingWritesEverythingInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := storageTestFinding()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.doc_review_findings`).
		WithArgs(int64(7), int64(116), "P5", "compliance", "info", "compliance_assessment", f.Title, "3-4", 0.5,
			`{"result_kind":"compliance_assessment","schema_version":1,"storage_version":2}`, nil, nil, findingStorageNormalized,
			3, 4, 2, 2, 3, 2, 2, `{"compatible":2}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(900))
	mock.ExpectExec(`INSERT INTO kb.doc_review_finding_details`).
		WithArgs(int64(900), "", "", "", "zh", "original", "zh", 1.0, "", `{"judgement":"m1"}`, `{}`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_activities`).
		WithArgs(int64(5000), int64(900), "a1", 0, "provision", "seal", 3, 3, "provision", "7_prv_1", nil, "", `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"activity_id"}).AddRow(5000))
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_searches`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_search_results .* VALUES \(\$1, .*\), \(\$9, `).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_candidates`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_activities`).
		WithArgs(int64(5001), int64(900), "a2", 1, "metric", "t", 4, 4, "metric", "7_mtc_1", nil, "", `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"activity_id"}).AddRow(5001))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_candidates`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	var asked int64
	ids := func(_ context.Context, n int64) (int64, error) { asked = n; return 5000, nil }
	id, err := saveComplianceFinding(context.Background(), db, ids, 7, 116, f)
	if err != nil || id != 900 || asked != 2 {
		t.Fatalf("id %d, asked %d IDs, err %v", id, asked, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSaveComplianceFindingRollsBackOnFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.doc_review_findings`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(900))
	mock.ExpectExec(`INSERT INTO kb.doc_review_finding_details`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_activities`).WillReturnRows(sqlmock.NewRows([]string{"activity_id"}).AddRow(5000))
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_searches`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_search_results`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_candidates`).WillReturnError(errors.New("disk full"))
	mock.ExpectRollback()

	ids := func(context.Context, int64) (int64, error) { return 5000, nil }
	if _, err := saveComplianceFinding(context.Background(), db, ids, 7, 116, storageTestFinding()); err == nil {
		t.Fatal("want the insert error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A retried insert finds the activity already there: it keeps that activity's ID and
// replaces its searches and candidates.
func TestInsertComplianceActivityReusesAnExistingActivityID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := storageTestFinding().Compliance.Activities[1]
	mock.ExpectQuery(`INSERT INTO kb.doc_review_compliance_activities .* ON CONFLICT \(finding_id, local_label\)`).
		WillReturnRows(sqlmock.NewRows([]string{"activity_id"}).AddRow(4321))
	mock.ExpectExec(`DELETE FROM kb.doc_review_compliance_searches WHERE activity_id`).WithArgs(int64(4321)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM kb.doc_review_compliance_candidates WHERE activity_id`).WithArgs(int64(4321)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO kb.doc_review_compliance_candidates`).WithArgs(int64(4321), "provision", "2_prv_1", int64(2), "", nil, "", nil, true,
		candidateJudged, verdictCompatible, "mandatory", "", "", "", `{}`, 1, nil).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := insertComplianceActivity(context.Background(), db, 900, 1, 9999, &a); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Reports render verdicts and activities from rows in the form the description used: verdict
// counts, one line per judged candidate (naming its activity in a group), notes, the
// finding-level text, then the activity lines. The judged-candidate list is not rendered.
func TestRenderComplianceReportText(t *testing.T) {
	verdicts := []complianceReportVerdict{
		{label: "a1", activityLabel: "seal", verdict: verdictNotCompliant, ctype: "provision", cid: "2_prv_1", record: 2, summary: "too short"},
		{label: "a2", activityLabel: "t", verdict: verdictUndetermined, ctype: "metric", cid: "3_mtc_1", record: 3, summary: "unclear", note: "The verdict could not be verified."},
	}
	acts := []complianceReportActivity{{label: "a1", kind: "provision", name: "seal", start: 3, end: 3}, {label: "a2", kind: "metric", name: "t", start: 4, end: 4}}
	got := renderComplianceReportText("", verdicts, acts)
	want := "Candidate verdicts: 1 not_compliant, 1 undetermined\n" +
		"not_compliant [provision 2_prv_1] record 2 (activity a1: seal): too short\n" +
		"undetermined [metric 3_mtc_1] record 3 (activity a2: t): unclear\n" +
		"  Note: The verdict could not be verified.\n\n" +
		"Activity a1 [provision, lines 3-3]: seal\n" +
		"Activity a2 [metric, lines 4-4]: t"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	single := renderComplianceReportText("No candidate was left.", nil, acts[:1])
	if single != "No candidate was left.\n\nActivity a1 [provision, lines 3-3]: seal" {
		t.Errorf("no-grounding text = %q", single)
	}
}
