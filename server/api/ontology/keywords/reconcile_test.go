package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// fakeEmbeddingClient returns a fixed vector per input text, keyed by exact
// text match -- deterministic and DB-free, so cosine similarity in tests is
// exactly computable by hand.
type fakeEmbeddingClient struct {
	vectors map[string][]float64
}

func (f *fakeEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, t := range texts {
		out[i] = f.vectors[t]
	}
	return out, nil
}

func TestReconcilerMergesCrossLingualProvisional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// One auto-created provisional concept ("亮度") plus one established
	// concept ("Luminance") that is NOT lexically similar (different
	// scripts) but IS semantically close (high cosine similarity) --
	// exactly the Appendix A Stage 4/5 shape tier 6 exists for.
	mock.ExpectQuery(regexp.QuoteMeta(`gloss_source = 'auto:d11'`)).
		WithArgs("_", 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_b", "亮度", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	// ListConcepts runs WHERE scope = $1 AND status IN ('active', 'provisional');
	// the status-IN fragment matches it (ordered mode consumes it before
	// SearchSimilarPrefLabel, which matches the similarity(...) expectation).
	mock.ExpectQuery(regexp.QuoteMeta(`status IN ('active', 'provisional')`)).
		WithArgs("_").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).
			AddRow("kwc_b", "亮度", nil, "_", "provisional", nil, "auto:d11", testNow, testNow).
			AddRow("kwc_l", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	// Lexical blocking finds nothing (disjoint scripts).
	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "亮度", reconcileLexicalBlockMin, "kwc_b", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}))
	// electMergeDirection loads surface counts for both sides.
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_b").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_l").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}).AddRow("kws_l", "kwc_l", "Luminance", "luminance", semid.CurrentNormalizerVersion, "pref", "pref", "und", "_", 1.0, "human:", false, nil, testNow, testNow))
	// MergeConcept(kwc_b -> kwc_l): mergeGuards reads both rows FOR UPDATE,
	// applyMerge tombstones + re-points surfaces, then GetConcept re-reads
	// the survivor. sqlmock's DB handle satisfies txBeginner, so
	// MergeConcept takes the transactional path.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_b").WillReturnRows(sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow("kwc_b", "亮度", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_l").WillReturnRows(sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow("kwc_l", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.semid_never_merge`)).
		WithArgs("keyword", "kwc_b", "kwc_l").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_concepts`)).
		WithArgs("kwc_b", "kwc_l").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_surfaces`)).
		WithArgs("kwc_b", "kwc_l").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_l").WillReturnRows(sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow("kwc_l", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	// Decision log entry.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	r := &Reconciler{
		DB:           db,
		ConceptStore: ConceptStore{DB: db},
		SurfaceStore: SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		Embeddings: &fakeEmbeddingClient{vectors: map[string][]float64{
			"亮度":        {1, 0, 0},
			"Luminance": {0.95, 0.05, 0}, // cosine ~0.998 -- well above tier6EmbeddingMinScore
		}},
		Scope: "_",
	}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Scanned != 1 || stats.Merged != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCosineSimilarity(t *testing.T) {
	if got := cosineSimilarity([]float64{1, 0}, []float64{1, 0}); got != 1 {
		t.Errorf("identical vectors: got %v, want 1", got)
	}
	if got := cosineSimilarity([]float64{1, 0}, []float64{0, 1}); got != 0 {
		t.Errorf("orthogonal vectors: got %v, want 0", got)
	}
}

func TestElectMergeDirectionPrefersRicherConcept(t *testing.T) {
	richer := Concept{ConceptID: "kwc_rich"}
	poorer := Concept{ConceptID: "kwc_poor"}
	absorbed, survivor := electMergeDirection(poorer, richer, 0, 3) // poorer has 0 surfaces, richer has 3
	if absorbed != "kwc_poor" || survivor != "kwc_rich" {
		t.Errorf("expected kwc_poor absorbed into kwc_rich, got absorbed=%s survivor=%s", absorbed, survivor)
	}
}
