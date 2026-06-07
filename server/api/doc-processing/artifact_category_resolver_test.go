package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// fakeCategoryCreator is a test double for categoryCreator.
type fakeCategoryCreator struct {
	called int
	out    createdCategory
	err    error
}

func (f *fakeCategoryCreator) CreateCategory(_ context.Context, _ string, _ string, _ map[string]any) (createdCategory, error) {
	f.called++
	return f.out, f.err
}

// newLoadRows builds a sqlmock row for loadIntoIndex (includes seen_count, no embedding).
func newLoadRows(id int64, key, status, matchKeys string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}).
		AddRow(id, key, status, "", []byte(matchKeys), int64(1))
}

func TestResolverReturnsExistingMatchWithoutCreating(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(7), "latency").
		WillReturnResult(sqlmock.NewResult(0, 1))

	creator := &fakeCategoryCreator{}
	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}
	id, err := cr.Resolve(context.Background(), "Latency", "metric", nil)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if id != 7 {
		t.Fatalf("Resolve id = %d, want 7", id)
	}
	if creator.called != 0 {
		t.Fatalf("creator called %d times, want 0", creator.called)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolverCreatesOnMiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))
	// absorbAlias for the original normKey
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WillReturnResult(sqlmock.NewResult(0, 1))

	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}
	id, err := cr.Resolve(context.Background(), "throughput", "metric", nil)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if id != 42 {
		t.Fatalf("Resolve id = %d, want 42", id)
	}
	if creator.called != 1 {
		t.Fatalf("creator called %d times, want 1", creator.called)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolverErrorsWhenNoCreatorOnMiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))

	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: nil, index: newCategoryIndex()}
	if _, err := cr.Resolve(context.Background(), "throughput", "metric", nil); err == nil {
		t.Fatal("expected error when no creator is configured and category is missing")
	}
}

func TestResolverCachesIndexAcrossCalls(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	// Only ONE DB load expected even though Resolve is called twice.
	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(7), "latency").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(7), "latency").
		WillReturnResult(sqlmock.NewResult(0, 1))

	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: &fakeCategoryCreator{}, index: newCategoryIndex()}
	if _, err := cr.Resolve(context.Background(), "latency", "metric", nil); err != nil {
		t.Fatalf("Resolve#1 error: %v", err)
	}
	if _, err := cr.Resolve(context.Background(), "latency", "metric", nil); err != nil {
		t.Fatalf("Resolve#2 error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestResolverAbsorbsNonEnglishKeyAsAlias verifies that after creating a category via a
// non-English rawKey, the key is indexed so the next call is a direct hit with no second
// LLM call.
func TestResolverAbsorbsNonEnglishKeyAsAlias(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	// First call: load (empty), create, absorbAlias for 调制解调器
	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("inventory_item").
		WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(10)))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Second call: index already loaded, Chinese key is now in the index → just absorbAlias
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WillReturnResult(sqlmock.NewResult(0, 1))

	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "modem"}}
	idx := newCategoryIndex()
	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: idx}

	id1, err := cr.Resolve(context.Background(), "调制解调器", "inventory_item", nil)
	if err != nil || id1 != 10 {
		t.Fatalf("first Resolve: id=%d err=%v", id1, err)
	}
	if creator.called != 1 {
		t.Fatalf("creator called %d times after first resolve, want 1", creator.called)
	}

	id2, err := cr.Resolve(context.Background(), "调制解调器", "inventory_item", nil)
	if err != nil || id2 != 10 {
		t.Fatalf("second Resolve: id=%d err=%v", id2, err)
	}
	if creator.called != 1 {
		t.Fatalf("creator called %d times after second resolve, want still 1 (index hit)", creator.called)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
