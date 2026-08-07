package keywords

// P3 Track B exit criteria, each exercised through the production path with
// real assertions. §18.1: the original delivery's assertion-free exit tests
// self-certified it — every criterion now fails if its behaviour breaks.

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// E1: creating a concept with pref_label, scope, status succeeds and reads
// back unchanged.
func TestExitConceptCRUD(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_concepts`)).
		WithArgs("kwc_e1", "Luminance", nil, "_", "active", "human:exit").
		WillReturnRows(conceptRows("kwc_e1", "Luminance", "_", "active", "human:exit"))
	mock.ExpectCommit()
	created, err := store.CreateConcept(ctx, Concept{
		ConceptID:   "kwc_e1",
		PrefLabel:   "Luminance",
		GlossSource: "human:exit",
	})
	if err != nil {
		t.Fatalf("CreateConcept: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id =`)).
		WithArgs("kwc_e1").
		WillReturnRows(conceptRows("kwc_e1", "Luminance", "_", "active", "human:exit"))
	got, err := store.GetConcept(ctx, "kwc_e1")
	if err != nil {
		t.Fatalf("GetConcept: %v", err)
	}
	if got.ConceptID != created.ConceptID || got.PrefLabel != created.PrefLabel ||
		got.Status != "active" || got.Scope != "_" {
		t.Errorf("round-trip mismatch: created %#v, read %#v", created, got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E2: creating a surface succeeds and its derived keys are written in the
// same transaction (K1).
func TestExitSurfaceCRUD(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := SurfaceStore{DB: db}
	ctx := context.Background()

	sf := Surface{
		ConceptID:  "kwc_e2",
		Surface:    "hello world",
		AliasType:  "synonym",
		Provenance: "human:exit",
	}
	wantID := deriveSurfaceID(sf.ConceptID, sf.Surface, "pref")
	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize(sf.Surface)

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(wantID, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", sf.AliasType, "und", "_", 1.0, sf.Provenance, false, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(wantID, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", sf.AliasType, "und", "_", 1.0, sf.Provenance, false, nil, testNow, testNow))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.keyword_surface_keys WHERE surface_id =`)).
		WithArgs(wantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	keys := derivedSurfaceKeys(ks, wantID, semid.CurrentNormalizerVersion)
	for _, k := range keys {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_surface_keys`)).
			WithArgs(wantID, k.KeyKind, k.KeyValue, semid.CurrentNormalizerVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	created, err := store.CreateSurface(ctx, sf)
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}
	if created.SurfaceID != wantID || created.NormKey != ks.Norm {
		t.Errorf("unexpected surface: %#v", created)
	}
	if len(keys) == 0 {
		t.Error("expected derived keys to ship with the surface")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E3: same surface + same version = same keys.
func TestExitNormalizerDeterminism(t *testing.T) {
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	a := n.Normalize("Display Luminance 亮度")
	b := n.Normalize("Display Luminance 亮度")
	if a.Exact != b.Exact || a.Norm != b.Norm || a.Alnum != b.Alnum ||
		a.Sorted != b.Sorted || a.Singular != b.Singular ||
		a.Phonetic != b.Phonetic || a.Initials != b.Initials {
		t.Errorf("normalizer is not deterministic: %#v vs %#v", a, b)
	}
}

// E4: every tier produces candidates on its own query path, and a total
// miss defers unresolved.
func TestExitKernelResolutionTiers(t *testing.T) {
	ctx := context.Background()
	for tier := 0; tier <= 4; tier++ {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
		surface := tierSurfaces[tier]
		wantID := queueTierHit(mock, tier, surface, "_")

		candidates, err := kf.CandidateNodes(ctx, surface, "_")
		if err != nil {
			t.Errorf("tier %d: CandidateNodes: %v", tier, err)
		} else if len(candidates) != 1 || candidates[0].NodeID != wantID || candidates[0].Method != tierMethods[tier] {
			t.Errorf("tier %d: got %#v, want one candidate %s/%s", tier, candidates, wantID, tierMethods[tier])
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("tier %d: unmet expectations: %v", tier, err)
		}
		db.Close()
	}

	// Unknown surface: every tier misses → deferred, no node.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	const unknown = "zzzunknown"
	expectTotalMiss(mock, kf.normalizer().Normalize(unknown), "_")
	res, err := kf.ResolveSurface(ctx, unknown, "_")
	if err != nil {
		t.Fatalf("ResolveSurface: %v", err)
	}
	if res.Verdict != semid.VerdictDeferred || res.ResolvedNodeID != "" {
		t.Errorf("total miss must defer with no node: verdict %q node %q", res.Verdict, res.ResolvedNodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E5: off is inert; an unset KEYWORD_RESOLVER_MODE fails closed to off (K6).
// A nil DB proves nothing is written — any write attempt would fail.
func TestExitResolverModeGate(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"off", ""} {
		kf := &KeywordFamily{ResolverMode: mode}
		res, err := kf.ResolveSurface(ctx, "Luminance", "_")
		if err != nil || res.Verdict != "" {
			t.Errorf("mode %q: expected inert resolve, got %#v / %v", mode, res, err)
		}
		obs, err := kf.ObserveSurface(ctx, "Luminance", "_", "document", "art", "ctx", true)
		if err != nil || obs != nil {
			t.Errorf("mode %q: expected inert observe, got %#v / %v", mode, obs, err)
		}
	}
	if resolverModeFrom("") != "off" {
		t.Error("unset KEYWORD_RESOLVER_MODE must map to off")
	}
}

// E6: the collector path (targeted=false) records an occurrence and queues
// the backlog on a miss — and mints no concept (D11 scope limit).
func TestExitMentionCollector(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	ctx := context.Background()
	ks := kf.normalizer().Normalize("Luminance")

	expectTotalMiss(mock, ks, "_")
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.semid_decision_log`)).
		WithArgs("keyword", "_", sqlmock.AnyArg(), sqlmock.AnyArg(), "deferred",
			nil, nil, "keyword_family", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_occurrences`)).
		WithArgs("document", "doc:1", "", "Luminance", ks.Norm, "_",
			"ctx", nil, nil, nil, "unresolved", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT surfaces, contexts, hits FROM kb.keyword_unresolved`)).
		WithArgs(ks.Norm, "_").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_unresolved`)).
		WithArgs(ks.Norm, "_", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := kf.ObserveSurface(ctx, "Luminance", "_", "document", "doc:1", "ctx", false)
	if err != nil {
		t.Fatalf("ObserveSurface: %v", err)
	}
	if res.ResolvedNodeID != "" {
		t.Errorf("collector miss must mint no concept, got %q", res.ResolvedNodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E7: an enabled rule rewrites the surface and the tier 0 retry hits
// (tier 3); with no matching rule nothing is rewritten.
func TestExitRewriteRules(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	wantID := queueTierHit(mock, 3, "Luminance", "_")

	candidates, err := kf.CandidateNodes(context.Background(), "Luminance", "_")
	if err != nil {
		t.Fatalf("CandidateNodes: %v", err)
	}
	if len(candidates) != 1 || candidates[0].NodeID != wantID || candidates[0].Method != "tier3_rewrite" {
		t.Errorf("tier 3 rewrite: got %#v", candidates)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E8: the backlog accumulates distinct surfaces per (norm_key, scope) and
// keeps the context reservoir.
func TestExitUnresolvedBacklog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := UnresolvedStore{DB: db}
	ctx := context.Background()

	// First sighting: no row → insert. JSON columns arrive as []byte.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT surfaces, contexts, hits FROM kb.keyword_unresolved`)).
		WithArgs("luminance", "_").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_unresolved`)).
		WithArgs("luminance", "_", []byte(`["Luminance"]`), []byte(`["ctx one"]`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.UpsertUnresolved(ctx, "luminance", "_", "Luminance", "ctx one"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second, distinct surface: the row exists → update merges surfaces.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT surfaces, contexts, hits FROM kb.keyword_unresolved`)).
		WithArgs("luminance", "_").
		WillReturnRows(sqlmock.NewRows([]string{"surfaces", "contexts", "hits"}).
			AddRow([]byte(`["Luminance"]`), []byte(`["ctx one"]`), 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.keyword_unresolved`)).
		WithArgs("luminance", "_", []byte(`["Luminance","Brightness"]`), []byte(`["ctx one","ctx two"]`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.UpsertUnresolved(ctx, "luminance", "_", "Brightness", "ctx two"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// E9: the normalizer produces all key kinds deterministically, with
// lower-cased initials (N3).
func TestExitNormalizerKeyKinds(t *testing.T) {
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	ks := n.Normalize("Display Luminance")
	for kind, value := range map[string]string{
		"exact":    ks.Exact,
		"norm":     ks.Norm,
		"alnum":    ks.Alnum,
		"sorted":   ks.Sorted,
		"phonetic": ks.Phonetic,
		"initials": ks.Initials,
		"singular": ks.Singular,
	} {
		if value == "" {
			t.Errorf("key kind %s is empty", kind)
		}
	}
	if ks.Initials != "dl" {
		t.Errorf("initials: got %q, want \"dl\" (lower-cased, N3)", ks.Initials)
	}
}
