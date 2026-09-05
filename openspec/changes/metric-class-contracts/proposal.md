## Why

`metric-ontology-v1.0-en.md` §11.1 and ADR `2026081701`/`2026082203` leave every metric class
"identity-only": `kb.ontology_class_contract_revisions` has zero rows and
`metric_lossless_writer.go` hardcodes every assertion's conformance state to
`semantic:not_evaluated`. Two claims resolving to the same metric definition are not provably
comparable today because no contract declares a value type, a permitted unit, or a validation
capability. Investigation for this change found that the contract machinery ADR `2026081701` built
(`classfoundation.ContractStore`, `CapabilityValidationDispatcher`, `ObservedProfileStore`) has never
been wired to the live class-resolution path: ADR `2026082203` (2026-08-22) resolves/creates a
metric's class as a plain `kb.ontology_terms` row via `keywords.synthesizeClass`, bypassing
`ContractStore.CreateIdentityOnlyClass` entirely. A pre-existing trigger
(`kb_sync_ontology_term_revision_after_insert`) does mirror every such term into
`kb.ontology_term_headers` automatically, so headers exist (4,639 live rows) — but nothing sets
`current_contract_revision_id` or writes to `kb.ontology_class_contract_revisions`: live in `miner`,
0 of those 4,639 headers have a contract, and the revisions table is empty. This change activates
that existing, tested machinery against the real write path instead of building a parallel one.

## What Changes

- Reconcile class identity: `resolveOrCreateMetricClass` ensures a `kb.ontology_term_headers` row
  and a first identity-only `kb.ontology_class_contract_revisions` row exist for whatever class
  term the live synthesis path selects or creates, so the append-only contract history is anchored
  to the class every metric assertion actually points at (`instance_of_term_id`).
- Record observed evidence: the metric write path appends one `ObservedProfileObservation` per
  instance (value type, unit, range type actually seen) via the existing, currently-uncalled
  `ObservedProfileStore` — evidence only, grants no contract authority by itself.
- Add deterministic contract synthesis: when a class's accumulated observations agree unambiguously
  on value type and a single resolved unit/quantity-kind, promote its contract from `identity_only`
  to `partially_defined` with that payload. Disagreement or insufficient evidence leaves the class
  identity-only — no guessing, no LLM adjudication (unchanged non-goal from ADR `2026081701`).
- Seed two governed capability terms (`semantic:can_instantiate`, `semantic:can_validate_value`) and
  implement one `CapabilityValidator` for each; `can_instantiate` passes for any class with a
  contract row, `can_validate_value` passes only for a `partially_defined`+ contract whose declared
  value type and unit match the instance being evaluated.
- Wire per-instance conformance: `metric_lossless_writer.go` calls the capability dispatcher against
  the assertion's resolved class contract and sets `ConformanceStateTermID` to `semantic:conforms` or
  `semantic:contract_violation` instead of always `semantic:not_evaluated`; `not_evaluated` remains
  the honest result only when the contract is still identity-only.
- Add a bounded backfill/retry hook so metrics written before this change (all currently
  `not_evaluated`) get re-evaluated once their class's contract changes, reusing the existing
  `class_contract_revision`/`validator_version` dependency axes already declared on
  `MetricAdapter.DependencyAxes()`.

**Explicitly out of scope:** cross-instance comparison capability (`can_compare`) and the DR22
comparison-matrix application — those depend on this change's contracts but are a separate,
larger effort (ADR `2026072901` §15.5, P4). No LLM involvement anywhere in this change.

## Capabilities

### New Capabilities

- `metric-class-contract-synthesis`: Deterministic, evidence-driven promotion of a metric class
  contract from `identity_only` to `partially_defined`, backed by recorded observed-profile
  evidence, never by raw observation union.
- `metric-capability-validation`: Per-instance capability evaluation (`can_instantiate`,
  `can_validate_value`) against a class's current contract, replacing the unconditional
  `not_evaluated` conformance state on new and backfilled metric assertions.

### Modified Capabilities

- `ontology-class-contracts`: Contract revisions and capability records go from spec'd-but-unused to
  actually populated by the live metric write path; the identity-only → partially_defined transition
  gets a concrete, deterministic synthesis mechanism where none existed.
- `observed-class-profiles`: Gains its first live writer (the metric path); still evidence-only, per
  the existing requirement that it never grants contract authority.
- `semantic-assertion-lifecycle`: `ConformanceStateTermID` on a metric assertion can now be
  `semantic:conforms` or `semantic:contract_violation`, not only `semantic:not_evaluated`.

## Impact

- Affects `server/api/ontology/classfoundation/` (contracts_store.go, capability_validation.go —
  gains real callers; class_resolution_service.go remains unused, superseded by
  `resolveOrCreateMetricClass`, not touched by this change), `server/api/ontology/assertions/`
  (metric_lossless_writer.go, class_synthesizer_registry.go), `server/api/ontology/keywords/`
  (class_synthesis.go), `server/api/ontology/seed/` (new capability terms), and
  `kb.ontology_class_contract_revisions` / `kb.ontology_class_contract_capabilities` /
  `kb.ontology_class_capability_validation_results` / the observed-profile tables (all already
  migrated, currently empty; `kb.ontology_term_headers` is already migrated and populated by an
  existing trigger, but every row's `current_contract_revision_id` is null).
- Depends on ADR `2026081701` (contract/capability data model, unchanged) and ADR `2026082203`
  (live class-resolution path, extended not replaced).
- Does not touch `normalize_assertions`, unit/quantity-kind resolution, or the value-range-type
  mapping — those stay exactly as documented in `metric-ontology-v1.0-en.md` §9.4/§10.2.
