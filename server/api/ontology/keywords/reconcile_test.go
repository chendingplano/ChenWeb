package keywords

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

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

// jsonContainsMethod is a sqlmock.Argument matcher for the decision-log
// Input/Output JSON: it verifies the merge method the Reconciler set (e.g.
// "tier6_embedding") is present without pinning the exact float rendering of
// the same blob's score field.
type jsonContainsMethod string

func (m jsonContainsMethod) Match(v driver.Value) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, `"method":`+string(m))
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
	expectKeywordIdentityLock(mock)
	expectKeywordIdentityLock(mock)
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
	// Decision log entry. The Method the Reconciler sets on the entry (the
	// merge evidence that produced it) is carried inside the Input/Output
	// JSON; jsonContainsMethod asserts it names tier 6 without pinning the
	// exact float rendering of the same blob's score field.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs(
			"keyword_reconcile",
			"_",
			jsonContainsMethod(`"tier6_embedding"`),
			jsonContainsMethod(`"tier6_embedding"`),
			"auto_merged",
			nil,
			nil,
			"keyword_reconciler",
			0,
		).
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

// TestReconcilerRunPropagatesDecisionLogError replicates the success-path mock
// sequence from TestReconcilerMergesCrossLingualProvisional through the merge
// (BeginTx ... Commit ... GetConcept survivor) but makes the decision-log
// append fail. The merge's audit row (ADR DR15) must not be silently lost:
// Run must surface the append error and report Merged=0.
func TestReconcilerRunPropagatesDecisionLogError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`gloss_source = 'auto:d11'`)).
		WithArgs("_", 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_b", "亮度", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`status IN ('active', 'provisional')`)).
		WithArgs("_").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).
			AddRow("kwc_b", "亮度", nil, "_", "provisional", nil, "auto:d11", testNow, testNow).
			AddRow("kwc_l", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "亮度", reconcileLexicalBlockMin, "kwc_b", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_b").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_l").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}).AddRow("kws_l", "kwc_l", "Luminance", "luminance", semid.CurrentNormalizerVersion, "pref", "pref", "und", "_", 1.0, "human:", false, nil, testNow, testNow))
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectKeywordIdentityLock(mock)
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
	injected := errors.New("decision log append failed")
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WillReturnError(injected)

	r := &Reconciler{
		DB:           db,
		ConceptStore: ConceptStore{DB: db},
		SurfaceStore: SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		Embeddings: &fakeEmbeddingClient{vectors: map[string][]float64{
			"亮度":        {1, 0, 0},
			"Luminance": {0.95, 0.05, 0},
		}},
		Scope: "_",
	}
	stats, err := r.Run(context.Background())
	if !errors.Is(err, injected) {
		t.Fatalf("expected decision-log append error to propagate, got %v", err)
	}
	// Scanned-before-merged semantics: the candidate is counted Scanned at the
	// top of its loop iteration, before the merge commits, so even a
	// post-commit append failure still reports Scanned=1 with Merged=0.
	if stats.Scanned != 1 || stats.Merged != 0 {
		t.Errorf("expected Scanned=1 and Merged=0 when the audit append fails, got %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestSurfaceCountsPropagatesError covers the surface-count reads that feed
// electMergeDirection: a transient ListSurfacesByConcept failure on either
// side must surface as an error rather than silently flipping the
// absorbed/survivor direction (e.g. absorbing an established concept into an
// auto-created provisional).
func TestSurfaceCountsPropagatesError(t *testing.T) {
	injected := errors.New("surface query failed")

	t.Run("first query errors", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		r := &Reconciler{SurfaceStore: SurfaceStore{DB: db}}
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
			WithArgs("a").WillReturnError(injected)
		_, _, err = r.surfaceCounts(context.Background(), "a", "b")
		if !errors.Is(err, injected) {
			t.Fatalf("expected first-query error to propagate, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("second query errors", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		r := &Reconciler{SurfaceStore: SurfaceStore{DB: db}}
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
			WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
		}))
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
			WithArgs("b").WillReturnError(injected)
		_, _, err = r.surfaceCounts(context.Background(), "a", "b")
		if !errors.Is(err, injected) {
			t.Fatalf("expected second-query error to propagate, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

// TestFindMergeTargetLexicalTier5 covers the tier-5 lexical path through the
// reconciler: a misspelled pref label is blocked by pg_trgm and survives
// fuzzyCandidateScore's guardrails (dist 1 at 9 runes, similarity 8/9 >=
// 0.88), so findMergeTarget returns it as a "tier5_fuzzy" target with no
// embedding evidence at all (the empty liveEmbeds map skips the semantic
// loop entirely).
func TestFindMergeTargetLexicalTier5(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "Luminence", reconcileLexicalBlockMin, "kwc_p", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_est", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))

	cand := Concept{ConceptID: "kwc_p", PrefLabel: "Luminence"}
	live := []Concept{
		cand,
		{ConceptID: "kwc_est", PrefLabel: "Luminance"},
	}
	r := &Reconciler{DB: db, ConceptStore: ConceptStore{DB: db}, Scope: "_"}

	target, method, score, ok, err := r.findMergeTarget(context.Background(), cand, live, map[string][]float64{}, map[string]bool{})
	if err != nil {
		t.Fatalf("findMergeTarget: %v", err)
	}
	if !ok {
		t.Fatalf("expected a lexical tier-5 target for Luminence/Luminance, got none")
	}
	if target.ConceptID != "kwc_est" || method != "tier5_fuzzy" {
		t.Errorf("expected kwc_est via tier5_fuzzy, got target=%s method=%s", target.ConceptID, method)
	}
	if score < 0.88 {
		t.Errorf("expected edit-distance score >= 0.88, got %v", score)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestFindMergeTargetDigitsDifferVeto covers §9.2's digit veto on the
// semantic path: the embedding cosine is well above tier6EmbeddingMinScore,
// but two pref labels that differ in any digit never merge. The veto runs on
// the raw pref labels in this branch (no normalization here, unlike the
// lexical path).
func TestFindMergeTargetDigitsDifferVeto(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// No lexical hit: the digit veto must fire in the semantic loop, not in
	// fuzzyCandidateScore.
	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "Release v2", reconcileLexicalBlockMin, "kwc_p", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}))

	cand := Concept{ConceptID: "kwc_p", PrefLabel: "Release v2"}
	target := Concept{ConceptID: "kwc_t", PrefLabel: "Release v3"}
	live := []Concept{cand, target}
	liveEmbeds := map[string][]float64{
		"kwc_p": {1, 0},
		"kwc_t": {0.99, 0.01}, // cosine ~0.99 -- well above tier6EmbeddingMinScore
	}
	r := &Reconciler{DB: db, ConceptStore: ConceptStore{DB: db}, Scope: "_"}

	_, _, _, ok, err := r.findMergeTarget(context.Background(), cand, live, liveEmbeds, map[string]bool{})
	if err != nil {
		t.Fatalf("findMergeTarget: %v", err)
	}
	if ok {
		t.Errorf("expected the digit veto to block Release v2 vs Release v3 despite high cosine, got a target")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestReconcilerRunCountsSkippedNoCandidate covers the no-target outcome of a
// Run: an auto-created provisional concept with no lexical or semantic match
// is Scanned (counted before the merge) but SkippedNoCandidate, with no merge
// or decision-log write (the mock has no merge/INSERT expectations, so any
// write fails the test).
func TestReconcilerRunCountsSkippedNoCandidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`gloss_source = 'auto:d11'`)).
		WithArgs("_", 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_p", "Lumination", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`status IN ('active', 'provisional')`)).
		WithArgs("_").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_p", "Lumination", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	// Lexical blocking finds no similar label (the concept excludes itself).
	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "Lumination", reconcileLexicalBlockMin, "kwc_p", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}))

	r := &Reconciler{
		DB:           db,
		ConceptStore: ConceptStore{DB: db},
		SurfaceStore: SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		// Embeddings nil: embedConcepts returns an empty map and tier 6 is
		// skipped, so the no-candidate decision rests on the lexical pass alone.
		Scope: "_",
	}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Scanned != 1 || stats.SkippedNoCandidate != 1 || stats.Merged != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestReconcilerRunCountsSkippedVetoed covers the vetoed outcome of a Run: a
// lexical target is found and surfaces counted, but MergeConcept refuses
// (persisted never_merge assertion -> ErrMergeRejected), so Run counts
// SkippedVetoed and skips the decision-log append entirely.
func TestReconcilerRunCountsSkippedVetoed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`gloss_source = 'auto:d11'`)).
		WithArgs("_", 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_p", "Luminence", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`status IN ('active', 'provisional')`)).
		WithArgs("_").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).
			AddRow("kwc_p", "Luminence", nil, "_", "provisional", nil, "auto:d11", testNow, testNow).
			AddRow("kwc_est", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	// Lexical blocking finds the misspelling target.
	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "Luminence", reconcileLexicalBlockMin, "kwc_p", reconcileTopK).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_est", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	// electMergeDirection reads surface counts for both sides (0 each; the
	// equal-count tie-break picks the lexicographically smaller id, so
	// absorbed=kwc_p, survivor=kwc_est).
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_p").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces`)).
		WithArgs("kwc_est").WillReturnRows(sqlmock.NewRows([]string{
		"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
	}))
	// MergeConcept: mergeGuards reads both rows FOR UPDATE, then the persisted
	// never_merge assertion (kwc_est < kwc_p lexicographically, per
	// isNeverMerge's pair normalization) blocks -> ErrMergeRejected, rollback.
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_p").WillReturnRows(sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow("kwc_p", "Luminence", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_est").WillReturnRows(sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow("kwc_est", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.semid_never_merge`)).
		WithArgs("keyword", "kwc_est", "kwc_p").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	r := &Reconciler{
		DB:           db,
		ConceptStore: ConceptStore{DB: db},
		SurfaceStore: SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		Scope:        "_",
	}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Scanned != 1 || stats.SkippedVetoed != 1 || stats.Merged != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestElectMergeDirectionTieBreaks covers the equal-surface tie-breaks of
// electMergeDirection that TestElectMergeDirectionPrefersRicherConcept does
// not: with equal surface counts, the earlier CreateTime survives; with both
// equal, the lexicographically smaller concept_id survives for determinism.
func TestElectMergeDirectionTieBreaks(t *testing.T) {
	early := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	late := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		cand, target Concept
		wantAbsorbed string
		wantSurvivor string
	}{
		{"equal surfaces, cand earlier", Concept{ConceptID: "kwc_a", CreateTime: early}, Concept{ConceptID: "kwc_b", CreateTime: late}, "kwc_b", "kwc_a"},
		{"equal surfaces, target earlier", Concept{ConceptID: "kwc_a", CreateTime: late}, Concept{ConceptID: "kwc_b", CreateTime: early}, "kwc_a", "kwc_b"},
		{"equal surfaces+time, cand lexicographically smaller", Concept{ConceptID: "kwc_a", CreateTime: early}, Concept{ConceptID: "kwc_b", CreateTime: early}, "kwc_b", "kwc_a"},
		{"equal surfaces+time, target lexicographically smaller", Concept{ConceptID: "kwc_b", CreateTime: early}, Concept{ConceptID: "kwc_a", CreateTime: early}, "kwc_b", "kwc_a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			absorbed, survivor := electMergeDirection(tc.cand, tc.target, 0, 0)
			if absorbed != tc.wantAbsorbed || survivor != tc.wantSurvivor {
				t.Errorf("electMergeDirection(%s, %s) = absorbed=%s survivor=%s, want absorbed=%s survivor=%s",
					tc.cand.ConceptID, tc.target.ConceptID, absorbed, survivor, tc.wantAbsorbed, tc.wantSurvivor)
			}
		})
	}
}

func TestCosineSimilarityZeroNorm(t *testing.T) {
	zero := []float64{0, 0}
	if got := cosineSimilarity(zero, []float64{1, 0}); got != 0 {
		t.Errorf("zero-norm first vector: got %v, want 0", got)
	}
	if got := cosineSimilarity([]float64{1, 0}, zero); got != 0 {
		t.Errorf("zero-norm second vector: got %v, want 0", got)
	}
	if got := cosineSimilarity([]float64{1}, []float64{1, 0}); got != 0 {
		t.Errorf("mismatched lengths: got %v, want 0", got)
	}
	if got := cosineSimilarity(nil, nil); got != 0 {
		t.Errorf("empty vectors: got %v, want 0", got)
	}
}

// shortVectorClient simulates an EmbedBatch response shorter than the live
// concept set (e.g. the provider returned fewer vectors than requested).
type shortVectorClient struct{}

func (shortVectorClient) EmbedBatch(_ context.Context, _ []string) ([][]float64, error) {
	return [][]float64{{1, 0}}, nil
}

// TestEmbedConcepts covers embedConcepts' defensive edges: a nil client and
// an empty live set short-circuit to an empty map (no EmbedBatch call), a
// short response maps only the vectors that are actually present, and a nil
// vector entry is dropped.
func TestEmbedConcepts(t *testing.T) {
	ctx := context.Background()
	c1 := Concept{ConceptID: "kwc_1", PrefLabel: "alpha"}
	c2 := Concept{ConceptID: "kwc_2", PrefLabel: "beta"}

	t.Run("nil client short-circuits to empty map", func(t *testing.T) {
		r := &Reconciler{}
		out, err := r.embedConcepts(ctx, []Concept{c1, c2})
		if err != nil {
			t.Fatalf("embedConcepts: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty map for nil client, got %+v", out)
		}
	})

	t.Run("empty live set short-circuits to empty map", func(t *testing.T) {
		r := &Reconciler{Embeddings: &fakeEmbeddingClient{}}
		out, err := r.embedConcepts(ctx, nil)
		if err != nil {
			t.Fatalf("embedConcepts: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected empty map for no live concepts, got %+v", out)
		}
	})

	t.Run("short vector list maps only present vectors", func(t *testing.T) {
		r := &Reconciler{Embeddings: shortVectorClient{}}
		out, err := r.embedConcepts(ctx, []Concept{c1, c2})
		if err != nil {
			t.Fatalf("embedConcepts: %v", err)
		}
		if len(out) != 1 {
			t.Errorf("expected only the first concept to map, got %+v", out)
		}
		if _, ok := out["kwc_1"]; !ok {
			t.Errorf("expected kwc_1 to have a vector, got %+v", out)
		}
		if _, ok := out["kwc_2"]; ok {
			t.Errorf("expected kwc_2 to be absent (short response), got %+v", out)
		}
	})

	t.Run("nil vectors skipped", func(t *testing.T) {
		r := &Reconciler{Embeddings: &fakeEmbeddingClient{vectors: map[string][]float64{
			"alpha": {1, 0}, // "beta" missing -> nil vector
		}}}
		out, err := r.embedConcepts(ctx, []Concept{c1, c2})
		if err != nil {
			t.Fatalf("embedConcepts: %v", err)
		}
		if len(out) != 1 {
			t.Errorf("expected one mapped vector, got %+v", out)
		}
		if _, ok := out["kwc_1"]; !ok {
			t.Errorf("expected kwc_1 mapped, got %+v", out)
		}
	})
}
