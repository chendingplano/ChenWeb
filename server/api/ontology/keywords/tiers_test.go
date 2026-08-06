package keywords

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// §18.2 per-tier query coverage: each tier's query runs with the caller's
// arguments and its match flows into the candidate set with an honest method
// tag. Score and adjudication coverage lives in semid's kernel tests.

var tierSurfaces = map[int]string{
	0: "Luminance",
	1: "Luminance",
	2: "Luminance",
	3: "Luminance",
	4: "DL",
}

var tierMethods = map[int]string{
	0: "tier0_exact",
	1: "tier1_norm",
	2: "tier2_altkey",
	3: "tier3_rewrite",
	4: "tier4_initials",
}

const (
	tier0SQL  = `SELECT s.concept_id FROM kb.keyword_surfaces s WHERE s.surface =`
	tier1SQL  = `SELECT s.concept_id, s.norm_key FROM kb.keyword_surfaces s WHERE s.norm_key =`
	altKeySQL = `SELECT s.concept_id, s.norm_key FROM kb.keyword_surface_keys sk`
	rulesSQL  = `SELECT ` + rewriteRuleColumns + ` ` + rewriteRuleFrom + ` WHERE enabled = true AND scope =`
)

func emptyConceptRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"concept_id"})
}

func emptyPairRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"concept_id", "norm_key"})
}

func emptyRuleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"rule_id", "pattern", "replacement", "scope", "enabled", "provenance", "create_time", "modify_time",
	})
}

// conceptRows builds a kb.keyword_concepts result set for one live concept.
func conceptRows(id, label, scope, status, glossSource string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow(id, label, nil, scope, status, nil, glossSource, testNow, testNow)
}

// queueTierHit queues expectations so every tier before `tier` misses and
// `tier` itself returns one candidate, kwc_t<tier>. It mirrors the exact
// query sequence CandidateNodes runs (§9.1 early exit).
func queueTierHit(mock sqlmock.Sqlmock, tier int, surface, scope string) string {
	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize(surface)
	wantID := fmt.Sprintf("kwc_t%d", tier)
	version := semid.CurrentNormalizerVersion

	if tier > 0 {
		mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
			WithArgs(ks.Exact, scope).
			WillReturnRows(emptyConceptRows())
	}
	if tier > 1 {
		mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
			WithArgs(ks.Norm, scope, version).
			WillReturnRows(emptyPairRows())
	}
	if tier > 2 {
		for _, pair := range []struct{ kind, value string }{
			{"alnum", ks.Alnum},
			{"sorted", ks.Sorted},
			{"singular", ks.Singular},
		} {
			if pair.value == "" {
				continue
			}
			mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
				WithArgs(pair.kind, pair.value, scope).
				WillReturnRows(emptyPairRows())
		}
	}

	if tier == 3 {
		// One enabled rule byte-equal to the raw surface; the tier 0 retry
		// with the rewritten form hits.
		mock.ExpectQuery(regexp.QuoteMeta(rulesSQL)).
			WithArgs(scope).
			WillReturnRows(emptyRuleRows().AddRow("kwr_1", surface, "Brightness", scope, true, "human:tester", testNow, testNow))
		rb := (semid.Normalizer{Version: version}).Normalize("Brightness")
		mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
			WithArgs(rb.Exact, scope).
			WillReturnRows(emptyConceptRows().AddRow(wantID))
		return wantID
	}
	if tier > 3 {
		mock.ExpectQuery(regexp.QuoteMeta(rulesSQL)).
			WithArgs(scope).
			WillReturnRows(emptyRuleRows())
	}

	switch tier {
	case 0:
		mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
			WithArgs(ks.Exact, scope).
			WillReturnRows(emptyConceptRows().AddRow(wantID))
	case 1:
		mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
			WithArgs(ks.Norm, scope, version).
			WillReturnRows(emptyPairRows().AddRow(wantID, ks.Norm))
	case 2:
		// The first non-empty alternate key (alnum) hits; later keys never run.
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs("alnum", ks.Alnum, scope).
			WillReturnRows(emptyPairRows().AddRow(wantID, ks.Norm))
	case 4:
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs("initials", ks.Norm, scope).
			WillReturnRows(emptyPairRows().AddRow(wantID, "display luminance"))
	}
	return wantID
}

func TestCandidateNodesPerTier(t *testing.T) {
	for tier := 0; tier <= 4; tier++ {
		t.Run(tierMethods[tier], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
			surface := tierSurfaces[tier]
			wantID := queueTierHit(mock, tier, surface, "_")

			candidates, err := kf.CandidateNodes(context.Background(), surface, "_")
			if err != nil {
				t.Fatalf("CandidateNodes: %v", err)
			}
			if len(candidates) != 1 {
				t.Fatalf("expected 1 candidate, got %d", len(candidates))
			}
			if candidates[0].NodeID != wantID {
				t.Errorf("NodeID: got %q, want %q", candidates[0].NodeID, wantID)
			}
			if candidates[0].Method != tierMethods[tier] {
				t.Errorf("Method: got %q, want %q", candidates[0].Method, tierMethods[tier])
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// A single unambiguous tier hit resolves end to end: score ≥ 0.8 with one
// candidate auto-accepts (§13.1 policy), and the merge chase confirms the
// survivor.
func TestResolveSurfaceTier0AutoAccepts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	wantID := queueTierHit(mock, 0, "Luminance", "_")
	// FollowMerge: the resolved id is a live concept.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+conceptColumns+` `+conceptFrom+` WHERE concept_id =`)).
		WithArgs(wantID).
		WillReturnRows(conceptRows(wantID, "Luminance", "_", "active", "human:tester"))

	res, err := kf.ResolveSurface(context.Background(), "Luminance", "_")
	if err != nil {
		t.Fatalf("ResolveSurface: %v", err)
	}
	if res.Verdict != semid.VerdictAutoAccept {
		t.Errorf("verdict: got %q, want auto_accepted", res.Verdict)
	}
	if res.ResolvedNodeID != wantID {
		t.Errorf("ResolvedNodeID: got %q, want %q", res.ResolvedNodeID, wantID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// §18.2 scope round-trip: a surface written at scope "ks" is found by a
// resolve at "ks" — scope is the caller's input end to end (K2), never a
// family constant.
func TestResolveScopeRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	const scope = "ks_ventilator"

	// Write at ks: CreateSurface carries the caller's scope into the row.
	store := SurfaceStore{DB: db, NormalizerVersion: semid.CurrentNormalizerVersion}
	sf := Surface{
		ConceptID:  "kwc_scope",
		Surface:    "Luminance",
		LabelRole:  "pref",
		AliasType:  "pref",
		Provenance: "human:tester",
		Scope:      scope,
	}
	sid := deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)
	ks := (semid.Normalizer{Version: semid.CurrentNormalizerVersion}).Normalize(sf.Surface)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.keyword_surfaces`)).
		WithArgs(sid, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", "pref", "en", scope, 1.0, "human:tester", false, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"surface_id", "concept_id", "surface", "norm_key", "norm_version",
			"label_role", "alias_type", "lang", "scope", "confidence",
			"provenance", "locked", "evidence", "create_time", "modify_time",
		}).AddRow(sid, sf.ConceptID, sf.Surface, ks.Norm, semid.CurrentNormalizerVersion,
			"pref", "pref", "en", scope, 1.0, "human:tester", false, nil, testNow, testNow))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.keyword_surface_keys WHERE surface_id =`)).
		WithArgs(sid).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, k := range derivedSurfaceKeys(ks, sid, semid.CurrentNormalizerVersion) {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.keyword_surface_keys`)).
			WithArgs(sid, k.KeyKind, k.KeyValue, semid.CurrentNormalizerVersion).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	if _, err := store.CreateSurface(ctx, sf); err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}

	// Read at ks: tier 0's query carries the same scope and finds it.
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs("Luminance", scope).
		WillReturnRows(emptyConceptRows().AddRow("kwc_scope"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+conceptColumns+` `+conceptFrom+` WHERE concept_id =`)).
		WithArgs("kwc_scope").
		WillReturnRows(conceptRows("kwc_scope", "Luminance", scope, "active", "human:tester"))

	res, err := kf.ResolveSurface(ctx, "Luminance", scope)
	if err != nil {
		t.Fatalf("ResolveSurface: %v", err)
	}
	if res.Verdict != semid.VerdictAutoAccept || res.ResolvedNodeID != "kwc_scope" {
		t.Errorf("scope round-trip failed: verdict %q node %q", res.Verdict, res.ResolvedNodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
