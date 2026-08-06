# Keyword Module Step 12 — `aligns_to_term` + Metric Integration (REQ-2/REQ-3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement build-order Step 12 of `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md` (§19 item 12, "§2 REQ-2/REQ-3"): seed and release the `aligns_to_term` predicate term (§16.1), extend `subject_ref_kind` so a keyword concept can be an assertion subject, build the alignment producer, add the metric columns, and place the consumer call (§16.3). When this lands, this module's responsibility for §2 is fully discharged: a keyword concept resolves to one governed `metric_definition` term (REQ-2), and every extracted metric row carries that term id (REQ-3).

**The architecture, in one paragraph.** An alignment is a `kb.semantic_assertions` row: `subject_ref_kind='keyword_concept'`, `subject_ref_id=<concept_id>`, `predicate_term_id='core:aligns_to_term'` (the seeded, released predicate), `object_ref_kind='ontology_term'`, `object_ref_id=<metric_definition term_id>`, `status='accepted'`, with the auto-assignment's `method`/`score`/`evidence` recorded in `qualifiers`/`confidence`/`decision_reason` (§16.1). Four pieces consume or produce it: **(1)** a migration extends the `subject_ref_kind` CHECKs and adds the two metric columns; **(2)** an `AlignmentsStore` writes/reads these rows idempotently and auditably; **(3)** `MergeConcept` gains §14.2's now-live gate (a merge where both sides are aligned to *different* terms is refused; an alignment on the absorbed side follows to the survivor inside the same tx); **(4)** the resolver reads an accepted alignment to produce `term_resolved` even when the raw name matches no released label, and the observe path auto-aligns a concept whose `pref_label` exactly matches a released `metric_definition` label (the D11 auto-assign producer, §16.1). REQ-3's consumer call is a `MetricsStore` decorator wrapping the metric store at its single construction point — both write paths (`FinalizeChunkBatch` and the enrichment path) already go through the same `p.Store` field, so one wrapper is the §16.3 "one place, not one per write path" seam.

**How the §2.2 acceptance test's steps 1–3 are satisfied.** "Luminance" resolves → the observe path auto-aligns its concept to the released `luminance` term (exact-label auto-accept). "显示亮度" auto-creates a provisional concept (D11, step 8) → the step-11 reconciler merges it into the "Luminance" concept → §14.2's follow re-points the alignment to the survivor, and the §14.2 conflict gate guarantees the merge is refused if the two concepts were ever aligned to different governed terms. Every metric named "Luminance" or "显示亮度" then resolves through the decorator to the one `metric_definition_term_id`.

**Tech Stack:** Go 1.25, PostgreSQL (goose migrations in `ChenWeb/project_migrations/`), `github.com/DATA-DOG/go-sqlmock` for store tests, the existing `assertions.AssertionStore` (`server/api/ontology/assertions/assertions_store.go`) and `semid.DecisionLogStore` for the audit row.

## Global Constraints

- Module path: `github.com/chendingplano/deepdoc`. Repo protocol: **jj-colocated — commit via `jj commit`, NEVER `git commit`, NEVER create branches** (Workspace CLAUDE.md; the step-11 plan's `git commit` instructions were wrong for this environment — this plan uses `jj`).
- Migrations live in `ChenWeb/project_migrations/`, named `YYYYMMDDHHMMSS_description.sql` with `-- +goose Up` / `-- +goose Down` markers. The latest keyword-module migration is `20260806000001` (step 11); the next slot is **`20260806000002`**.
- No hardcoded prompts (`ChenWeb/CLAUDE.md` §2) — not applicable; no LLM call is used anywhere in this plan (alignments are deterministic auto-assignments, §16.1).
- Thresholds are Go constants with a comment citing spec §22 Q1 ("ship conservative, tune"); no new env-var tuning knobs.
- Follow `ChenWeb/CLAUDE.md` §1.3 (Surgical Changes) and match the codebase's comment density/style: every exported/package function carries a spec-referencing doc comment.
- Run from `/Users/cding/Workspace/ChenWeb` for all Go commands.

## Non-goals for this plan (explicitly deferred, do not build)

- **The governed-catalog bootstrap (§16.1) and standards-glossary import (§13.2)** — creating/harvesting the *released `metric_definition` terms themselves* is a sibling workstream ("someone must own both", §19). This module aligns to terms that already exist as released `metric_definition` rows. Task 2 seeds `aligns_to_term` (the predicate this module owns); it does **not** seed metric definitions. The module's unit tests use sqlmock, so they don't need real terms; only the live §2.2 end-to-end run does, which is gated on that sibling workstream.
- **§17.1 AssociateSemantics refactor** and **§17.2 QUDT/`resolveUnitTerms`** — explicitly "after the pilot" / "data fix first"; out of scope.
- **P4/REQ-4 `metric_key`** — app-specific (§2.4); this module persists its own canonical `metric_definition_term_id` on every metric row and its shape does not bend to P4's internal key scheme (platform independence, §2.1).
- **The full R1–R7 pipeline, `on`-mode wiring, reversibility UI, offline batch alignment sweep** — a batch "align every live concept" pass is a reasonable future extension, but the observe-path auto-align + merge-follow is the minimum loop step 12 names.
- **Revoking/retracting an alignment** — not needed for the minimum loop; `EnsureAccepted` is idempotent and a wrong alignment is corrected by editing the assertion row manually (reversibility UI is already deferred, §14.4).

---

## Task 1: Migration — extend `subject_ref_kind`/`object_ref_kind` CHECKs + add the metric columns

**Files:**
- Create: `ChenWeb/project_migrations/20260806000002_aligns_to_term_ref_kinds_and_metric_term_columns.sql`

**Interfaces:**
- Produces: `'keyword_concept'` accepted in both `kb.semantic_assertions` ref-kind CHECKs (subject NOT NULL, object nullable); `kb.metrics.keyword_concept_id` + `kb.metrics.metric_definition_term_id`; an index for the alignment lookup. Consumed by Task 3's `AlignmentsStore` and Task 6's metric SQL.

- [ ] **Step 1: Read the current constraint names**

The `subject_ref_kind`/`object_ref_kind` CHECKs were created as inline column constraints in `20260801000001_create_kb_semantic_assertions.sql`, so they carry Postgres's auto-generated names (`semantic_assertions_subject_ref_kind_check`, `semantic_assertions_object_ref_kind_check`) — but **verify before writing the migration**:

```bash
psql "$PG_DSN" -c "SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'kb.semantic_assertions'::regclass AND contype = 'c';"
```

If the names differ, use the real names (or `DROP CONSTRAINT IF EXISTS` on both candidate names).

- [ ] **Step 2: Write the migration**

```sql
-- +goose Up
-- Spec 2026080403 §19 step 12 (REQ-2/REQ-3): a keyword concept must be a
-- legal assertion subject for aligns_to_term (subject side, NOT NULL CHECK)
-- and object side (nullable CHECK), and kb.metrics gains the two governed
-- identifiers REQ-3 persists. metric_definition_term_id deliberately has no
-- FK: kb.ontology_terms has no single-column unique constraint on term_id
-- (only (term_id, version)), the same reason every other term-id-reference
-- column in this schema has no DB FK (see §2.4 / AssociateSemantics.termExists).
-- keyword_concept_id *does* get an FK: kb.keyword_concepts.concept_id is a
-- TEXT PRIMARY KEY.

ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_subject_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_subject_ref_kind_check
    CHECK (subject_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal', 'keyword_concept'));

ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_object_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_object_ref_kind_check
    CHECK (object_ref_kind IS NULL OR object_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal', 'keyword_concept'));

ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS keyword_concept_id TEXT
    REFERENCES kb.keyword_concepts(concept_id);
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_definition_term_id TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_subject_concept
    ON kb.semantic_assertions (subject_ref_id)
    WHERE subject_ref_kind = 'keyword_concept' AND status = 'accepted';

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_subject_concept;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS metric_definition_term_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS keyword_concept_id;
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_object_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_object_ref_kind_check
    CHECK (object_ref_kind IS NULL OR object_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal'));
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_subject_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_subject_ref_kind_check
    CHECK (subject_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal'));
```

- [ ] **Step 3: Apply and verify** (via the project's goose runner / `mise dev` auto-apply, as with step 11)

```bash
psql "$PG_DSN" -c "\d kb.semantic_assertions" | grep ref_kind   # both CHECKs now list keyword_concept
psql "$PG_DSN" -c "\d kb.metrics" | grep -E "keyword_concept_id|metric_definition_term_id"
```

- [ ] **Step 4: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit project_migrations/20260806000002_aligns_to_term_ref_kinds_and_metric_term_columns.sql -m "Enable keyword_concept assertion ref kinds and add metric term columns (spec step 12)"
```

---

## Task 2: Seed and release the `aligns_to_term` predicate term

**Files:**
- Modify: `ChenWeb/server/cmd/ontology-seed/content.go` (the `coreModule` var — add one `seedTerm`)

**Interfaces:**
- Produces: a released, activated `core:aligns_to_term` property term (`term_kind='property'`, prefLabel "aligns to term") in `kb.ontology_terms`, with `status='included_in_release'`. This is the hard prerequisite §16.1 names: every assertion carries a `predicate_term_id`, and nothing may be written until the predicate is released. Consumed by Task 3's `AlignmentsStore` (it uses the literal id `core:aligns_to_term`).

- [ ] **Step 1: Add the term to `coreModule`**

In `server/cmd/ontology-seed/content.go`, append to `coreModule.Terms` (the `mea:measured_by` property at line ~121 is the precedent):

```go
{ID: "core:aligns_to_term", Kind: "property", Def: "Binds a keyword concept to the governed term it is an accepted alias of (spec 2026080403 §16.1/§16.2, REQ-2). The assignment side of the governed bridge: auto-proposed, auto-accepted above a threshold, with method/score/evidence recorded on the assertion.", Labels: enPref("aligns to term")},
```

`core` is the natural home per §16.1 — the relation is not measurement-specific (a concept may align to a unit or quantity-kind term later; REQ-2 happens to be metric_definition).

- [ ] **Step 2: Seed and release against the dev database**

```bash
cd /Users/cding/Workspace/ChenWeb && go run ./server/cmd/ontology-seed --module core
```

(Follow the command's own usage — `--module core` seeds and, unless `--author-only`, releases/activates. If the command requires a DB DSN/env, mirror how `cmd/keyword-reconcile`/the dev server connects.)

- [ ] **Step 3: Verify**

```bash
psql "$PG_DSN" -c "SELECT term_id, term_kind, status FROM kb.ontology_terms WHERE term_id = 'core:aligns_to_term';"
psql "$PG_DSN" -c "SELECT term_id, label, lang, role FROM kb.ontology_term_labels WHERE term_id = 'core:aligns_to_term';"
```

Expected: one row `core:aligns_to_term | property | included_in_release`, plus a released prefLabel "aligns to term". Also run `go run ./server/cmd/ontology-seed --module core` a second time and confirm it's idempotent (no error, nothing duplicated).

- [ ] **Step 4: Commit**

```bash
jj commit server/cmd/ontology-seed/content.go -m "seed: add core:aligns_to_term property term (spec step 12)"
```

---

## Task 3: The alignment store (`AlignmentsStore`)

**Files:**
- Create: `ChenWeb/server/api/ontology/keywords/alignment.go`
- Test: `ChenWeb/server/api/ontology/keywords/alignment_test.go`

**Interfaces:**
- Consumes: `assertions.AssertionStore.CreateAssertion` (existing — it already persists `subject_ref_kind='keyword_concept'` rows once the CHECK is extended in Task 1; `assertions.AllowedRefKinds` must gain `"keyword_concept"` so `validateAssertion` accepts them), `semid.DecisionLogStore.Append` (existing, DR15 audit).
- **Import-cycle check (do this first):** `assertions` is effectively a leaf (grep shows no `deepdoc/server` imports). If a cycle is found, fall back to writing the alignment INSERT directly in this file instead of importing `assertions` — but prefer the reuse; it buys the `logical_identity_key`/revision semantics for free.
- Produces: `keywords.ErrAlignmentConflict` sentinel, `keywords.AlignmentsStore` with `EnsureAccepted`, `AcceptedForConcept`, `FollowMerge`, `MergeConflict`. Consumed by Task 4 (`MergeConcept`) and Task 5 (`names.Resolver`).

- [ ] **Step 1: Extend `assertions.AllowedRefKinds`** (`server/api/ontology/assertions/assertions_store.go`, the map at the top)

```go
var AllowedRefKinds = map[string]bool{
	"object_node": true, "ontology_term": true, "assertion": true,
	"artifact": true, "literal": true, "keyword_concept": true,
}
```

(Check that `validateAssertion` validates both `SubjectRefKind` and `ObjectRefKind` against this map — it must, for `CreateAssertion` to accept the alignment; if `ObjectRefKind` is validated too, the same map entry covers it.)

- [ ] **Step 2: Write the failing tests**

```go
package keywords

// AlignmentsStore wraps an assertions store keyed to the core:aligns_to_term
// predicate (spec §16.1). The predicate id is a constant because ontology-seed
// (Task 2) authors it with exactly this term_id.
func TestAlignmentsStoreEnsureAccepted(t *testing.T) {
	// sqlmock: AssertionStore.CreateAssertion issues the big INSERT...RETURNING
	// with ~29 args. Rather than hand-derive it, run the test once with the
	// SubjectRefKind/Status assertions in place and read sqlmock's actual-args
	// failure to lock the mock. Then assert:
	//  - first call writes status='accepted', subject_ref_kind='keyword_concept',
	//    subject_ref_id=conceptID, predicate_term_id='core:aligns_to_term',
	//    object_ref_kind='ontology_term', object_ref_id=termID,
	//    qualifiers contains method/score/evidence;
	//  - second call (same concept, same term) is a no-op: no second INSERT.
	//  - a decision-log row is appended on the write (Family "keyword_align").
}

func TestAlignmentsStoreConflict(t *testing.T) {
	// EnsureAccepted(concept, termA) then EnsureAccepted(concept, termB) must
	// return ErrAlignmentConflict (not write a second accepted row).
}

func TestAlignmentsStoreAcceptedForConcept(t *testing.T) {
	// Returns the accepted alignment row for a concept; none when absent.
}

func TestAlignmentsStoreFollowMerge(t *testing.T) {
	// FollowMerge(ctx, absorbed, survivor) re-points subject_ref_id
	// absorbed -> survivor for accepted keyword_concept alignments, inside a
	// caller-owned tx if given a *sql.Tx.
}

func TestAlignmentsStoreMergeConflict(t *testing.T) {
	// MergeConflict(ctx, a, b) is true only when both concepts have accepted
	// alignments to *different* terms.
}
```

> Note: `AssertionStore.CreateAssertion` requires a non-empty `LogicalIdentityKey` and enforces the §9.3 status machine. Build the identity key as `fmt.Sprintf("kwc:%s|core:aligns_to_term|%s", conceptID, termID)` (the `subject|predicate|object` shape used elsewhere in the assertions package — verify against how `associate_semantics` builds keys). `FollowMerge` must accept a `DBX`/`*sql.Tx` so Task 4 can run it inside `MergeConcept`'s transaction.

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestAlignmentsStore|TestAlignment' -v`
Expected: compile error — none of `AlignmentsStore`/`ErrAlignmentConflict` exist yet.

- [ ] **Step 4: Implement**

```go
// ErrAlignmentConflict is returned when an accepted aligns_to_term assertion
// already exists for a concept to a *different* governed term (spec §14.2's
// conflict — two concepts aligned to two distinct terms are evidence they are
// not the same thing; and a single concept aligned to two terms is ambiguous
// identity, which the module must never auto-decide).
var ErrAlignmentConflict = errors.New("alignment conflict: concept already aligned to a different term")

// AlignmentsStore reads and writes keyword_concept -> ontology_term
// aligns_to_term assertions (spec 2026080403 §16.1/§16.2, REQ-2). Each
// alignment is a kb.semantic_assertions row: subject_ref_kind='keyword_concept',
// predicate_term_id='core:aligns_to_term', object_ref_kind='ontology_term',
// status='accepted', with the auto-assignment's method/score/evidence in
// qualifiers/confidence/decision_reason.
type AlignmentsStore struct {
	Assertions assertions.AssertionStore
	DecisionLog semid.DecisionLogStore
	Scope string
}

// alignPredicateTermID is the seeded predicate id (Task 2). A constant rather
// than a lookup because ontology-seed authors exactly this id.
const alignPredicateTermID = "core:aligns_to_term"
```

Full implementations for `EnsureAccepted(ctx, conceptID, termID, method string, score float64, evidence string)`, `AcceptedForConcept(ctx, conceptID)`, `FollowMerge(ctx, absorbedID, survivorID string)`, `MergeConflict(ctx, a, b string)` — see the shape in Task 7's test scaffolding and the note above. `EnsureAccepted` must verify (once, cheaply) that the object term is a *released* `metric_definition` (reuse the resolver's `releasedTermSQL` pattern or `associate_semantics`'s `termExists`) and the predicate is released; if not, return a clear error rather than silently accepting — Task 2 runs first, so this is a guard, not a blocker. Guard the no-op and conflict cases before any write; append the decision-log row (`Family: "keyword_align"`) on a real write.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... ./server/api/ontology/assertions/... -v`
Expected: PASS. (The sqlmock sequence for `CreateAssertion` is fiddly — iterate against the actual SQL text in `assertions_store.go` and sqlmock's failure output, as with step-11's Task 4 note.)

- [ ] **Step 6: Commit**

```bash
jj commit server/api/ontology/keywords/alignment.go server/api/ontology/keywords/alignment_test.go server/api/ontology/assertions/assertions_store.go -m "keywords: add aligns_to_term alignment store (spec step 12, REQ-2)"
```

---

## Task 4: `MergeConcept` §14.2 alignment-conflict gate + follow

**Files:**
- Modify: `ChenWeb/server/api/ontology/keywords/concepts_store.go` (`MergeConcept`, the transaction body)
- Test: `ChenWeb/server/api/ontology/keywords/concepts_store_test.go` (append)

**Interfaces:**
- Consumes: `AlignmentsStore.MergeConflict` / `.FollowMerge` (Task 3).
- Produces: §14.2's gate made live inside the merge transaction — the step-11 plan explicitly left this a no-op ("no aligns_to_term assertions exist until Step 12"); this task is what makes the spec §14.2 behavior real.

- [ ] **Step 1: Write the failing tests**

Three sqlmock cases inside `MergeConcept`'s `BEGIN`/`COMMIT` (the `txBeginner` path already tested in step 11's reconciler test):
1. **Follow:** absorbed has an accepted alignment to term T, survivor has none → merge proceeds and the assertion's `subject_ref_id` is re-pointed absorbed→survivor (one `UPDATE kb.semantic_assertions` inside the tx).
2. **Same term:** both aligned to T → merge proceeds; the absorbed-side alignment row re-points to survivor (or is deduped — either is acceptable, document which).
3. **Conflict:** absorbed aligned to T1, survivor aligned to T2 (T1≠T2) → `MergeConcept` returns an error wrapping `ErrMergeRejected` and the tx rolls back; no merge applied, no alignment moved.

Reuse the existing `TestReconcilerMergesCrossLingualProvisional` mock scaffolding (concept-row reads FOR UPDATE, never_merge check, tombstone UPDATE, surface re-point, commit) and interleave the alignment reads/writes at the right points.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -run 'TestMergeConceptAlignment' -v`
Expected: FAIL — `MergeConcept` has no alignment logic today (it has a comment saying the gate is a no-op).

- [ ] **Step 3: Implement**

In `MergeConcept`'s transaction, after the `mergeGuards` (never-merge) check and before `applyMerge`:

```go
// §14.2: an accepted aligns_to_term on the absorbed side follows to the
// survivor as part of the merge transaction; two concepts aligned to two
// distinct governed terms are evidence they are not the same thing, and that
// evidence outranks whatever similarity proposed the merge.
conflict, err := align.MergeConflict(ctx, absorbedID, survivorID)
if err != nil {
	return ..., err
}
if conflict {
	return ..., fmt.Errorf("%w: concepts %s and %s are aligned to different governed terms", ErrMergeRejected, absorbedID, survivorID)
}
if err := align.FollowMerge(ctx, absorbedID, survivorID); err != nil {
	return ..., err
}
```

`MergeConflict` must read with the tx's `DBX` (same transaction), and `FollowMerge` must run inside the same tx so the re-point is atomic with the merge. If `MergeConcept` doesn't currently accept an alignment store, add a field on `ConceptStore` (or construct `AlignmentsStore` from the same DB) — do not change the exported signature if the reconciler/other callers would break; prefer a `ConceptStore.Alignments AlignmentsStore` field defaulting to an empty store that skips the gate when its DB is nil (matches the codebase's nil-DB patterns), so existing callers that don't set it stay unchanged and the gate is live only when wired.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/keywords/... -v`
Expected: PASS, no regression (the step-11 reconciler test must still pass).

- [ ] **Step 5: Commit**

```bash
jj commit server/api/ontology/keywords/concepts_store.go server/api/ontology/keywords/concepts_store_test.go -m "keywords: make merge honor aligns_to_term conflict gate and follow (spec §14.2)"
```

---

## Task 5: Resolver alignment-follow + observe-path auto-align

**Files:**
- Modify: `ChenWeb/server/api/ontology/names/resolver.go`
- Test: `ChenWeb/server/api/ontology/names/resolver_test.go` (append)

**Interfaces:**
- Consumes: `AlignmentsStore.AcceptedForConcept` / `.EnsureAccepted` (Task 3).
- Produces: `StatusTermResolved` via an accepted alignment even when the raw name matches no released label (REQ-2); the D11 auto-assign producer that mints alignments on the observe path (§16.1).

- [ ] **Step 1: Write the failing tests**

1. **`TestResolveNameTermResolvedViaAlignment`:** released-term label layer finds nothing for the raw name ("亮度"); the lexical layer auto-accepts a concept (`VerdictAutoAccept`); `AcceptedForConcept` returns an alignment to term T → status `term_resolved`, `TermID=T`, `Method="aligns_to_term"`, `TermKind="metric_definition"`. Assert `ResolveName` made **no** write (no INSERT expectations — only the existing read queries).
2. **`TestResolveAndObserveAutoAlignsOnExactLabel`:** the resolved concept's `pref_label` ("Luminance") exactly matches a released `metric_definition` label → `EnsureAccepted(concept, term, "term_exact", 1.0, ...)` is called, one accepted alignment + one decision-log row written, and the returned resolution is `term_resolved`.
3. **`TestResolveAndObserveDisabledWritesNothing`:** resolver `off` → no alignment write, no assertion (mirrors the existing disabled test).
4. **`TestResolveNameAlignmentDoesNotOverrideDifferentTerm`:** a concept aligned to T2 while the governed layer's label match found T1 → the label match wins (the governed layer's exact label match is authoritative over a stale/conflicting alignment; document this precedence).

> The existing `matchReleasedTerm` queries `releasedTermSQL` for the *raw name*. The auto-align needs the same lookup against the concept's *pref_label* — either factor a shared `matchLabelToReleasedTerm(label, kinds) (termHit, error)` used by both, or run the same query with the pref label. Keep `ResolveName` write-free (the §9.5 read/write split is load-bearing; the auto-align write belongs only in `ResolveAndObserve`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ChenWeb && go test ./server/api/ontology/names/... -run 'TestResolveNameTermResolvedViaAlignment|TestResolveAndObserveAutoAlignsOnExactLabel|TestResolveNameAlignmentDoesNotOverrideDifferentTerm' -v`
Expected: FAIL — no alignment logic in the resolver yet.

- [ ] **Step 3: Implement**

In `ResolveName`, after the existing `switch res.Verdict` block and only when `term == nil`:

```go
// REQ-2 (spec §16.2): an accepted aligns_to_term on the resolved concept is
// the second way a name reaches a governed term — the cross-lingual/merged
// case, where the raw string is not a released label but the concept it
// resolved to is aligned to one. The governed layer's own exact label match
// (term != nil) stays authoritative.
if term == nil && r.alignments != nil && res.Verdict == semid.VerdictAutoAccept && res.ResolvedNodeID != "" {
	if hit, err := r.alignments.AcceptedForConcept(ctx, res.ResolvedNodeID); err == nil && hit != nil {
		out.Status = StatusTermResolved
		out.TermID = <object term id from the assertion>
		out.TermKind = "metric_definition"
		out.Method = "aligns_to_term"
		// TermPrefName: look up the released term's pref label (small query).
	}
}
```

Give `Resolver` an `alignments *AlignmentsStore` field (nil-safe: nil keeps today's behavior — matches the resolver's existing nil-`Family` handling). In `ResolveAndObserve`, after the resolution succeeds and a concept was resolved with a non-empty pref name, auto-align on exact released-label match:

```go
if res.Status == StatusLexicalResolved && res.ConceptPrefName != "" {
	if term, ok, err := r.matchLabelToReleasedTerm(ctx, res.ConceptPrefName, []string{"metric_definition"}); err == nil && ok {
		_ = r.alignments.EnsureAccepted(ctx, res.ConceptID, term.termID, "term_exact", 1.0,
			"pref_label exact match to a released metric_definition label (auto-assign, §16.1)")
	}
}
```

The auto-align threshold is **exact normalized label match only** — conservative, per D10's under-merge bias; no fuzzy/embedding alignment in the minimum loop.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/ontology/names/... ./server/api/ontology/keywords/... -v`
Expected: PASS, no regression (every existing `resolver_test.go` case must still pass — the nil-safe field keeps un-wired resolvers unchanged).

- [ ] **Step 5: Commit**

```bash
jj commit server/api/ontology/names/resolver.go server/api/ontology/names/resolver_test.go -m "names: follow aligns_to_term in resolution and auto-align on exact label (spec step 12, REQ-2)"
```

---

## Task 6: Metric columns + the §16.3 consumer call

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/extract-metrics.go` (`SaveMetrics`/`UpsertMetrics` SQL + the decorator), and the file where the metrics processor's `Store` is constructed (find it — the two write paths at lines ~505 and ~3235/3266 both use `p.Store`, the `MetricsStore` interface field at line 27; the decorator wraps at that field's assignment/construction point)
- Test: `ChenWeb/server/api/doc-processing/extract-metrics_test.go` (append)

**Interfaces:**
- Consumes: `names.Resolver` (Task 5), the migration from Task 1.
- Produces: §16.3's decision, made: the resolver call lives in **one** place — a `ResolvingMetricsStore` decorator wrapping the concrete metric store — so `FinalizeChunkBatch` and the enrichment path (both reach the same `p.Store`) get it without divergence (the D3 lesson: one seam, not one per write path).

- [ ] **Step 1: Write the failing test for the decorator**

```go
// A fake inner MetricsStore records the req it receives; a fake names
// resolver returns scripted NameResolutions per name. Assert:
//  - metric maps gain keyword_concept_id from resolution.ConceptID and
//    metric_definition_term_id from resolution.TermID (term_resolved only);
//  - an unresolved/ambiguous metric gets keyword_concept_id when the concept
//    resolved and NO metric_definition_term_id;
//  - a disabled resolver leaves both absent and passes through unchanged;
//  - SaveMetrics and UpsertMetrics both route through the resolution step.
```

- [ ] **Step 2: Add the two columns to `SaveMetrics`/`UpsertMetrics` SQL**

Append `keyword_concept_id` and `metric_definition_term_id` to the `INSERT INTO kb.metrics (...)` column list and `VALUES` placeholders in both `SaveMetrics` (line ~2593) and `UpsertMetrics` (line ~2747, including its `ON CONFLICT ... DO UPDATE` set clause), reading them from the metric map keys `metric_definition_term_id` / `keyword_concept_id` (the decorator sets them; absent keys → NULL). Match the existing per-metric-map field-extraction style in those functions.

- [ ] **Step 3: Implement the decorator**

```go
// ResolvingMetricsStore wraps a MetricsStore so every metric row is resolved
// through names.Resolver before it is persisted (spec §16.3, REQ-3): the one
// seam where the resolver is called, whatever write path reached the store.
// keyword_concept_id is populated from resolution.ConceptID (fast, ungoverned);
// metric_definition_term_id only from resolution.TermID on term_resolved
// (governed). extract_metrics itself is untouched — no prompt change.
type ResolvingMetricsStore struct {
	Inner    MetricsStore
	Resolver *names.Resolver
}
func (s *ResolvingMetricsStore) SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error) {
	resolved, err := s.resolveAll(ctx, req.Metrics)
	if err != nil { return 0, err }
	req.Metrics = resolved
	return s.Inner.SaveMetrics(ctx, req)
}
// ... UpsertMetrics mirrors it; the interface's other methods (MetricsExist,
// DeleteMetricsByInputRecordID, GetMetricsByInputRecordID) delegate verbatim.
func (s *ResolvingMetricsStore) resolveAll(ctx context.Context, metrics []map[string]any) ([]map[string]any, error) {
	// One ResolveNames batch over the distinct metric_name values (empty/skip
	// names with no string metric_name). Map each resolution back:
	//   keyword_concept_id = ConceptID when set; metric_definition_term_id =
	//   TermID when Status == StatusTermResolved. Never overwrite a value the
	//   extraction already wrote; never touch non-metric map keys.
}
```

Wire it where the metrics processor is constructed: locate the `MetricsSQLStore`/processor `Store` assignment (search for where a `MetricsSQLStore{...}` or `Store:` is set for the metrics processor) and wrap:

```go
store := &docprocessing.ResolvingMetricsStore{
	Inner:    <the concrete metrics store>,
	Resolver: names.NewResolver(keywords.NewKeywordFamily(<db>, ...)),
}
```

`NewResolver` needs a `*keywords.KeywordFamily` with a live DB — mirror how the REST handlers construct the family (see `server/api/ontology/keywords`'s wiring / the existing `names` package usage). This is the one integration point; do not duplicate it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/doc-processing/... -run 'TestResolvingMetricsStore' -v`
Expected: PASS. Then run the full package: `go test ./server/api/doc-processing/... -v` — **expect the pre-existing 15 environment-dependent failures** (§21, e.g. `TestMetricsSQLStoreSaveMetricsPersistsMetricCategoriesEn`); confirm the set is identical to before this task (your new test passes; no NEW failure appears). If a pre-existing failure changes shape because of the new SQL columns (e.g. a sqlmock `expected N args`), that's expected drift — update that *pre-existing test's* mock to the new column count and note it in the report, but do not fix unrelated pre-existing failures.

- [ ] **Step 5: Commit**

```bash
jj commit server/api/doc-processing/extract-metrics.go server/api/doc-processing/extract-metrics_test.go -m "doc-processing: resolve metric names to governed term ids before persist (spec step 12, REQ-3)"
```

---

## Task 7: Full verification and spec status update

**Files:**
- Modify: `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md` (§0 status table, §2.1 requirement table, §19 build order item 12, §21 implementation record, §14.2/§16.3/§20.1 status notes)

**Interfaces:** none (documentation + verification only).

- [ ] **Step 1: Run the full verification suite**

```bash
cd /Users/cding/Workspace/ChenWeb
go build ./...
go vet ./server/api/ontology/... ./server/api/doc-processing/... ./server/cmd/...
go test ./server/api/ontology/keywords/... ./server/api/ontology/semid/... ./server/api/ontology/names/... ./server/api/ontology/assertions/... -v
```

Expected: PASS/clean on the keyword/semid/names/assertions packages. The doc-processing package will still show the §21-documented pre-existing environment-dependent failures (15 in the step-11 baseline); Task 6 already confirmed the set did not grow. `go build ./...` must be clean.

- [ ] **Step 2: Update the spec's status sections**

In `2026080403-spec-keyword-canonicalization-and-reconciliation.md`:

- §0 "Status at a glance" **Not built** row: remove `aligns_to_term` and "the metric integration" (both now built); keep R1–R7, the online tier-6 resolve path, `on`-mode wiring, resource import. (Check the row's current wording from step 11's Task 9 edit.)
- §2.1 requirement table: mark **REQ-2** ✅ "accepted aligns_to_term assertion (§16.2)" and **REQ-3** ✅ "names.Resolver called by the consumer of extract_metrics, persisting metric_definition_term_id (§16.3)" — with the step-12 plan's scope note (auto-align on exact label + merge-follow is the minimum loop; governed-catalog bootstrap still gates the live end-to-end run).
- §19 build order item 12: mark done, noting the call-site decision (§16.3: `ResolvingMetricsStore` decorator, one seam at `p.Store` construction), the `subject_ref_kind` extension (migration `20260806000002`), the seeded `core:aligns_to_term`, and the two external prerequisites (§16.1 governed catalog, §13.2 import) that still gate the full §2.2 acceptance test.
- §21 "Implementation record": append a step-12 entry in the same style as the step-11 entry, using the actual `jj log` short hashes from ChenWeb.
- §14.2: the merge conflict gate + follow are now **live** (not just decided) — update the section header/status if it has one, and note it in the §21 entry.
- §16.3: record the call-site decision (decorator) in the text where the ⚠️ note currently says "The exact call site is not yet fixed, and choosing it is part of step 12."
- §20.1 "Deferred": remove `aligns_to_term` from the deferred list (mirroring step 11's Task 9 edit pattern for "Tiers 5–6").

- [ ] **Step 3: Commit the spec update**

KnowledgeStore is a separate jj-colocated repo. Check `jj status` first — it may carry pre-existing dirty files (`Diary/diary-202608.typ`, `Research/SemOS.typ` from an earlier session) — **do not touch or commit those**. Path-scope the commit:

```bash
cd /Users/cding/Workspace/KnowledgeStore
jj commit doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md -m "Update keyword spec status: step 12 (aligns_to_term + metric integration, REQ-2/REQ-3) implemented"
```

---

## Self-review notes (for whoever executes this plan)

- **Spec coverage:** Tasks 1–2 unblock the schema and the predicate (the two §16.1 hard prerequisites this module owns). Task 3 is the alignment store; Task 4 makes §14.2's gate real (the step-11 plan's explicit non-goal, now the current plan's job); Task 5 is REQ-2's resolver behavior + the §16.1 auto-assign producer; Task 6 is REQ-3's one-seam consumer call and the metric columns. Task 7 closes the spec loop (§21 is the living status source).
- **What this plan deliberately does not touch:** P4/REQ-4 (`metric_key`, §2.4), `extract_metrics`' prompt/extraction itself (only the persistence seam), §17.1 (`AssociateSemantics`), §17.2 (QUDT), reversibility UI (§14.4), and the governed-catalog bootstrap (§16.1) — all explicitly deferred/owned-elsewhere.
- **Known risk spots:** (1) the `assertions`/`keywords` import direction — verified `keywords` imports only `semid` and `assertions` looks like a leaf, but confirm before Task 3 (fallback: direct INSERT in `keywords/alignment.go`). (2) sqlmock sequences for `CreateAssertion` and the `MergeConcept` tx — iterate against real SQL text, as step-11's Tasks 4/5/7 did. (3) the exact `releasedTermSQL` reuse for the auto-align label match — the resolver already runs it once per resolve; do not add a second round-trip per metric if one batch query suffices (the decorator's `resolveAll` batches over distinct names). (4) Task 6's `UpsertMetrics` `ON CONFLICT` clause must include the two new columns or a re-merge would drop them.
