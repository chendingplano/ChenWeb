## Why

`2026081001-bug-pipeline-policies-vs-pipelines-schema-review.md` found `kb.pipelines` fully
mutable with no audit trail, and `kb.pipeline_policies` hashing the whole `kb.pipelines` table on
every compile without ever being re-checked after activation. A follow-up Q&A pass (recorded in
ADR `2026081001`) confirmed by reading every call site that `kb.pipeline_policies` is never
consulted at runtime as anything richer than a `WHERE status = 'active'` filter — the real gap is
a *persisted, per-pipeline* processor dependency graph, not a per-policy one. `kb.pipeline_rules`
already carries `Requires`/`Produces`-shaped fields in Go (`productionProcessorSpecs`) whose own
doc comment admits they are declared but never consumed by any planner; execution order today is
a fixed Phase-A/B/C bucket concatenation, not a dependency-ordered sort.

## What Changes

- **`kb.pipelines` becomes immutable and versioned.** Add `version`/`status` columns, `UNIQUE
  (name, version)`. Editing `processors[]` inserts a new row (`version = MAX+1`) instead of
  `UPDATE`ing in place; creating version N+1 marks version N `superseded`. **BREAKING:**
  `DeletePipeline`'s hard `DELETE` is removed — pipelines are never physically deleted again.
- **A pipeline version is authored atomically.** One transaction creates the `kb.pipelines` row,
  every `kb.pipeline_rules` row (gates + DAG edges), validates everything, and commits. There is
  no later "add a rule to an existing version" path.
- **`kb.pipeline_policies` is retired entirely. BREAKING.** `DROP TABLE kb.pipeline_policies`, its
  two handler endpoints (`ListPipelinePolicies`/`CreatePipelinePolicy`/`ActivatePipelinePolicy`),
  and every `policy_id` column on `kb.pipeline_bindings`/`kb.pipeline_rules`. The four
  `WHERE ... policy_id = (SELECT ... active ...)` call sites collapse to `WHERE active`. A new
  partial unique index on `kb.pipeline_bindings` (scoped to context columns, `binding_kind =
  'store_default'`, `WHERE active`) replaces the "at most one active policy" guarantee with "at
  most one active unconditional binding per context" — restoring the pre-policy constraint that
  the migration chain silently dropped along the way (`kb.pipeline_bindings` currently has **no**
  store/context uniqueness constraint at all; confirmed by reading the migration chain).
- **Provenance shifts from policy id+version to pipeline name+version.** Every place that records
  `ActivePolicyID`/`ActivePolicyVersion`/`PolicyID`/`PolicyVersion` — plan facts, alarm/audit
  events, D2 routing-clearance subjects and enforcement requests, binding-resolution trace,
  `policyaudit.Event` — switches to recording the winning `kb.pipeline_bindings.id` plus the
  resolved `kb.pipelines.name`+`version`. This surface is substantially larger than the ADR's own
  "Component" list: it includes the D2 routing-clearance/enforcement subsystem
  (`routing_clearance.go`, `routing_enforcement.go`, `pipeline_routing_clearances_handler.go`),
  `policy_promotion.go`, `control.go`, `pipeline_selection.go`, `ontology/policyaudit/store.go`,
  and the binding/rule CRUD handlers (`pipeline_bindings_handler.go`, `pipeline_rules_handler.go`)
  — all confirmed by grepping every `policy_id`/`PolicyID`/`PolicyVersion` reference in `server/`.
- **`kb.pipeline_rules` gains real DAG edges.** New `depends_on_processors TEXT[]` column: names
  of sibling processors, within the same pipeline version, that must finish before
  `target_processor` runs. Orthogonal to the existing `predicate`/`effect` gate columns.
- **New `kb.processor_registry` table** for processor dependency metadata, adopted incrementally:
  existing processors stay hardcoded in `productionProcessorSpecs` untouched; only new processors
  get a registry row. Anything needing the full picture reads the union of both sources.
- **Creation-time-only validation, hard-reject.** Three checks run once, when a pipeline version
  is authored, never again: (1) processor closure — every `Requires` fact has a producer in the
  same version's processor set; (2) DAG well-formedness — `depends_on_processors` is acyclic and
  every referenced name is in the version's own processor set; (3) gate-fact availability — every
  gate predicate's referenced facts are guaranteed produced upstream in the DAG before that gate
  evaluates.
- **`GateEffectDefer` is retired as a runtime-recoverable state. BREAKING.** Check 3 above proves
  ahead of time that no gate ever needs to defer. `effect = 'defer'` becomes a creation-time
  rejection (name the gate and the missing fact). At runtime, if the gate resolver ever still
  produces an indeterminate/defer outcome anyway (a safety net, not an expected path), it is now a
  hard processing failure (alarm + fail the run) instead of falling open to
  enable/skip via `resolveIndeterminateGate`'s current fallback.
- **Out of scope (explicit, per ADR):** the actual topological-sort execution engine that consumes
  `depends_on_processors` (today's `ExecutionOrder()` stays a fixed phase-bucket concatenation);
  a frontend for authoring conditional gate predicates.

## Capabilities

### New Capabilities

- `pipeline-versioning`: `kb.pipelines` immutability/versioning (`version`/`status` columns,
  atomic single-transaction authoring, no in-place edits, no hard delete).
- `pipeline-policy-retirement`: removal of `kb.pipeline_policies` and the policy_id/version
  provenance model, replaced by per-row `active` + a per-binding-target unique constraint and
  pipeline name+version provenance, propagated through routing, D2 clearance/enforcement, audit,
  and CRUD handlers.
- `pipeline-dependency-graph`: `depends_on_processors` DAG edges on `kb.pipeline_rules`, the new
  `kb.processor_registry` table, and the three creation-time validation checks (processor closure,
  DAG well-formedness, gate-fact availability).
- `gate-defer-retirement`: `defer` gate effect rejected at creation time; runtime indeterminate
  outcomes become hard failures instead of a fallback-to-enable/skip retry path.

### Modified Capabilities

(none — no existing `openspec/specs/` capability covers this area yet; this is the first change to
formally spec the pipeline-routing subsystem)

## Impact

**Schema (new goose migrations under `project_migrations/`):**
- `kb.pipelines`: add `version INT NOT NULL DEFAULT 1`, `status VARCHAR(16)` CHECK
  `('active','superseded')`; replace `UNIQUE (name)` with `UNIQUE (name, version)`.
- `kb.pipeline_bindings` / `kb.pipeline_rules`: drop `policy_id` (and its FK/index); add the new
  partial unique index on `kb.pipeline_bindings`.
- `kb.pipeline_rules`: add `depends_on_processors TEXT[] NOT NULL DEFAULT '{}'`.
- New `kb.processor_registry` table.
- `DROP TABLE kb.pipeline_policies` (after data migration below).
- **Migration-risk data decision (must be resolved by this change, not left open):** `miner` has 4
  `kb.pipeline_policies` rows; the active one's bindings/rules carry a real `policy_id`. Backfill
  keeps the active policy's bindings/rules (drop `policy_id`, keep `active` as-is); the 3 archived
  policies' rows are discarded (no separate audit/history table — confirmed as the simpler of the
  two options the ADR leaves open, see design.md).

**Go code — `server/api/doc-processing/`:** `policy_compile.go`, `policy_seed.go`,
`processor_plan.go`, `runtime.go`, `pipeline_bindings.go`, `pipeline_gates.go`,
`pipeline_rules_store.go`, `extract-doc-metadata-store.go`, `pipeline_registry_store.go`,
`pipeline_policies_store.go` (removed), `policy_promotion.go`, `control.go`,
`pipeline_selection.go`, `routing_clearance.go`, `routing_enforcement.go`.

**Go code — `server/api/kbhandler/`:** `pipeline_policies_handler.go` (removed),
`pipelines_handler.go`, `pipeline_bindings_handler.go`, `pipeline_rules_handler.go`,
`pipeline_routing_clearances_handler.go`.

**Go code — other:** `ontology/policyaudit/store.go`, `server/cmd/doc-processing-policy-seed/main.go`.

**Tests:** every `_test.go` paired with the files above, plus indirectly-affected suites
(`control_test.go`, `routing_enforcement_test.go`, `routing_clearance_test.go`,
`pipeline_gates_test.go`, `p5_migration_contract_test.go`, `policy_promotion_test.go`,
`policy_authorization_test.go`, `pipeline_registry_store_test.go`,
`pipeline_policies_store_test.go` (removed)).

**Docs:** ADR `2026081001` status → Accepted once implemented; `ChenWeb/docs/superpowers/specs/
2026-08-08-doc-processing-policy-design.md` needs a note that the `kb.pipeline_policies`-shaped
bootstrap behavior it documents no longer applies post-retirement.
