package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeCategoryCreator struct {
	called int
	out    createdCategory
	err    error
}

func (f *fakeCategoryCreator) CreateCategory(_ context.Context, _ string, _ string, _ map[string]any) (createdCategory, error) {
	f.called++
	return f.out, f.err
}

type fakeCategoryEmbedder struct {
	vec []float64
	ok  bool
}

func (f fakeCategoryEmbedder) EmbedCategory(_ context.Context, _ string) ([]float64, bool) {
	return f.vec, f.ok
}

func newLoadRows(id int64, key, status, matchKeys string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "embedding"}).
		AddRow(id, key, status, "", []byte(matchKeys), []byte(`[]`))
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
	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, creator, fakeCategoryEmbedder{ok: false}, 0.8)
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
		WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "embedding"}))
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))

	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, creator, fakeCategoryEmbedder{ok: false}, 0.8)
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
		WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "embedding"}))

	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, nil, fakeCategoryEmbedder{ok: false}, 0.8)
	if _, err := cr.Resolve(context.Background(), "throughput", "metric", nil); err == nil {
		t.Fatal("expected error when no creator is configured and category is missing")
	}
}

func TestResolverCachesSnapshotAcrossCalls(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	// Only ONE load expected even though Resolve is called twice.
	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(7), "latency").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(7), "latency").
		WillReturnResult(sqlmock.NewResult(0, 1))

	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, &fakeCategoryCreator{}, fakeCategoryEmbedder{ok: false}, 0.8)
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
