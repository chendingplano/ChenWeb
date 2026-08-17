## Context

ADR `2026081801` is a cross-cutting decision touching document processors, artifact stores, semantic
normalization, ontology association, error reporting, retry, projections, and Review Document. It
also acts as the Phase-0 dependency of ADR `2026081701` (canonical metric classes, instances, and
semantic relations).

Measured current state in the `miner` dev database:

| Fact | Value |
|---|---|
| `kb.metrics` rows | 7,074 across 58 input records |
| `kb.semantic_assertions` | 253 (236 `accepted`, 17 `superseded`) |
| `kb.assertion_evidence` active | 71 |
| Metric range types mapped `approved` | 4,050 |
| Metric range types with no map row | 2,395 |
| Metric range types mapped `ambiguous` | 629 |
| Metric rows with absent `value_range_type` | 1,366 |
| `kb.metric_value_range_type_map` | 63 approved / 5 proposed / 7 ambiguous |

The concrete loss points in code today:

- `associate_semantics.Run` returns an aggregate error whenever `report.MappingMisses > 0`
  ([associate_semantics.go:104](../../server/api/ontology/assertions/associate_semantics.go#L104)),
  which is exactly the ADR `2026081401` DR3/DR6 behavior DR12 supersedes.
- `processMetric` defers on unresolved referent, ungoverned assertion kind, and unreleased governed
  terms, and otherwise drives the assertion straight to `accepted`
  ([associate_semantics.go:434-437](../../server/api/ontology/assertions/associate_semantics.go#L434-L437)) —
  DR6 forbids both the drop and the automatic `accepted`.
- `kb.semantic_assertions` carries `CONSTRAINT chk_semantic_assertions_object_ref_or_literal`, which
  makes a genuine `missing`-value instance unrepresentable.

**Hard external dependency:** ADR `2026081701` has no migrations and no Go code — there is no
`claim_id`, canonical-key registry, class-resolution decision table, or assertion/term redirect. Every
DR that names those (DR2 canonical payload, DR5 class-resolution decision, DR6 provisional classes,
DR10 claim-registry redirects, all of Phase 3) is blocked until that ADR is implemented.

## Goals / Non-Goals

**Goals:**

- Land the additive shared foundation (DR3, DR4, DR6 lifecycle, DR9 vocabulary, DR10 retry records,
  DR13 fallback store) with **zero production behavior change**, per the ADR's Phase-1 gating.
- Produce the Phase-0 full-corpus baseline and capacity report the ADR requires before schema work.
- Make every invariant the ADR calls "database-enforced" actually enforced by a named constraint or
  partial unique index, not by application convention.
- Keep the framework family-generic so metrics are a vertical slice, not a special case.

**Non-Goals:**

- Implementing ADR `2026081701`. Class identity, claim registry, canonical payload bytes, and
  redirects stay owned by that ADR; this change defines only the seams they plug into.
- Enabling any lossless writer. `LOSSLESS_SEMANTIC_WRITES_METRIC` and
  `LOSSLESS_SEMANTIC_FALLBACK_WRITES` ship default-off and stay off in this change.
- Migrating non-metric families. Provisions, entities, inventory items, and products are Phase 4.
- Rewriting existing raw artifact rows. The migration is strictly additive.

## Decisions

### D1 — Outcome envelope and findings are two tables, not one JSON column

`kb.semantic_processing_outcomes` holds one row per `(input_record, artifact, stage)` attempt;
`kb.semantic_processing_findings` holds typed children. Rejected: a single table with a JSONB
`findings` array. DR4 requires *per-finding* active-row uniqueness, per-finding retry state, and
per-finding dependency fingerprints so a mapping change retries only the mapping finding — none of
which a JSONB array can enforce or index. `finding_count` and `highest_severity_term_id` stay
denormalized on the envelope and are transactionally derived from the child set, so reports never
scan children.

### D2 — Active-row invariants use partial unique indexes with explicit names

The ADR names three indexes; we create them verbatim so operational runbooks and the ADR agree:

- `uq_semantic_processing_outcomes_active` on `(outcome_key) WHERE active = true`
- `uq_semantic_processing_findings_active` on `(outcome_id, finding_key) WHERE active = true`
- `uq_unresolved_semantic_occurrences_active` on `(occurrence_key) WHERE active = true`

Plus the base uniqueness on `(outcome_key, input_fingerprint, dependency_fingerprint)`,
`(outcome_id, finding_key, dependency_fingerprint)`, and
`(occurrence_key, input_fingerprint, dependency_fingerprint)`.

These enforce **at most one**, never existence. Existence is a separate mechanism (D3). A deferred
constraint trigger rejects commit when an active finding references an inactive outcome — chosen over
an application check because supersession and child deactivation happen in one transaction and a
crash between them must not commit.

### D3 — Completeness is a projection, not an index

DR4/DR5/DR13 are explicit that uniqueness cannot prove "exactly one". We implement a
`SemanticCompletenessReport` that joins each current artifact occurrence against its adapter-declared
required stage set and reports missing outcomes, missing supporting links, and artifacts with neither
an assertion path nor an active unresolved occurrence. This report is the cutover gate; it runs in
Phase 0 against today's data and again before any writer gate flips.

### D4 — Dependency fingerprints are canonical JSON with a version prefix

Format: `v1:<sha256 of canonical JSON>` where the canonical JSON is a sorted-key object of the
declared dependency axes (mapping revision, parser version, class decision, contract revision,
validator version, unit vocabulary release, model/prompt version). Sorted keys plus explicit `null`
for absent axes makes the hash stable across Go map iteration order and across adding a new axis with
a version bump. The outcome's fingerprint is the canonical aggregate of its own stage dependency and
its current children's fingerprints, so a child-level change necessarily changes the envelope. Chosen
over hashing Go structs directly (fragile across field reordering) and over a compound natural key
(too wide to index).

### D5 — Governed vocabulary lives in `kb.ontology_terms`, not database enums

DR4 requires disposition/dimension/finding terms to be extensible ontology terms. New status values on
existing `CHECK` constraints (`represented`, `unsupported_prior_status` values) stay as CHECK
constraints because they are a closed state machine the database must enforce; the open-ended
finding/disposition/dimension vocabularies are seeded as governed terms and referenced by
`*_term_id TEXT`. `error_code` is a separate stable machine-readable string that does not participate
in identity, so a term rename never breaks programmatic consumers.

### D6 — Execution status stays binary, with the legacy projection as a pure function

`ExecutionStatus` is `completed | failed`. `LegacyProcStatus(status)` maps `completed → "success"`,
`failed → "failed"` and is the single call site anything writing `proc_status` uses, so no caller
invents a third value. "Completed with findings" exists only as a UI-derived label computed from
`finding_count > 0`. `has_failed_proc` and the failed-processor retry queue read `failed` only.

### D7 — `represented` is added to the state machine before any writer uses it

Migration order matters: the `status` CHECK constraint gains `represented`, the transition table gains
`represented → candidate`, the evidence-loss/restoration transitions, and `unsupported_prior_status`
(constrained to `represented|candidate|in_review|deferred|accepted`, permitted only when
`status = 'unsupported'`, required for evidence-loss-created unsupported rows) — all in Phase 1, while
no code writes `represented`. Every existing `status = 'accepted'` consumer is audited and given an
explicit documented policy in Phase 2 before Phase 3 flips a writer.

### D8 — Value-state-aware assertion constraint replaces the object-or-literal check

`chk_semantic_assertions_object_ref_or_literal` becomes a constraint keyed on the governed
`value_state_term_id`: `present` requires a normalized literal or reference; `unparsed`,
`datatype_mismatch`, and raw-valued `unknown` require a raw payload; `missing` requires subject,
class, applicability, and evidence but forbids a fabricated object; `not_applicable` requires explicit
non-applicability context. Because existing rows predate the state column, the migration backfills
`value_state_term_id = 'present'` for rows satisfying the old check before the new constraint is
validated, and uses `NOT VALID` + `VALIDATE CONSTRAINT` so the table is not long-locked.

### D9 — Adapter contract is a Go interface plus a runtime compliance registry

`SemanticFamilyAdapter` declares raw occurrence identity, required stages, decision scopes per stage,
minimum raw-preserved shape, capability-aware operations, and dependency axes. A
`kb.semantic_adapter_compliance` registry row records adapter name/version, writer mode,
conformance-suite version, and last verified result. Writer activation reads that registry and refuses
when the registered adapter has not passed the current suite — enforcement in code rather than a
deploy-time checklist, because the ADR makes activation-refusal normative.

### D10 — Phase 0 is a real command, not a document

`cmd/semantic-baseline` emits the required corpus report (occurrences, current
candidates/assertions/evidence, deferred/failure reasons, required stage counts, Review Document
visibility) plus the row/storage/throughput projection. It is checked in and re-runnable, so the same
tool produces the pre-cutover comparison the rollout section demands.

## Risks / Trade-offs

- **[Blocked on ADR `2026081701`]** → Phases 0–2 are scoped to be independent of it; Phase 3 tasks are
  explicitly marked blocked and are not started. Reconciliation notes (Phase 0 task 1) record every
  seam so the two ADRs cannot drift.
- **[100× write amplification: 71 links → 7,074 occurrences, each with evidence, class resolution, and
  stage outcomes]** → Phase 0 capacity model and load test gate Phase 1; outcome/finding rows are
  append-only and narrow; `last_seen`-only replay avoids row growth on unchanged reprocessing.
- **[Denormalized `finding_count`/`highest_severity_term_id` can drift from children]** → both are
  written in the same transaction as the child set, and the completeness report cross-checks them; a
  drift is a reported invariant violation, not silent.
- **[Deferred constraint trigger cost on high-volume writes]** → trigger is `CONSTRAINT TRIGGER
  DEFERRABLE INITIALLY DEFERRED` firing per statement on findings only; measured in the Phase 0 load
  test before Phase 1 sign-off.
- **[Adding `represented` widens a CHECK constraint that old code does not understand]** → old code
  never writes it and Phase 1 keeps writers off; the ADR's mixed-version rule (new readers with old
  writers, then gated new writers) is encoded as an ordering constraint in tasks.md.
- **[Backfilling `value_state_term_id` on 253 existing assertions could mislabel a row]** → the
  backfill only sets `present` where the old object-or-literal check already passed, which is every
  existing row by construction; anything else is left NULL and reported.
- **[Operators reading semantic findings as failures]** → DR11 reporting and the explicit
  `LegacyProcStatus` seam land in Phase 1 with the dashboards retrained in Phase 2, ahead of any
  writer.

## Migration Plan

1. **Phase 0** — run `cmd/semantic-baseline` against the full 58-record corpus; capture reconciliation
   notes against ADR `2026081701`; load-test outcome/finding/retry throughput; sign off capacity,
   coverage, latency, and rollback gates.
2. **Phase 1** — additive migrations only (new tables, new indexes, widened CHECKs, seeded terms) plus
   framework code behind no-op call sites; shadow-mode adapter comparison writes nothing
   consumer-visible.
3. **Phase 2** — deploy dual-read consumers, certify each against the reader compatibility suite,
   assign explicit lifecycle policy per consumer. Writers stay legacy.
4. **Phase 3** *(blocked on ADR `2026081701`)* — enable `LOSSLESS_SEMANTIC_WRITES_METRIC` after
   completeness and cardinality reports pass.
5. **Phase 4** — enable `LOSSLESS_SEMANTIC_FALLBACK_WRITES`, then migrate one family per slice.

**Rollback:** disable the writer gate and restore the legacy writer. Committed raw-preserved
assertions and outcome history are never deleted; dual-read consumers keep understanding them. Phase 1
migrations are additive and safe to leave in place on rollback.

## Open Questions

Carried from ADR §10, to be resolved before production cutover:

1. Physical split between assertion columns, validation-result tables, and
   `kb.semantic_processing_outcomes`, including where `unsupported_prior_status` lives.
2. Retention/compaction for large `raw_fragment` values when the source artifact and immutable
   invocation record already preserve identical content.
3. Default Review Document filters and warning presentation by outcome severity.
4. First non-metric family to migrate after the metric slice.

Added by this design:

5. Whether `kb.semantic_retry_queue` should share the existing failed-processor worker pool or run its
   own; resolved by the Phase 0 throughput test.
