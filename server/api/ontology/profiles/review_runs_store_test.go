package profiles

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReviewRunStoreCreateRunPersistsPinnedWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_review_runs")).
		WithArgs("scope-1", int64(99), "assertion:1234").
		WillReturnRows(sqlmock.NewRows([]string{"id", "review_scope_id", "input_record_id", "assertion_watermark", "create_time"}).
			AddRow(int64(7), "scope-1", int64(99), "assertion:1234", now))

	got, err := (ReviewRunStore{DB: db}).CreateRun(context.Background(), ReviewRun{
		ReviewScopeID: "scope-1", InputRecordID: 99, AssertionWatermark: "assertion:1234",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got.ID != 7 || got.ReviewScopeID != "scope-1" || got.AssertionWatermark != "assertion:1234" {
		t.Fatalf("CreateRun = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewRunStoreCreateRunRequiresProvenance(t *testing.T) {
	if _, err := (ReviewRunStore{DB: nil}).CreateRun(context.Background(), ReviewRun{}); err == nil {
		t.Fatal("expected an error for a nil DB")
	}
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := (ReviewRunStore{DB: db}).CreateRun(context.Background(), ReviewRun{ReviewScopeID: "scope-1"}); err == nil {
		t.Fatal("expected an error for a missing input_record_id/assertion_watermark")
	}
}
