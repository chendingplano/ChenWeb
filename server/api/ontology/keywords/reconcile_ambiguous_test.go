package keywords

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// fakeEmbedder returns a fixed vector per input text (by exact match),
// letting each test control cosine similarity directly instead of reasoning
// about a real model's output.
type fakeEmbedder struct {
	vectors map[string][]float64
	err     error
}

func (f *fakeEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float64, len(texts))
	for i, t := range texts {
		v, ok := f.vectors[t]
		if !ok {
			v = []float64{0, 0}
		}
		out[i] = v
	}
	return out, nil
}

func liveCandidate(id, prefLabel string) liveAmbiguousCandidate {
	return liveAmbiguousCandidate{
		Original: TiedCandidate{ConceptID: id, Score: 0.8, Method: "tier1_norm"},
		Concept:  Concept{ConceptID: id, PrefLabel: prefLabel, Status: "active"},
	}
}

func TestRankAmbiguousCandidatesAppliesOnClearMargin(t *testing.T) {
	r := &Reconciler{Embeddings: &fakeEmbedder{vectors: map[string][]float64{
		"ML":               {1, 0},
		"machine learning": {1, 0}, // identical direction -> cosine 1.0
		"millilitre":       {0, 1}, // orthogonal -> cosine 0.0
	}}}
	live := []liveAmbiguousCandidate{
		liveCandidate("kwc_ml", "machine learning"),
		liveCandidate("kwc_mil", "millilitre"),
	}
	audit := &ambiguousDecisionAudit{Candidates: []ambiguousCandidateEvidence{
		{ConceptID: "kwc_ml"}, {ConceptID: "kwc_mil"},
	}}

	winner, method, err := r.rankAmbiguousCandidates(context.Background(), "ML", live, audit)
	if err != nil {
		t.Fatalf("rankAmbiguousCandidates: %v", err)
	}
	if winner != "kwc_ml" || method != methodAmbiguousMargin {
		t.Errorf("got winner=%q method=%q, want kwc_ml/%s", winner, method, methodAmbiguousMargin)
	}
}

func TestRankAmbiguousCandidatesDefersWithoutMarginOrCorroboration(t *testing.T) {
	// Both candidates score close together and neither's label survives
	// tier 5's own edit-distance guardrails against "ML" -> no signal wins.
	r := &Reconciler{Embeddings: &fakeEmbedder{vectors: map[string][]float64{
		"ML":                 {1, 0.05},
		"machine learning":   {1, 0},
		"medical laboratory": {1, 0.1},
	}}}
	live := []liveAmbiguousCandidate{
		liveCandidate("kwc_ml", "machine learning"),
		liveCandidate("kwc_medlab", "medical laboratory"),
	}
	audit := &ambiguousDecisionAudit{Candidates: []ambiguousCandidateEvidence{
		{ConceptID: "kwc_ml"}, {ConceptID: "kwc_medlab"},
	}}

	winner, method, err := r.rankAmbiguousCandidates(context.Background(), "ML", live, audit)
	if err != nil {
		t.Fatalf("rankAmbiguousCandidates: %v", err)
	}
	if winner != "" || method != "" {
		t.Errorf("expected no auto-apply (neither margin nor corroboration cleared), got winner=%q method=%q", winner, method)
	}
	// Evidence must still be recorded for the human/next-run review, even
	// though nothing was applied -- D11's attributability requirement.
	for _, c := range audit.Candidates {
		if c.EmbeddingScore == nil {
			t.Errorf("expected an embedding score recorded for %s even on defer", c.ConceptID)
		}
	}
}

func TestRankAmbiguousCandidatesLexicalCorroborationAppliesBelowMargin(t *testing.T) {
	// A tiny margin alone would not clear defaultAmbiguousMargin, but the
	// top pick's label is also the one candidate that survives tier 5's own
	// edit-distance check against the query -- independent corroboration
	// should be enough on its own.
	r := &Reconciler{Embeddings: &fakeEmbedder{vectors: map[string][]float64{
		"kubernets":   {1, 0.1},
		"kubernetes":  {1, 0.09}, // nearly identical direction, tiny margin
		"kubecontrol": {1, 0.08},
	}}}
	live := []liveAmbiguousCandidate{
		liveCandidate("kwc_k8s", "kubernetes"),   // edit distance 1 from "kubernets"
		liveCandidate("kwc_kctl", "kubecontrol"), // not lexically close
	}
	audit := &ambiguousDecisionAudit{Candidates: []ambiguousCandidateEvidence{
		{ConceptID: "kwc_k8s"}, {ConceptID: "kwc_kctl"},
	}}

	winner, method, err := r.rankAmbiguousCandidates(context.Background(), "kubernets", live, audit)
	if err != nil {
		t.Fatalf("rankAmbiguousCandidates: %v", err)
	}
	if winner != "kwc_k8s" || method != methodAmbiguousLexical {
		t.Errorf("got winner=%q method=%q, want kwc_k8s/%s", winner, method, methodAmbiguousLexical)
	}
}

func TestRankAmbiguousCandidatesConfigurableMargin(t *testing.T) {
	// Same fixture as the "defers" test, but with the margin lowered enough
	// that the same score gap now clears it -- proves the threshold is
	// actually read from the struct field, not hardcoded.
	r := &Reconciler{
		Embeddings: &fakeEmbedder{vectors: map[string][]float64{
			"ML":               {1, 0},
			"machine learning": {1, 0.03},
			"millilitre":       {1, 0.3},
		}},
		AmbiguousMarginThreshold: 0.01,
	}
	live := []liveAmbiguousCandidate{
		liveCandidate("kwc_ml", "machine learning"),
		liveCandidate("kwc_mil", "millilitre"),
	}
	audit := &ambiguousDecisionAudit{Candidates: []ambiguousCandidateEvidence{
		{ConceptID: "kwc_ml"}, {ConceptID: "kwc_mil"},
	}}

	winner, _, err := r.rankAmbiguousCandidates(context.Background(), "ML", live, audit)
	if err != nil {
		t.Fatalf("rankAmbiguousCandidates: %v", err)
	}
	if winner != "kwc_ml" {
		t.Errorf("expected the configured 0.01 margin to apply, got winner=%q", winner)
	}
}

func TestRankAmbiguousCandidatesPropagatesEmbeddingError(t *testing.T) {
	r := &Reconciler{Embeddings: &fakeEmbedder{err: context.DeadlineExceeded}}
	live := []liveAmbiguousCandidate{liveCandidate("kwc_a", "a")}
	audit := &ambiguousDecisionAudit{Candidates: []ambiguousCandidateEvidence{{ConceptID: "kwc_a"}}}

	if _, _, err := r.rankAmbiguousCandidates(context.Background(), "q", live, audit); err == nil {
		t.Error("expected the embedding client's error to propagate")
	}
}

// mustValidReconciler returns a Reconciler whose validateConfiguration
// passes with no evidence providers and no alignment gate wired -- these
// decideAmbiguous tests exercise ambiguity re-ranking, not tier 6's merge
// path, so they stub the alignment-store requirement with a non-nil DB the
// same way other reconcile_test.go fixtures do.
func newAmbiguousTestReconciler(db *sql.DB) *Reconciler {
	return &Reconciler{
		DB: db,
		ConceptStore: ConceptStore{
			DB: db,
			Alignments: AlignmentsStore{
				Assertions: assertions.AssertionStore{DB: db},
			},
		},
		Scope: "_",
	}
}

func TestDecideAmbiguousNoActiveCandidateLeavesRowPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := newAmbiguousTestReconciler(db)
	ctx := context.Background()

	row := Unresolved{
		NormKey:  "ml",
		Scope:    "_",
		Surfaces: []string{"ML"},
		Candidates: []TiedCandidate{
			{ConceptID: "kwc_a", Score: 0.8, Method: "tier1_norm"},
			{ConceptID: "kwc_b", Score: 0.8, Method: "tier1_norm"},
		},
	}

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	// FollowMerge(kwc_a): no merged_into.
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRows("kwc_a", "A Label", "_", "provisional", "auto:d11"))
	// Explicit re-fetch for the live-candidate record: provisional, filtered out.
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRows("kwc_a", "A Label", "_", "provisional", "auto:d11"))
	// FollowMerge(kwc_b): no merged_into.
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRows("kwc_b", "B Label", "_", "provisional", "auto:d11"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRows("kwc_b", "B Label", "_", "provisional", "auto:d11"))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword_reconcile_ambiguous", "_", sqlmock.AnyArg(), sqlmock.AnyArg(),
			ambiguousOutcomeNoCandidate, nil, nil, "keyword_reconciler_ambiguous", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_unresolved`)).
		WithArgs("ml", "_", "pending", "keyword_reconciler_ambiguous").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	outcome, err := r.decideAmbiguous(ctx, row)
	if err != nil {
		t.Fatalf("decideAmbiguous: %v", err)
	}
	if outcome != ambiguousOutcomeNoCandidate {
		t.Errorf("got outcome %q, want %q", outcome, ambiguousOutcomeNoCandidate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDecideAmbiguousDefersWhenNeitherSignalClears(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := newAmbiguousTestReconciler(db)
	r.Embeddings = &fakeEmbedder{vectors: map[string][]float64{
		"ML":                 {1, 0.05},
		"machine learning":   {1, 0},
		"medical laboratory": {1, 0.1},
	}}
	r.EmbeddingModelID = "test-model"
	ctx := context.Background()

	row := Unresolved{
		NormKey:  "ml",
		Scope:    "_",
		Surfaces: []string{"ML"},
		Candidates: []TiedCandidate{
			{ConceptID: "kwc_ml", Score: 0.8, Method: "tier1_norm"},
			{ConceptID: "kwc_medlab", Score: 0.8, Method: "tier1_norm"},
		},
	}

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_ml").
		WillReturnRows(conceptRows("kwc_ml", "machine learning", "_", "active", "human:tester"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_ml").
		WillReturnRows(conceptRows("kwc_ml", "machine learning", "_", "active", "human:tester"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_medlab").
		WillReturnRows(conceptRows("kwc_medlab", "medical laboratory", "_", "active", "human:tester"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_medlab").
		WillReturnRows(conceptRows("kwc_medlab", "medical laboratory", "_", "active", "human:tester"))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword_reconcile_ambiguous", "_", sqlmock.AnyArg(), sqlmock.AnyArg(),
			ambiguousOutcomeDeferred, "test-model", nil, "keyword_reconciler_ambiguous", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_unresolved`)).
		WithArgs("ml", "_", "pending", "keyword_reconciler_ambiguous").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	outcome, err := r.decideAmbiguous(ctx, row)
	if err != nil {
		t.Fatalf("decideAmbiguous: %v", err)
	}
	if outcome != ambiguousOutcomeDeferred {
		t.Errorf("got outcome %q, want %q", outcome, ambiguousOutcomeDeferred)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDecideAmbiguousCollapsedWritesWinnerSurface proves the full write
// path: when the active-only filter leaves exactly one live survivor, that
// survivor is applied directly (no embedding call needed) -- a new `alt`
// surface is written for the query, a decision is logged, and the backlog
// row is marked resolved. Surface-key expectations are computed from the
// real normalizer/derivedSurfaceKeys, not hand-derived, so this test can't
// drift from CreateSurface's actual behavior.
func TestDecideAmbiguousCollapsedWritesWinnerSurface(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := newAmbiguousTestReconciler(db)
	ctx := context.Background()

	const query = "ML"
	row := Unresolved{
		NormKey:  "ml",
		Scope:    "_",
		Surfaces: []string{query},
		Candidates: []TiedCandidate{
			{ConceptID: "kwc_a", Score: 0.8, Method: "tier1_norm"},
			{ConceptID: "kwc_b", Score: 0.8, Method: "tier1_norm"},
		},
	}

	sf := Surface{ConceptID: "kwc_a", Surface: query, LabelRole: "alt"}
	sf.SurfaceID = deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)
	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize(query)
	derivedKeys := derivedSurfaceKeys(ks, sf.SurfaceID, semid.CurrentNormalizerVersion)

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	// kwc_a survives the active filter.
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRows("kwc_a", "Machine Learning", "_", "active", "human:tester"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_a").
		WillReturnRows(conceptRows("kwc_a", "Machine Learning", "_", "active", "human:tester"))
	// kwc_b is filtered out (still provisional).
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRows("kwc_b", "Millilitre", "_", "provisional", "auto:d11"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_b").
		WillReturnRows(conceptRows("kwc_b", "Millilitre", "_", "provisional", "auto:d11"))

	// applyAmbiguousWinner -> CreateSurface, reusing tx (no nested BEGIN;
	// withKeywordIdentityMutation detects the *sql.Tx and re-acquires the
	// lock in place rather than opening a new transaction).
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(sf.SurfaceID, "kwc_a", query, ks.Norm, semid.CurrentNormalizerVersion,
			"alt", "synonym", "und", "_", 0.9, "reconcile:ambiguous", false, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(sf.SurfaceID, "kwc_a", query, ks.Norm, semid.CurrentNormalizerVersion,
			"alt", "synonym", "und", "_", 0.9, "reconcile:ambiguous", false, nil, testNow, testNow))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.keyword_surface_keys WHERE surface_id = $1`)).
		WithArgs(sf.SurfaceID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, k := range derivedKeys {
		mock.ExpectExec(regexp.QuoteMeta(
			`INSERT INTO kb.keyword_surface_keys (surface_id, key_kind, key_value, norm_version)`)).
			WithArgs(sf.SurfaceID, k.KeyKind, k.KeyValue, semid.CurrentNormalizerVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword_reconcile_ambiguous", "_", sqlmock.AnyArg(), sqlmock.AnyArg(),
			ambiguousOutcomeCollapsed, nil, nil, "keyword_reconciler_ambiguous", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_unresolved`)).
		WithArgs("ml", "_", "resolved", "keyword_reconciler_ambiguous").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	outcome, err := r.decideAmbiguous(ctx, row)
	if err != nil {
		t.Fatalf("decideAmbiguous: %v", err)
	}
	if outcome != ambiguousOutcomeCollapsed {
		t.Errorf("got outcome %q, want %q", outcome, ambiguousOutcomeCollapsed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
