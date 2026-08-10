## 1. Schema — additive migrations (safe to ship first, no data loss risk)

- [x] 1.1 `project_migrations/20260810000001_add_version_status_to_kb_pipelines.sql`: added
      `version`/`status`, dropped the bare `pipelines_name_key` unique constraint (verified the
      real live constraint name via `\d kb.pipelines` rather than guessing), added
      `UNIQUE (name, version)`.
- [x] 1.2 `project_migrations/20260810000002_add_depends_on_processors_to_kb_pipeline_rules.sql`.
- [x] 1.3 `project_migrations/20260810000003_create_kb_processor_registry.sql`. Confirmed no
      `kb.*` table in this codebase is separately registered via `database.CreateTables(...)` —
      grepped for `kb.pipelines` near "CreateTable" and found zero matches; these tables are
      goose-migration-only, no companion Go registration needed.
- [x] 1.4 Applied via `mise dev`/air's auto-restart-on-`.go`-save (migrations under
      `project_migrations/` aren't in air's watched extensions, but every subsequent Go edit in
      this change triggered a rebuild that ran them). Confirmed via
      `SELECT version_id, is_applied FROM project_db_migration WHERE version_id LIKE '20260810%'`
      at the end of the session: all 8 of this change's migrations (000001-000008) show
      `is_applied = t`.

## 2. Processor dependency metadata resolver (ADR DR7 / design D4)

- [x] 2.1 Reused the existing seam-1 registry (`registries.go`'s `RegisterProcessor`/
      `LookupProcessor`, already seeded from `productionProcessorSpecs` at init) instead of a new
      parallel resolver. New `processor_dependency_registry.go`: `ProcessorRegistryStore`/
      `ProcessorRegistrySQLStore` read `kb.processor_registry`; `LoadProcessorRegistry` registers
      only names `LookupProcessor` doesn't already know, so hardcoded specs always win (DR7's
      union). Leaves `productionProcessorSpecs` (421-486) completely untouched.
- [x] 2.2 `processor_dependency_registry_test.go`: SQL store reads rows (sqlmock); unknown
      processor resolves from the registry table; a registry row for an already-hardcoded name
      does NOT override it; empty table is not an error; nil store/store error are errors.

## 3. Creation-time validation engine (ADR DR8, DR9 / design D5)

- [x] 3.1 `server/api/doc-processing/pipeline_version_validate.go`: `PipelineVersionDraft{Processors
      []string, Rules []PipelineGate}` + `ValidatePipelineVersion`. Added `PipelineGate.
      DependsOnProcessors []string` (pipeline_gates.go) as the Go home for DR6 edges (one row per
      target processor already carries predicate/effect, so DAG edges live on the same row/type).
      Three checks: (a) processor closure via `LookupProcessor` (seam 1, reused from §2), baseline
      = `static_analyzer`/`chunking`; (b) DAG well-formedness (cycle DFS + no dangling reference);
      (c) gate-fact availability via `semrules.Analyze` — **design decision made during
      implementation, not fully specified by the ADR:** fact paths are split into
      `baselineRoutingFactPaths` (the four `document.*` facts populated straight from the input
      record before any processor runs — always available) and everything else, which requires an
      upstream (DAG-reachable-or-baseline) processor whose `Produces` includes the coarse
      `"facets"` artifact kind (facet_tier1/facet_tier2/classify_document today) — the same
      coarse Requires/Produces vocabulary check (a) already uses, applied to document facts
      instead of processor artifact kinds. Flagged for the user's review; no finer-grained
      fact-path-to-producer governance table exists in the codebase to check against instead.
- [x] 3.2 Each check returns a specific, named error (processor+artifact kind; cycle path;
      gate+missing fact).
- [x] 3.3 `effect = 'defer'` is rejected inside `validateGateFactAvailability`, named per gate.
- [x] 3.4 `pipeline_version_validate_test.go`: all 9 scenarios from
      `specs/pipeline-dependency-graph/spec.md` and `specs/gate-defer-retirement/spec.md` pass.
      Full `go test ./server/api/doc-processing/...` run confirms no regressions (pre-existing
      unrelated failures on the clean tree, verified via `git stash`, e.g.
      `TestDeriveLineOverlapConnections_*`/`TestBuildEntityNameGraphConnections` — not touched by
      this change).

## 4. Atomic pipeline version authoring (ADR DR1, DR2 / design D1)

- [x] 4.1 **Re-scoped during implementation:** did NOT rework `policy_compile.go`'s whole-system
      `CompilePolicy` here — that function (and its D2-clearance subject checksums) is still
      needed by the still-live `kb.pipeline_policies` flow until §9 retires it, so touching it now
      would have coupled this group to §7/§9's rekey. Instead, `CreatePipeline` validates directly
      via §3's `ValidatePipelineVersion` (no whole-system compile step). `policy_compile.go`
      itself is left untouched here; it gets folded into or removed alongside §9.
- [x] 4.2 Rewrote `kbhandler/pipelines_handler.go`'s `CreatePipeline` into one atomic transaction:
      locks the current version row (`SELECT id, version ... FOR UPDATE`), computes
      `nextVersion`, runs `ValidatePipelineVersion` on the proposed `(processors, rules)` *before*
      opening the transaction (fail fast, zero DB writes on rejection), inserts the new
      `kb.pipelines` row (`version`, `status='active'`), supersedes the prior current version if
      one existed, inserts one `kb.pipeline_rules` row per rule (gate +
      `depends_on_processors`), commits. New request field `rules: []{name, priority,
      target_processor, effect, predicate, depends_on_processors}`. Added `version`/`status` to
      `pipelineRecord` and `pipelineListColumns`. `UpdatePipeline`'s `processors` case now
      rejects with 400 (author a new version instead) rather than mutating in place;
      `legacy_equivalent`/`display_name`/`description`/`is_system_default`/`name` remain
      in-place-editable (unrelated to DR1's processors[]-immutability concern).
      **Transitional note:** `kb.pipeline_rules.policy_id` is still `NOT NULL` in the live schema
      as of Group 1; migration `20260810000004` relaxed it to nullable so this INSERT (which
      never sets `policy_id`) works today — the column itself is dropped in §9 once nothing else
      writes it.
- [x] 4.3 Removed `DeletePipeline` entirely (function + `DELETE /kb/pipelines/:id` route in
      `routes.go`) — not disabled, deleted.
- [x] 4.4 Rewrote `kbhandler/pipelines_handler_test.go`: new-version insert (`TestCreatePipelineSuccess`),
      DAG-edge rule insert (`TestCreatePipelineWritesRulesWithDAGEdges`), version-2
      insert-plus-supersede (`TestCreatePipelineSecondVersionSupersedesPrior`), validation failure
      returns 400 with zero DB expectations set (`TestCreatePipelineRejectsFailedClosureValidationBeforeTouchingDB`),
      `processors` edit on `UpdatePipeline` now 400s (`TestUpdatePipelineRejectsProcessorsEdit`),
      `TestDeletePipelineNotFound` removed (endpoint gone). Full `go test ./server/api/kbhandler/...`
      confirms no regressions vs. the pre-existing baseline (verified via `git stash`).

## 5. Gate-defer retirement (ADR DR9 / design D5)

- [x] 5.1 `project_migrations/20260810000005_drop_defer_from_pipeline_rules_effect_check.sql`:
      replaced `ck_pipeline_rules_effect` with `CHECK (effect IN ('require','enable','skip'))`.
      Confirmed live `miner` data first: zero rows with `effect='defer'` (matches the ADR's
      claim that every seeded rule uses `effect='require'`).
- [x] 5.2 `pipeline_gates.go`: removed the `GateEffectDefer`-wins branch, `deferFingerprint`
      helper, `DeferredPaths`/`DeferFingerprint` fields on `ProcessorGateResolution`, `WouldDefer`
      on `ProcessorGateShadowPlan` (+ its population branch), and `GateEffectDefer`'s rank in
      `gateEffectRank` (falls to the same 0 as any invalid string now). `resolveIndeterminateGate`
      no longer branches on `OnConflict` at all — **every** call now returns
      `&PipelineGateResolutionError{Reason: "indeterminate_after_validated_pipeline"}`, in both
      `block` and `fallback` modes (previously only `block` hard-failed).
- [x] 5.3 `routing_enforcement.go:151`'s suppression check drops the `GateEffectDefer` half.
- [x] 5.4 Rewrote `pipeline_gates_test.go`'s defer-specific tests (renamed
      `TestResolveProcessorGateIndeterminateAlwaysHardFailsEvenInFallbackMode`, updated the
      true-vs-indeterminate rank table, removed `WouldDefer` assertions) and
      `p5_exit_test.go:54`'s name reference (this file's own `TestP5AcceptanceCriteriaCoverage`
      verifies every listed name resolves to a real test, so a stale reference would have failed
      loudly). **Consequence found and fixed, not just the isolated unit tests:**
      `BuildProductionProcessorPlanFromFacts` hardcodes fallback mode for gates today
      (`runtime.go`), so this change is live-behavior-affecting, not just internal —
      `TestHandleEvent_FallbackModeGateConflictStillExecutes` (renamed to
      `..._NowBlocksProcessing`) and
      `TestBuildProductionProcessorPlanFromFactsGateConflictRespectsOnConflictEnv` (renamed to
      `..._AlwaysErrors`) both asserted the *old* fall-open behavior and needed inverting, not
      just recompiling. Traced the block/alarm path (`control.go:763-794`,
      `AlarmForPlanError`/`IsDecisionRelevantPlanConflict` in `routing_alarm.go`) and confirmed
      it already classifies `*PipelineGateResolutionError` as `RoutingAlarmKindGateConflict` and
      blocks before any processor runs — no new alarm plumbing was needed, the existing
      block-mode path now simply also covers fallback mode.
      Full `go test ./server/api/doc-processing/...` matches the pre-existing baseline exactly
      (verified via `git stash`) — zero new failures.

## 6-9. Provenance rename, D2 clearance rekey, CRUD handlers, retire `kb.pipeline_policies` (executed as one consolidated sweep, not four separable phases)

**Re-scoped during implementation:** these four groups were planned as sequential phases, but
tracing the actual call graph (§6's `control.go` alarm/audit sites already reach into §7's D2
clearance subject construction; §8's CRUD handlers already reach into §9's `WHERE policy_id=...`
call sites) showed they share edit points that can't be landed independently without an
intermediate broken-compile state. Executed together, file by file, then verified as a whole.

- [x] **Provenance rename** (`processor_plan.go`, `pipeline_selection.go`, `control.go`,
      `ontology/policyaudit/store.go`): `ProductionPlanFacts.ActivePolicyID/ActivePolicyVersion/
      ActivePolicyChecksum` removed outright (not renamed) — design D2's original
      `ActiveBindingID`/`ActivePipelineName`/`ActivePipelineVersion` triple turned out redundant
      with data already available at every call site via `resolution.Spec`/`plan.PipelineSpec()`,
      so nothing needed to persist on `ProductionPlanFacts` itself. `P5RoutingSnapshot.PolicyID/
      PolicyVersion/PolicyChecksum` → `PipelineName string`/`PipelineVersion int` (`PolicyChecksum`
      dropped, not renamed — redundant with the pre-existing `SelectedPipelineChecksum`).
      `ProductionPipelineBindingResolution.PolicyID/PolicyVersion` → single
      `SelectedPipelineVersion int` (name already covered by the pre-existing `SelectedPipeline`
      field). `policyaudit.Event.PolicyID/PolicyVersion` → `PipelineName string`/`PipelineVersion
      int`; `kb.pipeline_policy_events`' backing columns renamed via migration `20260810000006`.
      **Bigger finding:** `control.go`'s `resolveProductionPlanFacts` had a whole "active-policy
      pointer that can fail to load" hard-fail path (spec 2026080102 §11, `PolicyLoadError` type,
      ~30 lines in `handleEvent`) that DR3 makes categorically unreachable — bindings load from an
      in-process cache now, not a per-event policy-store query — so this was deleted outright, not
      adapted, along with its 4 dedicated tests in `control_test.go` (the failure mode they tested
      no longer exists). `ControlService.PolicyStore` field and its `runtime.go` wiring removed;
      `pipeline_registry_store.go`/`policy_compile.go`'s `loadDefinition` gained a
      `WHERE status = 'active'` filter they were missing (a real latent bug DR1 exposed: without
      it, the runtime pipeline registry would load every historical version of every pipeline once
      any pipeline had more than one version).
- [x] **D2 routing-clearance/enforcement rekey** (`routing_clearance.go`, `routing_enforcement.go`,
      `pipeline_routing_clearances_handler.go`): `RoutingClearanceSubject`/`RoutingEnforcementRequest`
      `.PolicyID/.PolicyVersion` → `.PipelineName/.PipelineVersion`. Two new migrations (live DDL
      read via `psql \d`, not guessed): `20260810000007` rekeys `kb.pipeline_routing_clearances`+
      `kb.pipeline_routing_clearance_coverage` (both confirmed empty, 0 rows — pure structural
      change); `kb.pipeline_policy_events` handled in the migration above. Left
      `RoutingClearanceApproval.PolicyChecksum`/`routingClearanceRequest.policy_checksum` (the
      benchmark-evidence integrity checksum, a distinct concept from policy *identity*) untouched —
      narrower blast radius, matches the ADR's own scope.
- [x] **CRUD handlers drop `policy_id`** (`pipeline_bindings_handler.go`, `pipeline_rules_handler.go`):
      field removed from payloads; the "active policy cannot be edited" guard (queried
      `kb.pipeline_policies` via a JOIN) removed entirely — no replacement guard, since DR3 makes
      per-row `active` the only "is this live" signal and there's no more system-wide "active
      policy" to protect. **Added, not in the original plan:** every mutating binding/rule/pipeline
      endpoint now calls a new best-effort `reloadAfterPipelineWrite` (new
      `kbhandler/pipeline_reload.go`) so a running process picks up the change immediately — under
      the old model this only happened once, at policy activation; DR3 removes that one choke
      point, so each endpoint now plays that role directly (same alarm-on-reload-failure pattern
      the retired `ActivatePipelinePolicy` used).
- [x] **Retire `kb.pipeline_policies`** (ADR DR3 / design D6): re-verified live `miner` counts
      before writing the migration (numbers had drifted from the ADR's snapshot — 6 bindings/14
      rules, not 2/2 — but all non-active-policy rows were confirmed unreachable dead data
      regardless, matching D6's reasoning). `project_migrations/20260810000008_retire_kb_pipeline_policies.sql`:
      deletes bindings/rules pointing at an archived policy, drops `policy_id` + its FKs/indexes,
      adds `active`-only replacement indexes and the new
      `idx_kb_pipeline_bindings_one_active_default_per_context` partial unique index, drops the
      table. Deleted `pipeline_policies_handler.go`(+test), `pipeline_policies_store.go`(+test),
      the `/kb/pipeline-policies*` routes, and — found to be genuinely dead code once its only
      callers were gone — `policy_compile.go`'s `PolicyCompilerSQLStore`/`PolicyCompiler`/
      `loadDefinition`/`loadBindings`/`loadGates` (the pure `CompilePolicy(PolicyDefinition)`
      function, its types, and the two D2 subject-checksum functions stayed — still used).
      `policy_promotion.go`'s `EnsureDraftFromModuleRelease` reworked: it used to mint a
      `kb.pipeline_policies` draft row grouping bindings under a separately-activated envelope;
      now it inserts `active=false` conditional bindings directly (identified by name for
      idempotency) — `active=false` stands in for "draft" under the per-row model, and the
      existing binding-update endpoint is the new "activate" step. `policy_authorization_test.go`
      needed no changes (its checks were already generic authorization tests, not
      policy-object-specific).

## 10. Bootstrap tool: `doc-processing-policy-seed`

- [x] Rewrote `SeedDocProcessingPolicies`: authors a new `kb.pipelines` version per configured
      policy via a new `authorDocProcessingPipelineVersion` helper (same atomic lock/validate/
      insert/supersede/insert-rules pattern as `kbhandler.CreatePipeline`, duplicated rather than
      shared across the package boundary), then upserts (not replaces) the two configured binding
      kinds via a new `upsertDocProcessingBinding` helper. **Re-scoped, explicitly narrower than
      before:** the old "activation is a full replacement" semantic (deactivates everything not
      in this run) has no equivalent under the per-row `active` model — a binding authored outside
      this config is now left untouched by a reseed, not deactivated. Documented in the new doc
      comment and `main.go`'s usage notes as an intentional behavior change, not an oversight.
      `DocProcessingPolicySeedResult.PolicyID/PolicyVersion` → `PipelineVersions map[string]int`.
      `policy_seed_config.go` needed no changes (confirmed: pure TOML→struct parsing, no policy
      envelope concept). Stale root-level `doc-processing-policy-seed` binary removed (it was
      already gitignored, matching `.gitignore:52`; not a tracked-file concern).

## 11. Test sweep and full-repo verification

- [x] Every test file the plan named (`pipeline_bindings_test.go`, `pipeline_gates_test.go`,
      `pipeline_rules_store_test.go`, `extract_doc_metadata_store_test.go`,
      `routing_clearance_test.go`, `routing_enforcement_test.go`,
      `pipeline_routing_clearances_handler_test.go`, `policy_promotion_test.go`,
      `pipeline_registry_store_test.go`, `policy_seed_test.go`) plus several the plan didn't
      anticipate (`control_test.go`'s deleted `PolicyLoadError` tests,
      `pipeline_bindings_handler_test.go`/`pipeline_rules_handler_test.go`'s full SQL-mock
      rewrites, `p5_exit_test.go`'s acceptance-criteria manifest — criterion 13's proof pointers
      renamed to a new `TestCreatePipelineMidTransactionFailureRollsBackSupersede`,
      `doc_process_plan_store_test.go`'s golden-JSON literal, `policy_compile_test.go`'s dead
      SQL-store test) were updated. `p5_migration_contract_test.go` needed no changes (checked --
      no policy-id-shaped assertions in it). Full-repo grep for `PolicyID`/`PolicyVersion`/
      `GateEffectDefer`/`DeferredPaths`/`DeferFingerprint`/`pipeline_policies` after all of the
      above: zero remaining matches outside (a) historical-compat comments, (b) the intentionally-
      kept pure `CompilePolicy`/checksum functions, (c) `GateEffectDefer`'s intentionally-kept
      creation-time-rejection sentinel use, (d) `semrules/types.go`'s unrelated `PolicyID` (a
      different type, confirmed by the ADR itself).

## 12. Verification and docs

- [x] 12.1 `go build ./...`, `go vet ./...` clean workspace-wide. `go test ./...`: 28 pre-existing
      failures (14 in `doc-processing`, 14 in `kbhandler`, all in files this change never touched —
      confirmed via `git status --short` showing no diff in those files, and via `git stash`
      diffing earlier in the session) plus 2 more pre-existing failures found in packages outside
      doc-processing/kbhandler (`llmusage`, `qudt-import` — also unrelated, confirmed the same
      way). Zero new failures anywhere.
- [x] 12.2 `shared/go` untouched; no `go work sync` needed.
- [x] 12.3 Live-DB smoke check via `psql` instead of hitting HTTP endpoints directly (no running
      auth session available in this session): confirmed `kb.pipeline_policies` is gone from
      `\dt kb.pipeline*`, `kb.pipelines` rows carry correct `version`/`status`, the new partial
      unique index and `depends_on_processors`/`kb.processor_registry` all exist as designed, and
      the live `mise dev`/air-managed server process rebuilt and stayed up through every change in
      this session (verified process start time against last-edited file mtimes). Unit/integration
      tests (§3, §4, §11) cover the three DR8 checks and the binding-conflict constraint at the
      logic level.
- [x] 12.4 ADR `2026081001` status updated to "Accepted — implemented," Component line expanded to
      the real file list, and a change-log entry added documenting the wider-than-scoped surface
      area and the DR8-check-3 fact→producer design decision made during implementation.
- [x] 12.5 Superseding note added to `2026-08-08-doc-processing-policy-design.md`.
- [x] 12.6 Knowledge that changed: this tasks.md (now the authoritative record of what actually
      shipped vs. what was planned) and design.md's decisions. Docs updated: the ADR itself (12.4)
      and the superseded bootstrap spec (12.5). Intentionally left undocumented / open for
      follow-up review: the DR8-check-3 fact→producer mapping (baseline facts vs. coarse `"facets"`
      artifact kind — flagged in §3.1 and in the ADR change log, not elsewhere formalized); the
      exact `authorDocProcessingPipelineVersion`/`upsertDocProcessingBinding` helper duplication
      between `policy_seed.go` and `kbhandler/pipelines_handler.go` (same atomic-authoring pattern
      implemented twice rather than extracted to a shared package function — a reasonable follow-up
      refactor, not done here to keep this change's diff scoped to behavior, not restructuring).
