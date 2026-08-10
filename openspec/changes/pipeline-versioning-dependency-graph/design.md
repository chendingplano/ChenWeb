## Context

This transcribes ADR `2026081001` (`KnowledgeStore/doc-repo/adrs/202608/
2026081001-adr-pipeline-policy-versioning-and-dependency-graph.md`) plus a full code-map pass done
before writing this design (every `policy_id`/`PolicyID`/`PolicyVersion` reference in `server/`,
every SQL statement touching the four affected tables, current DDL, and every `GateEffectDefer`
call site). The code-map pass found the real surface area is larger than the ADR's own
"Component" line: `kb.pipeline_policies.id` provenance also flows through the D2 routing-clearance
subsystem (`routing_clearance.go`, `routing_enforcement.go`,
`pipeline_routing_clearances_handler.go`), `policy_promotion.go` (ontology-module-release
promotion mints new policy versions), `control.go` (plan-fact resolution, alarm/audit emission,
routing finalization), `pipeline_selection.go` (binding-resolution trace), `ontology/policyaudit/
store.go`, and the binding/rule CRUD handlers (`pipeline_bindings_handler.go`,
`pipeline_rules_handler.go`). This design treats that full surface as in-scope, not just the 8
files the ADR names — DR3's "drop `policy_id` columns" and "replace `ActivePolicyID`/
`ActivePolicyVersion` provenance" decisions are meaningless if applied to only half their call
sites.

Current state, confirmed by reading the migration chain in order (`20260731000004` through
`20260809000001`):
- `kb.pipelines` has no `version`/`status` columns; `UNIQUE (name)` only.
- `kb.pipeline_bindings` has **no store/context uniqueness constraint at all** today — the original
  `UNIQUE (ks_store_id)` was widened to `UNIQUE (ks_store_id, policy_id)` by `20260731000012`, then
  `20260801000015` dropped that index and never recreated any replacement. This is a live gap this
  change closes as a side effect of DR3, not a regression it introduces.
- `kb.pipeline_rules` has no `depends_on_processors` column.
- `kb.processor_registry` does not exist (confirmed via repo-wide grep, zero matches).
- `resolveIndeterminateGate` (`pipeline_gates.go:172-190`) is the only place that currently falls
  open to `enable`/`skip` on an indeterminate gate; no scheduler anywhere reads `DeferFingerprint`
  or `DeferredPaths` today (repo-wide grep confirms — the "retry later" mechanism the ADR
  describes as the historical reason for `defer` does not actually exist in the current code; the
  field is computed and shadowed/persisted but dead).
- `productionProcessorSpecs` (`processor_plan.go:421-486`) already has `Requires`/`Produces`
  fields per processor; its own doc comment (414-420) says nothing reads them yet.
- `DeletePipeline` (`pipelines_handler.go:407-434`) is a raw `DELETE FROM kb.pipelines WHERE id =
  $1` with no pre-check — it would hit the existing `ON DELETE RESTRICT` FK from
  `kb.pipeline_bindings`/`kb.pipeline_rules` today, surfaced only as a generic 500.

## Goals / Non-Goals

**Goals:**
- Implement DR1–DR9 of ADR `2026081001` exactly as decided (this ADR is a closed decision, not
  something this change re-litigates).
- Propagate the policy→pipeline provenance change through every real call site found in the code
  map, not just the ADR's example list.
- Resolve the `miner` migration-risk data decision the ADR explicitly left open (§3.3): discard
  the 3 archived policy rows rather than building a separate audit/history table — the ADR frames
  this as "not resolved by this ADR" but a decision is required to write a working migration, and
  a plain-audit-table is unjustified complexity for 3 rows with no consumer reading them today
  (confirmed: nothing in the code map queries archived `kb.pipeline_policies` rows for their
  content, only for `status = 'active'`).

**Non-Goals (explicit, per ADR §4):**
- The topological-sort execution engine that consumes `depends_on_processors` — `ExecutionOrder()`
  stays a fixed phase-bucket concatenation. This change only makes the DAG data real and validated;
  wiring the runtime loop to walk it is follow-on work.
- A frontend for authoring conditional gate predicates (DR5) — no schema change needed there;
  every seeded predicate stays vacuously-true.
- Migrating any *existing* processor in `productionProcessorSpecs` into `kb.processor_registry`
  (DR7) — the Go literal stays untouched; only new processors, if any are added later, would use
  the table.

## Decisions

### D1 — `kb.pipelines`: version/status columns, atomic authoring (ADR DR1, DR2)

`ALTER TABLE kb.pipelines ADD COLUMN version INT NOT NULL DEFAULT 1, ADD COLUMN status VARCHAR(16)
NOT NULL DEFAULT 'active' CHECK (status IN ('active','superseded'))`; drop `UNIQUE (name)`, add
`UNIQUE (name, version)`. `CreatePipeline`/`UpdatePipeline` (`pipelines_handler.go`) collapse into
one "author a new version" path: `INSERT` a new row with `version = COALESCE((SELECT MAX(version)
FROM kb.pipelines WHERE name=$1), 0) + 1`, then in the same transaction mark the prior current
version (`status='active'` for that `name`) as `superseded`, then insert every `kb.pipeline_rules`
row (gates + DAG edges) for the new version, then run DR8 validation, then commit. `display_name`/
`description` stay mutable in place (cosmetic, no new version) — a separate small `UPDATE` path,
unchanged from today. `DeletePipeline` is removed outright (not soft-deprecated): a version that's
immutable and possibly still bound can't be deleted; superseding via a new version is the only
"remove" operation.

Alternative considered: keep `UPDATE`-in-place for `processors[]` and bolt on an audit-log table.
Rejected — this is exactly the "no audit trail" problem `2026081001-bug` identified; a bolt-on log
can drift from the live row, whereas immutable rows can't.

### D2 — retire `kb.pipeline_policies`, propagate provenance everywhere it's actually read (ADR DR3)

`DROP TABLE kb.pipeline_policies` after the backfill (D6 below). Drop `policy_id` from
`kb.pipeline_bindings`/`kb.pipeline_rules`. Every `WHERE ... policy_id = (SELECT id FROM
kb.pipeline_policies WHERE status='active' LIMIT 1)` (4 call sites: `pipeline_bindings.go:125`,
`pipeline_gates.go:253`, `pipeline_rules_store.go:35`, `extract-doc-metadata-store.go:68`) becomes
`WHERE active`.

New partial unique index on `kb.pipeline_bindings`:
```sql
CREATE UNIQUE INDEX idx_kb_pipeline_bindings_one_active_default_per_context
    ON kb.pipeline_bindings (
        COALESCE(ks_store_id, -1), COALESCE(user_id, ''),
        COALESCE(tenant_id, ''), COALESCE(input_record_id, -1)
    )
    WHERE active AND binding_kind = 'store_default';
```

Provenance replacement — every current `PolicyID`/`PolicyVersion` field becomes a
`BindingID`/`PipelineName`/`PipelineVersion` triple (naming the actual resolved pipeline, strictly
more precise than an opaque system-wide counter, per ADR §3.3):
- `ProductionPlanFacts.ActivePolicyID`/`ActivePolicyVersion` (`processor_plan.go:78-79`) →
  `ActiveBindingID int64`, `ActivePipelineName string`, `ActivePipelineVersion int`.
- `P5RoutingSnapshot.PolicyID`/`PolicyVersion` (`processor_plan.go:95-96`) → same fields, same
  JSON-persisted struct — this is a schema change to a JSON column (`kb.doc_process_plans`'s
  routing-snapshot payload); old persisted rows keep their old shape (read-compat handled by
  leaving old field names as deprecated `json:"-,omitempty"`-free tolerant decode — see D7).
- `control.go`'s `resolveProductionPlanFacts` (1640-1682): `s.PolicyStore.GetActivePolicy(ctx)` is
  replaced by resolving the winning `kb.pipeline_bindings` row directly (already loaded by
  `ResolveProductionPipelineBinding`) and reading its pipeline's `name`+`version`;
  `sql.ErrNoRows`-tolerant legacy-fallback semantics stay (no active binding is not a hard error).
- `emitAlarmAuditEvent`/`recordRoutingDecisionEvents`/`finalizeRoutingPlan` (`control.go:775, 915,
  2119-2120, 2172-2183, 2196`) and `policyaudit.Event` (`ontology/policyaudit/store.go:44-45`):
  `PolicyID`/`PolicyVersion` columns/fields → `BindingID`/`PipelineName`/`PipelineVersion`.
- `pipeline_selection.go`'s `ProductionPipelineBindingResolution.PolicyID`/`PolicyVersion`
  (31-36, stamped at every return path 113-163) → same triple.
- D2 routing-clearance subsystem: `RoutingClearanceSubject.PolicyID`/`PolicyVersion`
  (`routing_clearance.go:21-22`, SQL at 67/85/162/170/172/207/213/218-219) and
  `RoutingEnforcementRequest.PolicyID`/`PolicyVersion` (`routing_enforcement.go:28-29`, consumed by
  `ConditionalBindingSubjectChecksum`/`ProcessorGateSubjectChecksum` at 111/113/178/180/239) both
  re-key on `BindingID`/`PipelineName`/`PipelineVersion`. This is a real schema change to
  `kb.pipeline_routing_clearances` (has `policy_id`/`policy_version` columns per the migration
  read) — those columns get replaced, not just the Go structs. `pipeline_routing_clearances_handler.go`'s
  approval-request payload changes shape to match (**BREAKING** for any existing clearance-approval
  API client, but this system is pre-production per the ADR's staging-server context).
- `policy_promotion.go` (mints a new `kb.pipeline_policies` row when an ontology module release is
  promoted, `69-113`) is rewritten to mint a new `kb.pipelines` *version* instead — promotion
  becomes "author a new pipeline version whose processor/rule set includes the newly-approved
  module's bindings," going through the same D1 atomic-authoring path.
- CRUD handlers `pipeline_bindings_handler.go`/`pipeline_rules_handler.go`: drop the `PolicyID
  *int64 json:"policy_id"` field from create/update payloads (`policy_id` no longer exists to
  assign); `active` alone now governs the per-binding "is this live" question DR3 established.

### D3 — `kb.pipeline_rules.depends_on_processors` (ADR DR6)

`ALTER TABLE kb.pipeline_rules ADD COLUMN depends_on_processors TEXT[] NOT NULL DEFAULT '{}'`.
Populated only at version-authoring time (D1's atomic transaction), read by DR8's validator.
Not consumed by `ExecutionOrder()` — explicit non-goal.

### D4 — `kb.processor_registry`, incremental adoption (ADR DR7)

Table exactly as specified in the ADR (§3.7). A small reader helper,
`ResolveProcessorSpec(name string) (ProcessorSpec, bool)`, checks `productionProcessorSpecs`
first (existing behavior, zero risk to existing processors), falls back to `kb.processor_registry`
only for names not found there. DR8's validator and any future planner call this helper instead of
indexing `productionProcessorSpecs` directly.

### D5 — creation-time validation (ADR DR8), replaces `defer` (ADR DR9)

New `ValidatePipelineVersion(ctx, tx, processors []string, rules []PipelineRule) error` (or similar
— exact signature decided during implementation) runs inside D1's atomic transaction, after all
rows are inserted but before commit:
1. **Processor closure** — for every `Requires` fact of every selected processor (via D4's
   resolver), some other selected processor's `Produces` covers it, or it's covered by the
   baseline (`static_analyzer`/`chunking`).
2. **DAG well-formedness** — `depends_on_processors` edges reference only processors in the same
   version's set, and the edge set has no cycle (standard DFS cycle check).
3. **Gate-fact availability** — for every rule with a non-trivial `predicate`, every fact path
   `semrules.Analyze(predicate).RequiredPaths` references must be produced by an upstream
   processor reachable via `depends_on_processors`, or a baseline processor. Reuses the existing
   `semrules.Analyze` call already used by `policy_compile.go:106-129`'s D2-clearance analysis —
   same analyzer, new consumer.

All three failures reject the whole creation with a specific, named error (gate id + missing fact;
cycle path; missing producer). This replaces `policy_compile.go:113-115`'s existing (narrower)
"defer effect requires dependency facts" check — that check only verified a defer gate *had*
declared dependency facts, not that those facts were DAG-reachable; DR8 check 3 subsumes it.

`GateEffectDefer` becomes creation-time-rejected (`effect = 'defer'` in a submitted rule row is a
validation failure, not a storable value — `ck_pipeline_rules_effect`'s CHECK constraint drops
`'defer'` from its allowed set). At runtime, `resolveIndeterminateGate`'s fallback branch
(`pipeline_gates.go:178-181`, currently falls open to `enable`/`skip`) is replaced: any
indeterminate resolution becomes `&PipelineGateResolutionError{Reason:
"indeterminate_after_validated_pipeline"}` regardless of `OnConflict` mode — this is a safety-net
path expected never to trigger once all live pipeline versions pass DR8, so the two-mode
(`block`/`fallback`) distinction collapses to always-hard-fail for this specific case.

### D6 — `miner` backfill, resolving the ADR's open data question

The ADR (§3.3) explicitly leaves "discard the 3 archived policies vs. move to a history table" as
"a data decision, not an architecture one, not resolved by this ADR." Decision for this change:
**discard.** Rationale: nothing in the current code map reads archived `kb.pipeline_policies` rows
for their content (only `status='active'` is ever queried); the 3 archived rows carry no bindings
or rules of their own that aren't already superseded by the active one; and DR1's new
`kb.pipelines.version`/`status` history is the actual replacement audit trail going forward — a
separate table preserving 3 rows of a retired concept adds a permanent migration-compat burden for
zero query value. Migration steps, in order, inside one transaction:
1. Identify the active policy's `id`.
2. `UPDATE kb.pipeline_bindings SET policy_id = NULL WHERE policy_id = <active id>` (immediately
   before the `DROP COLUMN policy_id`, so this is really just "stop referencing it") — `active`
   column is untouched, already correct per-row.
3. Same for `kb.pipeline_rules`.
4. Any bindings/rules pointing at a *non-active* (archived) policy are deleted outright (the ADR's
   "discard" framing extends to their rows, not just the policy row — confirmed there are none in
   `miner` today per the ADR's own count: only the active policy's 2 bindings/rules exist with a
   real `policy_id`).
5. Add the new partial unique index (D2) — must happen after step 2-4 clean the data, since a
   pre-existing duplicate-context binding would violate it.
6. `DROP TABLE kb.pipeline_policies` (cascades to `kb.pipeline_routing_clearances` etc. only if
   those still FK to it after D2's clearance-subsystem rewrite lands first — sequencing matters,
   see tasks.md).

### D7 — JSON-column compatibility for `P5RoutingSnapshot`

`P5RoutingSnapshot` is persisted as JSON inside `kb.doc_process_plans`. Old rows have
`policy_id`/`policy_version` keys; new rows will have `binding_id`/`pipeline_name`/
`pipeline_version`. No migration of historical JSON blobs — they're read-only audit history, not
re-validated (matches ADR DR8's "nothing is re-checked at runtime" spirit, applied here to
historical data too). Any endpoint that reads old snapshots and expects the new field names should
tolerate absence (`omitempty`, zero-value fallback), not error.

## Risks / Trade-offs

- **[Risk] Scope creep beyond the ADR's own file list could stall implementation.** The D2
  routing-clearance/enforcement rewrite is real production-shaped code (checksums, conflict
  detection) the ADR doesn't mention in its Consequences section beyond a generic "anything
  downstream needs the equivalent update." → **Mitigation:** tasks.md sequences D2's clearance
  changes as their own explicit phase with its own tests, so this doesn't get silently dropped or
  half-done.
- **[Risk] `DROP TABLE kb.pipeline_policies` is irreversible once run.** → **Mitigation:** staging
  server (per workspace CLAUDE.md, destructive ops are acceptable here), and the Down migration
  recreates the table shape (not the discarded data) so schema rollback is still possible even
  though the 3 archived rows are gone for good the moment Up runs.
- **[Risk] Collapsing `resolveIndeterminateGate`'s fallback to always-hard-fail could break a
  pipeline version that validates today but has a predicate `semrules.Analyze` can't fully
  resolve statically** (e.g., a predicate whose required paths are only knowable after DR8's
  analyzer runs against a different processor set than expected). → **Mitigation:** DR8 check 3
  uses the same `semrules.Analyze` already trusted for D2-clearance analysis; if there's a real
  static-analysis gap it will surface at creation time (loud, before any document runs), not
  silently at runtime — which is strictly better than today's silent fallback either way.
- **[Trade-off] `depends_on_processors` and `kb.processor_registry` are added but not yet consumed
  by an execution engine (explicit non-goal).** The DAG is provably correct but doesn't yet change
  runtime behavior — `ExecutionOrder()` stays phase-bucket-based. This is the ADR's own scoping
  choice, not a gap this change introduces.

## Migration Plan

1. Ship schema migrations (D1, D2's index + policy_id drops deferred until after D6 backfill, D3,
   D4) — see tasks.md for exact goose file ordering; `kb.pipeline_policies` DROP is the *last*
   schema step, after all Go code stops reading/writing it.
2. Ship Go changes in dependency order: D4 (processor registry reader) → D5 (validator, can be
   built and tested against the *existing* schema first since it only reads) → D1 (atomic
   authoring + version/status) → D3 (DAG edges wired into D5) → D2 (provenance propagation +
   policy retirement, done last since it's the widest blast radius and depends on D1's
   `name+version` existing to redirect provenance to) → D9/gate-defer removal.
3. Run D6's backfill against `miner` inside the same transaction as the `policy_id` column drops.
4. Update `doc-processing-policy-seed` (`policy_seed.go`, `main.go`) to author pipeline versions
   directly instead of minting policy rows — this tool is the one caller that both reads and
   writes the retired concept end-to-end, so it needs its own task-list entry, not just a
   find-and-replace.
5. No rollback of `miner`'s discarded archived-policy data (D6) — schema-level Down migrations
   restore table shape, not discarded rows, consistent with this being a staging environment.

## Open Questions

None outstanding — the ADR's one explicitly-deferred question (archived-policy data handling) is
resolved by D6 above. Exact Go signatures (validator function names, helper placement) are left to
implementation and noted as "decided during implementation" in D5, since they don't affect the
architecture.
