package keywords

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// These tests exercise the Tier-6 identity path against a live PostgreSQL
// database (the same schema the reconciler command runs against). They skip
// unless TEST_DATABASE_URL points at a migrated test database, e.g.:
//
//	TEST_DATABASE_URL='host=/tmp user=cding dbname=chenweb_test sslmode=disable' \
//	    go test ./server/api/ontology/keywords/ -run 'TestReconcileIdentityIntegration'
//
// Prerequisites: project migrations applied, stage-1 fixtures imported
// (terminology-import import ...), and the tier6-primary deployment pointing
// at iec-60050-845/2020.

func openReconcileTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return db
}

type reconcileIntegrationFixture struct {
	db                    *sql.DB
	luminance, brightness string
	luminanceSurface      string
	brightnessSurface     string
	candidateIDs          []string
	surfaceIDs            []string
	counter               int
	promotionFile         ReviewedPromotionFile
}

func newReconcileIntegrationFixture(t *testing.T) *reconcileIntegrationFixture {
	t.Helper()
	db := openReconcileTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	f := &reconcileIntegrationFixture{
		db: db,
		// Per-run concept IDs keep the fixture self-contained alongside any
		// provisioned pilot corpus (e.g. the stored coverage-report corpus).
		luminance:  "kw:luminance_itg_" + suffix,
		brightness: "kw:brightness_itg_" + suffix,
	}
	// Register cleanup before any write so a failed fixture (or a previous
	// crashed run) can never leave keyword rows behind.
	t.Cleanup(func() { f.cleanup(t) })
	f.cleanup(t)
	reviewedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cs := ConceptStore{DB: db}
	for _, c := range []Concept{
		{ConceptID: f.luminance, PrefLabel: "Luminance", Scope: "display", Status: "active", GlossSource: "reviewed"},
		{ConceptID: f.brightness, PrefLabel: "Brightness", Scope: "display", Status: "active", GlossSource: "reviewed"},
	} {
		if _, err := cs.CreateConcept(ctx, c); err != nil {
			t.Fatalf("create concept %s: %v", c.ConceptID, err)
		}
	}
	ss := SurfaceStore{DB: db}
	lumiSurface, err := ss.CreateSurface(ctx, Surface{
		ConceptID: f.luminance, Surface: "Luminance", LabelRole: "pref", AliasType: "pref",
		Provenance: "reviewed:integration", Scope: "display", Lang: "en",
	})
	if err != nil {
		t.Fatalf("create luminance surface: %v", err)
	}
	f.luminanceSurface = lumiSurface.SurfaceID
	brightSurface, err := ss.CreateSurface(ctx, Surface{
		ConceptID: f.brightness, Surface: "Brightness", LabelRole: "pref", AliasType: "pref",
		Provenance: "reviewed:integration", Scope: "display", Lang: "en",
	})
	if err != nil {
		t.Fatalf("create brightness surface: %v", err)
	}
	f.brightnessSurface = brightSurface.SurfaceID
	f.surfaceIDs = append(f.surfaceIDs, f.luminanceSurface, f.brightnessSurface)

	f.promotionFile = ReviewedPromotionFile{
		SchemaVersion: 1, Source: "iec-60050-845", Release: "2020", Scope: "display",
		DeploymentKey: "tier6-primary",
		Promotions: []PositivePromotion{{
			ExternalID: "845-21-050", ConceptID: f.luminance, Relation: "exact_equivalent",
			UnitConstraints: []string{"cd/m2"}, DimensionConstraints: []string{"J.L-2"},
			Surfaces: []PromotedSurface{{SurfaceID: f.luminanceSurface, Evidence: "reviewed exact mapping"}},
			Reviewer: "domain-board", ReviewedAt: reviewedAt,
			ProvenanceLocator: "https://example.test/promotion/845-21-050",
		}},
		NegativePromotions: []NegativePromotion{{
			NodeA: f.luminance, NodeB: f.brightness, Scope: "display",
			Reason: "luminance is photometric density; brightness is a perceived attribute",
			Triples: []PromotionTriple{{
				Source: "iec-60050-845", Release: "2020",
				SubjectExternalID: "845-21-050", ObjectExternalID: "845-22-059",
				Relation: "different_from", ProvenanceLocator: "https://example.test/review/1",
			}},
			Reviewer: "domain-board", ReviewedAt: reviewedAt,
			ProvenanceLocator: "https://example.test/negative/1",
		}},
	}

	store := PromotionStore{DB: db}
	first, err := store.ApplyReviewedPromotion(ctx, f.promotionFile)
	if err != nil {
		t.Fatalf("apply reviewed promotion: %v", err)
	}
	if first.ExternalIDs != 1 || first.SurfaceEvidence != 1 || first.NeverMerges != 1 {
		t.Fatalf("promotion result=%+v", first)
	}
	second, err := store.ApplyReviewedPromotion(ctx, f.promotionFile)
	if err != nil {
		t.Fatalf("replay reviewed promotion: %v", err)
	}
	if second != first {
		t.Fatalf("promotion replay counts differ: first=%+v second=%+v", first, second)
	}
	// Idempotence is enforced at the database: exactly one mapping, one
	// evidence row, and one veto survive both applications.
	var mappings, evidenceRows, vetoes int
	if err := f.db.QueryRowContext(ctx, `
SELECT (SELECT count(*) FROM kb.keyword_external_ids WHERE source = 'iec-60050-845' AND external_id = '845-21-050' AND release = '2020'),
       (SELECT count(*) FROM kb.keyword_surface_evidence WHERE surface_id = $1 AND source = 'iec-60050-845' AND external_id = '845-21-050' AND release = '2020'),
       (SELECT count(*) FROM kb.semid_never_merge WHERE family = 'keyword' AND node_a = $2 AND node_b = $3)`,
		f.luminanceSurface, f.brightness, f.luminance).Scan(&mappings, &evidenceRows, &vetoes); err != nil {
		t.Fatalf("verify idempotent promotion state: %v", err)
	}
	if mappings != 1 || evidenceRows != 1 || vetoes != 1 {
		t.Fatalf("promotion replay left duplicate state: mappings=%d evidence=%d vetoes=%d", mappings, evidenceRows, vetoes)
	}

	return f
}

func (f *reconcileIntegrationFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	allConcepts := append(append([]string(nil), f.candidateIDs...), f.luminance, f.brightness)
	statements := []struct {
		query string
		args  []any
	}{
		// Candidate ids are content-independent here (kwc_intg_ prefix is
		// unique to this integration fixture), so a crashed run's orphaned
		// audit rows are removed as well as this run's tracked candidates.
		{`DELETE FROM kb.semid_decision_log WHERE family = 'keyword_reconcile' AND ((input->>'candidate_concept_id') = ANY($1) OR (input->>'candidate_concept_id') LIKE 'kwc_intg\_%')`, []any{pq.Array(f.candidateIDs)}},
		{`DELETE FROM kb.semid_never_merge WHERE family = 'keyword' AND node_a = $1 AND node_b = $2`, []any{f.brightness, f.luminance}},
		{`DELETE FROM kb.keyword_external_ids WHERE concept_id = ANY($1)`, []any{pq.Array([]string{f.luminance, f.brightness})}},
		// Surface evidence and surface keys cascade with their surface rows;
		// deleting by concept also removes surfaces re-pointed by a merge.
		{`DELETE FROM kb.keyword_occurrences WHERE concept_id = ANY($1)`, []any{pq.Array(allConcepts)}},
		{`DELETE FROM kb.keyword_surfaces WHERE concept_id = ANY($1)`, []any{pq.Array(allConcepts)}},
		{`DELETE FROM kb.keyword_concepts WHERE concept_id = ANY($1)`, []any{pq.Array(allConcepts)}},
	}
	for _, statement := range statements {
		if _, err := f.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
}

func (f *reconcileIntegrationFixture) addCandidate(t *testing.T, prefLabel, surface, lang string) (conceptID, surfaceID string) {
	t.Helper()
	f.counter++
	conceptID = fmt.Sprintf("kwc_intg_%d_%d", f.counter, time.Now().UnixNano())
	ctx := context.Background()
	if _, err := (ConceptStore{DB: f.db}).CreateConcept(ctx, Concept{
		ConceptID: conceptID, PrefLabel: prefLabel, Scope: "display", Status: "provisional", GlossSource: "auto:d11",
	}); err != nil {
		t.Fatalf("create candidate %s: %v", conceptID, err)
	}
	created, err := (SurfaceStore{DB: f.db}).CreateSurface(ctx, Surface{
		ConceptID: conceptID, Surface: surface, LabelRole: "pref", AliasType: "pref",
		Provenance: "auto:resolver", Scope: "display", Lang: lang,
	})
	if err != nil {
		t.Fatalf("create candidate surface %s: %v", surface, err)
	}
	f.candidateIDs = append(f.candidateIDs, conceptID)
	f.surfaceIDs = append(f.surfaceIDs, created.SurfaceID)
	return conceptID, created.SurfaceID
}

func (f *reconcileIntegrationFixture) addCandidateEvidence(t *testing.T, surfaceID, externalID, evidence string) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.db.ExecContext(ctx, `
INSERT INTO kb.keyword_surface_evidence (surface_id, source, external_id, release, evidence, confidence)
VALUES ($1, 'iec-60050-845', $2, '2020', $3, 1.0)`, surfaceID, externalID, evidence); err != nil {
		t.Fatalf("insert candidate evidence %s: %v", externalID, err)
	}
}

func (f *reconcileIntegrationFixture) promoteBrightness(t *testing.T) {
	t.Helper()
	reviewedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	file := ReviewedPromotionFile{
		SchemaVersion: 1, Source: "iec-60050-845", Release: "2020", Scope: "display",
		DeploymentKey: "tier6-primary",
		Promotions: []PositivePromotion{{
			ExternalID: "845-22-059", ConceptID: f.brightness, Relation: "exact_equivalent",
			Surfaces: []PromotedSurface{{SurfaceID: f.brightnessSurface, Evidence: "reviewed exact mapping"}},
			Reviewer: "domain-board", ReviewedAt: reviewedAt,
			ProvenanceLocator: "https://example.test/promotion/845-22-059",
		}},
	}
	result, err := (PromotionStore{DB: f.db}).ApplyReviewedPromotion(context.Background(), file)
	if err != nil {
		t.Fatalf("promote brightness mapping: %v", err)
	}
	if result.ExternalIDs != 1 || result.SurfaceEvidence != 1 {
		t.Fatalf("brightness promotion result=%+v", result)
	}
}

func (f *reconcileIntegrationFixture) reconciler(embeddings EmbeddingClient) *Reconciler {
	return &Reconciler{
		DB: f.db,
		ConceptStore: ConceptStore{DB: f.db, Alignments: AlignmentsStore{
			Assertions:  assertions.AssertionStore{DB: f.db},
			DecisionLog: semid.DecisionLogStore{DB: f.db},
			Scope:       "display",
		}},
		EvidenceProviders: []IdentityEvidenceProvider{
			TripleEvidenceProvider{DeploymentKeys: []string{"tier6-primary"}},
		},
		Embeddings:       embeddings,
		EmbeddingModelID: "integration-fake",
		Scope:            "display",
	}
}

// labelEmbeddingClient ranks every candidate highest against Brightness
// (cosine 0.99) and lowest against Luminance (cosine ~0.14) regardless of the
// candidate's true identity. The identity path must win anyway; embeddings
// only rank diagnostic proposals.
type labelEmbeddingClient struct{}

func (labelEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		switch text {
		case "Brightness":
			out[i] = []float64{1, 0}
		case "Luminance":
			out[i] = []float64{0, 1}
		default:
			out[i] = []float64{0.99, 0.141}
		}
	}
	return out, nil
}

func TestReconcileIdentityIntegrationExactMergeVetoAndDefers(t *testing.T) {
	f := newReconcileIntegrationFixture(t)
	ctx := context.Background()
	embeddings := labelEmbeddingClient{}

	// Scenario A: the bilingual surface 亮度 carries corpus evidence for IEC
	// 845-21-050. The reviewed promotion maps 845-21-050 to active
	// kw:luminance. The embedding client ranks kw:brightness first (cosine
	// ~0.99) and kw:luminance far behind (~0.14): exact identity must still
	// merge 亮度 into kw:luminance, regardless of cosine.
	lumiCandidate, lumiCandidateSurface := f.addCandidate(t, "亮度", "亮度", "zh")
	f.addCandidateEvidence(t, lumiCandidateSurface, "845-21-050", "corpus-observed zh surface for IEC 845-21-050")
	stats, err := f.reconciler(embeddings).Run(ctx)
	if err != nil {
		t.Fatalf("reconcile 亮度: %v", err)
	}
	if stats != (ReconcileStats{Scanned: 1, Decided: 1, Merged: 1}) {
		t.Fatalf("亮度 stats=%+v, want one identity merge", stats)
	}
	merged, err := (ConceptStore{DB: f.db}).GetConcept(ctx, lumiCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != "merged" || merged.MergedInto == nil || *merged.MergedInto != f.luminance {
		t.Fatalf("亮度 candidate=%+v, want merged into %s", merged, f.luminance)
	}

	// Scenario B1: 视亮度 carries evidence for IEC 845-22-059 but the
	// reviewed mapping does not exist yet: the targetless claim must NOT
	// authorize, so the candidate defers instead of guessing.
	brightCandidate, brightCandidateSurface := f.addCandidate(t, "视亮度", "视亮度", "zh")
	f.addCandidateEvidence(t, brightCandidateSurface, "845-22-059", "corpus-observed zh surface for IEC 845-22-059")
	stats, err = f.reconciler(embeddings).Run(ctx)
	if err != nil {
		t.Fatalf("reconcile 视亮度 before mapping: %v", err)
	}
	if stats != (ReconcileStats{Scanned: 1, Decided: 1, DeferredUnvalidated: 1}) {
		t.Fatalf("视亮度 stats=%+v, want deferred without reviewed mapping", stats)
	}

	// The reviewed luminance/brightness non-equivalence must veto a
	// deliberately conflicting positive proposal: promoting IEV 845-22-059 to
	// kw:luminance is refused because the authoritative different_from
	// staging decision shows 845-21-050 and 845-22-059 must never converge
	// (845-21-050 already maps to kw:luminance).
	conflicting := ReviewedPromotionFile{
		SchemaVersion: 1, Source: "iec-60050-845", Release: "2020", Scope: "display",
		DeploymentKey: "tier6-primary",
		Promotions: []PositivePromotion{{
			ExternalID: "845-22-059", ConceptID: f.luminance, Relation: "exact_equivalent",
			Surfaces: []PromotedSurface{{SurfaceID: f.luminanceSurface, Evidence: "conflicting proposal"}},
			Reviewer: "domain-board", ReviewedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			ProvenanceLocator: "https://example.test/promotion/conflict",
		}},
	}
	if _, err := (PromotionStore{DB: f.db}).ApplyReviewedPromotion(ctx, conflicting); err == nil ||
		!strings.Contains(err.Error(), "authoritative non-equivalence") {
		t.Fatalf("conflicting positive proposal error = %v, want authoritative non-equivalence veto", err)
	}

	// Scenario B2: the reviewed promotion for 845-22-059 -> kw:brightness is
	// applied; the same candidate now converges by exact identity.
	f.promoteBrightness(t)
	stats, err = f.reconciler(embeddings).Run(ctx)
	if err != nil {
		t.Fatalf("reconcile 视亮度 after mapping: %v", err)
	}
	if stats != (ReconcileStats{Scanned: 1, Decided: 1, Merged: 1}) {
		t.Fatalf("视亮度 stats=%+v, want identity merge after reviewed mapping", stats)
	}
	brightMerged, err := (ConceptStore{DB: f.db}).GetConcept(ctx, brightCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if brightMerged.Status != "merged" || brightMerged.MergedInto == nil || *brightMerged.MergedInto != f.brightness {
		t.Fatalf("视亮度 candidate=%+v, want merged into %s", brightMerged, f.brightness)
	}

	// Scenario C: a bare context-sensitive surface with high cosine but no
	// identity evidence defers; a candidate carrying conflicting authoritative
	// evidence for both mappings rejects. Every decision is audited.
	bareCandidate, _ := f.addCandidate(t, "brightness (display context)", "brightness (display context)", "en")
	conflictCandidate, conflictCandidateSurface := f.addCandidate(t, "luminance brightness mixed", "luminance brightness mixed", "en")
	f.addCandidateEvidence(t, conflictCandidateSurface, "845-21-050", "conflicting evidence A")
	f.addCandidateEvidence(t, conflictCandidateSurface, "845-22-059", "conflicting evidence B")
	stats, err = f.reconciler(embeddings).Run(ctx)
	if err != nil {
		t.Fatalf("reconcile bare/conflicting: %v", err)
	}
	if stats != (ReconcileStats{Scanned: 2, Decided: 2, DeferredUnvalidated: 1, Rejected: 1}) {
		t.Fatalf("bare/conflicting stats=%+v, want one defer and one conflict reject", stats)
	}
	if concept, err := (ConceptStore{DB: f.db}).GetConcept(ctx, bareCandidate); err != nil || concept.Status != "provisional" {
		t.Fatalf("bare brightness candidate=%+v err=%v, want still provisional", concept, err)
	}
	if concept, err := (ConceptStore{DB: f.db}).GetConcept(ctx, conflictCandidate); err != nil || concept.Status != "provisional" {
		t.Fatalf("conflicting candidate=%+v err=%v, want rejected but unmerged", concept, err)
	}

	// Every decided candidate has exactly one audit row per decision (the
	// 视亮度 candidate was decided twice: defer then merge).
	rows, err := f.db.QueryContext(ctx, `
SELECT (input->>'candidate_concept_id')::text AS candidate_id, count(*) AS decisions
FROM kb.semid_decision_log
WHERE family = 'keyword_reconcile' AND (input->>'candidate_concept_id') = ANY($1)
GROUP BY 1 ORDER BY 1`, pq.Array(f.candidateIDs))
	if err != nil {
		t.Fatalf("query reconcile audit rows: %v", err)
	}
	defer rows.Close()
	decisions := map[string]int{}
	for rows.Next() {
		var candidateID string
		var count int
		if err := rows.Scan(&candidateID, &count); err != nil {
			t.Fatal(err)
		}
		decisions[candidateID] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantDecisions := map[string]int{lumiCandidate: 1, brightCandidate: 2, bareCandidate: 1, conflictCandidate: 1}
	for candidateID, want := range wantDecisions {
		if decisions[candidateID] != want {
			t.Fatalf("audit rows for %s = %d, want %d (all=%+v)", candidateID, decisions[candidateID], want, decisions)
		}
	}
	if len(decisions) != len(wantDecisions) {
		t.Fatalf("audit rows=%+v, want %+v", decisions, wantDecisions)
	}
}

func TestReconcileIdentityIntegrationFamilyLockSerializesConcurrentMutations(t *testing.T) {
	f := newReconcileIntegrationFixture(t)
	ctx := context.Background()

	// The promotion replay and the identity merge both acquire the keyword
	// family advisory lock inside their transactions; running them
	// concurrently must serialize instead of deadlocking, and the final state
	// must be exactly one mapping, one evidence row, and one never_merge veto.
	lumiCandidate, lumiCandidateSurface := f.addCandidate(t, "亮度", "亮度", "zh")
	f.addCandidateEvidence(t, lumiCandidateSurface, "845-21-050", "corpus-observed zh surface for IEC 845-21-050")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		stats, err := f.reconciler(labelEmbeddingClient{}).Run(ctx)
		if err == nil && stats != (ReconcileStats{Scanned: 1, Decided: 1, Merged: 1}) {
			err = fmt.Errorf("concurrent reconcile stats=%+v", stats)
		}
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := (PromotionStore{DB: f.db}).ApplyReviewedPromotion(ctx, f.promotionFile)
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent mutation: %v", err)
		}
	}

	var mappings, evidenceRows, vetoes int
	if err := f.db.QueryRowContext(ctx, `
SELECT (SELECT count(*) FROM kb.keyword_external_ids WHERE source = 'iec-60050-845' AND external_id = '845-21-050' AND release = '2020'),
       (SELECT count(*) FROM kb.keyword_surface_evidence WHERE surface_id = $1 AND source = 'iec-60050-845' AND external_id = '845-21-050'),
       (SELECT count(*) FROM kb.semid_never_merge WHERE family = 'keyword' AND node_a = $2 AND node_b = $3)`,
		f.luminanceSurface, f.brightness, f.luminance).Scan(&mappings, &evidenceRows, &vetoes); err != nil {
		t.Fatalf("verify concurrent state: %v", err)
	}
	if mappings != 1 || evidenceRows != 1 || vetoes != 1 {
		t.Fatalf("concurrent state inconsistent: mappings=%d evidence=%d vetoes=%d", mappings, evidenceRows, vetoes)
	}
	merged, err := (ConceptStore{DB: f.db}).GetConcept(ctx, lumiCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != "merged" || merged.MergedInto == nil || *merged.MergedInto != f.luminance {
		t.Fatalf("concurrent reconcile candidate=%+v, want merged into %s", merged, f.luminance)
	}
}
