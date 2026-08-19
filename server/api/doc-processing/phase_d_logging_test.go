package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLogAssociateSemanticsFailureWritesError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_proc_logs (")).
		WithArgs(
			sqlmock.AnyArg(), "associate_semantics", sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), "error", sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "process candidate 548: persist represented assertion: boom",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(1, 1))

	recordID := int64(416)
	err = logAssociateSemanticsFailure(context.Background(), db, recordID, errors.New("process candidate 548: persist represented assertion: boom"))
	if err != nil {
		t.Fatalf("logAssociateSemanticsFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLogPhaseDErrorWritesError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_proc_logs (")).WithArgs(
		sqlmock.AnyArg(), "project_semantics", sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), "error", sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "projection failed",
		sqlmock.AnyArg(), sqlmock.AnyArg(), "MID-yyyymmdd-01",
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))

	recordID := int64(416)
	if err := logPhaseDError(context.Background(), db, recordID, "project_semantics", errors.New("projection failed")); err != nil {
		t.Fatalf("logPhaseDError: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLogHighDeferredCandidateRateWritesError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_proc_logs (")).WithArgs(
		sqlmock.AnyArg(), "associate_semantics", sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), "error", sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), "MID-yyyymmdd-01",
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))

	recordID := int64(416)
	err = logHighDeferredCandidateRate(context.Background(), db, recordID, 90, 149, map[string]int{
		"no_governed_deontic_predicate": 89,
		"unresolved_referent":           1,
	})
	if err != nil {
		t.Fatalf("logHighDeferredCandidateRate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
