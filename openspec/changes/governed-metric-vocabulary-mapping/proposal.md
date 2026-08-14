## Why

`kb.metrics.value_range_type` is free text from an LLM extractor, not a constrained enum — a
production survey found 309 distinct strings across 7,036 rows. `CanonicalMetricValueRangeType`
recognizes only a hardcoded Go `switch` of synonyms; every future LLM phrasing drift requires a
Go code change and a redeploy, and the full vocabulary is invisible except via ad-hoc SQL. Worse,
independently: `associate_semantics`' `HandleEvent` is a no-op that reports `"success"` before its
real work (`PostProcessIndex`) even runs, and the shared Phase C harness (`runPostProcessIndexing`)
never writes any `PostProcessIndex` failure into `kb.inputs.status` for *any* processor — so a
mapping miss (or any other Phase C failure) is structurally invisible to the retry mechanism and
the admin dashboard today. ADR `2026081401` decides both problems together: move the closed
vocabulary into a governed, auto-discovering DB table (extending the auto-promoted-terms
governance pattern from ADR `2026081201`), and fix the Phase C harness gap that would otherwise
make the new failure signal just as invisible as the mapping problem itself.

## What Changes

- Add `kb.metric_value_range_type_map`: a governed lookup table (`raw_value` PK, `canonical_bucket`,
  `status` ∈ `proposed`/`approved`/`ambiguous`, `occurrence_count`, first/last-seen record ids,
  audit columns) replacing `CanonicalMetricValueRangeType`'s hardcoded Go switch. A lookup miss
  auto-inserts a `'proposed'` row instead of silently falling through; only `'approved'` rows
  classify a metric.
- Seed the table on creation with commit `56b6`'s existing synonyms (`status='approved'`) and the
  known direction-ambiguous strings (`status='ambiguous'`) so cutover reproduces today's behavior
  exactly, with no false-alarm failure storm.
- `MetricNormalizer`/`resolveMetricValue` tag `proposed_payload.value_range_type_lookup` with the
  lookup outcome (`approved`/`ambiguous`/`proposed`/`absent`).
- `AssociateSemantics.Run` gains a `MappingMisses` counter and returns a non-nil aggregate error
  when any candidate's lookup outcome is `"proposed"` (vocabulary nobody has triaged yet) — other
  deferral reasons, including `"ambiguous"`, are unaffected.
- `extract_metrics`'s `PostProcessIndex` (DR6) runs the same DR1 lookup over its own just-persisted
  `kb.metrics` rows immediately after extraction: on a miss it upserts the proposal row, sets a new
  `kb.metrics.value_range_type_error` column on the specific row, and returns a non-nil aggregate
  error after logging one `kb.doc_proc_logs` entry — surfacing the problem at extraction time
  instead of only three stages later at Phase D. `NormalizeAssertionsProcessor` gains
  `PostProcessDependsOn() → ["extract_metrics"]` so the two Phase C passes can't race.
- New `kb.doc_proc_logs` entry type `'assertion_mapping_miss'`, written once per record per run
  (not once per candidate) from both `associate_semantics` and `extract_metrics`.
- **BREAKING (behavioral, not schema):** fix `runPostProcessIndexing` so a `PostProcessIndex`
  error is persisted via `persistProcessorRuntimeStatus(..., "failed", ...)` instead of only being
  logged — this applies uniformly to *every* `PostProcessIndexer` (scene blocks, summaries, topics,
  provisions, entity/relation, product structure, metrics, inventory items, semantic projections,
  all three Phase D processors), not just `associate_semantics`/`extract_metrics`. Any processor
  that was silently failing Phase C today will start showing up as `failed` in `kb.inputs.status`
  and the admin dashboard, and become eligible for `--all=failed-procs` retry.

## Capabilities

### New Capabilities
- `phase-c-failure-propagation`: the harness-level fix making `PostProcessIndex` errors reach
  `kb.inputs.status`/`pipeline_state`/`has_failed_proc` for every Phase C processor. Foundational —
  without it, DR3/DR6's new error returns would be no more visible than today's silent gap.
- `governed-metric-vocabulary`: the `kb.metric_value_range_type_map` table, its auto-discovering
  lookup, DR5 seed data, `associate_semantics`' `MappingMisses`/backstop failure, `extract_metrics`'
  extraction-time detection and self-fail (DR6), and the shared `assertion_mapping_miss` log entry
  type.

### Modified Capabilities
(none — no pre-existing `openspec/specs/` capabilities in this repo to modify; both capabilities
above are new specs.)

## Impact

- **Schema:** three new goose migrations — create `kb.metric_value_range_type_map` (with DR5 seed
  `INSERT`s), widen `kb.doc_proc_logs_entry_type_check` to add `'assertion_mapping_miss'`, add
  `kb.metrics.value_range_type_error TEXT` (nullable, additive, no backfill).
- **Code:** `server/api/ontology/assertions/{metric_normalizer.go,associate_semantics.go}`,
  `server/api/doc-processing/{phase_d.go,control.go,doc_proc_log_store.go,extract-metrics.go}`.
- **Docs:** `KnowledgeStore/doc-repo/user-manuals/metric-assertion-semantic-processing-v1.1-en.md`
  bumps to v1.2 with a new subsection under §5.1/§5.2 describing the `'proposed'` failure mode,
  the new table, `value_range_type_error`, and the operator approve/correct workflow. Commit
  `56b6`'s inline comment in `metric_normalizer.go` needs a follow-up note once DR1 lands.
- **Downstream:** the failed-processor retry mechanism (`--all=failed-procs`) and the admin
  dashboard's per-processor status column start reflecting real Phase C failures for the first
  time — operationally significant even though no code in either consumer changes.
