package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

func TestKeywordFamilyName(t *testing.T) {
	kf := &KeywordFamily{}
	if kf.FamilyName() != "keyword" {
		t.Errorf("FamilyName: got %q, want %q", kf.FamilyName(), "keyword")
	}
}

func TestKeywordFamilyAutoAcceptPolicy(t *testing.T) {
	kf := &KeywordFamily{}
	p := kf.AutoAcceptPolicy()
	if !p.Enabled {
		t.Error("auto-accept should be enabled for keywords")
	}
	if p.MinScore != 0.8 {
		t.Errorf("MinScore: got %v, want 0.8", p.MinScore)
	}
	if p.MaxCandidates != 1 {
		t.Errorf("MaxCandidates: got %v, want 1", p.MaxCandidates)
	}
}

// D3: the family carries no normalizer of its own — it runs the one shared
// semid pipeline, defaulting to the current normalizer version.
func TestKeywordFamilyDefaultsToCurrentNormalizerVersion(t *testing.T) {
	kf := &KeywordFamily{}
	kf.ensureDefaults()
	if kf.NormalizerVersion != semid.CurrentNormalizerVersion {
		t.Errorf("NormalizerVersion: got %d, want %d", kf.NormalizerVersion, semid.CurrentNormalizerVersion)
	}
	if kf.normalizer().Version != semid.CurrentNormalizerVersion {
		t.Errorf("normalizer().Version: got %d, want %d", kf.normalizer().Version, semid.CurrentNormalizerVersion)
	}
}

func TestKeywordFamilyResolveSurfaceOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	res, err := kf.ResolveSurface(nil, "test", "_")
	if err != nil {
		t.Fatalf("ResolveSurface (off): %v", err)
	}
	if res.Verdict != "" {
		t.Errorf("expected zero resolution in off mode, got verdict %q", res.Verdict)
	}
}

func TestKeywordFamilyResolveSurfaceNoDB(t *testing.T) {
	// No DB + observe mode should still no-op gracefully.
	kf := &KeywordFamily{ResolverMode: "observe"}
	res, err := kf.ResolveSurface(nil, "test", "_")
	if err != nil {
		t.Fatalf("ResolveSurface (observe, no DB): %v", err)
	}
	if res.Verdict != "" {
		t.Errorf("expected zero resolution when DB is nil, got verdict %q", res.Verdict)
	}
}

func TestKeywordFamilyObserveSurfaceOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	res, err := kf.ObserveSurface(nil, "test", "_", "document", "art", "ctx", false)
	if err != nil {
		t.Fatalf("ObserveSurface (off): %v", err)
	}
	if res != nil {
		t.Error("expected nil resolution in off mode")
	}
}

func TestKeywordFamilyCandidateNodesOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	candidates, err := kf.CandidateNodes(nil, "test", "_")
	if err != nil {
		t.Fatalf("CandidateNodes (off): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates in off mode, got %d", len(candidates))
	}
}

func TestKeywordFamilyCandidateNodesNoDB(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "observe"}
	candidates, err := kf.CandidateNodes(nil, "test", "_")
	if err != nil {
		t.Fatalf("CandidateNodes (observe, no DB): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates with nil DB, got %d", len(candidates))
	}
}

func TestTier5FuzzyMatchGuardrails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()

	t.Run("too short, no query at all", func(t *testing.T) {
		matches, ok := kf.tier5FuzzyMatch(ctx, kf.normalizer().Normalize("abcd"), "_")
		if ok || matches != nil {
			t.Errorf("expected no fuzzy match for a 4-rune string, got ok=%v matches=%+v", ok, matches)
		}
	})

	t.Run("canonical veto short-circuits before the blocking query", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
			WithArgs("Kubernetes").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		matches, ok := kf.tier5FuzzyMatch(ctx, kf.normalizer().Normalize("Kubernetes"), "_")
		if ok || matches != nil {
			t.Errorf("expected no fuzzy match when the query is itself a pref label, got ok=%v", ok)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("misspelling within guardrails matches", func(t *testing.T) {
		ks := kf.normalizer().Normalize("kubernets")
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
			WithArgs(ks.Exact).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
			WithArgs("_", kf.NormalizerVersion, ks.Norm).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).
				AddRow("kwc_k8s", "kubernetes"))
		matches, ok := kf.tier5FuzzyMatch(ctx, ks, "_")
		if !ok || len(matches) != 1 {
			t.Fatalf("expected one fuzzy match, got ok=%v matches=%+v", ok, matches)
		}
		if matches[0].NodeID != "kwc_k8s" || matches[0].Method != "tier5_fuzzy" {
			t.Errorf("unexpected candidate: %+v", matches[0])
		}
		if matches[0].PrecomputedScore == nil || *matches[0].PrecomputedScore < 0.8 {
			t.Errorf("expected PrecomputedScore >= 0.8, got %v", matches[0].PrecomputedScore)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("digit veto rejects an otherwise-close candidate", func(t *testing.T) {
		ks := kf.normalizer().Normalize("release v2")
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
			WithArgs(ks.Exact).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
			WithArgs("_", kf.NormalizerVersion, ks.Norm).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).
				AddRow("kwc_v3", "release v3"))
		matches, ok := kf.tier5FuzzyMatch(ctx, ks, "_")
		if ok || matches != nil {
			t.Errorf("expected the digit veto to reject 'release v2' vs 'release v3', got ok=%v matches=%+v", ok, matches)
		}
	})

	// The two subtests below cover the 5-8 rune band of fuzzyCandidateScore
	// (dist>1 cutoff plus the extra firstRune check), which the subtests
	// above never exercise: "kubernets" and "release v2" both normalize to
	// 9+ runes and only reach the >=9 band's dist<=2/similarity>=0.88 rule.
	t.Run("5-8 rune band: edit distance 1 with matching first rune matches", func(t *testing.T) {
		ks := kf.normalizer().Normalize("nginx") // 5 runes
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
			WithArgs(ks.Exact).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
			WithArgs("_", kf.NormalizerVersion, ks.Norm).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).
				AddRow("kwc_nginx", "nginz")) // substitution x->z, dist 1, first rune 'n' == 'n'
		matches, ok := kf.tier5FuzzyMatch(ctx, ks, "_")
		if !ok || len(matches) != 1 {
			t.Fatalf("expected one fuzzy match for nginx/nginz, got ok=%v matches=%+v", ok, matches)
		}
		if matches[0].NodeID != "kwc_nginx" || matches[0].Method != "tier5_fuzzy" {
			t.Errorf("unexpected candidate: %+v", matches[0])
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("5-8 rune band: edit distance 1 with mismatched first rune does not match", func(t *testing.T) {
		ks := kf.normalizer().Normalize("nginx") // 5 runes
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
			WithArgs(ks.Exact).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
			WithArgs("_", kf.NormalizerVersion, ks.Norm).
			WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).
				AddRow("kwc_anginx", "anginx")) // leading insertion, dist 1, first rune 'n' != 'a'
		matches, ok := kf.tier5FuzzyMatch(ctx, ks, "_")
		if ok || matches != nil {
			t.Errorf("expected the first-rune gate to reject nginx/anginx despite dist 1, got ok=%v matches=%+v", ok, matches)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

// TestKeywordFamilyCandidateNodesReachesTier5 drives CandidateNodes end to
// end so tiers 0-4 all miss and tier 5 is what actually produces the
// candidate. The tier 0-4 mock sequence mirrors queueTierHit's ground truth
// in tiers_test.go (same package) rather than re-deriving the SQL shapes:
// tier0SQL/tier1SQL/altKeySQL/rulesSQL are exactly what tier0ExactMatch,
// tier1NormKeyMatch, tier2AlternateKeyMatch and tier3RewriteMatch issue.
// Tier 4 (initials) issues no query at all here: "kubernets" normalizes to
// 9 runes, above tier4InitialsMatch's 2-8 rune gate, so it returns false
// before ever touching the DB.
func TestKeywordFamilyCandidateNodesReachesTier5(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()
	ks := kf.normalizer().Normalize("kubernets")
	version := kf.NormalizerVersion

	// Tier 0: exact surface match misses.
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs(ks.Exact, "_").
		WillReturnRows(emptyConceptRows())
	// Tier 1: norm key match misses.
	mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
		WithArgs(ks.Norm, "_", version).
		WillReturnRows(emptyPairRows())
	// Tier 2: alnum, sorted, singular alternate keys all miss. "kubernets"
	// has Alnum == Sorted == Norm and a distinct Singular ("kubernet"), so
	// all three kinds issue their own query (tier2AlternateKeyMatch does not
	// skip a kind merely because its value repeats an earlier one).
	for _, pair := range []struct{ kind, value string }{
		{"alnum", ks.Alnum},
		{"sorted", ks.Sorted},
		{"singular", ks.Singular},
	} {
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs(pair.kind, pair.value, "_", version).
			WillReturnRows(emptyPairRows())
	}
	// Tier 3: no enabled rewrite rules -> immediate miss, no retry queries.
	mock.ExpectQuery(regexp.QuoteMeta(rulesSQL)).
		WithArgs("_").
		WillReturnRows(emptyRuleRows())
	// Tier 5: canonical veto query misses, then the trigram-blocked fuzzy
	// query hits.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs(ks.Exact).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs("_", version, ks.Norm).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).AddRow("kwc_k8s", "kubernetes"))

	candidates, err := kf.CandidateNodes(ctx, "kubernets", "_")
	if err != nil {
		t.Fatalf("CandidateNodes: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Method != "tier5_fuzzy" {
		t.Fatalf("expected exactly one tier5_fuzzy candidate, got %+v", candidates)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestResolveSurfaceTier5AutoAccepts drives the whole stack end to end (task
// 5, spec §18.2's required-test-set framing): KeywordFamily.ResolveSurface ->
// normalizer -> tier ladder (miss tiers 0-4, hit tier 5) -> Kernel.Resolve's
// scoring/adjudication -> ConceptStore.FollowMerge's post-resolution chase,
// ending in auto_accepted for a real misspelling ("kubernets" ->
// "Kubernetes"). The tier 0-4 mock sequence mirrors
// TestKeywordFamilyCandidateNodesReachesTier5 above (Task 4); this test
// additionally exercises Resolve's scoring/adjudication and the FollowMerge
// lookup that CandidateNodes alone never reaches.
func TestResolveSurfaceTier5AutoAccepts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// "observe" is the family's shipped KEYWORD_RESOLVER_MODE: resolution runs
	// but no downstream consumer is connected. modeActive() returns true only
	// for "observe"/"on" — without it the family's entry points no-op and
	// this test would exercise nothing.
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()
	ks := kf.normalizer().Normalize("kubernets")
	version := kf.NormalizerVersion

	// Tier 0: exact surface match misses.
	mock.ExpectQuery(regexp.QuoteMeta(tier0SQL)).
		WithArgs(ks.Exact, "_").
		WillReturnRows(emptyConceptRows())
	// Tier 1: norm key match misses.
	mock.ExpectQuery(regexp.QuoteMeta(tier1SQL)).
		WithArgs(ks.Norm, "_", version).
		WillReturnRows(emptyPairRows())
	// Tier 2: alnum, sorted, singular alternate keys all miss. "kubernets"
	// has Alnum == Sorted == Norm and a distinct Singular ("kubernet"), so
	// all three kinds issue their own query.
	for _, pair := range []struct{ kind, value string }{
		{"alnum", ks.Alnum},
		{"sorted", ks.Sorted},
		{"singular", ks.Singular},
	} {
		mock.ExpectQuery(regexp.QuoteMeta(altKeySQL)).
			WithArgs(pair.kind, pair.value, "_", version).
			WillReturnRows(emptyPairRows())
	}
	// Tier 3: no enabled rewrite rules -> immediate miss, no retry queries.
	mock.ExpectQuery(regexp.QuoteMeta(rulesSQL)).
		WithArgs("_").
		WillReturnRows(emptyRuleRows())
	// Tier 4 (initials) issues no query at all: "kubernets" normalizes to 9
	// runes, above tier4InitialsMatch's 2-8 rune gate.
	// Tier 5: the canonical-pref veto query misses, then the trigram-blocked
	// fuzzy query hits with a single candidate ("kubernetes", edit distance
	// 1, similarity 0.9) -- above the family's 0.8 auto-accept threshold.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs(ks.Exact).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs("_", version, ks.Norm).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).AddRow("kwc_k8s", "kubernetes"))
	// FollowMerge inside ResolveSurface reads the resolved concept: a live
	// concept (merged_into nil) resolves to itself.
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kwc_k8s").
		WillReturnRows(conceptRows("kwc_k8s", "Kubernetes", "_", "active", "none"))

	res, err := kf.ResolveSurface(ctx, "kubernets", "_")
	if err != nil {
		t.Fatalf("ResolveSurface: %v", err)
	}
	if len(res.Matches) != 1 {
		t.Errorf("expected exactly one auto-accepted match, got %d (%+v)", len(res.Matches), res.Matches)
	}
	if res.Verdict != semid.VerdictAutoAccept {
		t.Errorf("expected auto_accepted, got verdict=%s matches=%+v", res.Verdict, res.Matches)
	}
	if res.ResolvedNodeID != "kwc_k8s" {
		t.Errorf("expected kwc_k8s, got %s", res.ResolvedNodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
