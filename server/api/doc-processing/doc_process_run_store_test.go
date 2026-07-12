package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateDocProcessRun_InsertsAndReturnsID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_process_runs (record_id, event_id, mode, run_number, processors, parameters)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(run_number), 0) + 1 FROM kb.doc_process_runs WHERE record_id = $1), $4::jsonb, $5::jsonb)
RETURNING id`)

	eventID := "evt-abc123"
	mock.ExpectQuery(insertQuery).
		WithArgs(int64(4821), &eventID, "auto", `["extract_metrics","generate_topics"]`, `{"force":true,"force_clear":false}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))

	store := SQLStore{DB: db}
	runID, err := store.CreateDocProcessRun(context.Background(), DocProcessRunRecord{
		RecordID:   4821,
		EventID:    eventID,
		Mode:       "auto",
		Processors: []string{"extract_metrics", "generate_topics"},
		Parameters: map[string]any{"force": true, "force_clear": false},
	})
	if err != nil {
		t.Fatalf("CreateDocProcessRun: %v", err)
	}
	if runID != 5 {
		t.Fatalf("runID=%d, want 5", runID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCreateDocProcessRun_NullEventIDAndEmptyParametersDefaultToEmptyObject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_process_runs (record_id, event_id, mode, run_number, processors, parameters)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(run_number), 0) + 1 FROM kb.doc_process_runs WHERE record_id = $1), $4::jsonb, $5::jsonb)
RETURNING id`)

	mock.ExpectQuery(insertQuery).
		WithArgs(int64(11), nil, "dev", `[]`, `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	store := SQLStore{DB: db}
	runID, err := store.CreateDocProcessRun(context.Background(), DocProcessRunRecord{
		RecordID: 11,
		Mode:     "dev",
	})
	if err != nil {
		t.Fatalf("CreateDocProcessRun: %v", err)
	}
	if runID != 1 {
		t.Fatalf("runID=%d, want 1", runID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCreateDocProcessRun_RequiresRecordIDAndMode(t *testing.T) {
	store := SQLStore{DB: nil}
	if _, err := store.CreateDocProcessRun(context.Background(), DocProcessRunRecord{Mode: "auto"}); err == nil {
		t.Fatal("expected error for missing record_id")
	}

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()
	store = SQLStore{DB: db}
	if _, err := store.CreateDocProcessRun(context.Background(), DocProcessRunRecord{RecordID: 1}); err == nil {
		t.Fatal("expected error for missing mode")
	}
}

func TestCloseDocProcessRun_UpdatesStatusAndErrorMessage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	updateQuery := regexp.QuoteMeta(`
UPDATE kb.doc_process_runs
SET status = $2, error_message = $3, end_time = NOW()
WHERE id = $1`)

	errMsg := "boom"
	mock.ExpectExec(updateQuery).
		WithArgs(int64(5), "failed", &errMsg).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	if err := store.CloseDocProcessRun(context.Background(), 5, "failed", &errMsg); err != nil {
		t.Fatalf("CloseDocProcessRun: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCloseDocProcessRun_NilErrorMessageOnSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	updateQuery := regexp.QuoteMeta(`
UPDATE kb.doc_process_runs
SET status = $2, error_message = $3, end_time = NOW()
WHERE id = $1`)

	mock.ExpectExec(updateQuery).
		WithArgs(int64(9), "success", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	if err := store.CloseDocProcessRun(context.Background(), 9, "success", nil); err != nil {
		t.Fatalf("CloseDocProcessRun: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
