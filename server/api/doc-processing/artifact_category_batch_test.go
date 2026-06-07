package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func emptyCategoryLoadRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"})
}

// ResolveBatch dedups the same key (across casing/whitespace) to a single create and
// maps every surface form to the minted id.
func TestResolveBatchDedupCreatesOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(emptyCategoryLoadRows())
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))
	mock.ExpectExec("UPDATE kb\\.artifact_categories"). // absorbAlias for normKey
		WillReturnResult(sqlmock.NewResult(0, 1))

	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}

	reqs := []categoryRequest{
		{RawKey: "Throughput"},
		{RawKey: "throughput"},
		{RawKey: "  THROUGHPUT "},
	}
	ids, errs := cr.ResolveBatch(context.Background(), "metric", reqs, 4)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if creator.called != 1 {
		t.Fatalf("creator called %d times, want 1", creator.called)
	}
	if ids["throughput"] != 42 {
		t.Fatalf("ids[throughput] = %d, want 42", ids["throughput"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// ResolveBatch matches a pre-existing category from the index and does not create.
func TestResolveBatchMatchesExistingWithoutCreate(t *testing.T) {
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

	ids, errs := cr.ResolveBatch(context.Background(), "metric", []categoryRequest{{RawKey: "Latency"}}, 4)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if creator.called != 0 {
		t.Fatalf("creator called %d times, want 0", creator.called)
	}
	if ids["latency"] != 7 {
		t.Fatalf("ids[latency] = %d, want 7", ids["latency"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// ResolveBatch creates distinct misses independently (no clustering).
func TestResolveBatchCreatesDistinctMissesIndependently(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(emptyCategoryLoadRows())
	// Two inserts expected — one per distinct miss.
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(10)))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(11)))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WillReturnResult(sqlmock.NewResult(0, 1))

	calls := 0
	creator := &fakeCategoryCreatorFn{fn: func(key string) createdCategory {
		calls++
		return createdCategory{CategoryKey: key}
	}}
	cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}

	reqs := []categoryRequest{{RawKey: "latency"}, {RawKey: "throughput"}}
	ids, errs := cr.ResolveBatch(context.Background(), "metric", reqs, 1) // serial for determinism
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if calls != 2 {
		t.Fatalf("creator called %d times, want 2", calls)
	}
	if ids["latency"] == 0 || ids["throughput"] == 0 {
		t.Fatalf("ids = %v, want both non-zero", ids)
	}
}

// fakeCategoryCreatorFn drives the creator with a per-key function, used when tests
// need to return different categories for different keys.
type fakeCategoryCreatorFn struct {
	fn func(key string) createdCategory
}

func (f *fakeCategoryCreatorFn) CreateCategory(_ context.Context, rawKey, _ string, _ map[string]any) (createdCategory, error) {
	return f.fn(rawKey), nil
}
