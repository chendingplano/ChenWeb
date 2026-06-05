package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// programmableCategoryEmbedder returns a caller-specified vector per text, so tests
// can control which keys are semantic neighbors during clustering.
type programmableCategoryEmbedder struct {
	vecs map[string][]float64
}

func (p programmableCategoryEmbedder) EmbedCategory(_ context.Context, text string) ([]float64, bool) {
	v, ok := p.vecs[text]
	return v, ok
}

func emptyCategoryLoadRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "embedding"})
}

// clusterCategoryMisses groups near-identical embeddings, keeps dissimilar ones apart,
// and gives un-embeddable keys their own cluster.
func TestClusterCategoryMisses(t *testing.T) {
	vecs := [][]float64{
		{1, 0},       // 0: a
		{0.99, 0.01}, // 1: ~a (cosine > 0.8 with 0)
		{0, 1},       // 2: orthogonal to a
		nil,          // 3: no embedding -> own cluster
	}
	clusters := clusterCategoryMisses(vecs, 0.8)
	if len(clusters) != 3 {
		t.Fatalf("clusters = %v, want 3 clusters", clusters)
	}
	if len(clusters[0]) != 2 || clusters[0][0] != 0 || clusters[0][1] != 1 {
		t.Fatalf("clusters[0] = %v, want [0 1]", clusters[0])
	}
	if len(clusters[1]) != 1 || clusters[1][0] != 2 {
		t.Fatalf("clusters[1] = %v, want [2]", clusters[1])
	}
	if len(clusters[2]) != 1 || clusters[2][0] != 3 {
		t.Fatalf("clusters[2] = %v, want [3]", clusters[2])
	}
}

// ResolveBatch dedups the same key (across casing/whitespace and across metrics) to a
// single create and maps every surface form to the minted id.
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

	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, creator, fakeCategoryEmbedder{ok: false}, 0.8)

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

// Two novel keys that embed as neighbors collapse to one create (Phase 3a); the
// non-representative is absorbed as an alias and mapped to the same id.
func TestResolveBatchClustersSynonymsIntoOneCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(emptyCategoryLoadRows())
	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(50)))
	// The cluster's second member is absorbed as an alias of the created row.
	mock.ExpectExec("UPDATE kb\\.artifact_categories").
		WithArgs(int64(50), "response latency").
		WillReturnResult(sqlmock.NewResult(0, 1))

	emb := programmableCategoryEmbedder{vecs: map[string][]float64{
		"response time":    {1, 0.001},
		"response latency": {1, 0.0},
	}}
	creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "response time"}}
	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, creator, emb, 0.8)

	reqs := []categoryRequest{
		{RawKey: "Response Time"},
		{RawKey: "response latency"},
	}
	// maxConcurrency=1 keeps the single cluster's create→absorb order deterministic.
	ids, errs := cr.ResolveBatch(context.Background(), "metric", reqs, 1)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if creator.called != 1 {
		t.Fatalf("creator called %d times, want 1", creator.called)
	}
	if ids["response time"] != 50 || ids["response latency"] != 50 {
		t.Fatalf("ids = %v, want both = 50", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// A pre-existing category short-circuits create: the batch matches it in the snapshot
// and only absorbs the new surface form.
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
	cr := newCategoryResolver(artifactCategoryRegistry{DB: db}, creator, fakeCategoryEmbedder{ok: false}, 0.8)

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
