## Context

Source of truth for this change is ADR `2026081401`
(`KnowledgeStore/doc-repo/adrs/202608/2026081401-adr-governed-metric-vocabulary-and-phase-d-failure-reporting.md`),
written the same day from a live investigation, and re-verified against current code while
writing this design doc (all cited line numbers/behaviors below were re-checked, not just quoted
from the ADR):

- `CanonicalMetricValueRangeType` (`metric_normalizer.go:222-253`) is a hardcoded `switch` over
  synonym groups for four buckets (`lower_bound`/`upper_bound`/`exact`/`range`, plus special forms
  `qualitative`/`limit_absent`); its `default` case returns the normalized-but-unrecognized string
  unchanged, and `resolveMetricValue`'s trailing `switch` (`metric_normalizer.go:275-342`) turns
  any bucket it doesn't recognize into the terminal `ValueForm: "unparsed", AssertionKind:
  "unparsed"` (confirmed at `metric_normalizer.go:341-342`). Production survey: 309 distinct
  `value_range_type` strings across 7,036 `kb.metrics` rows.
- `AssociateSemantics.processMetric` (`associate_semantics.go:300-`) hits `!supported` at
  `metricAssertionKindTermID(p.AssertionKind)` and defers via `deferCandidate` →
  `deferCandidateWithDependency` (`associate_semantics.go:529-535`), whose `dependencyFingerprint`
  defaults to the reason string itself — confirmed this makes the deferral fingerprint-stable
  across reprocessing (the existing backlog drain re-tries a deferred candidate only when its
  fingerprint's underlying condition changes; a static reason string never changes on its own).
- `AssociateSemantics.Run` (`associate_semantics.go:40-99`) returns `(AssociateReport{Examined,
  Accepted, Deferred, Rejected}, error)`; the `error` return today is reserved for genuine
  processing errors (a candidate `process` call failing), never for "N candidates deferred" —
  confirmed by reading the full loop, which only ever does `return report, fmt.Errorf(...)` on an
  unexpected per-candidate error.
- `AssociateSemanticsProcessor.HandleEvent` (`phase_d.go:73`, one-liner `return nil`) runs through
  Phase A/B's `runSingleProcessorCollect` (`control.go:1154-1203`), which unconditionally persists
  `"success"` via `persistProcessorRuntimeStatus` at the end — confirmed this write happens purely
  because `HandleEvent` trivially succeeds, before `PostProcessIndex` (the real work) has run.
- `runPostProcessIndexing` (`control.go:1051-1134`) confirmed: on `PostProcessIndex` error
  (`control.go:1111-1120`), it records an OTel span and logs (`s.Logger.Error`), then `return`s
  from the goroutine — no call to `persistProcessorRuntimeStatus` anywhere in that branch. This is
  the shared harness for every `PostProcessIndexer`, not something `associate_semantics`-specific;
  the same gap exists uniformly for every Phase C processor registered via `PostProcessIndexer`.
- `NormalizeAssertionsProcessor`, `AssociateSemanticsProcessor`, `ProjectSemanticsProcessor`
  (`phase_d.go:48-153`) already use `PostProcessDependsOn` to sequence
  `normalize_assertions → associate_semantics → project_semantics`; the scheduler in
  `runPostProcessIndexing` (`control.go:1086-1098`) waits on named dependents' `done` channels only
  when the dependency was itself invoked this run — the exact mechanism DR6 needs for
  `extract_metrics → normalize_assertions` too.
- `MetricsProcessor.PostProcessIndex` (`extract-metrics.go:754-790`) today always returns `nil`
  except on a record/metrics-existence load failure; its main job is reindexing search/graph state
  for metrics already persisted by `HandleEvent`/Phase B — DR6 adds a new concern (vocabulary
  mapping check) to a function that currently has none.
- Migration precedent confirmed: `20260731000018_create_kb_ontology_candidates.sql`'s audit-column
  shape (`create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(), create_by TEXT, modify_time ... ,
  modify_by TEXT`) and `20260814000001_add_resolve_metric_entry_type.sql`'s
  drop-constraint/recreate-with-new-value pattern for `doc_proc_logs_entry_type_check` are the two
  patterns this change reuses directly.

## Goals / Non-Goals

**Goals:**
- Replace the closed, hardcoded `value_range_type` synonym switch with a governed, DB-backed,
  auto-discovering lookup — new vocabulary becomes a data fix (approve one row), not a code
  change and redeploy.
- Make a `value_range_type` mapping miss an actual, visible processor failure — at the earliest
  point the information exists (`extract_metrics`, DR6) and as a backstop wherever Phase D still
  independently runs (`associate_semantics`, DR3) — without inventing a second bespoke reporting
  path, by fixing the one shared harness gap (DR4) that currently swallows every Phase C failure.
- Preserve today's classification behavior exactly on cutover (DR5 seeding) so this ships as a
  pure visibility/governance improvement, not a one-time failure storm.
- Keep every extracted metric persisted exactly as today, mapped or not — flagging, never
  dropping.

**Non-Goals:**
- An admin UI for triaging `'proposed'` rows (ADR §7 — future work; operators query the table
  directly for now).
- Extending this governance pattern to `kb.metrics.value_class`'s equivalent free-text gap (ADR
  §7 — explicitly out of scope, needs its own investigation).
- Designing a precise DR6 bucket-guessing heuristic beyond "reuse `parseThresholdOrTarget`'s
  existing direction-keyword cues where cheap to apply" — it only ever produces a non-authoritative
  `'proposed'` suggestion, so getting it wrong has low cost (ADR §7).
- Building reconciliation/merge tooling for the mapping table — `raw_value` is already a natural
  dedup key (PK), so there is no duplicate-entity problem analogous to `kb.ontology_terms`.

## Decisions

### D1 (ADR DR1). Governed table + in-process cache replaces the Go switch

New table `kb.metric_value_range_type_map` (`raw_value` PK — normalized identically to
`CanonicalMetricValueRangeType`'s existing lowercase/trim/`-`,` `→`_` step, so the migration's
seed data and the runtime lookup key are guaranteed to agree), `canonical_bucket` nullable text,
`status` (`proposed`/`approved`/`ambiguous`, default `proposed`), `occurrence_count bigint default
0`, `first_seen_record_id`/`last_seen_record_id`, `note`, and the standard audit columns matching
`kb.ontology_candidates`'s convention.

A new `ValueRangeTypeMapper` type wraps this table behind a full-table, short-TTL,
invalidate-on-write in-process cache (per-call DB round trips are not acceptable — the table is
read on every metric row normalized). `CanonicalMetricValueRangeType`'s call sites switch to
`ValueRangeTypeMapper.Lookup(raw)`, which:
- returns `(canonicalBucket, status)` on a hit against `'approved'` or `'ambiguous'`;
- on a miss (`raw_value` not present at all), inserts a `'proposed'` row (`occurrence_count=1`)
  and returns `(bucket_guess_or_empty, "proposed")`;
- on a repeat hit against an existing `'proposed'`/`'ambiguous'` row, increments
  `occurrence_count`/updates `last_seen_record_id` (no duplicate insert — `raw_value` PK already
  prevents this by construction) and returns the current state unchanged.

**Alternative considered:** the two-table `candidates`/`terms` split ADR `2026081201` established
for governed ontology vocabulary. Rejected here (ADR §3.7) — that split manages versioned term
*identity and content*; a `value_range_type` synonym is a flat string→bucket fact with no
versioning need, so a single `status`-columned table carries the same auto-discover/queryable/
non-blocking philosophy with less machinery.

### D2 (ADR DR2/DR6-log). One `kb.doc_proc_logs` row per record per run, not per candidate

New `entry_type = 'assertion_mapping_miss'` (family-level name, not `metric_`-prefixed, since the
same free-text shape exists for `value_class` and will recur for provisions' modality vocabulary —
ADR §3.2). Migration follows `20260814000001`'s exact drop/recreate pattern for
`doc_proc_logs_entry_type_check`. A new `DocProcLogger.LogAssertionMappingMiss(ctx, rec, loc)`
helper mirrors `LogResolveMetric`'s existing shape (`doc_proc_log_store.go:214`), added to
`allowedDocProcLogEntryType` (`doc_proc_log_store.go:315`). Called once per run — from
`associate_semantics` after processing all candidates (DR3) and, separately, once from
`extract_metrics`'s `PostProcessIndex` (DR6) — each with its own `doc_proc_name`, both reusing
this one entry type. Rejected alternative: one row per deferred/flagged candidate — would spam the
log table for zero extra information beyond what `occurrence_count` already carries durably per
string (ADR §3.7).

### D3 (ADR DR3). `associate_semantics` fails only on `"proposed"`, never on `"ambiguous"`/`"absent"`

`resolveMetricValue`'s caller in `normalize_assertions` tags `proposed_payload` with
`value_range_type_lookup: "approved"|"ambiguous"|"proposed"|"absent"` (`"absent"` = legacy
empty/NULL `value_range_type`, not a mapping problem). `AssociateSemantics.processMetric`'s
existing `!supported` branch (`associate_semantics.go:316`) reads this tag: `"proposed"`
increments a new `AssociateReport.MappingMisses` field (the candidate still ends up `deferred` via
the existing path — unchanged); any other tag value leaves today's behavior untouched. After the
existing per-candidate loop in `Run` completes with no processing error, a new check —
`if report.MappingMisses > 0 { return report, fmt.Errorf(...) }` — added just before the final
`return report, nil` (`associate_semantics.go:99`). This is additive to the existing control flow:
no early-return path changes, only the final line gains a condition.

**Alternative considered:** treat every deferral as a failure. Rejected (ADR §2.3/§3.3) — most
deferral reasons (`unresolved_referent`, `governed_term_not_released`, a source metric with no
number) are already-modeled, expected outcomes with their own recovery path (backlog drain, term
release); only ungoverned vocabulary nobody has triaged is an operational gap worth alarming on.

### D4 (ADR DR4). Fix the shared Phase C harness, not `associate_semantics` alone

`runPostProcessIndexing`'s existing error branch (`control.go:1111-1120`) gains one call —
`s.persistProcessorRuntimeStatus(ctx, recordID, name, "failed", err.Error())` — mirroring
`runSingleProcessorCollect`'s Phase A/B pattern (`control.go:1187`) exactly. This is a harness-wide
fix: every `PostProcessIndexer` (scene blocks, summaries, topics, provisions, entity/relation,
product structure, metrics, inventory items, semantic projections, all three Phase D stages) gains
real Phase C failure visibility into `kb.inputs.status`/`pipeline_state`/`has_failed_proc`
simultaneously — confirmed there is no per-processor hook into this shared loop to scope the fix
narrower, and no identified downside for any of the other processors (`ProjectSemanticsProcessor`'s
deliberate self-swallow of its own report-build failure, `phase_d.go:118-120`, is unaffected since
it doesn't return that error today and this change doesn't touch it).

DR3's non-nil `Run` error and DR6's non-nil `PostProcessIndex` error both propagate through this
one fix with no further plumbing — D3, D4, and D6 compose through paths that already exist.

**Alternative considered:** a special-cased call path bypassing the shared harness for
`associate_semantics` only. Rejected (ADR §3.7) — would mean duplicating the harness or branching
on processor name, both worse than fixing the shared loop once.

### D5 (ADR DR5). Seed migration prevents a cutover failure storm

The `CREATE TABLE` migration includes seed `INSERT`s for two disjoint sets, generated from the
same data this ADR's investigation already produced (not re-derived by hand, per ADR §10's stated
risk):
1. Every synonym in commit `56b6`'s current Go switch → `status='approved'`, matching
   `canonical_bucket`.
2. Every string identified as direction-ambiguous in the survey/tests (`threshold`, `discrete`,
   `categorical`, `ordinal`, `continuous`, `binary`, `tolerance`, `ratio`, `target`, and the rest
   of `TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed`'s list) → `status='ambiguous'`.

Anything in the 309-string survey outside both sets legitimately starts `'proposed'` — that's the
new signal this change exists to produce, not a gap to seed away.

### D6 (ADR DR6). Detect at extraction time; `normalize_assertions` waits on it

`MetricsProcessor.PostProcessIndex` (`extract-metrics.go:754`), after its existing reindex/object
work, runs every `kb.metrics` row it just wrote for this record through `ValueRangeTypeMapper.
Lookup` (D1). Extraction/persistence of `kb.metrics` rows is unchanged — this is a check *over*
already-persisted rows, not a gate on writing them (explicit user requirement, ADR §3.7: never
drop evidence for being unclassifiable). On a miss, per row:
1. Upsert the proposal row via the same D1 lookup path (best-effort `canonical_bucket` guess when
   a cheap direction cue applies — reusing `parseThresholdOrTarget`'s existing keyword cues;
   `NULL` otherwise; either way `status` stays `'proposed'`).
2. `UPDATE kb.metrics SET value_range_type_error = <short message> WHERE metric_id = ...` — new
   nullable `TEXT` column, additive migration, no backfill (existing rows correctly stay `NULL`;
   DR6 only flags rows it processes going forward).
3. Accumulate a per-record miss count; if `>0` after the loop, call `LogAssertionMappingMiss`
   (`doc_proc_name='extract_metrics'`, D2) and return a non-nil aggregate error from
   `PostProcessIndex` — which D4's fix now correctly turns into `kb.inputs.status`'s authoritative
   `extract_metrics` outcome via the existing latest-write-wins rollup
   (`project_migrations/20260609000002_add_kb_inputs_status_rollups.sql`, `ORDER BY s.proc, s.ord
   DESC`) — a clean chunk-extraction pass (Phase B, `persistMetricsStatus`) followed by a Phase C
   mapping miss correctly ends as `failed`.

`NormalizeAssertionsProcessor` gains `PostProcessDependsOn() []string { return
[]string{"extract_metrics"} }` (mirroring the existing `phase_d.go` dependency declarations
exactly) so `runPostProcessIndexing`'s scheduler guarantees DR6's pass finishes first — without
this, `normalize_assertions` could read a `kb.metrics` row before its `value_range_type_error` flag
or `kb.metric_value_range_type_map` proposal exists, a race with no other guard today.

**Alternative considered:** detect only at `associate_semantics` (D3 alone, no D6). Rejected (ADR
§3.7) — Phase D is independently gated by `SEMANTIC_ASSOCIATION_ENABLED` and can run much later
than extraction; a deployment with that flag off would never see the signal at all even though
`extract_metrics` already has everything needed to raise it. D6 makes D3 a backstop, not the only
line of defense — both stages will legitimately keep signaling failure on the same still-unapproved
string until it's approved, which is intentional, not a duplicate-alarm bug.

## Risks / Trade-offs

- **[Risk] In-process cache staleness** — a just-approved mapping is not instantly visible to an
  in-flight or very-recently-started run **→ Mitigation:** accepted per ADR §10; documents are not
  reprocessed instantly regardless, and the fixed short TTL / invalidate-on-write already bounds
  the staleness window. Worth confirming the chosen TTL against real operator workflow if it
  becomes a friction point in practice — not designed further here.
- **[Risk] Incorrect DR5 seed data** (missing a genuinely ambiguous string) → a one-time false
  failure storm on cutover, now doubled in visibility since both `extract_metrics` and
  `associate_semantics` would flag it **→ Mitigation:** the seed list is generated from the same
  survey/test data this ADR cites (`TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed`),
  not re-derived by hand; the regression tests in ADR §11 re-run those exact tests against the
  DB-backed mapper to confirm no behavior change during cutover.
- **[Risk] DR4 surfaces pre-existing silent failures in unrelated Phase C processors** — any
  processor that has been failing `PostProcessIndex` quietly until now will suddenly show up as
  `failed` and become eligible for `--all=failed-procs` retry, which could be a large, unexpected
  batch **→ Mitigation:** none built into this change; flagged explicitly in the proposal's
  **BREAKING (behavioral)** note. Recommend checking current Phase C error log volume across all
  processors before/immediately after rollout so an unexpected spike is diagnosed as
  newly-surfaced (good) rather than newly-caused (bug).
- **[Trade-off] DR6 adds a per-record DB round trip (or small batch of them) to `extract_metrics`'s
  Phase C step** that didn't exist before — expected cheap given the table's small size and the
  cache, but not yet measured against a large-batch extraction run (ADR §10). No load test is
  scoped into this change; worth a follow-up if `extract_metrics` Phase C latency regresses
  noticeably post-rollout.

## Migration Plan

1. Ship the three migrations (§ Database Migrations below) together — the seed data in migration 1
   must land before any runtime code reads the table, so the create+seed migration is not safe to
   split from the code deploy that starts querying it.
2. Deploy D1–D6 code changes together (no feature flag — `SEMANTIC_ASSOCIATION_ENABLED` already
   gates Phase D independently and is unaffected; DR6 in `extract_metrics` has no gate of its own,
   matching "every extracted metric is persisted exactly as today" being unconditional).
3. Immediately after rollout, spot-check `kb.metric_value_range_type_map` for unexpectedly large
   `'proposed'` `occurrence_count` values — a large one likely means D5's seed list missed a common
   ambiguous string that should be `'ambiguous'`, not a genuine new-vocabulary signal; correct via
   `UPDATE ... SET status='ambiguous'`, not a code change.
4. **Rollback:** reverting the code deploy is sufficient to stop new failures being raised — the
   table and `value_range_type_error` column are additive and nothing else depends on their
   presence. No data migration needed to roll back; the three migrations' `Down` sections drop what
   `Up` created, in case the schema itself needs to be reverted too.

## Open Questions

Carried forward from ADR §7 as explicitly out of scope for this change:
- Whether `kb.metrics.value_class`'s equivalent free-text gap should reuse this table's pattern or
  needs its own — not surveyed here.
- Whether/when an admin triage UI for `'proposed'` rows becomes worth building (same shape as the
  existing "Resolve Ambiguous Objects" page for `kb.object_nodes`).
- Whether DR6's bucket-guessing heuristic needs more precision than "reuse `parseThresholdOrTarget`'s
  cues" — deferred until the approve/correct ratio in practice shows it's worth improving.
