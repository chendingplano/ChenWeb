# ADR 2026081701 Foundation Shadow-Mode Confirmation

Task 6.1 confirmation record for ADR `2026081801` Phase 3. Produced by
`server/cmd/metric-foundation-shadow-report`, run against the full `miner`
corpus (58 metric-bearing input records, 7,074 `kb.metrics` rows) on
2026-08-18, after task 6.2's duplicate-support cleanup and task 6.3's
`uq_assertion_evidence_current_metric_support` index landed.

Before this run, `MetricAdapter.RunShadow` had **zero production callers** —
only `lifecycle_integration_test.go` exercised it. Nothing had ever executed
the ADR `2026081701` shadow path against real data. This command is the
corpus-wide, re-runnable equivalent of Phase 0's `semantic-baseline` (D10:
"Phase 0 is a real command, not a document"), applied here to the Phase 3
foundation check.

## Result

```
input records examined: 58 (failures: 0)
metrics examined: 7074
existing supported (reachable today): 54
existing unreachable today: 7020
would be normalized: 2618
would become raw-preserved: 4456
intended outcome envelopes: 21222
duplicate current support remaining: 0
foundation class resolved (stable, existing): 89
foundation class unavailable: 6985
foundation claim key candidates (not registered -- shadow only): 89
foundation profile observation candidates (not persisted -- shadow only): 89
intended findings by term:
  semantic:mapping_ambiguous: 629
  semantic:mapping_unresolved: 803
  semantic:value_missing: 3575
```

Every write-guarded table (the ADR `2026081701` foundation tables plus
`kb.semantic_assertions`/`kb.assertion_evidence`) had **zero row-count
drift** across the run — the shadow path is confirmed write-free against
real data, not just by code inspection.

## Per-foundation confirmation

| Foundation (as named in task 6.1) | Status | Evidence |
|---|---|---|
| Redirect resolution | **Active** | `termRedirectLookup` issued a real `kb.ontology_term_redirects` lookup for all 7,074 metrics this run; genuinely exercised against production data, not just unit-tested. |
| Canonical claim key computation | **Active (computation only)** | `SerializeCanonicalClaimKey` computed for all 89 metrics with a resolved class; this is the first time it has run over real data. |
| Claim/canonical-key registries (`kb.semantic_claim_identities`, `kb.semantic_canonical_key_versions`) | **Dormant, not active** | 0 rows before and after. `ClaimIdentityStore.FindOrCreateShadow` — the only writer of this table — has zero production callers anywhere in the codebase. The registry exists (migrated, unit-tested) but has never registered a single claim. |
| Stable class assignment (`ResolutionResolvedExisting`) | **Active** | 89 of 7,074 metrics (1.3%) resolve to an existing class via their `metric_definition_term_id` and redirect resolution. |
| Provisional class assignment (`ResolutionProvisionalNew`) | **Not exercised, by design** | Shadow mode never calls `ClassResolutionService`'s provisional-creation path — doing so would write to `kb.ontology_term_headers`/`kb.ontology_class_contract_revisions`, which Phase 1's "no consumer-visible write" rule forbids. `MetricFoundationClassUnavailable` (6,985 metrics, 98.7%) is shadow mode's honest report of "would need this path," not a run of it. **This is the dominant real-world case, and it has never been exercised against real data in any mode.** |
| Observed class profiles (`ObservedProfileStore`) | **Dormant, not active** | 0 rows; zero production callers, consistent with `reader-compatibility-certification.md`'s prior finding. |
| Class resolution decisions (`kb.ontology_class_resolution_decisions`) | **Dormant, not active** | 0 rows; `ClassResolutionService` has zero production callers. |

## What this means for the rest of Phase 3

Task 6.1 asked to "confirm the foundations are active in shadow mode." The
honest answer is **partially**: the read-only pieces (redirect resolution,
claim-key computation, stable-class lookup) are now confirmed active against
real data for the first time. The pieces that require a write (claim
registration, provisional class creation, profile recording, resolution
decisions) are correctly inert in shadow mode by construction — but that
also means they are **completely unproven against real data**.

Since 98.7% of the corpus falls into "class unavailable" today, task 6.5's
atomic metric semantic transaction will overwhelmingly be exercising the
provisional-class-creation path the very first time it runs for real, not
the already-proven stable-class path. This is worth weighing before
implementing 6.5: the 89-metric stable-class path has real evidence behind
it; the ~6,985-metric provisional path does not, beyond its own unit tests.

## Note on backfills (decision, 2026-08-18)

The current `miner` corpus data is expected to be deleted/reloaded, so any
one-off backfill written against *today's* rows is throwaway effort — it
would need to be re-run (or re-derived) against whatever data eventually
replaces it, and produces no durable evidence about the real corpus. Going
forward in this change:

- Do not invest further engineering effort in backfill scripts that operate
  against the current `miner` data specifically. A general-purpose,
  reusable, idempotent tool (as task 6.2's `metric-support-cleanup` already
  is) is fine to keep; a bespoke one-shot data massage is not worth writing.
- Task 6.2 (duplicate current-metric-support link cleanup) already ran
  before this decision was made. It is left as-is: the tool is generic and
  idempotent (safe to re-run against reloaded data), it unblocked task 6.3's
  schema-level unique index (which is not data, so it persists across any
  reload), and undoing an audited soft-delete would add risk for no benefit.
- Task 7.3 ("Backfill or explicitly report historical artifacts skipped
  before lossless processing") is the one other explicitly-named backfill
  task remaining in this change's `tasks.md`. Its backfill half is
  deprioritized under this policy; its "explicitly report" half (a
  diagnostic, not a data mutation) is unaffected and can proceed normally
  when Phase 4 is reached.
- `tasks.md` task 2.9 (the `value_state_term_id` backfill) and this
  document's own task 6.2 write-up both predate this decision and are
  historical — no retroactive action needed.
- Corpus-derived *reports* (e.g., task 6.10's cardinality/stage-outcome
  proof, task 8.1's pre-cutover reports) are not backfills — they read and
  describe data rather than mutate it — so this policy does not defer them.
  If the corpus is reloaded before those tasks run, simply re-run the report
  tooling against the new data; nothing about the tooling itself changes.
