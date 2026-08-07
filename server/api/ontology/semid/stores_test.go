package semid

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDecisionLogStoreAppendAcceptsTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
		WithArgs("keyword", nil, "{}", "{}", "merged", nil, nil, nil, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectRollback()

	id, err := (DecisionLogStore{DB: tx}).Append(context.Background(), DecisionLogEntry{
		Family: "keyword", Verdict: "merged",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireKeywordIdentityMutationLockExactStatement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	if err := AcquireKeywordIdentityMutationLock(context.Background(), tx); err != nil {
		t.Fatalf("AcquireKeywordIdentityMutationLock: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNeverMergeStoreKeywordAddLocksInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.semid_never_merge")).
		WithArgs("keyword", "kw:a", "kw:b", "distinct", "tester").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := (NeverMergeStore{DB: db}).Add(context.Background(), "keyword", "kw:b", "kw:a", "distinct", "tester"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
