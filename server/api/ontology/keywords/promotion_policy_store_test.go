package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPromotionPolicyStoreIsEnabledDefaultsTrueWhenAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT auto_promote_enabled FROM kb.keyword_source_promotion_policy WHERE source = $1")).
		WithArgs("qudt").
		WillReturnRows(sqlmock.NewRows([]string{"auto_promote_enabled"}))

	store := PromotionPolicyStore{DB: db}
	enabled, err := store.IsEnabled(context.Background(), "qudt")
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if !enabled {
		t.Fatal("expected enabled=true when no policy row exists")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromotionPolicyStoreIsEnabledReflectsExistingRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT auto_promote_enabled FROM kb.keyword_source_promotion_policy WHERE source = $1")).
		WithArgs("wikidata").
		WillReturnRows(sqlmock.NewRows([]string{"auto_promote_enabled"}).AddRow(false))

	store := PromotionPolicyStore{DB: db}
	enabled, err := store.IsEnabled(context.Background(), "wikidata")
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false when policy row disables it")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromotionPolicyStoreSetUpserts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_source_promotion_policy")).
		WithArgs("qudt", false, "alice@example.test").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := PromotionPolicyStore{DB: db}
	got, err := store.Set(context.Background(), "qudt", false, "alice@example.test")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got.Source != "qudt" || got.AutoPromote != false || got.SetBy != "alice@example.test" {
		t.Fatalf("unexpected policy: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromotionPolicyStoreNilDBErrors(t *testing.T) {
	store := PromotionPolicyStore{DB: nil}
	if _, err := store.IsEnabled(context.Background(), "qudt"); err == nil {
		t.Fatal("expected error for nil db")
	}
	if _, err := store.Set(context.Background(), "qudt", true, "tester"); err == nil {
		t.Fatal("expected error for nil db")
	}
}
