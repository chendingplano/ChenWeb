package classfoundation

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMetricSupportCleanupReportsCurrentDuplicateOccurrences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.assertion_evidence e")).
		WillReturnRows(sqlmock.NewRows([]string{"artifact_type", "artifact_id", "input_record_id", "support_links"}).
			AddRow("metric", "metric-7", int64(41), 2))

	duplicates, err := (MetricSupportCleanup{DB: db}).ReportCurrentDuplicates(context.Background())
	if err != nil {
		t.Fatalf("ReportCurrentDuplicates: %v", err)
	}
	if len(duplicates) != 1 || duplicates[0].ArtifactType != "metric" || duplicates[0].ArtifactID != "metric-7" ||
		duplicates[0].InputRecordID == nil || *duplicates[0].InputRecordID != 41 || duplicates[0].SupportLinks != 2 {
		t.Fatalf("duplicates = %#v", duplicates)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricSupportCleanupSoftDeletesSurplusRowsAfterAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("WITH ranked AS")).
		WillReturnResult(sqlmock.NewResult(0, 2))

	retired, err := (MetricSupportCleanup{DB: db}).RetireCurrentDuplicates(context.Background())
	if err != nil {
		t.Fatalf("RetireCurrentDuplicates: %v", err)
	}
	if retired != 2 {
		t.Fatalf("retired = %d, want 2", retired)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
