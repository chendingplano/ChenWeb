package docprocessing

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPipelinePolicySQLStoreGetActivePolicyReadsActiveRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT id, version, status, COALESCE(source_ref, ''), activated_at, COALESCE(activated_by, '')
FROM kb.pipeline_policies
WHERE status = 'active'
LIMIT 1`)
	activatedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"id", "version", "status", "source_ref", "activated_at", "activated_by",
	}).AddRow(int64(1), 1, "active", "bootstrap", activatedAt, "system"))

	store := PipelinePolicySQLStore{DB: db}
	got, err := store.GetActivePolicy(context.Background())
	if err != nil {
		t.Fatalf("GetActivePolicy: %v", err)
	}
	want := ProductionPipelinePolicy{ID: 1, Version: 1, Status: "active", SourceRef: "bootstrap", ActivatedAt: activatedAt, ActivatedBy: "system"}
	if got != want {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPipelinePolicySQLStoreGetActivePolicyPropagatesNoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT id, version, status, COALESCE(source_ref, ''), activated_at, COALESCE(activated_by, '')
FROM kb.pipeline_policies
WHERE status = 'active'
LIMIT 1`)
	mock.ExpectQuery(query).WillReturnError(sql.ErrNoRows)

	store := PipelinePolicySQLStore{DB: db}
	if _, err := store.GetActivePolicy(context.Background()); err == nil {
		t.Fatal("expected error when no policy is active")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPipelinePolicySQLStoreGetActivePolicyRejectsNilDB(t *testing.T) {
	store := PipelinePolicySQLStore{}
	if _, err := store.GetActivePolicy(context.Background()); err == nil {
		t.Fatal("expected error for nil db")
	}
}
