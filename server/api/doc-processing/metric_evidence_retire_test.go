package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Re-extraction must retire a record's metric evidence before deleting its
// metric rows, because the new rows reuse metric_id values (ADR 2026100603).
func TestDeleteMetricsByInputRecordIDRetiresEvidenceFirst(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS kb`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id FROM kb.assertion_evidence\s+WHERE artifact_type = 'metric' AND input_record_id = \$1 AND NOT deleted`).
		WithArgs(int64(416)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`DELETE FROM kb.metrics WHERE input_record_id = \$1`).
		WithArgs(int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 45))
	mock.ExpectExec(`DELETE FROM kb.metrics_dropped WHERE input_record_id = \$1`).
		WithArgs(int64(416)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	n, err := (MetricsSQLStore{DB: db}).DeleteMetricsByInputRecordID(context.Background(), 416)
	if err != nil {
		t.Fatalf("DeleteMetricsByInputRecordID: %v", err)
	}
	if n != 45 {
		t.Fatalf("deleted = %d, want 45", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("statement order: %v", err)
	}
}
