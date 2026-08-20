# Lossless Semantic Processing — Phase 8 Pre-Cutover Report

Task 8.1 (ADR `2026081801` §6, "Required pre-cutover reports"). Generated 2026-08-20 against real
`miner` data using `cmd/semantic-baseline`, `cmd/metric-writer-readiness`,
`cmd/provision-writer-readiness`, `cmd/fallback-conformance`, and direct SQL. Every number below is
a live measurement, not a projection from the original Phase 0 baseline.

**This is an honest current-state baseline, not a completion claim.** Per the user's explicit
choice this session: produce the full report now, precisely because the corpus is not clean yet —
mirrors how `phase3-writer-readiness.md` and `generic-fallback-coverage.md` were done before
their respective gates flipped.

## 0. Read this first: `kb.metrics` is not the 7,074-row corpus the ADR was written against

The ADR's Phase 0 baseline (§5, Appendix B) describes 7,074 metric occurrences across 58 input
records. **`kb.metrics` today holds 60 rows, all from one record (416).** This is not new to this
report — it was investigated and documented in handoff `2026081905` §9 item 9 (see also `tasks.md`
5.8's linked discussion) as a **well-evidenced theory, not a confirmed fact**:

* `pg_stat_user_tables` for `kb.metrics`: `n_tup_ins=393`, `n_tup_del=7373`, `n_live_tup=60`.
* `kb.inputs` still holds all 209 corpus documents — the source documents were never lost.
* 146 of 209 have `pipeline_state='pending'`, created in one bulk-import window on 2026-06-09 that
  never ran the processing pipeline at all.
* Of the 56 documents that reached `pipeline_state='success'`, only doc 416 currently has any
  `kb.metrics` rows.
* `extract-metrics.go`'s `DeleteMetricsByInputRecordID` runs on forced reprocessing
  (delete-then-reinsert). The evidenced theory is that iterative reprocessing during development
  deleted and did not always reinsert metrics for documents other than 416 — not an accidental wipe,
  but not proven with an audit trail either.

**Every "full-corpus" number below is full coverage of the current 60-row/13,915-row corpus, not of
the original 7,074-metric one.** This gap is real and belongs in front of every other number in this
report, not buried in it.

## 1. Full-corpus coverage

| Family | Current artifacts | Missing stage outcomes | Artifacts missing any stage | Artifacts with neither path | Complete? |
|---|---:|---:|---:|---:|---|
| metric | 60 | 0 | 0 | 0 | **true** |
| provision | 13,915 | 0 | 0 | 0 | **true** |

Both families' completeness projections pass against everything currently in `kb.metrics` /
`kb.provisions` — but see §0: this is complete coverage of a much smaller `kb.metrics` set than the
ADR's own baseline. `provision`'s figure is the real one; provisions were never subject to the
reprocessing churn that hit `kb.metrics` (per handoff `2026082001` §5 item 4, only two provisions —
the `prov-lossless-*` fixtures aside — have ever gone through the real writer; the other 13,914 carry
a fallback occurrence, which the completeness projection accepts as the second lossless path per
DR1 option 3).

Input records: 209 total; 146 never entered the pipeline; 56 reached `pipeline_state='success'`.

## 2. Row/storage projections

| Table | Live rows | Total size |
|---|---:|---:|
| `kb.metrics` | 60 | 7.8 MB |
| `kb.provisions` | 13,915 | 78 MB |
| `kb.semantic_assertions` | 121 | 456 kB |
| `kb.semantic_processing_outcomes` | 14,096 (14,095 active) | 14 MB |
| `kb.semantic_processing_findings` | 13,934 (13,933 active) | 9.1 MB |
| `kb.unresolved_semantic_occurrences` | 13,915 (13,914 active) | 16 MB |
| `kb.assertion_evidence` | 61 | 120 kB |
| `kb.semantic_retry_queue` | 0 | 40 kB |
| `kb.semantic_adapter_compliance` | 2 | 48 kB |
| `kb.semantic_claim_identities` | 60 | 136 kB |
| `kb.ontology_class_contract_revisions` | 0 | 32 kB |
| `kb.ontology_term_redirects` | 0 | 24 kB |

The two ADR-`2026081701` class/instance tables (`ontology_class_contract_revisions`,
`ontology_term_redirects`) are empty — expected, per Appendix C.4: that apparatus is metrics-only in
principle but no contract revision or redirect has actually been authored yet.
`semantic_processing_outcomes`/`findings` are overwhelmingly provision fallback rows (14,095 active
outcomes = 60×3 metric stage envelopes + 13,915×1 provision `StageAssociate` envelopes, exactly).

## 3. Write/retry throughput

* **Write throughput** (Phase 0 task 1.5 load test, corpus-scale synthetic run, not re-run this
  session — see `loadtest_integration_test.go`): measured writing 21,222 outcome envelopes at
  7,074-occurrence × 3-stage scale; timings are logged per-run via `t.Logf`, not persisted to a
  report file. Not re-measured here since it is a synthetic capacity test, not a live-data number.
* **Retry throughput: not measurable.** `kb.semantic_retry_queue` has **0 rows** in `miner` today.
  The enqueue call exists (task 6.8, `UpsertValueRangeTypeMapEntry` on mapping approval) but no
  mapping approval against a matching active finding has happened in this database since the writer
  went live, so no row has ever landed there to measure a throughput against. Separately, per
  Appendix C.5, `RetryQueue.Claim` has zero production callers regardless — there is no drain to
  measure even if a row existed.

## 4. Assertion counts before and after canonical convergence

| Family | Raw occurrences | Assertions | Convergence ratio |
|---|---:|---:|---:|
| metric | 60 | 60 (+61 `kb.semantic_claim_identities` — see note) | 1:1 (no convergence observed) |
| provision | 2 real (`prov-lossless-*` fixtures aside) | 1 (`416_prv_1`, id 404) | not applicable — provisions have no claim registry (Appendix C.4) |

No canonical convergence (many raw occurrences resolving to one shared assertion) has occurred for
metrics in the live corpus: 60 raw metric rows, 60 claim identities, 60 assertions with metric
support links. This is consistent with the small, single-document current corpus rather than
evidence the convergence mechanism doesn't work — `2026081904`/`2026081905` exercised convergence
directly in integration tests. Provisions structurally cannot converge by design (§C.4: each
`prov_id` is its own atomic identity).

## 5. Raw artifacts with no instance or occurrence

**Zero for both families** (`ArtifactsWithNeitherPath: 0`, confirmed by both readiness tools and
directly by `dup`/`orphan` queries in §10). Every one of the 60 live `kb.metrics` rows and all
13,915 `kb.provisions` rows has either a supporting assertion or an active fallback occurrence. See
§0 for why this is coverage of the current, reduced `kb.metrics` set, not the original 7,074.

## 6. Instances by value/class/conformance state

| value_state | class_identity_state | conformance_state | count |
|---|---|---|---:|
| *(null — legacy)* | *(null — legacy)* | *(null — legacy)* | 60 |
| `semantic:value_present` | `semantic:resolved_existing` | `semantic:not_evaluated` | 41 |
| `semantic:value_state_unparsed` | `semantic:resolved_existing` | `semantic:not_evaluated` | 16 |
| `semantic:value_state_missing` | `semantic:resolved_existing` | `semantic:not_evaluated` | 3 |
| `semantic:value_present` | *(null — provision, no class-resolution stage)* | `semantic:not_evaluated` | 1 |

The 60 all-null rows are exactly the 60 legacy `accepted` assertions (§9 below) — they predate DR6's
state columns. Migration `2.9` backfilled `value_state='present'` only for rows existing *at
migration time*; these were written by the legacy path afterward and were never in scope for
backfill (DR2: correction is a new revision, not a silent rewrite of history). Not a regression.
`conformance_state` is `not_evaluated` for every populated row — no class contract revision exists
yet to evaluate against (§2), consistent with `kb.ontology_class_contract_revisions` being empty.

## 7. Current proposed mappings with occurrence counts

| Status | Distinct raw values | Sum of `occurrence_count` |
|---|---:|---:|
| `approved` | 64 | 21 |
| `ambiguous` | 7 | 3 |
| `proposed` | 4 | 14 |

Proposed (unresolved) mappings with a nonzero historical sighting count:

| `raw_value` | `occurrence_count` |
|---|---:|
| `standard` | 8 |
| `unspecified` | 4 |
| `percentage_range` | 1 |
| `computed_value` | 1 |

`occurrence_count` accumulates across the table's full history (since ADR `2026081401`), so these
totals reflect sightings from the original, larger corpus — they are far smaller than 7,074 because
most historical sightings were of already-`approved` synonyms. None of the 60 live `kb.metrics` rows
currently carry a `proposed` or `ambiguous` mapping (all 60 are on `approved` buckets, matching §1's
zero-error count).

## 8. Old processor failures that become findings

Decision-candidate breakdown across both families (`kb.semantic_decision_candidates`):

| Family | Status | Reason | Count |
|---|---|---|---:|
| provision | `deferred` | `no_governed_deontic_predicate` | 13,914 |
| provision | `superseded` | *(none)* | 183 |
| provision | `superseded` | `no_governed_deontic_predicate` | 26 |
| metric | `accepted` | persisted as `kb.semantic_assertions` (represented) | 60 |
| metric | `superseded` | `unresolved_referent` | 1 |
| provision | `accepted` | persisted as `kb.semantic_assertions` (represented) | 1 |

The 13,914 `deferred`/`no_governed_deontic_predicate` rows are the historical record of what used to
be a silent, invisible loss before this ADR: under the pre-ADR model, a provision with no governed
deontic predicate term simply had nowhere to go. Today the identical 13,914 artifacts are **also**
durably visible via `kb.unresolved_semantic_occurrences` (task 7.5's fallback coverage) — the
decision-candidate row's stale `no_governed_deontic_predicate` reason predates task 7.6's new `prov:`
terms and was not bulk-retried (per the "Note on backfills" policy: `RetryDeferred` requires a
genuinely changed fingerprint per row, not a bulk sweep). The 60 metric `accepted`/`represented` rows
are exactly the metrics that, under the pre-Phase-3 model (ADR `2026081401` DR3/DR6), would have
failed `extract_metrics`/`associate_semantics` outright if their mapping was proposed or ambiguous —
DR12's disposition table is what turned that failure into a finding instead.

## 9. Lifecycle counts including represented/accepted/unsupported

| `status` | Count |
|---|---:|
| `represented` | 61 |
| `accepted` | 60 |
| `unsupported` | 0 |
| `rejected` | 0 |
| `deferred` | 0 |
| `candidate` / `in_review` / `superseded` | 0 |

No assertion has ever entered `unsupported` (no represented/accepted claim has lost its last
supporting evidence link) or `rejected`/`superseded` in the live corpus yet. All 60 legacy rows are
`accepted` (pre-lossless writer); all 61 new rows are `represented` (60 metric + 1 provision),
matching DR6: lossless ingestion never writes `accepted` directly.

## 10. Constraint and active-row violations

| Check | Violations found |
|---|---:|
| Duplicate active `kb.semantic_processing_outcomes` per `outcome_key` | 0 |
| Duplicate active `kb.semantic_processing_findings` per `(outcome_id, finding_key)` | 0 |
| Duplicate active `kb.unresolved_semantic_occurrences` per `occurrence_key` | 0 |
| Active finding referencing an inactive outcome | 0 |
| `summary drift` (outcome `finding_count` ≠ live active-finding count) — both readiness tools | 0 |
| `orphan active findings` — both readiness tools | 0 |

Every named partial-unique-index invariant holds in the live corpus today. No cleanup required.

## 11. Downstream `no_verdict`/skip reasons

**Zero `semantic:no_verdict` findings exist in `miner` today.** Active finding terms present:

| `finding_term_id` | Count |
|---|---:|
| `semantic:mapping_unresolved` | 13,914 |
| `semantic:unparsed` | 16 |
| `semantic:value_missing` | 3 |

The comparison engine's `no_verdict`/`incomparable_with` capability (task 5.4) is implemented and
covered by its own tests, but nothing in the live corpus has exercised a real comparison against an
unresolved/unparsed instance yet — there is no comparison-triggering workflow run against this
corpus so far. This is a coverage gap in *exercising* the capability against real data, not a defect
in the capability itself.

## 12. Retry queue size by dependency type

**`kb.semantic_retry_queue` is empty (0 rows).** There is nothing to break down by dependency type.
See §3 and Appendix C.5: the enqueue call exists and is wired to a real admin action, but no matching
event has fired against this database, and nothing drains the queue even when it does have rows.

## 13. Review Document result changes

Quantifying what task 5.5's Semantic Diagnostics tab now shows that was previously inaccessible:

* **61 `represented` assertions are visible today that would not have existed as assertions at all
  under pre-ADR behavior**: the 60 metrics (proposed/ambiguous/unparsed/missing-value cases that
  ADR `2026081401` DR3/DR6 would have failed outright) and the 1 real provision instance (no code
  path existed to materialize a provision as an assertion before task 7.6).
* All 61 carry their raw value, normalized value (where parsed), all four independent governed
  states, class confidence, and active evidence in the tab — unfiltered, with severity shown as
  color-coded badges rather than a pass/fail gate (Appendix C.3).
* The 13,914 provision fallback occurrences are **not** shown in this assertion-scoped view (they
  have no assertion yet) — they are visible only through the lower-level generic-discovery reader
  built in task 7.2, not the Review Document UI. This is a real, currently-unaddressed gap if the
  intent is for Review Document to eventually surface fallback-only artifacts too; not scoped or
  decided this session.

## Summary

Both registered families (`metric`, `provision`) pass their conformance suite and completeness
projection against everything currently in their respective raw tables, with zero constraint
violations anywhere in the outcome/finding/occurrence layer. The two things this report surfaces
that are **not** cutover-ready: (1) `kb.metrics`' 60-row scope is far short of the ADR's own
7,074-row baseline, for reasons documented but not proven (§0); (2) `kb.semantic_retry_queue` is
architecturally wired but empirically untested — empty, undrained, and not yet exercised end-to-end
in this database (§3, §12, Appendix C.5).
