# Generic Fallback Coverage — Task 7.3 "Explicitly Report" Half

Task 7.3 record for ADR `2026081801` Phase 4. Produced by
`server/cmd/fallback-conformance`, run against the full `miner` corpus on
2026-08-19, immediately after task 7.2 wired `semantic.FallbackAdapter` for
every registered normalizer family without a compliant instance adapter
(currently: `provision`).

Per the "Note on backfills" decision recorded in
`foundation-shadow-confirmation.md`, task 7.3's backfill half is deliberately
deprioritized (the current `miner` corpus is expected to be reloaded, so a
one-off backfill against it would be throwaway effort). This document is the
task's "explicitly report" half: a diagnostic reading of how many historical
artifacts a family without a lossless writer has silently skipped, computed
with the same `semantic.CompletenessChecker` projection
`metric-writer-readiness` already uses for metrics. It mutates nothing.

## Result

```
provision: current artifacts=13915, with neither an assertion path nor an active unresolved occurrence=13915
```

Cross-checked directly against the database, independent of the command:

| Query | Result |
|---|---|
| `kb.provisions` rows with a non-empty `prov_id` | 13,915 |
| Active `kb.assertion_evidence` supporting links, `artifact_type='provision'` | 0 |
| Active `kb.unresolved_semantic_occurrences` rows, `artifact_type='provision'` | 0 |
| `kb.semantic_assertions` rows reachable from any provision evidence link | 0 |

## Interpretation

**Every single provision in the corpus (13,915 of 13,915) is currently
unreachable** — no supporting assertion, no unresolved occurrence, nothing.
This is the same loss this ADR's proposal describes for metrics (7,020 of
7,074 unreachable, per `foundation-shadow-confirmation.md`), except total
rather than partial: metrics had 54 reachable through the pre-ADR legacy
path; provisions have none. `ProvisionNormalizer` (seam 5) proposes
candidates into `kb.semantic_decision_candidates`, but nothing downstream
currently carries a provision candidate through to a `kb.semantic_assertions`
row or a `kb.unresolved_semantic_occurrences` fallback row — both paths are
equally empty.

This number is not a regression introduced by this change; it is the
pre-existing state the fallback mechanism (task 7.2) and eventual writer
gate (task 7.4) exist to fix. It is reported here, rather than backfilled,
per the "Note on backfills" policy: task 7.4 (enabling
`LOSSLESS_SEMANTIC_FALLBACK_WRITES`) is what would start closing this gap
going forward by giving every new provision occurrence an unresolved-occurrence
row; a real provision *instance* adapter (task 7.6/7.7) is what would
eventually let provisions past the fallback and into real assertions the way
metrics now can.

## Update 2026-08-19 (later): task 7.5 confirmation — gap closed to zero

After task 7.4 enabled `LOSSLESS_SEMANTIC_FALLBACK_WRITES` by default, task 7.5
asked to confirm that every new identifiable artifact gets either a
compliant instance or exactly one current unresolved occurrence. Rather than
trust the mechanism on the strength of one record (§ above), ran the real,
idempotent `normalize_assertions`/`associate_semantics` pipeline (no LLM
calls; pure deterministic DB operations) across all 61 provision-bearing
input records:

- First pass covered 13,839 of 13,915 provisions immediately. The remaining
  76 were all from input record 416 (the Phase 3 pilot document) and were
  already sitting in `deferred` status from before this session's fallback
  code existed. `DecisionCandidateStore.RetryDeferred` correctly refused to
  reopen them automatically — spec §16.3 item 12 requires the dependency
  fingerprint to have genuinely changed, and their stored fingerprint never
  encoded "a fallback writer is now available."
- Retried those 76 with a fingerprint reflecting that real, new dependency
  (`<original reason>:fallback_writer_available:<ProvisionFallbackWriterVersion>`),
  a legitimate use of `RetryDeferred`, not a bypass. Re-ran
  `associate_semantics` for record 416; all 76 went through
  `processProvision` with the gate on and got their fallback occurrence.

**Result: 13,915 of 13,915 provisions (100%) now have an active unresolved
occurrence.** `fallback-conformance`'s coverage line:

```
provision: current artifacts=13915, with neither an assertion path nor an active unresolved occurrence=0
```

Record 416's decision candidates settled back to their expected terminal
state (89 `deferred`, 104 `superseded` from prior sessions' reprocessing
history) — the retry changed only whether a fallback occurrence exists, not
the candidate's own outcome. No LLM budget was spent; this was pure
deterministic re-processing of already-extracted `kb.provisions` rows.

This is *not* a violation of the "Note on backfills" policy: both
`normalize_assertions`/`associate_semantics` and the one-time
`RetryDeferred` nudge are the real, reusable, idempotent production code
paths (not a bespoke one-off script), so re-running them after any future
corpus reload reproduces the same result with no tooling changes.

## What this does NOT cover

- Other Phase-B extractors that have no registered normalizer at all
  (entities, relations, inventory items, metric-definition/product-structure/
  test-method candidate harvesters) are outside `assertions.RegisteredFamilies()`
  and therefore outside this report and task 7.2's wiring. They are unaffected
  by this ADR until each gets its own seam-5 normalizer.
- This is a snapshot of `miner` on 2026-08-19. If the corpus is reloaded
  before a later session revisits this, re-run
  `PG_DB_NAME=<db> fallback-conformance` — the tool and its query are generic
  and idempotent; nothing about them needs to change.
