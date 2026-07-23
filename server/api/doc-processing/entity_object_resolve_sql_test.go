package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEntityObjectResolveSQLStoreLoadResolvableReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.entities").
		WithArgs(entityObjectLinkPending, entityObjectLinkDeferred, 200).
		WillReturnRows(sqlmock.NewRows([]string{
			"entity_id", "input_record_id", "entity", "entity_en", "entity_type", "entity_type_en",
			"aliases", "aliases_en", "desc_text", "desc_text_en",
			"object_link_attempts", "object_link_fingerprint",
		}).AddRow(
			"9_ent_1", int64(9), "Pump A", "Pump A", "equipment", "equipment",
			[]byte(`[]`), []byte(`[]`), "", "",
			int64(1), "abc123",
		))

	store := EntityObjectResolveSQLStore{DB: db}
	rows, err := store.LoadResolvable(context.Background(), 200)
	if err != nil {
		t.Fatalf("LoadResolvable: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	got := rows[0]
	if got.Entity.EntityID != "9_ent_1" || got.Entity.InputRecordID != 9 || got.Attempts != 1 || got.Fingerprint != "abc123" {
		t.Fatalf("got %+v, unexpected shape", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEntityObjectResolveSQLStoreMarkExcludedWritesTerminalStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.entities")).
		WithArgs(entityObjectLinkExcluded, "9_ent_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := EntityObjectResolveSQLStore{DB: db}
	if err := store.MarkExcluded(context.Background(), "9_ent_1"); err != nil {
		t.Fatalf("MarkExcluded: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEntityObjectResolveSQLStoreMarkLinkedWritesTerminalStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.entities")).
		WithArgs(entityObjectLinkLinked, "9_ent_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := EntityObjectResolveSQLStore{DB: db}
	if err := store.MarkLinked(context.Background(), "9_ent_1"); err != nil {
		t.Fatalf("MarkLinked: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEntityObjectResolveSQLStoreMarkAttemptedWritesAttemptsAndFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.entities")).
		WithArgs(entityObjectLinkDeferred, 2, "fp-2", "9_ent_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := EntityObjectResolveSQLStore{DB: db}
	if err := store.MarkAttempted(context.Background(), "9_ent_1", entityObjectLinkDeferred, 2, "fp-2"); err != nil {
		t.Fatalf("MarkAttempted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
