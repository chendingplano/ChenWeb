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
	mock.ExpectQuery(regexp.QuoteMeta(`finding_type <> 'compliance_unchecked'`)).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"finding_type", "title", "location", "description"}).
			AddRow("compliance_assessment", "Compliance assessment: seal", "3-4", "- [provision 2_prv_1] record 2 T: text\n"))
	rows, err := LoadReviewRows(context.Background(), db, 5)
	if err != nil || len(rows) != 1 || !rows[0].Candidates[2] {
		t.Fatalf("rows = %+v, err %v", rows, err)
	}
}
