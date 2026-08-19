package semantic

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func occurrenceColumnNames() []string {
	return []string{"id", "occurrence_key", "input_record_id", "artifact_type", "artifact_id",
		"source_revision", "raw_payload", "provenance", "materialization_state", "resulting_assertion_id",
		"current_outcome_id", "supersedes_occurrence_id", "input_fingerprint", "dependency_fingerprint",
		"active", "lease_token", "lease_expires_at", "saga_idempotency_key", "saga_completed_at",
		"last_seen", "create_by"}
}

// TestActiveOccurrencesForInputRecordReturnsOnlyActiveRowsForThatRecord locks
// in task 7.1's generic-discovery reader (DR13: "the current row is
// queryable through the same generic semantic-discovery API as
// assertions") -- scoped by input_record_id, the same dimension
// AssertionListFilter uses, and restricted to active = true so a superseded
// occurrence never shadows the row that replaced it.
func TestActiveOccurrencesForInputRecordReturnsOnlyActiveRowsForThatRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows(occurrenceColumnNames()).AddRow(
		int64(1), "metric:416:m1", int64(416), "metric", "m1",
		"", []byte("{}"), []byte("{}"), MaterializationPending, nil,
		nil, nil, "fp-in", "fp-dep",
		true, "", nil, "", nil, time.Now(), "test",
	)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE input_record_id = $1 AND active = true")).
		WithArgs(int64(416)).
		WillReturnRows(rows)

	occs, err := (OccurrenceStore{DB: db}).ActiveOccurrencesForInputRecord(context.Background(), 416)
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 1 || occs[0].OccurrenceKey != "metric:416:m1" {
		t.Fatalf("occs = %+v, want exactly one row for metric:416:m1", occs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActiveOccurrencesForInputRecordReturnsEmptyNotErrorWhenNoneActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("WHERE input_record_id = $1 AND active = true")).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows(occurrenceColumnNames()))

	occs, err := (OccurrenceStore{DB: db}).ActiveOccurrencesForInputRecord(context.Background(), 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 0 {
		t.Fatalf("occs = %+v, want empty slice", occs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
