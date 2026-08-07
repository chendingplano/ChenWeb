package keywords

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

func authoritativeClaim(candidate, target string) IdentityClaim {
	ref := EvidenceRef{Key: "authority", Kind: "authority_configuration", Locator: "fixture:authority"}
	return IdentityClaim{ProviderID: TripleEvidenceProviderID, CandidateConceptID: candidate, TargetConceptID: target, Relation: IdentityRelationExactEquivalent, Authority: IdentityAuthorityAuthoritative, AuthorityRef: ref.Key, EvidenceRefs: []EvidenceRef{ref}}
}

func TestCandidateDecisionTable(t *testing.T) {
	tests := []struct {
		name        string
		tier5       tier5Result
		claims      []IdentityClaim
		proposals   []ReconcileProposal
		wantOutcome string
		wantTarget  string
	}{
		{"tier5 unique", tier5Result{State: tier5Unique, TargetConceptID: "a"}, nil, nil, outcomeValidate, "a"},
		{"tier5 same authority", tier5Result{State: tier5Unique, TargetConceptID: "a"}, []IdentityClaim{authoritativeClaim("c", "a")}, nil, outcomeValidate, "a"},
		{"tier5 contradictory authority", tier5Result{State: tier5Unique, TargetConceptID: "a"}, []IdentityClaim{authoritativeClaim("c", "b")}, nil, outcomeReject, ""},
		{"tier5 multiple authority", tier5Result{State: tier5Unique, TargetConceptID: "a"}, []IdentityClaim{authoritativeClaim("c", "a"), authoritativeClaim("c", "b")}, nil, outcomeReject, ""},
		{"tie one authority", tier5Result{State: tier5Tie}, []IdentityClaim{authoritativeClaim("c", "a")}, nil, outcomeValidate, "a"},
		{"tie multiple authority", tier5Result{State: tier5Tie}, []IdentityClaim{authoritativeClaim("c", "a"), authoritativeClaim("c", "b")}, nil, outcomeReject, ""},
		{"tie no authority", tier5Result{State: tier5Tie}, nil, nil, outcomeDeferred, ""},
		{"none one authority", tier5Result{State: tier5None}, []IdentityClaim{authoritativeClaim("c", "a")}, nil, outcomeValidate, "a"},
		{"none multiple authority", tier5Result{State: tier5None}, []IdentityClaim{authoritativeClaim("c", "a"), authoritativeClaim("c", "b")}, nil, outcomeReject, ""},
		{"nonauthorizing evidence defers", tier5Result{State: tier5None}, []IdentityClaim{{TargetConceptID: "a", Relation: IdentityRelationRelated, Authority: IdentityAuthorityNonAuthoritative}}, nil, outcomeDeferred, ""},
		{"exact nonauthoritative evidence defers", tier5Result{State: tier5None}, []IdentityClaim{{TargetConceptID: "a", Relation: IdentityRelationExactEquivalent, Authority: IdentityAuthorityNonAuthoritative}}, nil, outcomeDeferred, ""},
		{"targetless evidence defers", tier5Result{State: tier5None}, []IdentityClaim{{Relation: IdentityRelationExactEquivalent, Authority: IdentityAuthorityNonAuthoritative}}, nil, outcomeDeferred, ""},
		{"embedding only defers", tier5Result{State: tier5None}, nil, []ReconcileProposal{{TargetConceptID: "a", Methods: []ReconcileProposalMethod{{Method: methodEmbedding}}}}, outcomeDeferred, ""},
		{"nothing", tier5Result{State: tier5None}, nil, nil, outcomeNoCandidate, ""},
	}
	active := map[string]Concept{"a": {ConceptID: "a", Status: "active"}, "b": {ConceptID: "b", Status: "active"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseCandidateDecision(tt.tier5, tt.claims, tt.proposals, active)
			if got.Outcome != tt.wantOutcome || got.TargetConceptID != tt.wantTarget {
				t.Fatalf("got outcome=%s target=%s, want %s/%s", got.Outcome, got.TargetConceptID, tt.wantOutcome, tt.wantTarget)
			}
		})
	}
}

func TestAllNonAuthorizingRelationsDefer(t *testing.T) {
	for _, relation := range []IdentityRelation{IdentityRelationRelated, IdentityRelationBroader, IdentityRelationNarrower, IdentityRelationTranslation, IdentityRelationProbabilistic, IdentityRelationOther} {
		t.Run(string(relation), func(t *testing.T) {
			decision := chooseCandidateDecision(tier5Result{}, []IdentityClaim{{TargetConceptID: "a", Relation: relation, Authority: IdentityAuthorityNonAuthoritative}}, nil, map[string]Concept{"a": {ConceptID: "a", Status: "active"}})
			if decision.Outcome != outcomeDeferred {
				t.Fatalf("got %s", decision.Outcome)
			}
		})
	}
}

func TestEmbeddingScoreNeverAuthorizesAndProviderRankNeverTruncates(t *testing.T) {
	active := map[string]Concept{"outside": {ConceptID: "outside", Status: "active"}}
	high := 0.999999
	embeddingOnly := chooseCandidateDecision(tier5Result{}, nil, []ReconcileProposal{{TargetConceptID: "wrong", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 1, Score: &high}}}}, active)
	if embeddingOnly.Outcome != outcomeDeferred {
		t.Fatalf("high cosine authorized: %+v", embeddingOnly)
	}
	low := -0.75
	providerOnly := chooseCandidateDecision(tier5Result{}, []IdentityClaim{authoritativeClaim("c", "outside")}, []ReconcileProposal{{TargetConceptID: "wrong", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 10, Score: &low}}}}, active)
	if providerOnly.Outcome != outcomeValidate || providerOnly.TargetConceptID != "outside" {
		t.Fatalf("provider-only target lost: %+v", providerOnly)
	}
	for _, status := range []string{"provisional", "merged", "deprecated"} {
		inactive := map[string]Concept{"outside": {ConceptID: "outside", Status: status}}
		got := chooseCandidateDecision(tier5Result{}, []IdentityClaim{authoritativeClaim("c", "outside")}, nil, inactive)
		if got.Outcome != outcomeDeferred {
			t.Fatalf("%s authoritative target got %+v", status, got)
		}
	}
}

func TestCrossScopeAuthorityIsCandidateGlobalThenPairVetoed(t *testing.T) {
	claims := []IdentityClaim{authoritativeClaim("candidate", "cross")}
	globalActive := map[string]Concept{"cross": {ConceptID: "cross", Status: "active", Scope: "other"}}
	contradiction := chooseCandidateDecision(tier5Result{State: tier5Unique, TargetConceptID: "local"}, claims, nil, globalActive)
	if contradiction.Outcome != outcomeReject {
		t.Fatalf("Tier5 cross-scope contradiction was ignored: %+v", contradiction)
	}
	chosen := chooseCandidateDecision(tier5Result{}, claims, nil, globalActive)
	if chosen.Outcome != outcomeValidate || chosen.TargetConceptID != "cross" {
		t.Fatalf("cross-scope authority did not reach pair validation: %+v", chosen)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	expectNeverMergeFalse(mock, "candidate", "cross")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "cross")
	r := Reconciler{Scope: "_", ConceptStore: ConceptStore{Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
	vetoes, err := r.pairVetoes(context.Background(), tx, Concept{ConceptID: "candidate", Status: "provisional", GlossSource: "auto:d11", Scope: "_"}, globalActive["cross"], nil, nil, candidateDecision{Method: methodIdentity, TargetConceptID: "cross"}, claims, globalActive)
	if err != nil || !contains(strings.Join(vetoes, ","), "scope_conflict") {
		t.Fatalf("vetoes=%v err=%v", vetoes, err)
	}
	mock.ExpectRollback()
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPositiveAuthorityHardVetoes(t *testing.T) {
	tests := []struct {
		name                              string
		candidateSurfaces, targetSurfaces []Surface
		neverMerge, alignmentConflict     bool
		want                              string
	}{
		{name: "absorbed surface lock", candidateSurfaces: []Surface{{Surface: "metric", Locked: true}}, want: "absorbed_surface_locked"},
		{name: "digit set", candidateSurfaces: []Surface{{Surface: "metric 2"}}, targetSurfaces: []Surface{{Surface: "metric 3"}}, want: "digit_conflict"},
		{name: "never merge", neverMerge: true, want: "never_merge"},
		{name: "alignment", alignmentConflict: true, want: "alignment_conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("FROM kb.semid_never_merge").WithArgs("keyword", "candidate", "target").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.neverMerge))
			if tt.alignmentConflict {
				expectAlignment(mock, "candidate", "term-a")
				expectAlignment(mock, "target", "term-b")
			} else {
				expectNoAlignment(mock, "candidate")
				expectNoAlignment(mock, "target")
			}
			claim := authoritativeClaim("candidate", "target")
			active := map[string]Concept{"target": {ConceptID: "target", Status: "active", Scope: "_"}}
			r := Reconciler{Scope: "_", ConceptStore: ConceptStore{Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
			vetoes, err := r.pairVetoes(context.Background(), tx, Concept{ConceptID: "candidate", Status: "provisional", GlossSource: "auto:d11", Scope: "_"}, active["target"], tt.candidateSurfaces, tt.targetSurfaces, candidateDecision{Method: methodIdentity, TargetConceptID: "target"}, []IdentityClaim{claim}, active)
			if err != nil || !contains(strings.Join(vetoes, ","), tt.want) {
				t.Fatalf("vetoes=%v err=%v", vetoes, err)
			}
			mock.ExpectRollback()
			_ = tx.Rollback()
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuditProposalsDedupeTargetsAndAreByteStableAcrossInputOrder(t *testing.T) {
	score := .42
	embedding := ReconcileProposal{TargetConceptID: "target", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 2, Score: &score, ModelID: "m", CandidateSetSize: 11, CandidateSetLimit: 10}}, Claims: []IdentityClaim{}}
	scoreZ := .8
	embeddingZ := ReconcileProposal{TargetConceptID: "z", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 1, Score: &scoreZ, ModelID: "m", CandidateSetSize: 11, CandidateSetLimit: 10}}, Claims: []IdentityClaim{}}
	claim := authoritativeClaim("candidate", "target")
	other := IdentityClaim{ProviderID: TripleEvidenceProviderID, CandidateConceptID: "candidate", TargetConceptID: "z", Relation: IdentityRelationExactEquivalent, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "z", Kind: "mapping", Locator: "fixture:z"}}}
	tier5 := tier5Result{Scored: []tier5ScoredTarget{{ConceptID: "target", Score: .9}}}
	one := mergeAuditProposals([]ReconcileProposal{embedding, embeddingZ}, tier5, []IdentityClaim{claim, other})
	two := mergeAuditProposals([]ReconcileProposal{embeddingZ, embedding}, tier5, []IdentityClaim{other, claim})
	left, _ := json.Marshal(one)
	right, _ := json.Marshal(two)
	if string(left) != string(right) {
		t.Fatalf("audit changed with claim order:\n%s\n%s", left, right)
	}
	if len(one) != 2 || one[0].TargetConceptID != "target" || len(one[0].Methods) != 3 || len(one[0].Claims) != 1 {
		t.Fatalf("target aggregation failed: %+v", one)
	}
}

func TestTier5FourCharacterBoundaryAndTie(t *testing.T) {
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	active := []Concept{{ConceptID: "a", PrefLabel: "kuber", Status: "active"}, {ConceptID: "b", PrefLabel: "kuberx", Status: "active"}}
	if got := recomputeTier5("kube", active, n); got.State != tier5None {
		t.Fatalf("four characters must be excluded: %+v", got)
	}
	if got := recomputeTier5("kubers", active, n); got.State != tier5Tie {
		t.Fatalf("equal top score must tie: %+v", got)
	}
	near := []tier5ScoredTarget{{ConceptID: "a", Score: .9000000000}, {ConceptID: "b", Score: .8999999995}}
	if got := selectTier5(near); got.State != tier5Tie {
		t.Fatalf("scores within 1e-9 must tie: %+v", got)
	}
}

func TestTier5SortIsExactAndTieEpsilonOnlyAppliesToTopTwo(t *testing.T) {
	input := []tier5ScoredTarget{{ConceptID: "c", Score: 0.9000000000}, {ConceptID: "a", Score: 0.9000000015}, {ConceptID: "b", Score: 0.9000000008}}
	for _, shuffled := range [][]tier5ScoredTarget{input, {input[2], input[0], input[1]}, {input[1], input[2], input[0]}} {
		got := selectTier5(shuffled)
		if got.State != tier5Tie {
			t.Fatalf("expected top-two epsilon tie: %+v", got)
		}
		if got.Scored[0].ConceptID != "a" || got.Scored[1].ConceptID != "b" || got.Scored[2].ConceptID != "c" {
			t.Fatalf("non-exact/transitive ordering: %+v", got.Scored)
		}
	}
}

func TestAllSurfaceDigitSignatureVeto(t *testing.T) {
	surfaces := func(values ...string) []Surface {
		out := make([]Surface, len(values))
		for i, value := range values {
			out[i].Surface = value
		}
		return out
	}
	if !equalDigitSignatureSets(surfaces("v2", "release 3"), surfaces("版本２", "release-3")) {
		t.Fatal("equivalent all-surface digit sets rejected")
	}
	if equalDigitSignatureSets(surfaces("v2", "release 3"), surfaces("v2")) {
		t.Fatal("missing digit-bearing surface was not vetoed")
	}
	if equalDigitSignatureSets(surfaces("v2"), nil) {
		t.Fatal("one-sided digits were not vetoed")
	}
}

type orderedEmbeddingClient struct {
	vectors [][]float64
	err     error
}

func (f orderedEmbeddingClient) EmbedBatch(context.Context, []string) ([][]float64, error) {
	return f.vectors, f.err
}

func TestEmbeddingTopTenBoundAndAffineScaleIndependence(t *testing.T) {
	concepts := []Concept{{ConceptID: "candidate", PrefLabel: "candidate", Status: "provisional"}}
	for i := 1; i <= 11; i++ {
		concepts = append(concepts, Concept{ConceptID: string(rune('a' + i - 1)), PrefLabel: "target", Status: "active"})
	}
	vecs := make(map[string][]float64)
	vecs["candidate"] = []float64{1, 0}
	for i := 1; i <= 11; i++ {
		angle := float64(i) * .04
		vecs[concepts[i].ConceptID] = []float64{math.Cos(angle), math.Sin(angle)}
	}
	one := embeddingProposals(concepts[0], concepts, vecs, "model-a")
	if len(one) != reconcileEmbeddingTopK || one[9].Methods[0].Rank != 10 {
		t.Fatalf("top-10 boundary wrong: %+v", one)
	}
	for _, proposal := range one {
		if proposal.Methods[0].CandidateSetSize != 11 || proposal.Methods[0].CandidateSetLimit != 10 {
			t.Fatalf("wrong full candidate pool metadata: %+v", proposal)
		}
	}
	for id, vector := range vecs {
		vecs[id] = []float64{vector[0] * 7, vector[1] * 7}
	}
	two := embeddingProposals(concepts[0], concepts, vecs, "model-b")
	for i := range one {
		if one[i].TargetConceptID != two[i].TargetConceptID {
			t.Fatalf("ranking changed at %d", i)
		}
	}
	if one[9].TargetConceptID == concepts[11].ConceptID {
		t.Fatal("position 11 entered bounded proposals")
	}
}

func TestAffineScoreTransformPreservesDecisionAndProposalOrder(t *testing.T) {
	makeProposals := func(scale, shift float64) []ReconcileProposal {
		result := []ReconcileProposal{}
		for rank, item := range []struct {
			id    string
			score float64
		}{{"a", .8}, {"b", .5}} {
			score := item.score*scale + shift
			result = append(result, ReconcileProposal{TargetConceptID: item.id, Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: rank + 1, Score: &score, ModelID: "m", CandidateSetSize: 2, CandidateSetLimit: 10}}, Claims: []IdentityClaim{}})
		}
		return result
	}
	base, transformed := makeProposals(1, 0), makeProposals(7, 13)
	for _, proposals := range [][]ReconcileProposal{base, transformed} {
		if got := chooseCandidateDecision(tier5Result{}, nil, proposals, nil); got.Outcome != outcomeDeferred {
			t.Fatalf("score authorized outcome: %+v", got)
		}
		audit := mergeAuditProposals(proposals, tier5Result{}, nil)
		if audit[0].TargetConceptID != "a" || audit[1].TargetConceptID != "b" {
			t.Fatalf("proposal semantics changed: %+v", audit)
		}
	}
}

func TestAffineScoreTransformPreservesTransactionalDeferredAudit(t *testing.T) {
	for _, transform := range []struct {
		name         string
		scale, shift float64
	}{{"base", 1, 0}, {"affine", 7, 13}} {
		t.Run(transform.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			now := time.Unix(7, 0).UTC()
			expectDecisionPrefix(mock, now, "亮度", sqlmock.NewRows(conceptTestColumns()))
			scoreA, scoreB := .8*transform.scale+transform.shift, .5*transform.scale+transform.shift
			proposals := []ReconcileProposal{
				{TargetConceptID: "a", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 1, Score: &scoreA, ModelID: "model", CandidateSetSize: 2, CandidateSetLimit: 10}}, Claims: []IdentityClaim{}},
				{TargetConceptID: "b", Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Rank: 2, Score: &scoreB, ModelID: "model", CandidateSetSize: 2, CandidateSetLimit: 10}}, Claims: []IdentityClaim{}},
			}
			mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeDeferred, "model", nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mock.ExpectCommit()
			r := Reconciler{DB: db, Scope: "_", EmbeddingModelID: "model", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{}}}
			outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, proposals)
			if err != nil || outcome != outcomeDeferred {
				t.Fatalf("outcome=%s err=%v", outcome, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateReconcilerConfiguration(t *testing.T) {
	triple := TripleEvidenceProvider{}
	tests := []struct {
		name string
		r    Reconciler
		want string
	}{
		{"missing provider", Reconciler{}, "required identity evidence provider"},
		{"embedding missing model", Reconciler{EvidenceProviders: []IdentityEvidenceProvider{triple}, Embeddings: orderedEmbeddingClient{}}, "EmbeddingModelID"},
		{"model without embeddings", Reconciler{EvidenceProviders: []IdentityEvidenceProvider{triple}, EmbeddingModelID: "model"}, "EmbeddingModelID"},
		{"missing alignment", Reconciler{EvidenceProviders: []IdentityEvidenceProvider{triple}}, "alignment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.r.validateConfiguration(); err == nil || !contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunBindsAllScanReadsToReconcilerDB(t *testing.T) {
	primary, primaryMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()
	other, otherMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	primaryMock.ExpectQuery("status = 'provisional'.*gloss_source = 'auto:d11'").WithArgs("_", 500).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	primaryMock.ExpectQuery("status IN \\('active', 'provisional'\\)").WithArgs("_").WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	r := Reconciler{DB: primary, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{}}, ConceptStore: ConceptStore{DB: other, Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: primary}}}}
	stats, err := r.Run(context.Background())
	if err != nil || stats != (ReconcileStats{}) {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	if err := primaryMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := otherMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("split scan handle was used: %v", err)
	}

	// A nil embedded store handle is equally safe: Reconciler.DB owns scans.
	primaryMock.ExpectQuery("status = 'provisional'.*gloss_source = 'auto:d11'").WithArgs("_", 500).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	primaryMock.ExpectQuery("status IN \\('active', 'provisional'\\)").WithArgs("_").WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	r.ConceptStore.DB = nil
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("nil ConceptStore.DB leaked into scan: %v", err)
	}
	if err := primaryMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatsInvariants(t *testing.T) {
	for _, s := range []ReconcileStats{{Scanned: 4, Decided: 4, Merged: 1, DeferredUnvalidated: 1, Rejected: 1, NoCandidate: 1}, {Scanned: 2, Decided: 1, Failed: 1, Merged: 1}} {
		if err := s.Validate(); err != nil {
			t.Fatalf("valid stats rejected: %v", err)
		}
	}
	if err := (ReconcileStats{Scanned: 3, Decided: 1, Failed: 1}).Validate(); err == nil {
		t.Fatal("invalid partial failure accepted")
	}
	if err := (ReconcileStats{Scanned: 1, Decided: 1, Merged: -1, NoCandidate: 2}).Validate(); err == nil {
		t.Fatal("negative outcome counter accepted")
	}
}

func TestEmbeddingOutputFailsClosed(t *testing.T) {
	concepts := []Concept{{ConceptID: "c", PrefLabel: "candidate"}}
	for _, vectors := range [][][]float64{nil, {{}}, {{math.NaN()}}, {{math.Inf(1)}}} {
		r := Reconciler{Embeddings: orderedEmbeddingClient{vectors: vectors}}
		if _, err := r.embedConcepts(context.Background(), concepts); err == nil {
			t.Fatalf("accepted malformed vectors %#v", vectors)
		}
	}
}

type failingSecondProvider struct{ calls int }

func (p *failingSecondProvider) ProviderID() string { return TripleEvidenceProviderID }
func (p *failingSecondProvider) LoadClaims(_ context.Context, _ *sql.Tx, _ CandidateIdentityContext) ([]IdentityClaim, error) {
	p.calls++
	if p.calls == 2 {
		return nil, errors.New("provider unavailable")
	}
	return []IdentityClaim{}, nil
}

type authoritativeFixtureProvider struct{ target string }

func (p authoritativeFixtureProvider) ProviderID() string { return TripleEvidenceProviderID }
func (p authoritativeFixtureProvider) LoadClaims(_ context.Context, _ *sql.Tx, candidate CandidateIdentityContext) ([]IdentityClaim, error) {
	return []IdentityClaim{authoritativeClaim(candidate.CandidateConceptID, p.target)}, nil
}

type markedAuthoritativeProvider struct{ target string }

func (p markedAuthoritativeProvider) ProviderID() string { return TripleEvidenceProviderID }
func (p markedAuthoritativeProvider) LoadClaims(ctx context.Context, tx *sql.Tx, candidate CandidateIdentityContext) ([]IdentityClaim, error) {
	var marker int
	if err := tx.QueryRowContext(ctx, "SELECT provider_marker").Scan(&marker); err != nil {
		return nil, err
	}
	return []IdentityClaim{authoritativeClaim(candidate.CandidateConceptID, p.target)}, nil
}

type panicProvider struct{}

func (panicProvider) ProviderID() string { return TripleEvidenceProviderID }
func (panicProvider) LoadClaims(context.Context, *sql.Tx, CandidateIdentityContext) ([]IdentityClaim, error) {
	panic("stale candidate must not query providers")
}

type claimsFixtureProvider struct{ claims []IdentityClaim }

func (p claimsFixtureProvider) ProviderID() string { return TripleEvidenceProviderID }
func (p claimsFixtureProvider) LoadClaims(context.Context, *sql.Tx, CandidateIdentityContext) ([]IdentityClaim, error) {
	return p.claims, nil
}

func conceptTestColumns() []string {
	return []string{"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time"}
}

func addConceptRow(rows *sqlmock.Rows, id, label, status string, created time.Time) *sqlmock.Rows {
	return addConceptRowScope(rows, id, label, "_", status, created)
}

func addConceptRowScope(rows *sqlmock.Rows, id, label, scope, status string, created time.Time) *sqlmock.Rows {
	return rows.AddRow(id, label, nil, scope, status, nil, "auto:d11", created, created)
}

func surfaceTestRows(now time.Time, conceptID string, values ...Surface) *sqlmock.Rows {
	rows := sqlmock.NewRows(strings.Split(surfaceColumns, ", "))
	for i, surface := range values {
		rows.AddRow(fmt.Sprintf("s%d", i), conceptID, surface.Surface, surface.Surface, semid.CurrentNormalizerVersion, "pref", "alias", "und", "_", 1.0, "fixture", surface.Locked, nil, now, now)
	}
	return rows
}

func TestReconcilerUniqueTier5NoAuthorityMergeThenThreeCandidatePartialFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(1, 0).UTC()
	candidates := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(candidates, "c1", "kubernets", "provisional", now)
	for _, id := range []string{"c2", "c3"} {
		addConceptRow(candidates, id, id, "provisional", now)
	}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.keyword_concepts\n\t\tWHERE scope = $1 AND status = 'provisional' AND gloss_source = 'auto:d11'")).
		WithArgs("_", 500).WillReturnRows(candidates)
	live := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(live, "c1", "kubernets", "provisional", now)
	for _, id := range []string{"c2", "c3"} {
		addConceptRow(live, id, id, "provisional", now)
	}
	addConceptRow(live, "target", "kubernetes", "active", now)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE scope = $1 AND status IN ('active', 'provisional')")).WithArgs("_").WillReturnRows(live)

	// Candidate one reaches a deterministic Tier-5 merge and audit and commits.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("c1").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "c1", "kubernets", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+ORDER BY surface_id FOR UPDATE").WithArgs("c1").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("WHERE scope = \\$1 AND status = 'active'").WithArgs("_", "kubernets", reconcileLexicalBlockMin, "c1", reconcileLexicalTopK).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "kubernetes", "active", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "kubernetes", "active", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+ORDER BY surface_id FOR UPDATE").WithArgs("target").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	expectNeverMergeFalse(mock, "c1", "target")
	expectNoAlignment(mock, "c1")
	expectNoAlignment(mock, "target")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("c1").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "c1", "kubernets", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "kubernetes", "active", now))
	expectNeverMergeFalse(mock, "c1", "target")
	expectNoAlignment(mock, "c1")
	expectNoAlignment(mock, "target")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.semantic_assertions").WithArgs("c1", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE kb.keyword_concepts").WithArgs("c1", "target").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.keyword_surfaces").WithArgs("c1", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeAutoMerge, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	// Candidate two fails during the exhaustive provider read. Its transaction
	// rolls back; candidate three is never attempted.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("c2").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "c2", "c2", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+ORDER BY surface_id FOR UPDATE").WithArgs("c2").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("WHERE scope = \\$1 AND status = 'active'").WithArgs("_", "c2", reconcileLexicalBlockMin, "c2", reconcileLexicalTopK).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	mock.ExpectRollback()

	provider := &failingSecondProvider{}
	r := Reconciler{
		DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{provider},
		ConceptStore: ConceptStore{DB: db, Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}},
	}
	stats, err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("got %v", err)
	}
	want := ReconcileStats{Scanned: 2, Decided: 1, Failed: 1, Merged: 1}
	if stats != want {
		t.Fatalf("got %+v, want %+v", stats, want)
	}
	if err := stats.Validate(); err != nil {
		t.Fatalf("partial counters invalid: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls=%d", provider.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcilerTier5TieLowCosineExactIdentityOutsideTopTenMerges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(2, 0).UTC()
	mock.ExpectQuery("status = 'provisional'.*gloss_source = 'auto:d11'").WithArgs("_", 500).
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "kubers", "provisional", now))
	live := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(live, "candidate", "kubers", "provisional", now)
	addConceptRow(live, "target", "Luminance", "active", now)
	for i := 0; i < 10; i++ {
		addConceptRow(live, fmt.Sprintf("decoy-%02d", i), fmt.Sprintf("Decoy %d", i), "active", now)
	}
	mock.ExpectQuery("status IN \\('active', 'provisional'\\)").WithArgs("_").WillReturnRows(live)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "kubers", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	tieRows := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(tieRows, "tie-a", "kuber", "active", now)
	addConceptRow(tieRows, "tie-b", "kuberx", "active", now)
	mock.ExpectQuery("WHERE scope = \\$1 AND status = 'active'").WithArgs("_", "kubers", reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).
		WillReturnRows(tieRows)
	mock.ExpectQuery("SELECT provider_marker").WillReturnRows(sqlmock.NewRows([]string{"marker"}).AddRow(1))
	mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))

	// Validation locks and rechecks the chosen target only after the provider
	// has returned the exhaustive authoritative set.
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+ORDER BY surface_id FOR UPDATE").WithArgs("target").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	expectNeverMergeFalse(mock, "candidate", "target")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "target")

	// MergeConceptTx revalidates guards inside the same caller-owned tx, then
	// follows alignments, applies candidate -> target, and only then audits.
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "kubers", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").
		WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))
	expectNeverMergeFalse(mock, "candidate", "target")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "target")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.semantic_assertions").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE kb.keyword_concepts").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.keyword_surfaces").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", providerOnlyAuditMatcher{target: "target"}, providerOnlyAuditMatcher{target: "target"}, outcomeAutoMerge, "orthogonal-model", nil, "keyword_reconciler", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectCommit()

	r := Reconciler{
		DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{markedAuthoritativeProvider{target: "target"}},
		ConceptStore: ConceptStore{DB: db, Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}},
		Embeddings: orderedEmbeddingClient{vectors: func() [][]float64 {
			vectors := [][]float64{{1, 0}, {0, 1}}
			for i := 0; i < 10; i++ {
				angle := float64(i+1) * .03
				vectors = append(vectors, []float64{math.Cos(angle), math.Sin(angle)})
			}
			return vectors
		}()}, EmbeddingModelID: "orthogonal-model",
	}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats != (ReconcileStats{Scanned: 1, Decided: 1, Merged: 1}) {
		t.Fatalf("stats=%+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type providerOnlyAuditMatcher struct{ target string }

func (m providerOnlyAuditMatcher) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var audit reconcileAudit
	if json.Unmarshal([]byte(raw), &audit) != nil {
		return false
	}
	embeddingCount := 0
	targetIsProviderOnly := false
	for _, proposal := range audit.Proposals {
		if proposal.TargetConceptID == m.target {
			if len(proposal.Claims) == 0 {
				return false
			}
			for _, method := range proposal.Methods {
				if method.Method == methodEmbedding {
					return false
				}
			}
			targetIsProviderOnly = true
		}
		for _, method := range proposal.Methods {
			if method.Method == methodEmbedding {
				embeddingCount++
				if method.Rank < 1 || method.Rank > 10 || method.CandidateSetSize != 11 || method.CandidateSetLimit != 10 {
					return false
				}
			}
		}
	}
	return targetIsProviderOnly && embeddingCount == 10
}

func TestReconcilerHighCosineWithoutIdentityDefersAndAudits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(3, 0).UTC()
	mock.ExpectQuery("status = 'provisional'.*gloss_source = 'auto:d11'").WithArgs("_", 500).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	live := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(live, "candidate", "亮度", "provisional", now)
	addConceptRow(live, "target", "Luminance", "active", now)
	mock.ExpectQuery("status IN \\('active', 'provisional'\\)").WithArgs("_").WillReturnRows(live)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("similarity\\(pref_label, \\$2\\)").WithArgs("_", "亮度", reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeDeferred, "high-cosine", nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectCommit()
	r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{&failingSecondProvider{}}, Embeddings: orderedEmbeddingClient{vectors: [][]float64{{1, 0}, {.999999, .001}}}, EmbeddingModelID: "high-cosine", ConceptStore: ConceptStore{DB: db, Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats != (ReconcileStats{Scanned: 1, Decided: 1, DeferredUnvalidated: 1}) {
		t.Fatalf("stats=%+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcilerPositiveAuthorityHardVetoesAuditReject(t *testing.T) {
	tests := []struct {
		name              string
		candidate, target []Surface
		targetScope       string
		never, alignment  bool
	}{
		{name: "absorbed surface lock", candidate: []Surface{{Surface: "metric", Locked: true}}, targetScope: "_"},
		{name: "scope", targetScope: "other"},
		{name: "never merge", targetScope: "_", never: true},
		{name: "alignment", targetScope: "_", alignment: true},
		{name: "all surface digits", candidate: []Surface{{Surface: "metric 2"}}, target: []Surface{{Surface: "metric 3"}}, targetScope: "_"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			now := time.Unix(8, 0).UTC()
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
			mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(surfaceTestRows(now, "candidate", tt.candidate...))
			mock.ExpectQuery("similarity\\(pref_label, \\$2\\)").WithArgs("_", "亮度", reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
			mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(addConceptRowScope(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", tt.targetScope, "active", now))
			mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").WillReturnRows(addConceptRowScope(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", tt.targetScope, "active", now))
			mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("target").WillReturnRows(surfaceTestRows(now, "target", tt.target...))
			mock.ExpectQuery("FROM kb.semid_never_merge").WithArgs("keyword", "candidate", "target").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.never))
			if tt.alignment {
				expectAlignment(mock, "candidate", "term-a")
				expectAlignment(mock, "target", "term-b")
			} else {
				expectNoAlignment(mock, "candidate")
				expectNoAlignment(mock, "target")
			}
			mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeReject, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mock.ExpectCommit()
			r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{claims: []IdentityClaim{authoritativeClaim("candidate", "target")}}}, ConceptStore: ConceptStore{Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
			outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
			if err != nil || outcome != outcomeReject {
				t.Fatalf("outcome=%s err=%v", outcome, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReconcilerTripleProviderCrossScopeAuthorityReachesCoreScopeVeto(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(9, 0).UTC()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(surfaceTestRows(now, "candidate", Surface{Surface: "亮度"}))
	mock.ExpectQuery("similarity\\(pref_label, \\$2\\)").WithArgs("_", "亮度", reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "source", "external_id", "release"}).AddRow(1, "catalog", "metric-1", "r1"))
	mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePoliciesSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"source", "release", "identity_authority", "allowed_scopes"}).AddRow("catalog", "r1", true, "{_}"))
	mock.ExpectQuery(regexp.QuoteMeta(tripleConfiguredDeploymentsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"deployment_key", "source", "release", "enabled"}).AddRow("production", "catalog", "r1", true))
	mock.ExpectQuery(regexp.QuoteMeta(tripleExternalMappingsSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"source", "external_id", "release", "concept_id", "status", "scope"}).AddRow("catalog", "metric-1", "r1", "cross", "active", "other"))
	mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(addConceptRowScope(sqlmock.NewRows(conceptTestColumns()), "cross", "Luminance", "other", "active", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("cross").WillReturnRows(addConceptRowScope(sqlmock.NewRows(conceptTestColumns()), "cross", "Luminance", "other", "active", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("cross").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	expectNeverMergeFalse(mock, "candidate", "cross")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "cross")
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeReject, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{TripleEvidenceProvider{DeploymentKeys: []string{"production"}}}, ConceptStore: ConceptStore{Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
	outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
	if err != nil || outcome != outcomeReject {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleCandidateIsAuditedBeforeTier5OrProviderReads(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(4, 0).UTC()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "stale", "active", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeReject, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectCommit()
	r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{panicProvider{}}, ConceptStore: ConceptStore{Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
	outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
	if err != nil || outcome != outcomeReject {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionLogFailureRollsBackMergeTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(5, 0).UTC()
	mock.ExpectQuery("status = 'provisional'.*gloss_source = 'auto:d11'").WithArgs("_", 500).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	live := sqlmock.NewRows(conceptTestColumns())
	addConceptRow(live, "candidate", "亮度", "provisional", now)
	addConceptRow(live, "target", "Luminance", "active", now)
	mock.ExpectQuery("status IN \\('active', 'provisional'\\)").WithArgs("_").WillReturnRows(live)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("similarity\\(pref_label, \\$2\\)").WithArgs("_", "亮度", reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).WillReturnRows(sqlmock.NewRows(conceptTestColumns()))
	mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("target").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	expectNeverMergeFalse(mock, "candidate", "target")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "target")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", "亮度", "provisional", now))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("target").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "target", "Luminance", "active", now))
	expectNeverMergeFalse(mock, "candidate", "target")
	expectNoAlignment(mock, "candidate")
	expectNoAlignment(mock, "target")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.semantic_assertions").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE kb.keyword_concepts").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.keyword_surfaces").WithArgs("candidate", "target").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()
	r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{authoritativeFixtureProvider{target: "target"}}, ConceptStore: ConceptStore{DB: db, Alignments: AlignmentsStore{Assertions: assertions.AssertionStore{DB: db}}}}
	stats, err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "audit unavailable") {
		t.Fatalf("err=%v", err)
	}
	if stats != (ReconcileStats{Scanned: 1, Failed: 1}) {
		t.Fatalf("stats=%+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTargetlessDecisionBranchesAuditWithoutTargetLock(t *testing.T) {
	now := time.Unix(6, 0).UTC()
	t.Run("tier5 tie", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectDecisionPrefix(mock, now, "kubers", addConceptRow(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "a", "kuber", "active", now), "b", "kuberx", "active", now))
		mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeDeferred, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{}}}
		outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
		if err != nil || outcome != outcomeDeferred {
			t.Fatalf("%s %v", outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("inactive authority", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectDecisionPrefix(mock, now, "亮度", sqlmock.NewRows(conceptTestColumns()))
		mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "inactive", "Luminance", "provisional", now))
		mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeDeferred, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{claims: []IdentityClaim{authoritativeClaim("candidate", "inactive")}}}}
		outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
		if err != nil || outcome != outcomeDeferred {
			t.Fatalf("%s %v", outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("tier5 tie multiple authority", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		tieRows := sqlmock.NewRows(conceptTestColumns())
		addConceptRow(tieRows, "tie-a", "kuber", "active", now)
		addConceptRow(tieRows, "tie-b", "kuberx", "active", now)
		expectDecisionPrefix(mock, now, "kubers", tieRows)
		rows := sqlmock.NewRows(conceptTestColumns())
		addConceptRow(rows, "a", "A", "active", now)
		addConceptRow(rows, "b", "B", "active", now)
		mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)
		mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeReject, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{claims: []IdentityClaim{authoritativeClaim("candidate", "a"), authoritativeClaim("candidate", "b")}}}}
		outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
		if err != nil || outcome != outcomeReject {
			t.Fatalf("%s %v", outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("none no claims no embeddings", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectDecisionPrefix(mock, now, "亮度", sqlmock.NewRows(conceptTestColumns()))
		mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", emptyProposalAuditMatcher{}, emptyProposalAuditMatcher{}, outcomeNoCandidate, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{}}}
		outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
		if err != nil || outcome != outcomeNoCandidate {
			t.Fatalf("%s %v", outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("tier5 contradictory authority", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectDecisionPrefix(mock, now, "kubernets", addConceptRow(sqlmock.NewRows(conceptTestColumns()), "local", "kubernetes", "active", now))
		mock.ExpectQuery("WHERE concept_id = ANY\\(\\$1\\)").WithArgs(sqlmock.AnyArg()).WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "other", "K8s", "active", now))
		mock.ExpectQuery("INSERT INTO kb.semid_decision_log").WithArgs("keyword_reconcile", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), outcomeReject, nil, nil, "keyword_reconciler", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		r := Reconciler{DB: db, Scope: "_", EvidenceProviders: []IdentityEvidenceProvider{claimsFixtureProvider{claims: []IdentityClaim{authoritativeClaim("candidate", "other")}}}}
		outcome, err := r.decideCandidate(context.Background(), Concept{ConceptID: "candidate"}, nil)
		if err != nil || outcome != outcomeReject {
			t.Fatalf("%s %v", outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

type emptyProposalAuditMatcher struct{}

func (emptyProposalAuditMatcher) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var audit reconcileAudit
	return json.Unmarshal([]byte(raw), &audit) == nil && len(audit.Proposals) == 0 && len(audit.Claims) == 0 && audit.Outcome == outcomeNoCandidate
}

func expectDecisionPrefix(mock sqlmock.Sqlmock, now time.Time, label string, shortlist *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE concept_id = \\$1[[:space:]]+FOR UPDATE").WithArgs("candidate").WillReturnRows(addConceptRow(sqlmock.NewRows(conceptTestColumns()), "candidate", label, "provisional", now))
	mock.ExpectQuery("ORDER BY surface_id FOR UPDATE").WithArgs("candidate").WillReturnRows(sqlmock.NewRows(strings.Split(surfaceColumns, ", ")))
	mock.ExpectQuery("similarity\\(pref_label, \\$2\\)").WithArgs("_", label, reconcileLexicalBlockMin, "candidate", reconcileLexicalTopK).WillReturnRows(shortlist)
}

func expectNeverMergeFalse(mock sqlmock.Sqlmock, a, b string) {
	if a > b {
		a, b = b, a
	}
	mock.ExpectQuery("FROM kb.semid_never_merge").WithArgs("keyword", a, b).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
}

func expectNoAlignment(mock sqlmock.Sqlmock, conceptID string) {
	mock.ExpectQuery("FROM kb.semantic_assertions").WithArgs(conceptID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason"}))
}

func expectAlignment(mock sqlmock.Sqlmock, conceptID, termID string) {
	mock.ExpectQuery("FROM kb.semantic_assertions").WithArgs(conceptID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason"}).
			AddRow(1, conceptID, "ontology_term", termID, "accepted", []byte(`{"method":"fixture"}`), 1.0, "fixture"))
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
