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
