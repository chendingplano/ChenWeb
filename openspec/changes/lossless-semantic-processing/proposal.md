## Why

The metric pipeline drops the artifacts that carry the most information. Of 7,074 rows in
`kb.metrics` across all 58 metric-bearing input records, only 253 semantic assertions and 71 active
supporting evidence links exist: a content-level normalization problem (unmapped range type, unparsed
literal, unresolved referent) removes the metric from semantic processing entirely, so Review
Document, search, and comparison never see it. ADR `2026081401` made those misses *fail* the
processor, which surfaces an alert but still leaves the claim semantically unreachable and creates
retry storms no retry can clear until external knowledge changes.

ADR `2026081801` decides that SemOS stores source-backed claims even when they are malformed,
ambiguous, nonconforming, or not yet understood — and that only system execution failures fail a run.
ADR `2026081701` requires this decision as its Phase-0 dependency, so it blocks the metric
class/instance work as well.

## What Changes

- **Lossless invariant (DR1/DR2).** Every identifiable source-backed artifact yields a normalized
  instance, a raw-preserved instance, or a durable unresolved semantic occurrence. Raw values, units,
  datatypes, wording, and line spans are preserved independently of every normalized representation.
- **New append-only stores (DR4/DR13).** `kb.semantic_processing_outcomes` (one envelope per
  artifact × required stage) with typed child `kb.semantic_processing_findings`, and
  `kb.unresolved_semantic_occurrences` for families that cannot yet instantiate. Active-row
  invariants are enforced by named partial unique indexes, not application checks alone.
- **BREAKING (schema, additive-then-tightened): assertion lifecycle (DR6).** Adds `represented` and
  `unsupported_prior_status` to `kb.semantic_assertions`. Lossless ingestion writes `represented`,
  never `accepted`. The existing `chk_semantic_assertions_object_ref_or_literal` constraint is
  replaced by a value-state-aware constraint so a genuine `missing`-value instance can exist.
- **BREAKING (behavior): execution status (DR3/DR12).** Execution status becomes binary
  `completed`/`failed` with a mandatory finding summary on completed runs, projecting to the legacy
  `success`/`failed`. A proposed/ambiguous range-type mapping stops failing `extract_metrics` and
  `associate_semantics` and stops entering `has_failed_proc`; it becomes a `mapping_unresolved` /
  `mapping_ambiguous` finding on a raw-preserved assertion. Supersedes ADR `2026081401` DR3/DR6 on
  that point only.
- **Dependency-driven semantic retry (DR10).** Semantic findings leave the failed-processor queue and
  gain a retry path keyed on `(outcome_id, finding_id, target_dependency_fingerprint)` with
  transactional claim, leases, and stale-job detection.
- **Capability-aware consumers (DR8).** Comparison records `no_verdict`/`incomparable_with` instead of
  dropping instances; search, Review Document, and diagnostics expose represented assertions with
  warnings; governance and profile-rule evaluation stay accepted-only.
- **Family adapters (DR13).** A versioned adapter per artifact family declares raw identity, required
  stages, decision scopes, and retry triggers, and must pass a shared conformance suite before its
  writer gate is enabled.

Rollout follows the ADR's own gating: Phase 0 baseline → Phase 1 additive foundation → Phase 2
readers → Phase 3 `LOSSLESS_SEMANTIC_WRITES_METRIC` → Phase 4 `LOSSLESS_SEMANTIC_FALLBACK_WRITES`.

## Capabilities

### New Capabilities
- `lossless-semantic-processing`: The DR1/DR2 cross-cutting invariant — what must exist for every
  identifiable artifact, raw/normalized ownership, and the canonical identity-value branch rules.
- `semantic-processing-outcomes`: `kb.semantic_processing_outcomes` and
  `kb.semantic_processing_findings` — schema, deterministic keys, idempotent replay, supersession,
  active-row uniqueness, and governed disposition/finding/dimension vocabulary (DR4).
- `semantic-execution-status`: Binary `completed`/`failed` execution status, mandatory finding
  summaries, legacy `success`/`failed` projection, and the `system_failure` /
  `source_or_output_unrecoverable` / `semantic_finding` / `semantic_success` classification (DR3).
- `semantic-assertion-lifecycle`: The `represented` lifecycle, `unsupported_prior_status`, evidence
  loss/restoration as claim-preserving revisions, the value-state-aware payload constraint, and the
  cross-family minimum assertion field/state contract (DR6/DR9).
- `dependency-driven-semantic-retry`: Dependency fingerprints, the retry queue, transactional claim
  with leases, stale-job handling, and supersession on dependency change (DR10).
- `unresolved-semantic-occurrences`: The generic option-3 fallback store, transactional
  materialization/saga recovery, generic discovery API, and the family-adapter contract plus runtime
  compliance registry (DR13).
- `metric-lossless-writer`: The metric vertical slice — DR5 cardinality and atomic persistence
  boundary, `uq_assertion_evidence_current_metric_support`, and the DR12 value-range disposition table
  behind `LOSSLESS_SEMANTIC_WRITES_METRIC`.
- `semantic-consumer-compatibility`: Per-consumer lifecycle policy, dual-read certification, and
  capability-aware downstream behavior for search, comparison, projections, and Review Document
  (DR8/DR11).

### Modified Capabilities
<!-- No `openspec/specs/` baseline exists in this project; ADR-level supersession of
     ADR 2026081401 DR3/DR6 is captured in `semantic-execution-status` and
     `metric-lossless-writer` above rather than as a delta spec. -->

## Impact

- **Schema (`project_migrations/`):** new `kb.semantic_processing_outcomes`,
  `kb.semantic_processing_findings`, `kb.unresolved_semantic_occurrences`, `kb.semantic_retry_queue`;
  altered `kb.semantic_assertions` (status check, `unsupported_prior_status`, value-state constraint);
  new partial unique indexes on evidence and the three new stores; governed term seeds.
- **Go (`server/api/ontology/assertions/`):** `associate_semantics.go` (aggregate error on mapping
  miss removed; `accepted` → `represented`), `metric_normalizer.go`, `assertions_store.go`,
  `evidence_store.go`, `state_machine.go`, plus new outcome/finding stores, fingerprinting, adapter
  registry, and conformance suite.
- **Go (`server/api/doc-processing/`):** `control.go` runtime status persistence and `has_failed_proc`
  rollup must distinguish semantic findings from execution failures.
- **Consumers:** `kbhandler` ontology review/comparison handlers, `ontology/profiles/review_service.go`,
  `kbsearch`, projections in `projection_registry.go`, and the Review Document UI.
- **Dependencies:** ADR `2026081701` (claim registry, canonical keys, class resolution, redirects) is
  unimplemented; Phase 1 item 6, Phase 2 redirects, and all of Phase 3 are blocked on it. Phases 0–1
  in this change are deliberately independent of it.
- **Operational:** ~100× growth in semanticized workload (71 links → 7,074 occurrences); dashboards
  and alerts must stop reading semantic findings as failures.
