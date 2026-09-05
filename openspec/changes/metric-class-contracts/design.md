## Context

`classfoundation` (ADR `2026081701`, archived change `2026-08-18-canonical-metric-class-foundations`)
built `ContractStore`, `CapabilityValidationDispatcher`, `ObservedProfileStore`, and
`kb.ontology_term_headers` as shadow-mode foundations, explicitly deferring "automatic contract
activation" and "policy version and approval actor for autonomous contract activation" as open
questions. ADR `2026082203` (2026-08-22) then implemented the actual live class-resolution path
(`resolveOrCreateMetricClass` → `matchClassBySignature` → `assertions.SynthesizeClass` →
`keywords.synthesizeClass` → `terms.TermStore.CreateTerm`) without routing through `ContractStore`,
because at that point activating the contract layer was out of scope. The result, confirmed by reading both
code paths and by a live read-only query against `miner`: `kb.ontology_term_headers` is in fact
populated (4,639 rows) — but not by `ContractStore`; a pre-existing trigger
(`kb_sync_ontology_term_revision_after_insert`, migration `20260818000009`) mirrors every
`kb.ontology_terms` insert into `kb.ontology_term_headers` automatically, independent of
`classfoundation`. What that trigger does *not* do is touch `current_contract_revision_id` or
`kb.ontology_class_contract_revisions` at all — confirmed live: 0 of the 4,639 headers have
`current_contract_revision_id` set, and `kb.ontology_class_contract_revisions` has 0 rows.
`CapabilityValidationDispatcher.ValidateAndPersist` has zero non-test callers, and
`metric_lossless_writer.go` hardcodes `ConformanceStateTermID: semantic.ConformanceNotEvaluated`
unconditionally. So the real gap is narrower than first assumed: headers already exist for every
class; only the *contract* — the append-only revision chain and its capabilities — was never
started. This is the open question the prior change left for later; this change answers it
deterministically (no LLM, no human-approval workflow yet — see Non-Goals).

Two class identifier concepts exist in the schema:
- The **class term** (`kb.ontology_terms`, `term_kind='metric_definition'` or `'class'`) — what an
  assertion's `instance_of_term_id` actually points at today. Selected/created by
  `resolveOrCreateMetricClass`.
- The **class header** (`kb.ontology_term_headers`) — the stable anchor `classfoundation`'s
  append-only contract revisions hang off, per ADR `2026082203`'s own fix
  (`REFERENCES kb.ontology_term_headers(term_id)`, since `kb.ontology_terms` has no unique
  constraint on bare `term_id`).

This change makes every class term also have a header row, closing that gap, rather than
introducing a third mechanism.

## Goals / Non-Goals

**Goals:**
- Every class term `resolveOrCreateMetricClass` selects or creates ends up with a
  `kb.ontology_term_headers` row and at least one contract revision (idempotent — no duplicate
  identity-only revisions on repeat writes to the same class).
- Record observed-profile evidence per metric write (first live caller of `ObservedProfileStore`).
- Deterministically promote a contract from `identity_only` to `partially_defined` when — and only
  when — recorded evidence unambiguously agrees on value type and unit, sourced from multiple
  documents, never from a single occurrence or from malformed/unparsed evidence.
- Give `kb.semantic_assertions.conformance_state_term_id` a real value (`semantic:conforms` /
  `semantic:contract_violation`) once a contract can support it; keep `semantic:not_evaluated` as
  the honest answer while it can't.
- Let already-written `not_evaluated` metrics catch up once their class's contract is later promoted.

**Non-Goals:**
- Cross-instance comparison capability (`can_compare`) or anything in the DR22 comparison-matrix
  application. A contract that can validate one instance's value is not the same as a contract that
  can compare two — that's follow-on work once this lands.
- Any LLM involvement in contract synthesis or capability validation. Everything here is
  deterministic SQL/Go over already-extracted, already-resolved fields.
- A human curator review/approval UI for contract promotion. This change's synthesis is
  auto-promoted, mirroring the existing `auto-promoted` term pattern (ADR `2026081201`) — reviewable
  later, not gated on review now. `kb.ontology_class_contract_revisions.synthesis_method` records
  that it was automatic so a future review surface can find these.
- A background scheduler for the backfill/retry step. A one-shot admin command
  (`server/cmd/metric-contract-backfill`, matching the existing `metric-support-cleanup` /
  `qudt-import` pattern) is sufficient and simpler.
- Touching `class_resolution_service.go` (`ClassResolutionService`, `CreateIdentityOnlyClass`'s
  direct caller) — it has no callers today and this change doesn't give it one; it's dead code this
  change doesn't clean up (out of scope; flagged for a future surgical removal, not this change,
  which is additive).

## Decisions

### Reconcile via an idempotent `ContractStore.EnsureHeader`, not a rewrite of class resolution

`CreateIdentityOnlyClass`'s own doc comment already names the gap: "callers that need idempotent
resolution must resolve first (task 5.2)" — that follow-up was never written. Add
`EnsureHeader(ctx, tx, ClassIdentity) (ContractRevision, error)`: upsert the header row
(`ON CONFLICT (term_id) DO NOTHING` — a no-op in practice for terms the `kb.ontology_terms` insert
trigger already mirrored, and the actual backfill path for the rare term that predates that trigger
or was inserted some other way), then read `current_contract_revision_id`; if unset, call the
existing `AppendContractRevision` with `DefinitionIdentityOnly`; if set, return the existing current
revision unchanged. `resolveOrCreateMetricClass` calls this once, after it has settled on
`selectedClassTermID`, regardless of which of its three branches produced that ID (signature match,
existing-term reuse, or fresh synthesis) — so every class gets a first contract revision no matter
how it was resolved, including the 294 `metric_definition` and 6 `class` headers that already exist
live with no revision behind them.

**Alternative considered:** route class creation itself through `ContractStore.CreateIdentityOnlyClass`
instead of `terms.TermStore.CreateTerm`. Rejected — that would re-litigate ADR `2026082203`'s
signature-matching and keyword-concept-alignment logic (`keywords.synthesizeClass`), which has
nothing to do with contracts and was verified live only three weeks ago. Reconciling downstream of
class selection is strictly additive and cannot regress that mechanism.

### Contract-level capability vs. instance-level conformance are different checks, kept separate

`CapabilityValidationInput` carries a contract revision and a capability term — no per-instance data.
It answers "can this contract, as defined, support X at all" — evaluated once per
(contract revision, capability) and cached in `kb.ontology_class_contract_capabilities`. It is not,
and is not changed to be, a per-metric check.

Per-instance conformance (`conforms` vs. `contract_violation` on one assertion) is new, separate
logic added directly in `writeMetricLossless`: read the class's current contract
(`ContractStore.Current`, new small read method mirroring `EnsureHeader`'s read), and when its
`definition_state` is `partially_defined` or `validated`, compare this instance's resolved
`unitTermID`/value type against the contract's declared `permitted_unit_term_ids`/`value_type`.
Match → `semantic:conforms`; mismatch → `semantic:conformance_contract_violation` (both terms already
declared in `semantic/terms.go`, unused until now). `identity_only` contracts keep
`semantic:not_evaluated` — still the honest answer, per the user manual's own framing of that state.

### Synthesis eligibility: unambiguous agreement across ≥2 documents, never a single occurrence

Reuses the existing `kb.ontology_observed_class_attribute_observations` distribution row
(`observed_count`, `document_count`, already tracked per `(profile_id, attribute_key,
logical_datatype, value_form, unit_term_id, observation_state)`). For a class's `attribute_key =
'value'` rows with `observation_state = 'present'` (excludes `unparsed`/`missing`/`malformed`
evidence from ever *granting* authority, though it stays visible as evidence — spec requirement
already on file for `ontology-class-contracts`): eligible for promotion only if exactly one distinct
`(logical_datatype, unit_term_id)` pair appears and its summed `document_count ≥ 2`. Anything else
(multiple pairs, or a single pair from one document) leaves the class untouched — not an error, a
normal identity-only state, matching the existing "not_evaluated is not a fault" framing.

**Why document_count ≥ 2, not 1:** a contract's entire purpose is cross-document comparability; one
document's idiosyncratic unit choice is not evidence of that. Why not higher: this change doesn't
have a real threshold study to justify a bigger number, and a low bar that's transparent
(`synthesis_method` records it) is easier to correct later than a hidden heuristic. This is the one
place this design invents a number rather than reusing an existing one — flagged here rather than
buried in code.

**Alternative considered:** synthesize off the full observed-profile union (whatever value types/units
have ever been seen). Rejected outright — this is exactly what the pre-existing
`ontology-class-contracts` spec requirement already forbids ("Contract revision does not arise from
raw observation union").

### Synthesis and capability declaration run synchronously, inside the same write transaction

No new job, queue, or scheduler. Immediately after `ObservedProfileStore.Record` in
`writeMetricLossless`'s existing transaction: attempt synthesis (no-op if already promoted or
ineligible); if the contract was just promoted or already supports validation, declare
`semantic:can_validate_value` via `CapabilityValidationDispatcher.ValidateAndPersist` (idempotent —
`ON CONFLICT DO NOTHING` on the capability row; a fresh validation-result row is still appended each
time, which is correct: it's an append-only result history, not a cache). `semantic:can_instantiate`
is declared once, at `EnsureHeader` time, for every class regardless of definition state. All of this
is cheap (a handful of indexed queries) and keeps the "one atomic metric transaction" property the
existing code already documents and relies on for retry semantics.

### Backfill is a one-shot admin command, not automatic

`MetricAdapter.DependencyAxes()` already lists `class_contract_revision` and `validator_version` —
declared for exactly this purpose but never consumed. New command
`server/cmd/metric-contract-backfill`: for every `kb.semantic_assertions` row with
`conformance_state_term_id = 'semantic:not_evaluated'` and `instance_of_term_id` pointing at a class
whose contract has since left `identity_only`, re-run the instance-conformance comparison above and
update the row in place (not a new assertion — conformance state is a re-evaluation of an existing
represented claim, not a new claim). Manual, operator-run, matching `qudt-import` /
`metric-support-cleanup`'s existing pattern; not wired into `mise dev` or any request path.

## Risks / Trade-offs

- [A class flips `identity_only` → `partially_defined` mid-corpus, and later evidence from a
  differently-worded document contradicts it] → contracts are append-only; the promoted contract is
  left unchanged and the contradicting occurrence's own conformance state (task 7) records the
  disagreement, so it's visible on the specific assertion rather than silently absorbed into the
  contract or lost — this change does not auto-revert a contract, only auto-promotes from
  identity-only once. (Simplification made during implementation: an earlier draft of this design
  additionally proposed a duplicate signal via `ObservedProfileStore`'s exception recording;
  dropped as redundant with the conformance state, which already ties the disagreement to its
  specific evidence.)
- [The 2-document synthesis threshold is too permissive for high-noise corpora] → `synthesis_method`
  and `provenance` on the revision record the exact counts that triggered it, so a bad promotion is
  auditable and correctable (append a corrected revision) rather than silent.
- [Backfill command run against a large corpus is slow] → it only touches rows already
  `not_evaluated` whose class contract changed since the assertion was written; bounded by
  `class_contract_revision` in the dependency fingerprint, not a full-table rescan every run.
- [`EnsureHeader` races two concurrent writers resolving the same new class] → both branches use
  `ON CONFLICT (term_id) DO NOTHING` / existing-revision-check inside the caller's own transaction;
  the loser simply reads back what the winner committed, matching the existing idempotency pattern
  `matchClassBySignature`/`SynthesizeClass` already rely on for the same race.

## Migration Plan

1. Add `ContractStore.EnsureHeader` and `ContractStore.Current`; unit tests only (no schema change —
   all tables already exist from ADR `2026081701`'s migrations).
2. Wire `EnsureHeader` into `resolveOrCreateMetricClass`; add `ObservedProfileStore.Record` call in
   `writeMetricLossless`. No behavior change yet (contracts are still all identity-only; capability
   dispatcher not yet called) — verify via existing integration tests plus one new one asserting a
   header row now exists after a metric write.
3. Add the synthesis function, the two `CapabilityValidator` implementations, and wire the dispatcher
   + instance-conformance comparison into `writeMetricLossless`. This is the behavior change; gate
   nothing behind a new env var — the existing `LOSSLESS_SEMANTIC_WRITES_METRIC` gate already governs
   whether this code path runs at all.
4. Seed `semantic:can_instantiate` and `semantic:can_validate_value` governed terms (append to
   `server/api/ontology/seed/content.go`, released via the existing `ontology-seed` mechanism — no
   new seed command).
5. Add and run `metric-contract-backfill` once against the dev database as part of verification; do
   not run it automatically.

Rollback: everything is additive (new store methods, new terms, one new command); reverting the
`writeMetricLossless` edit alone restores the unconditional `not_evaluated` behavior without any
data loss, since contract revisions and capability rows are append-only and simply stop accumulating.

## Open Questions

- Should a contract, once `partially_defined`, ever be reconsidered (not just superseded by a human)
  if new evidence broadens the unambiguous set (e.g., a second unit later proves compatible via a
  dimension check)? Left for a future change — this one only ever promotes from `identity_only` once.
- Where does a curator eventually review auto-promoted contracts, mirroring the still-`to-be-developed`
  term-review surface the user manual names? Not built here; out of scope per Non-Goals.
