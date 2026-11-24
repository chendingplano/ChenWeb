package complianceeval

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadReviewRowsRefusesFailedRuns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.doc_review_runs`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(int64(825)))
	mock.ExpectQuery(regexp.QuoteMeta(`proc_status = 'failed'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	if _, err := LoadReviewRows(context.Background(), db, 5); err == nil || !strings.Contains(err.Error(), "marked failed") {
		t.Fatalf("err = %v, want the failed run refused", err)
	}
}

func TestLoadReviewRowsSkipsUncheckedFindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.doc_review_runs`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(int64(825)))
	mock.ExpectQuery(regexp.QuoteMeta(`proc_status = 'failed'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`c.status = 'judged'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"finding_id", "candidate_type", "candidate_id", "candidate_record_id", "cosine"}))
	mock.ExpectQuery(regexp.QuoteMeta(`finding_type <> 'compliance_unchecked'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "finding_type", "title", "location", "description", "storage_version"}).
			AddRow(1, "compliance_assessment", "Compliance assessment: seal", "3-4", "- [provision 2_prv_1] record 2 T: text\n", 1))
	rows, err := LoadReviewRows(context.Background(), db, 5)
	if err != nil || len(rows) != 1 || !rows[0].Candidates[2] {
		t.Fatalf("rows = %+v, err %v", rows, err)
	}
}

// A normalized finding's judged candidates come from its candidate rows, once each and in
// judgement order, with the cosine at the precision legacy descriptions printed; its
// description is not parsed.
func TestLoadReviewRowsReadsJudgedCandidateRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.doc_review_runs`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(int64(825)))
	mock.ExpectQuery(regexp.QuoteMeta(`proc_status = 'failed'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`c.status = 'judged'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"finding_id", "candidate_type", "candidate_id", "candidate_record_id", "cosine"}).
			AddRow(9, "provision", "812_prv_167", 812, 0.714629748).
			AddRow(9, "provision", "811_prv_161", 811, nil).
			AddRow(9, "provision", "812_prv_167", 812, 0.71)) // judged for a second activity of the group
	mock.ExpectQuery(regexp.QuoteMeta(`finding_type <> 'compliance_unchecked'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "finding_type", "title", "location", "description", "storage_version"}).
			AddRow(9, "compliance_assessment", "Compliance assessment: run test", "469-471", "- [provision 1_prv_1] record 1 stale: x\n", 2))
	rows, err := LoadReviewRows(context.Background(), db, 5)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, err %v", rows, err)
	}
	r := rows[0]
	if len(r.Artifacts) != 2 || r.Artifacts[0].Key != "provision/812_prv_167" || *r.Artifacts[0].Cosine != 0.715 || r.Artifacts[1].Cosine != nil ||
		!r.Candidates[812] || !r.Candidates[811] || r.Candidates[1] || r.LineStart != 469 {
		t.Fatalf("row = %+v", r)
	}
}
