# Keyword Module Step 11 — Tier 5 Fuzzy Matching + Minimum Reconciliation Loop

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement build-order Step 11 of `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md` (§19 item 11, "§2 REQ-1 — tiers 5-6 and the minimum reconciliation loop that unifies translations"): wire Tier 5 (fuzzy, edit-distance) into the online resolve path, and build an offline batch reconciler that uses Tier 6 (multilingual embedding) to merge D11 auto-created provisional concepts into their true match — the mechanism Appendix A Stage 5 describes.

**Architecture:**
- **Tier 5 (online):** `KeywordFamily.CandidateNodes` gains a fifth tier, blocked by a new `pg_trgm` index and gated by §9.2's exact length-banded edit-distance rules plus three absolute vetoes (digit, canonical, negation/affix). Because tier 5's evidence (edit distance) isn't a shared derived key, `semid.NodeCandidate` gains a `PrecomputedScore` field that `Kernel.Resolve` uses instead of recomputing via `Score()` — the existing discrete tiers 0-4 are unaffected (nil pointer, unchanged behavior). No change to `Adjudicate`/`AutoAcceptPolicy` is needed: the guardrail bands are constructed so every candidate that survives them already scores ≥ the family's existing 0.8 `MinScore` (worst case, length-5 with edit distance 1, scores exactly 0.8), so gating happens once, in the guardrail, not twice.
- **Tier 6 + reconciliation (offline):** per the user's explicit decision, tier 6 (cross-lingual embedding) never runs on the online path — it stays inside a new `keywords.Reconciler`, run by a new `cmd/keyword-reconcile` batch binary, mirroring the existing `doc-processing` entity-reconciliation block→adjudicate→apply shape. It scans `kb.keyword_concepts` rows the D11 auto-create path minted (`status='provisional' AND gloss_source='auto:d11'`), blocks candidates two ways — lexical (`pg_trgm` on `pref_label`) and semantic (embedding cosine, computed in Go, no pgvector column) — and merges through the already-built, guardrailed `ConceptStore.MergeConcept`.
- **Scope decisions (see "Non-goals" below):** this is deliberately the *minimum* loop the build-order step names, not the full R1–R7 pipeline. No LLM call is used anywhere in this plan — D11/§13.1 only requires deterministic, attributable scoring to auto-accept a merge; an LLM-adjudicated middle band is explicitly out of scope here.

**Tech Stack:** Go 1.25, PostgreSQL (`pg_trgm` extension, goose migrations), the existing `shared/go/api/llm` client (`OpenAIJSONClient.EmbedBatch`) for tier 6 embeddings, `github.com/DATA-DOG/go-sqlmock` for store tests.

## Global Constraints

- Module path: `github.com/chendingplano/deepdoc`. All new imports use this path, e.g. `github.com/chendingplano/deepdoc/server/api/ontology/keywords`.
- Migrations live in `ChenWeb/project_migrations/`, named `YYYYMMDDHHMMSS_description.sql` with `-- +goose Up` / `-- +goose Down` markers (goose Project Track). The latest existing keyword-module migration is `20260805000003`; the next slot is `20260806000001`.
- No hardcoded prompts (`ChenWeb/CLAUDE.md` §2) — not applicable to this plan, since no LLM prompt is used.
- No new env-var *tuning* knobs for thresholds (match existing style: `KeywordFamily.AutoAcceptPolicy()`'s `MinScore: 0.8` is a hardcoded constant, not env-configurable) — thresholds in this plan are Go constants with a comment citing spec §22 Q1 ("ship conservative, tune"). The one exception is `KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME`, which selects *infrastructure* (which model/server to call), the same category as the existing `EMBEDDING_MODEL_NAME`/`STRUCTURE_MODEL_NAME` env vars — not a tuning knob.
- Follow `ChenWeb/CLAUDE.md` §1.3 (Surgical Changes): don't reformat or refactor adjacent code; match existing comment density and doc-comment style exactly (every exported/package function in this codebase carries a spec-referencing doc comment — do the same).
- Run from `/Users/cding/Workspace/ChenWeb` for all Go commands.

## Non-goals for this plan (explicitly deferred, do not build)

- **R1 harvest** (Schwartz-Hearst parenthetical extraction from raw text) — the reconciler only processes rows the online path already wrote (auto-created provisional concepts); it does not mine new candidates from document prose.
- **R2 prune's negative-cache / R4 assemble / R5 decide (LLM bulk adjudication)** — no LLM call anywhere in this plan. Tiers 5-6 are deterministic (edit distance, embedding cosine) per D11/§13.1, which permits auto-accept without a model in the loop.
- **A `kb.keyword_reconcile_runs` watermark/run-tracking table** (mirroring `kb.reconcile_runs`) — the reconciler re-scans the full `status='provisional' AND gloss_source='auto:d11'` set on every run; a merged concept's status flips to `'merged'` so it naturally drops out of the next scan. This is simple and correct at pilot scale; a watermark is a reasonable future addition if the provisional-concept count grows large enough that a full rescan becomes expensive — not needed now.
- **Snapshot activation, rewrite-rule auto-promotion, tier-6-online** — all explicitly out of scope per the spec's own deferred list (§20.1) and the user's decision to keep tier 6 reconciliation-only.
- **§14.2's alignment-conflict gate** — a no-op today (no `aligns_to_term` assertions exist until Step 12); `ConceptStore.MergeConcept` already has a comment noting this, unchanged by this plan.

---

## Task 1: Enable `pg_trgm` and add trigram indexes

**Files:**
- Create: `ChenWeb/project_migrations/20260806000001_enable_pg_trgm_and_keyword_trigram_indexes.sql`

**Interfaces:**
- Produces: the `similarity(text, text)` SQL function (from `pg_trgm`), usable in any query in later tasks; a GIN index on `kb.keyword_surfaces.norm_key` and on `kb.keyword_concepts.pref_label`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Spec 2026080403 §19 step 11: pg_trgm backs Tier 5's fuzzy blocking query
-- (online, KeywordFamily.tier5FuzzyMatch) and the reconciler's lexical
-- blocking pass (offline, keywords.Reconciler) — the codebase had no
-- pg_trgm before this (2026080601-bug's inventory confirmed no existing
-- enablement). Indexes are on norm_key (surfaces) and pref_label (concepts)
-- because both blocking queries compare against those columns, not the
-- verbatim `surface` column.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_keyword_surfaces_norm_key_trgm
    ON kb.keyword_surfaces USING gin (norm_key gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_keyword_concepts_pref_label_trgm
    ON kb.keyword_concepts USING gin (pref_label gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_keyword_concepts_pref_label_trgm;
DROP INDEX IF EXISTS kb.idx_keyword_surfaces_norm_key_trgm;
-- pg_trgm itself is not dropped: other objects in the database may depend
-- on it, and CREATE EXTENSION IF NOT EXISTS on Up is idempotent regardless.
```

- [ ] **Step 2: Apply and verify**

Run (from `ChenWeb/`, with `mise dev`/the app's own migration runner already applying goose migrations on startup — or invoke the project's goose runner directly if the dev server isn't live):

```bash
psql "$PG_DSN" -c "SELECT extname FROM pg_extension WHERE extname = 'pg_trgm';"
```

Expected: one row, `pg_trgm`. Then:

```bash
psql "$PG_DSN" -c "SELECT indexname FROM pg_indexes WHERE tablename IN ('keyword_surfaces','keyword_concepts') AND indexname LIKE '%trgm%';"
```

Expected: both `idx_keyword_surfaces_norm_key_trgm` and `idx_keyword_concepts_pref_label_trgm`.

- [ ] **Step 3: Commit**

```bash
git add project_migrations/20260806000001_enable_pg_trgm_and_keyword_trigram_indexes.sql
git commit -m "Enable pg_trgm and add trigram indexes for keyword tier 5/reconciliation blocking"
```

---

## Task 2: `semid.NodeCandidate.PrecomputedScore` + `Kernel.Resolve`

**Files:**
- Modify: `ChenWeb/server/api/ontology/semid/kernel.go:18-25` (the `NodeCandidate` struct), `:92-98` (the scoring loop in `Resolve`)
- Test: `ChenWeb/server/api/ontology/semid/kernel_test.go` (new file — no existing `kernel_test.go`; check with `ls ChenWeb/server/api/ontology/semid/*_test.go` first since other `_test.go` files exist for `normalizer.go`/`score.go`/`termfamily.go`)

**Interfaces:**
- Produces: `semid.NodeCandidate.PrecomputedScore *float64` — when non-nil, `Kernel.Resolve` uses this value directly instead of calling `Score(bundle, c.KeyBundle)`. Consumed by Task 4's `tier5FuzzyMatch`.

- [ ] **Step 1: Write the failing test**

```go
package semid

import (
	"context"
	"testing"
)

// fixedScoreFamily is a minimal FamilyAdapter whose one candidate carries a
// PrecomputedScore that Score(bundle, KeyBundle{}) could never produce on
// its own (KeyBundle{} has no canonical/alternate keys, so Score returns 0).
type fixedScoreFamily struct {
	score float64
}

func (f fixedScoreFamily) FamilyName() string { return "fixed" }
func (f fixedScoreFamily) AutoAcceptPolicy() AutoAcceptPolicy {
	return AutoAcceptPolicy{Enabled: true, MinScore: 0.8, MaxCandidates: 1}
}
func (f fixedScoreFamily) CandidateNodes(ctx context.Context, input, scope string) ([]NodeCandidate, error) {
	s := f.score
	return []NodeCandidate{{NodeID: "n1", Method: "tier5_fuzzy", PrecomputedScore: &s}}, nil
}

func TestKernelResolveUsesPrecomputedScore(t *testing.T) {
	k := Kernel{Family: fixedScoreFamily{score: 0.9}, Normalizer: Normalizer{Version: CurrentNormalizerVersion}}
	res, err := k.Resolve(context.Background(), "kubernets", "_")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Matches) != 1 || res.Matches[0].Score != 0.9 {
		t.Fatalf("expected one match scored 0.9, got %+v", res.Matches)
	}
	if res.Verdict != VerdictAutoAccept {
		t.Fatalf("expected auto_accepted at score 0.9 >= MinScore 0.8, got %s", res.Verdict)
	}
}

func TestKernelResolveDropsZeroPrecomputedScore(t *testing.T) {
	k := Kernel{Family: fixedScoreFamily{score: 0}, Normalizer: Normalizer{Version: CurrentNormalizerVersion}}
	res, err := k.Resolve(context.Background(), "x", "_")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Matches) != 0 || res.Verdict != VerdictDeferred {
		t.Fatalf("expected deferred with zero matches, got verdict=%s matches=%+v", res.Verdict, res.Matches)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/ontology/semid/... -run TestKernelResolveUsesPrecomputedScore -v`
Expected: compile error — `NodeCandidate` has no field `PrecomputedScore`.

- [ ] **Step 3: Implement**

In `kernel.go`, change the `NodeCandidate` struct (currently lines 18-25):

```go
type NodeCandidate struct {
	NodeID    string
	KeyBundle KeyBundle
	// Payload carries family-specific detail (e.g. the term row) for the
	// change-set a governed family produces.
	Payload any
	Method  string
	// PrecomputedScore carries a continuous similarity score the family
	// computed itself (tier 5 edit distance, tier 6 embedding cosine) for
	// evidence that isn't a shared derived key and so can't be scored by
	// Score(surface, candidate KeyBundle) alone (spec 2026080403 §13.1).
	// nil for tiers 0-4, whose evidence is a shared key and is scored by
	// Score as before.
	PrecomputedScore *float64
}
```

And in `Resolve` (currently lines 92-98):

```go
	matches := make([]ScoredMatch, 0, len(candidates))
	for _, c := range candidates {
		s := Score(bundle, c.KeyBundle)
		if c.PrecomputedScore != nil {
			s = *c.PrecomputedScore
		}
		if s > 0 {
			matches = append(matches, ScoredMatch{NodeID: c.NodeID, Score: s, Reason: scoreReason(s), Method: c.Method})
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/semid/... -v`
Expected: PASS, including the two new tests and every pre-existing `semid` test (no regression — tiers 0-4 never set `PrecomputedScore`, so `Score(bundle, c.KeyBundle)` still runs for them exactly as before).

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/semid/kernel.go server/api/ontology/semid/kernel_test.go
git commit -m "semid: let a candidate carry a precomputed continuous score (spec step 11)"
```

---

## Task 3: Fuzzy-matching guardrail primitives

**Files:**
- Create: `ChenWeb/server/api/ontology/keywords/fuzzy.go`
- Test: `ChenWeb/server/api/ontology/keywords/fuzzy_test.go`

**Interfaces:**
- Produces: `runeLevenshtein(a, b string) int`, `normalizedSimilarity(a, b string) float64`, `firstRune(s string) rune`, `digitsDiffer(a, b string) bool`, `hasNegationAffixMismatch(a, b string) bool` — all package-private, shared by Task 4's online `tier5FuzzyMatch` and Task 6's offline `Reconciler`.

- [ ] **Step 1: Write the failing tests**

```go
package keywords

import "testing"

func TestRuneLevenshtein(t *testing.T) {
	cases := []struct{ a, b string; want int }{
		{"kubernets", "kubernetes", 1},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"显示亮度", "显示屏幕亮度", 2},
	}
	for _, c := range cases {
		if got := runeLevenshtein(c.a, c.b); got != c.want {
			t.Errorf("runeLevenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizedSimilarity(t *testing.T) {
	// worst case at the length-5 boundary with edit distance 1 must be
	// exactly 0.8 -- the floor that keeps every tier5 candidate at or above
	// KeywordFamily's existing 0.8 MinScore without a separate threshold.
	if got := normalizedSimilarity("abcde", "abcdf"); got != 0.8 {
		t.Errorf("normalizedSimilarity boundary case = %v, want 0.8", got)
	}
	if got := normalizedSimilarity("kubernets", "kubernetes"); got < 0.88 {
		t.Errorf("normalizedSimilarity(kubernets,kubernetes) = %v, want >= 0.88", got)
	}
}

func TestDigitsDiffer(t *testing.T) {
	if !digitsDiffer("cd/m2", "cd/m3") {
		t.Error("cd/m2 vs cd/m3 should veto: digits differ")
	}
	if digitsDiffer("v2 engine", "v2engine") {
		t.Error("same digits, different spacing should not veto")
	}
	if digitsDiffer("kubernets", "kubernetes") {
		t.Error("neither string has a digit; must not veto")
	}
}

func TestHasNegationAffixMismatch(t *testing.T) {
	if !hasNegationAffixMismatch("compliant", "noncompliant") {
		t.Error("compliant vs noncompliant must veto")
	}
	if !hasNegationAffixMismatch("encrypted", "unencrypted") {
		t.Error("encrypted vs unencrypted must veto")
	}
	if !hasNegationAffixMismatch("harm", "harmless") {
		t.Error("harm vs harmless must veto -- root vs root+'-less' is the negation-suffix case §9.2 names")
	}
	if hasNegationAffixMismatch("design", "deploy") {
		t.Error("design vs deploy must not veto -- 'de' is not a real negation prefix here")
	}
	if hasNegationAffixMismatch("kubernets", "kubernetes") {
		t.Error("kubernets vs kubernetes must not veto")
	}
	// "harmful" vs "harmless" (two different derivational suffixes on the
	// same root, not a bare root vs "root+less") is deliberately NOT vetoed
	// here: §9.2 names "-less" as a negation suffix, not a same-root
	// different-suffix detector, and a same-root-prefix check for that
	// broader pattern is unsafe -- it also fires on unrelated words that
	// merely share a prefix before "-less", including real typo pairs tier
	// 5 exists to catch:
	if hasNegationAffixMismatch("wireless", "wireles") {
		t.Error("wireless vs wireles is a one-letter-deletion typo (edit distance 1) and must not veto")
	}
	if hasNegationAffixMismatch("classless", "classic") {
		t.Error("classless vs classic share no negation relationship and must not veto")
	}
}

func TestFirstRune(t *testing.T) {
	if firstRune("kubernetes") != 'k' {
		t.Error("expected 'k'")
	}
	if firstRune("") != 0 {
		t.Error("expected 0 for empty string")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestRuneLevenshtein|TestNormalizedSimilarity|TestDigitsDiffer|TestHasNegationAffixMismatch|TestFirstRune' -v`
Expected: compile error — none of these functions exist yet.

- [ ] **Step 3: Implement**

```go
package keywords

import (
	"strings"
	"unicode/utf8"
)

// runeLevenshtein computes the Levenshtein edit distance between a and b,
// counting runes (not bytes) so multi-byte CJK characters count as one edit
// unit each — spec §9.2's fuzzy guardrails are stated in character terms.
func runeLevenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// normalizedSimilarity is 1 - (edit distance / longer length), used both as
// tier 5's continuous score (spec §13.1) and as the len>=9 band's own
// similarity>=0.88 guardrail (§9.2). At the length-5, edit-distance-1
// boundary this is exactly 0.8 -- by construction, every candidate that
// survives the §9.2 guardrails already scores at or above KeywordFamily's
// existing 0.8 MinScore, so no separate tier-5 threshold is needed.
func normalizedSimilarity(a, b string) float64 {
	dist := runeLevenshtein(a, b)
	maxLen := utf8.RuneCountInString(a)
	if bl := utf8.RuneCountInString(b); bl > maxLen {
		maxLen = bl
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(dist)/float64(maxLen)
}

// firstRune returns s's first rune, or 0 for an empty string -- used by the
// length-5-to-8 band's "first character must match" rule (§9.2).
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// digitsOnly extracts the digit runes from s, in order, discarding everything
// else -- the comparison key for the digit veto.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// digitsDiffer implements §9.2's digit veto: "strings differing in any digit
// never match," applied before any threshold. Two strings with no digits at
// all trivially agree (both empty digit sequences) and are not vetoed here.
func digitsDiffer(a, b string) bool {
	return digitsOnly(a) != digitsOnly(b)
}

// negationAffixPrefixes are the negation prefixes named in spec §9.2. "-less"
// is handled as a suffix separately.
var negationAffixPrefixes = []string{"un", "non", "de", "anti"}

// hasNegationAffixMismatch implements §9.2's negation/affix veto: exactly one
// of a/b is the other with a negation prefix or the "-less" suffix attached
// (e.g. "compliant"/"noncompliant", "encrypted"/"unencrypted",
// "harm"/"harmless"). This is deliberately a *pairwise* check -- testing
// "does this affix prefix this one string" in isolation would veto ordinary
// words like "design" or "deploy" that merely start with "de". The "-less"
// branch is exact-match only (root vs root+"less"), not a prefix check: a
// prefix check ("does b start with a's root") also fires on unrelated words
// sharing a prefix before "-less", including real typo pairs tier 5 exists
// to catch (e.g. "wireless"/"wireles", edit distance 1) -- do not broaden
// this to catch same-root-different-suffix pairs like "harmful"/"harmless";
// that is a different, unsafe-to-approximate linguistic pattern, not the
// negation suffix §9.2 names.
func hasNegationAffixMismatch(a, b string) bool {
	for _, p := range negationAffixPrefixes {
		if a == p+b || b == p+a {
			return true
		}
	}
	if a != b {
		if strings.TrimSuffix(a, "less") == b || strings.TrimSuffix(b, "less") == a {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestRuneLevenshtein|TestNormalizedSimilarity|TestDigitsDiffer|TestHasNegationAffixMismatch|TestFirstRune' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/keywords/fuzzy.go server/api/ontology/keywords/fuzzy_test.go
git commit -m "keywords: add fuzzy-matching guardrail primitives (spec §9.2)"
```

---

## Task 4: Tier 5 fuzzy match, wired into `CandidateNodes`

**Files:**
- Modify: `ChenWeb/server/api/ontology/keywords/keywordfamily.go` — doc comment at line 91 ("Tiers 5-6 (fuzzy/ANN) are deferred and return empty."), the `CandidateNodes` body (lines 122-130, the tier-4-to-miss transition)
- Test: `ChenWeb/server/api/ontology/keywords/keywordfamily_test.go` (append)

**Interfaces:**
- Consumes: `runeLevenshtein`, `normalizedSimilarity`, `firstRune`, `digitsDiffer`, `hasNegationAffixMismatch` (Task 3); `semid.NodeCandidate.PrecomputedScore` (Task 2).
- Produces: `KeywordFamily.canonicalPrefExists(ctx, surface string) (bool, error)`, `KeywordFamily.tier5FuzzyMatch(ctx, ks semid.KeySet, scope string) ([]semid.NodeCandidate, bool)` — the latter is the fifth entry in the `CandidateNodes` tier ladder.

- [ ] **Step 1: Write the failing test**

```go
// In keywordfamily_test.go, append:

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
}

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

	// Tiers 0-4 all miss.
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs(ks.Exact, "_").WillReturnRows(sqlmock.NewRows([]string{"concept_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs(ks.Norm, "_", kf.NormalizerVersion).WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	for _, kind := range []string{"alnum", "sorted", "singular"} {
		if kf.normalizer().Normalize("kubernets"); kind == "alnum" || kind == "sorted" {
			mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surface_keys sk`)).
				WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"})).Times(1)
		}
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT enabled`)) // rewrite rules (tier 3) -- see rewrite_rules_store.go for the exact query text used by ListEnabledRules
	// Tier 5 hits.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs(ks.Exact).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs("_", kf.NormalizerVersion, ks.Norm).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).AddRow("kwc_k8s", "kubernetes"))

	candidates, err := kf.CandidateNodes(ctx, "kubernets", "_")
	if err != nil {
		t.Fatalf("CandidateNodes: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Method != "tier5_fuzzy" {
		t.Fatalf("expected exactly one tier5_fuzzy candidate, got %+v", candidates)
	}
}
```

> Note for the implementer: the second test above (`TestKeywordFamilyCandidateNodesReachesTier5`) mocks every tier in sequence, which is brittle to the exact SQL text of tiers 2-4 (already-implemented code you are not changing). Before writing the mock expectations, run `go test ./server/api/ontology/keywords/... -run TestKeywordFamilyCandidateNodesReachesTier5 -v` once with only the tier-0/1 mocks in place, read the failure (sqlmock reports the actual unmatched query), and adjust the tier-2/3/4 mock `WithArgs`/`WillReturnRows` to match what's actually issued rather than guessing further — this is faster and more reliable than hand-deriving the exact query shapes for tiers 2-4 from source.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestTier5FuzzyMatchGuardrails|TestKeywordFamilyCandidateNodesReachesTier5' -v`
Expected: compile error — `tier5FuzzyMatch`/`canonicalPrefExists` don't exist yet.

- [ ] **Step 3: Implement**

Add to `keywordfamily.go` (after `tier4InitialsMatch`, before `retagMethod`):

```go
// canonicalPrefExists implements §9.2's canonical veto: "a query that is
// itself an exact pref_label never fuzzy-matches elsewhere." The check is
// scope-independent by design -- if "Iran" is a real, established word
// anywhere in the store, it must never be treated as a typo of some
// unrelated concept just because tiers 0-4 missed within one caller's
// scope.
func (kf *KeywordFamily) canonicalPrefExists(ctx context.Context, surface string) (bool, error) {
	var exists bool
	err := kf.DB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM kb.keyword_surfaces
			WHERE surface = $1 AND label_role = 'pref'
		)`, surface).Scan(&exists)
	return exists, err
}

// tier5FuzzyMatch implements the fuzzy tier (spec §9.1 tier 5): misspellings
// like kubernets -> Kubernetes. Blocked by a pg_trgm similarity() query
// (cheap, indexed, migration 20260806000001) against norm_key; the real
// gating is the length-banded edit-distance rule and the three absolute
// vetoes from §9.2, applied in Go because pg_trgm's similarity() alone
// cannot express them. Scores are continuous (§13.1) and carried via
// PrecomputedScore -- Score(surface, candidate KeyBundle) has no way to
// compute an edit distance from two derived-key bundles alone.
func (kf *KeywordFamily) tier5FuzzyMatch(ctx context.Context, ks semid.KeySet, scope string) ([]semid.NodeCandidate, bool) {
	runeLen := utf8.RuneCountInString(ks.Norm)
	if runeLen <= 4 {
		return nil, false // §9.2: no fuzzy matching at all below length 5
	}
	if canonical, err := kf.canonicalPrefExists(ctx, ks.Exact); err != nil || canonical {
		return nil, false
	}

	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.norm_key
		FROM kb.keyword_surfaces s
		WHERE s.scope = $1 AND s.norm_version = $2
		  AND similarity(s.norm_key, $3) > 0.3
		ORDER BY similarity(s.norm_key, $3) DESC
		LIMIT 20`, scope, kf.NormalizerVersion, ks.Norm)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	best := map[string]float64{}
	for rows.Next() {
		var conceptID, candNorm string
		if err := rows.Scan(&conceptID, &candNorm); err != nil {
			continue
		}
		if score, ok := fuzzyCandidateScore(ks.Norm, candNorm, runeLen); ok && score > best[conceptID] {
			best[conceptID] = score
		}
	}
	if len(best) == 0 {
		return nil, false
	}
	candidates := make([]semid.NodeCandidate, 0, len(best))
	for conceptID, score := range best {
		s := score
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:           conceptID,
			Method:           "tier5_fuzzy",
			PrecomputedScore: &s,
		})
	}
	return candidates, true
}

// fuzzyCandidateScore applies §9.2's three absolute vetoes and the
// length-banded edit-distance rule to one query/candidate pair, returning
// the continuous score to use if the pair survives. queryRuneLen is the
// query's own length (bands are keyed on the query, not the candidate).
// Shared with the offline reconciler's lexical blocking pass (Task 6).
func fuzzyCandidateScore(queryNorm, candNorm string, queryRuneLen int) (float64, bool) {
	if candNorm == queryNorm {
		return 0, false // tier 1 already handles exact norm-key equality
	}
	if digitsDiffer(queryNorm, candNorm) || hasNegationAffixMismatch(queryNorm, candNorm) {
		return 0, false
	}
	dist := runeLevenshtein(queryNorm, candNorm)
	switch {
	case queryRuneLen <= 8:
		if dist > 1 || firstRune(queryNorm) != firstRune(candNorm) {
			return 0, false
		}
	default: // queryRuneLen >= 9
		if dist > 2 {
			return 0, false
		}
		if sim := normalizedSimilarity(queryNorm, candNorm); sim < 0.88 {
			return 0, false
		}
	}
	return normalizedSimilarity(queryNorm, candNorm), true
}
```

Update the `CandidateNodes` doc comment (line 89-91) from:

```go
// CandidateNodes implements semid.FamilyAdapter. Multi-tier candidate
// generation with early exit at the first tier that produces candidates
// (§9.1). Tiers 5-6 (fuzzy/ANN) are deferred and return empty.
```

to:

```go
// CandidateNodes implements semid.FamilyAdapter. Multi-tier candidate
// generation with early exit at the first tier that produces candidates
// (§9.1). Tier 5 (fuzzy edit-distance) is built. Tier 6 (multilingual
// embedding, cross-lingual identity) is reconciliation-only -- it never
// runs on this online path (spec §22 Q2; keywords.Reconciler, offline).
```

And change the tier-4-to-miss transition (currently):

```go
	// Tiers 5-6: fuzzy/ANN — deferred. Tier 7: miss — the caller writes the
	// backlog (or auto-creates on targeted names, D11).
	return nil, nil
```

to:

```go
	// Tier 5: fuzzy (trigram-blocked, edit-distance-gated) — misspellings.
	if matches, ok := kf.tier5FuzzyMatch(ctx, ks, scope); ok {
		return matches, nil
	}

	// Tier 7: miss — the caller writes the backlog (or auto-creates on
	// targeted names, D11).
	return nil, nil
```

Add the `unicode/utf8` import to `keywordfamily.go` if not already present (it is — `tier4InitialsMatch` already imports and uses it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -v`
Expected: PASS, including every pre-existing test in the package (no regression to tiers 0-4).

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/keywords/keywordfamily.go server/api/ontology/keywords/keywordfamily_test.go
git commit -m "keywords: wire tier 5 fuzzy matching into CandidateNodes (spec step 11)"
```

---

## Task 5: Kernel-level end-to-end test for tier 5

**Files:**
- Test: `ChenWeb/server/api/ontology/keywords/keywordfamily_test.go` (append)

**Interfaces:**
- Consumes: `KeywordFamily.ResolveSurface` (existing), everything from Task 4.

This task exists separately from Task 4 because it proves the *whole* stack — normalizer, tier ladder, kernel scoring, adjudication — not just `tier5FuzzyMatch` in isolation, matching spec §18.2's required-test-set framing ("a normalizer table containing ... a scope round-trip").

- [ ] **Step 1: Write the failing test**

```go
// In keywordfamily_test.go, append:

func TestResolveSurfaceTier5AutoAccepts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kf := &KeywordFamily{DB: db, ResolverMode: "observe"}
	kf.ensureDefaults()
	ctx := context.Background()
	ks := kf.normalizer().Normalize("kubernets")

	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs(ks.Exact, "_").WillReturnRows(sqlmock.NewRows([]string{"concept_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs(ks.Norm, "_", kf.NormalizerVersion).WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surface_keys sk`)).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"})).Times(3)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT enabled`)).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "pattern", "replacement", "scope", "enabled", "provenance"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (`)).
		WithArgs(ks.Exact).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_surfaces s`)).
		WithArgs("_", kf.NormalizerVersion, ks.Norm).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "norm_key"}).AddRow("kwc_k8s", "kubernetes"))
	// FollowMerge inside ResolveSurface reads the resolved concept.
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.keyword_concepts`)).
		WithArgs("kwc_k8s").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_k8s", "Kubernetes", nil, "_", "active", nil, "none", testNow, testNow))

	res, err := kf.ResolveSurface(ctx, "kubernets", "_")
	if err != nil {
		t.Fatalf("ResolveSurface: %v", err)
	}
	if res.Verdict != semid.VerdictAutoAccept {
		t.Fatalf("expected auto_accepted, got verdict=%s matches=%+v", res.Verdict, res.Matches)
	}
	if res.ResolvedNodeID != "kwc_k8s" {
		t.Errorf("expected kwc_k8s, got %s", res.ResolvedNodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
```

> Note for the implementer: as with Task 4's ladder test, read the actual `rewrite_rules_store.go` `ListEnabledRules` query and the exact `SurfaceKeyStore` lookup query text before finalizing these mocks — adjust `WithArgs`/column names to match rather than guessing. Running the test once and reading sqlmock's "call to Query ... was not expected" failure message is the fastest way to get this exactly right.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run TestResolveSurfaceTier5AutoAccepts -v`
Expected: FAIL (either compile-clean-but-mock-mismatch, since Task 4 already exists by this point — the failure here is about getting the exact mock sequence right, not a missing symbol).

- [ ] **Step 3: Fix the mock sequence until it passes**

Iterate on the `WithArgs`/`WillReturnRows` calls per the note above until the test passes. No production code changes in this task — Task 4 already implemented everything this test exercises.

- [ ] **Step 4: Run full package tests**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... ./server/api/ontology/semid/... -v`
Expected: PASS, no regressions.

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/keywords/keywordfamily_test.go
git commit -m "keywords: add kernel-level end-to-end test for tier 5 auto-accept"
```

---

## Task 6: `ConceptStore` reconciliation queries

**Files:**
- Modify: `ChenWeb/server/api/ontology/keywords/concepts_store.go` (append two new methods after `ListConcepts`)
- Test: `ChenWeb/server/api/ontology/keywords/concepts_store_test.go` (append)

**Interfaces:**
- Produces: `ConceptStore.ListAutoCreatedProvisional(ctx, scope string, limit int) ([]Concept, error)`, `ConceptStore.SearchSimilarPrefLabel(ctx, label, scope, excludeConceptID string, minSimilarity float64, limit int) ([]Concept, error)`. Consumed by Task 7's `Reconciler`.

- [ ] **Step 1: Write the failing tests**

```go
// In concepts_store_test.go, append:

func TestListAutoCreatedProvisional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`WHERE scope = $1 AND status = 'provisional' AND gloss_source = 'auto:d11'`)).
		WithArgs("_", 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_l", "Luminance", nil, "_", "provisional", nil, "auto:d11", testNow, testNow))

	out, err := store.ListAutoCreatedProvisional(ctx, "_", 0)
	if err != nil {
		t.Fatalf("ListAutoCreatedProvisional: %v", err)
	}
	if len(out) != 1 || out[0].ConceptID != "kwc_l" {
		t.Errorf("unexpected result: %+v", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSearchSimilarPrefLabel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(`similarity(pref_label, $2) > $3`)).
		WithArgs("_", "Luminence", 0.3, "kwc_l", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kwc_established", "Luminance", nil, "_", "active", nil, "none", testNow, testNow))

	out, err := store.SearchSimilarPrefLabel(ctx, "Luminence", "_", "kwc_l", 0.3, 10)
	if err != nil {
		t.Fatalf("SearchSimilarPrefLabel: %v", err)
	}
	if len(out) != 1 || out[0].ConceptID != "kwc_established" {
		t.Errorf("unexpected result: %+v", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestListAutoCreatedProvisional|TestSearchSimilarPrefLabel' -v`
Expected: compile error — methods don't exist yet.

- [ ] **Step 3: Implement**

Add to `concepts_store.go`, after `ListConcepts`:

```go
// ListAutoCreatedProvisional returns concepts D11's targeted-miss path
// minted (status='provisional', gloss_source='auto:d11') -- the population
// keywords.Reconciler scans for a merge target, per spec §14.3: this
// population is the only one automatic reconciliation merges (a fresh
// provisional concept has no curated content to lose).
func (s ConceptStore) ListAutoCreatedProvisional(ctx context.Context, scope string, limit int) ([]Concept, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE scope = $1 AND status = 'provisional' AND gloss_source = 'auto:d11'
		ORDER BY create_time
		LIMIT $2`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SearchSimilarPrefLabel is reconciliation's lexical blocking query (spec
// §13 R3: "lexical (pg_trgm) ... to k candidates"), backed by migration
// 20260806000001's trigram index on pref_label. excludeConceptID keeps a
// concept from matching itself.
func (s ConceptStore) SearchSimilarPrefLabel(ctx context.Context, label, scope, excludeConceptID string, minSimilarity float64, limit int) ([]Concept, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE scope = $1 AND status IN ('active', 'provisional')
		  AND concept_id != $4
		  AND similarity(pref_label, $2) > $3
		ORDER BY similarity(pref_label, $2) DESC
		LIMIT $5`, scope, label, minSimilarity, excludeConceptID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/keywords/concepts_store.go server/api/ontology/keywords/concepts_store_test.go
git commit -m "keywords: add ConceptStore queries for reconciliation blocking"
```

---

## Task 7: The offline reconciler (`keywords.Reconciler`)

**Files:**
- Create: `ChenWeb/server/api/ontology/keywords/reconcile.go`
- Test: `ChenWeb/server/api/ontology/keywords/reconcile_test.go`

**Interfaces:**
- Consumes: `ConceptStore.ListAutoCreatedProvisional`, `ConceptStore.ListConcepts`, `ConceptStore.SearchSimilarPrefLabel`, `ConceptStore.MergeConcept`, `SurfaceStore.ListSurfacesByConcept` (existing), `semid.DecisionLogStore.Append`, `semid.Normalizer{Version: semid.CurrentNormalizerVersion}.Normalize(s).Norm` (existing), `fuzzyCandidateScore`/`digitsDiffer` (Task 3/4).
- **Note on never-merge:** `ConceptStore.MergeConcept` already checks `kb.semid_never_merge` internally (`mergeGuards` → `isNeverMerge`) and returns an error wrapping `ErrMergeRejected` when blocked. `Reconciler.Run` does not duplicate that check — it just calls `MergeConcept` and counts an `ErrMergeRejected` result as `SkippedVetoed`, so there is exactly one never-merge guardrail, not two.
- **Note on `fuzzyCandidateScore`'s inputs:** Task 4 defined `fuzzyCandidateScore(queryNorm, candNorm string, queryRuneLen int)` to take *normalized* keys (it's called in `tier5FuzzyMatch` with `ks.Norm` values) — its 0.8-at-length-5 score floor and the digit/negation vetoes assume case-folded, NFKC'd input. `findMergeTarget` below normalizes both `PrefLabel`s before calling it, for the same reason: comparing raw, differently-cased pref labels would distort edit distances for reasons that have nothing to do with real spelling differences.
- Produces: `keywords.EmbeddingClient` interface, `keywords.Reconciler` struct + `Run(ctx) (ReconcileStats, error)`. Consumed by Task 8's `cmd/keyword-reconcile`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestReconcilerMergesCrossLingualProvisional|TestCosineSimilarity|TestElectMergeDirectionPrefersRicherConcept' -v`
Expected: compile error — none of `Reconciler`/`cosineSimilarity`/`electMergeDirection`/`EmbeddingClient` exist yet.

- [ ] **Step 3: Implement**

```go
package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// EmbeddingClient is reconciliation's tier-6 seam (spec §13.1, §22 Q2): text
// in, one vector per text out, in the same order. Kept out of the online
// resolve path by design -- only keywords.Reconciler calls this. A concrete
// implementation wraps shared/go/api/llm's OpenAIJSONClient.EmbedBatch; see
// cmd/keyword-reconcile.
type EmbeddingClient interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

const (
	// tier6EmbeddingMinScore is the cosine-similarity floor for an automatic
	// tier-6 merge. Conservative default per spec §22 Q1 ("not derivable
	// from first principles -- measure against the gold set, ship
	// conservative, tune"); D10 biases toward under-merging.
	tier6EmbeddingMinScore = 0.90
	// reconcileLexicalBlockMin is the pg_trgm similarity() floor used only
	// to shortlist candidates before the real edit-distance gate
	// (fuzzyCandidateScore) decides -- deliberately permissive.
	reconcileLexicalBlockMin = 0.30
	// reconcileTopK bounds both the lexical and semantic candidate lists.
	reconcileTopK = 10
)

// ReconcileStats summarizes one Reconciler.Run call.
type ReconcileStats struct {
	Scanned             int
	Merged              int
	SkippedVetoed       int
	SkippedNoCandidate  int
}

// Reconciler is the offline batch job that unifies D11 auto-created
// provisional concepts with their true match (spec §13, the "minimum
// reconciliation loop" build-order step 11 names) -- the mechanism
// Appendix A Stage 5 describes. It merges through the already-guardrailed
// ConceptStore.MergeConcept; it never writes kb.keyword_concepts/surfaces
// directly.
type Reconciler struct {
	DB           *sql.DB
	ConceptStore ConceptStore
	SurfaceStore SurfaceStore
	DecisionLog  semid.DecisionLogStore
	Embeddings   EmbeddingClient
	Scope        string
}

// Run scans every D11 auto-created provisional concept in scope and, for
// each, looks for a merge target via lexical (pg_trgm) and semantic
// (embedding cosine) blocking. Eligibility is structurally guaranteed by
// scanning only the auto-created-provisional population (spec §14.3: "an
// auto-created provisional concept... into an established one" is
// automatic because the provisional side always has no curated content to
// lose -- the forbidden case, two established concepts, cannot arise here
// since one side is always drawn from this population). A merged concept's
// status flips to 'merged' and drops out of the population on the next
// Run, so no watermark/run-tracking table is needed at this scale.
func (r *Reconciler) Run(ctx context.Context) (ReconcileStats, error) {
	var stats ReconcileStats
	candidates, err := r.ConceptStore.ListAutoCreatedProvisional(ctx, r.Scope, 0)
	if err != nil {
		return stats, err
	}
	live, err := r.ConceptStore.ListConcepts(ctx, r.Scope)
	if err != nil {
		return stats, err
	}
	liveEmbeds, err := r.embedConcepts(ctx, live)
	if err != nil {
		return stats, err
	}

	merged := map[string]bool{}
	for _, cand := range candidates {
		stats.Scanned++
		if merged[cand.ConceptID] {
			continue
		}
		target, method, score, ok, err := r.findMergeTarget(ctx, cand, live, liveEmbeds, merged)
		if err != nil {
			return stats, err
		}
		if !ok {
			stats.SkippedNoCandidate++
			continue
		}

		absorbCount, survCount := r.surfaceCounts(ctx, cand.ConceptID, target.ConceptID)
		absorbedID, survivorID := electMergeDirection(cand, target, absorbCount, survCount)

		// ConceptStore.MergeConcept already checks kb.semid_never_merge
		// internally (mergeGuards -> isNeverMerge) -- no separate check here.
		if _, err := r.ConceptStore.MergeConcept(ctx, absorbedID, survivorID); err != nil {
			if errors.Is(err, ErrMergeRejected) {
				stats.SkippedVetoed++
				continue
			}
			return stats, err
		}
		merged[absorbedID] = true

		input, _ := json.Marshal(map[string]any{"absorbed": absorbedID, "survivor": survivorID, "method": method, "score": score})
		_, _ = r.DecisionLog.Append(ctx, semid.DecisionLogEntry{
			Family:  "keyword_reconcile",
			Scope:   r.Scope,
			Input:   input,
			Output:  input,
			Verdict: "auto_merged",
			Actor:   "keyword_reconciler",
		})
		stats.Merged++
	}
	return stats, nil
}

// findMergeTarget runs lexical and semantic blocking for one candidate and
// returns the better-scoring eligible target, if any.
func (r *Reconciler) findMergeTarget(ctx context.Context, cand Concept, live []Concept, liveEmbeds map[string][]float64, merged map[string]bool) (Concept, string, float64, bool, error) {
	var (
		bestTarget Concept
		bestScore  float64
		bestMethod string
		found      bool
	)

	lexRows, err := r.ConceptStore.SearchSimilarPrefLabel(ctx, cand.PrefLabel, r.Scope, cand.ConceptID, reconcileLexicalBlockMin, reconcileTopK)
	if err != nil {
		return Concept{}, "", 0, false, err
	}
	// fuzzyCandidateScore's guardrails (digit veto, negation/affix veto, the
	// length-5-to-8 first-rune rule, and the 0.8-at-length-5 score floor)
	// all assume normalized input, matching tier5FuzzyMatch's own usage —
	// comparing raw, differently-cased pref labels would distort edit
	// distances on casing differences that carry no spelling signal.
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	queryNorm := n.Normalize(cand.PrefLabel).Norm
	queryRuneLen := len([]rune(queryNorm))
	for _, row := range lexRows {
		if merged[row.ConceptID] {
			continue
		}
		candNorm := n.Normalize(row.PrefLabel).Norm
		if score, ok := fuzzyCandidateScore(queryNorm, candNorm, queryRuneLen); ok && score > bestScore {
			bestTarget, bestScore, bestMethod, found = row, score, "tier5_fuzzy", true
		}
	}

	candEmbed, ok := liveEmbeds[cand.ConceptID]
	if ok {
		for _, target := range live {
			if target.ConceptID == cand.ConceptID || merged[target.ConceptID] {
				continue
			}
			targetEmbed, ok := liveEmbeds[target.ConceptID]
			if !ok || digitsDiffer(cand.PrefLabel, target.PrefLabel) {
				continue
			}
			score := cosineSimilarity(candEmbed, targetEmbed)
			if score >= tier6EmbeddingMinScore && score > bestScore {
				bestTarget, bestScore, bestMethod, found = target, score, "tier6_embedding", true
			}
		}
	}

	return bestTarget, bestMethod, bestScore, found, nil
}

// embedConcepts batches one embedding call for every live concept's
// pref_label, so a run with N auto-created candidates against M live
// concepts pays for M embeddings once, not once per candidate pair.
func (r *Reconciler) embedConcepts(ctx context.Context, live []Concept) (map[string][]float64, error) {
	if r.Embeddings == nil || len(live) == 0 {
		return map[string][]float64{}, nil
	}
	texts := make([]string, len(live))
	for i, c := range live {
		texts[i] = c.PrefLabel
	}
	vecs, err := r.Embeddings.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]float64, len(live))
	for i, c := range live {
		if i < len(vecs) && vecs[i] != nil {
			out[c.ConceptID] = vecs[i]
		}
	}
	return out, nil
}

func (r *Reconciler) surfaceCounts(ctx context.Context, a, b string) (int, int) {
	as, _ := r.SurfaceStore.ListSurfacesByConcept(ctx, a)
	bs, _ := r.SurfaceStore.ListSurfacesByConcept(ctx, b)
	return len(as), len(bs)
}

// electMergeDirection picks the survivor: prefer the concept with more
// surface forms (richer -- mirrors doc-processing's entity-reconciliation
// electSurvivor), then the earlier CreateTime, then the lexically smaller
// concept_id for determinism. Both directions are always safe here because
// Run only ever calls this with at least one side drawn from the
// auto-created-provisional population (spec §14.3).
func electMergeDirection(cand, target Concept, candSurfaces, targetSurfaces int) (absorbedID, survivorID string) {
	if candSurfaces != targetSurfaces {
		if candSurfaces > targetSurfaces {
			return target.ConceptID, cand.ConceptID
		}
		return cand.ConceptID, target.ConceptID
	}
	if !cand.CreateTime.Equal(target.CreateTime) {
		if cand.CreateTime.Before(target.CreateTime) {
			return target.ConceptID, cand.ConceptID
		}
		return cand.ConceptID, target.ConceptID
	}
	ids := []string{cand.ConceptID, target.ConceptID}
	sort.Strings(ids)
	if ids[0] == cand.ConceptID {
		return target.ConceptID, cand.ConceptID
	}
	return cand.ConceptID, target.ConceptID
}

// cosineSimilarity is tier 6's score. Returns 0 for a zero-norm vector
// (e.g. a missing embedding) rather than NaN.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -v`
Expected: PASS. If the sqlmock expectation sequence in `TestReconcilerMergesCrossLingualProvisional` doesn't match on the first attempt (likely, given `MergeConcept`'s internal transaction shape), read sqlmock's failure output (it reports the actual query/args it received) and adjust the mock steps to match — do not guess further; the production code in `concepts_store.go` (already read in full during planning) is the ground truth for the exact SQL text and argument order.

- [ ] **Step 5: Commit**

```bash
git add server/api/ontology/keywords/reconcile.go server/api/ontology/keywords/reconcile_test.go
git commit -m "keywords: add offline reconciler (tier 6 embedding merge, spec step 11)"
```

---

## Task 8: `cmd/keyword-reconcile` batch entrypoint

**Files:**
- Create: `ChenWeb/server/cmd/keyword-reconcile/main.go`

**Interfaces:**
- Consumes: `keywords.Reconciler` (Task 7), `ApiTypes.LLMModelsFile`/`LLMModelDef` (existing, `shared/go/api/ApiTypes`), `llmclients.NewOpenAIJSONClientFromConfig`/`EmbedBatch` (existing, `shared/go/api/llm`), `kbsearch.EmbeddingDimensionsForModel` (existing).

- [ ] **Step 1: Write the command**

```go
// Command keyword-reconcile runs one offline reconciliation pass over the
// keyword lexicon (spec 2026080403 §19 step 11): it merges D11 auto-created
// provisional concepts into their true match using tier 5 (edit distance)
// and tier 6 (multilingual embedding) evidence. Not wired into any live
// server or cron -- run manually or via an external scheduler, per the
// spec's §22 Q2 decision to keep tier 6 off the online resolve path.
//
// Usage:
//
//	keyword-reconcile --scope=_
//
// Requires KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME to name an entry in
// .models.toml (e.g. qwen3-embedding-0-6b-llama-cpp or
// nomic-embed-v2-moe-llama-cpp -- both already locally hosted per .models.toml).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func main() {
	log.SetFlags(0)
	scope := flag.String("scope", "_", "keyword scope to reconcile")
	flag.Parse()

	db := connect()
	defer db.Close()
	ctx := context.Background()

	embedClient, modelName, err := embeddingClientFromEnv()
	if err != nil {
		log.Fatalf("embedding client: %v", err)
	}

	r := &keywords.Reconciler{
		DB:           db,
		ConceptStore: keywords.ConceptStore{DB: db},
		SurfaceStore: keywords.SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		Embeddings:   &llmEmbeddingClient{client: embedClient, modelName: modelName},
		Scope:        *scope,
	}
	stats, err := r.Run(ctx)
	if err != nil {
		log.Fatalf("reconcile run failed: %v", err)
	}
	fmt.Printf("keyword-reconcile scope=%s scanned=%d merged=%d skipped_vetoed=%d skipped_no_candidate=%d\n",
		*scope, stats.Scanned, stats.Merged, stats.SkippedVetoed, stats.SkippedNoCandidate)
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// llmEmbeddingClient adapts shared/go/api/llm's OpenAIJSONClient to
// keywords.EmbeddingClient, keeping the ontology/keywords package itself
// free of any LLM-client dependency (only this cmd binary wires them
// together, mirroring how doc-processing's entity reconciliation keeps its
// LLM seam (MergeAdjudicator) separate from the DB seam).
type llmEmbeddingClient struct {
	client    *llmclients.OpenAIJSONClient
	modelName string
}

func (c *llmEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	return c.client.EmbedBatch(ctx, llmclients.EmbedBatchInput{
		ModelName:  c.modelName,
		InputTexts: texts,
		CallReason: "keyword_reconcile_tier6",
		CallLoc:    "cmd/keyword-reconcile",
	})
}

func embeddingClientFromEnv() (*llmclients.OpenAIJSONClient, string, error) {
	modelRef := strings.TrimSpace(os.Getenv("KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME"))
	if modelRef == "" {
		return nil, "", fmt.Errorf("KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME is not set")
	}
	path, err := modelsFilePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	var models ApiTypes.LLMModelsFile
	if err := toml.Unmarshal(raw, &models); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}
	def, ok := models[modelRef]
	if !ok {
		return nil, "", fmt.Errorf("model %q not found in %s", modelRef, path)
	}
	client, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:            def.ModelName,
		APIKey:               def.APIKey,
		BaseURL:              def.BaseURL,
		Provider:             llmclients.ProviderOpenAICompatible,
		TimeoutSec:           def.TimeoutSec,
		EmbeddingDimensions:  kbsearch.EmbeddingDimensionsForModel(def.ModelName, def.BaseURL),
		MaxInflight:          def.MaxInflight,
		MaxRequestsPerMinute: def.MaxRequestsPerMinute,
		MaxTokensPerMinute:   def.MaxTokensPerMinute,
		TokenReservePerCall:  def.TokenReservePerCall,
	}, nil)
	if err != nil {
		return nil, "", err
	}
	return client, def.ModelName, nil
}

// modelsFilePath walks up from the working directory looking for
// .models.toml, mirroring doc-processing's resolveModelsFilePath.
func modelsFilePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MODELS_FILE")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("MODELS_FILE %q is invalid: %w", override, err)
		}
		return override, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, ".models.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", fmt.Errorf(".models.toml not found; set MODELS_FILE or place .models.toml in the current directory tree")
}
```

- [ ] **Step 2: Build it**

Run: `cd ChenWeb && go build ./server/cmd/keyword-reconcile/...`
Expected: builds clean.

- [ ] **Step 3: Manual smoke test against the dev database**

```bash
cd ChenWeb
export KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME=qwen3-embedding-0-6b-llama-cpp
go run ./server/cmd/keyword-reconcile --scope=_
```

Expected: either a clean `scanned=0 merged=0 ...` line (if no auto-created provisional concepts exist yet in the dev DB — likely, since D11 auto-create only fires on a targeted-name miss) or a real merge if some exist. If the local llama.cpp server on port 18082 isn't running, expect a clear connection-refused error naming that base URL — confirm the error message identifies the right cause rather than failing silently.

- [ ] **Step 4: Commit**

```bash
git add server/cmd/keyword-reconcile/main.go
git commit -m "Add cmd/keyword-reconcile: offline batch entrypoint for keyword reconciliation"
```

---

## Task 9: Full verification and spec status update

**Files:**
- Modify: `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md` (§0 status table, §19 build order, §21 implementation record — following the exact pattern already used for steps 1-10)

**Interfaces:** none (documentation + verification only).

- [ ] **Step 1: Run the full verification suite**

```bash
cd ChenWeb
go build ./... && go vet ./server/api/ontology/... ./server/api/kbhandler/... ./server/api/doc-processing/... ./server/cmd/keyword-reconcile/...
go test ./server/api/ontology/keywords/... ./server/api/ontology/semid/... ./server/api/ontology/names/... -v
```

Expected: PASS/clean, matching the spec's own §21 verification-command pattern for steps 1-10. Pre-existing environment-dependent failures in `kbhandler`/`doc-processing` (noted in §21 as already present before this plan) are not this plan's responsibility to fix — confirm they're the *same* pre-existing failures, not new ones introduced here.

- [ ] **Step 2: Update the spec's status sections**

In `2026080403-spec-keyword-canonicalization-and-reconciliation.md`:

- §0 "Status at a glance" row **Not built**: remove "Tiers 5-6" (tier 5 is now built; tier 6 is built but reconciliation-only) — reword to state tier 6 is reconciliation-only by design decision (§22 Q2), not merely unbuilt.
- §19 build order item 11: mark done, noting the explicit scope limits from this plan's "Non-goals" section (no R1/R2/R4/R5, no watermark table).
- §21 "Implementation record": append an entry in the same style as the existing `5fb6`/`11f7`/.../`d1ab` step entries, once this plan's commits exist, e.g.:

  ```
  · `<short-hash>` step 11 — tier 5 fuzzy matching (pg_trgm-blocked, §9.2
  guardrails) wired into CandidateNodes; keywords.Reconciler + cmd/keyword-
  reconcile (tier 6 embedding merge, offline only per §22 Q2). No LLM call;
  R1/R2/R4/R5 and a runs/watermark table are explicitly deferred (minimum
  loop, not the full R1-R7 pipeline). Migration 20260806000001.
  ```

- Section 22 "Open questions": mark Q2 answered — "Decided 2026-08-06: reconciliation-only. Two local multilingual embedding models are already in `.models.toml` (qwen3-embedding-0-6b, nomic-embed-v2-moe via llama.cpp), but the online resolve path was kept free of that runtime dependency by explicit choice." Leave Q1 (per-tier thresholds) open — this plan ships conservative constants (`tier6EmbeddingMinScore = 0.90`), not a tuned value.

- [ ] **Step 3: Commit the spec update**

This is a `KnowledgeStore` doc-repo change, a separate git repository from `ChenWeb` — commit it there, matching the project's document ID/repo conventions (`KnowledgeStore/CLAUDE.md`), not as part of the `ChenWeb` commits above.

```bash
cd /Users/cding/Workspace/KnowledgeStore
git add doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md
git commit -m "Update keyword spec status: step 11 (tier 5 + minimum reconciliation loop) implemented"
```

---

## Self-review notes (for whoever executes this plan)

- **Spec coverage:** Task 1-5 cover §9.1 tier 5 + §9.2's three vetoes and length bands. Task 6-8 cover §13's "minimum reconciliation loop," §14.3's automatic-merge eligibility, and §22 Q2's now-decided placement of tier 6. Task 9 closes the loop the project's own `2026080403-spec` document expects (§21's implementation record is the living status source).
- **What this plan deliberately does not touch:** `names.Resolver`, the REST API (§12), the mention collector (§11), and everything in Step 12 (§16, `aligns_to_term`) — those are unaffected by tier 5/6 landing and are out of scope for this plan by the user's own sequencing decision.
- **Known risk spot:** the sqlmock expectation sequences in Tasks 4, 5, and 7 are written against the *current* SQL text of `tier0ExactMatch`/`tier1NormKeyMatch`/`tier2AlternateKeyMatch`/`tier3RewriteMatch`/`ListEnabledRules`/`MergeConcept`, all read in full during planning — but sqlmock is unforgiving about exact argument order and query substrings. Budget real iteration time on Tasks 4/5/7's tests; the notes inline at each flag this explicitly rather than hiding it.
