## Why

`extract_metrics` drops rows in three places: rows the LLM tags with a drop reason, pure
requirements (spec `metric-pure-requirement-exclusion`), and, since commit `pxwp`
(2026-10-08), agreed activity schedules caught by a word list (`isAgreedActivitySchedule`).
Every drop is physical: the row is gone, and except for the pure-requirement log nobody can see
what was dropped or why. A wrong drop shows up only as a missed row in a later score run.

The word list itself is the wrong tool. Telling an activity's schedule (gold rule X2, e.g.
`416_mtc_3` 餐厨垃圾收运时间和频次, "agreed between the parties") from a quantity of an object whose
value is left open (A4, e.g. 比能耗, 单室体积, 抗压强度由供需双方商定) is a judgement, and the
wording differs by domain. Without the word list the structural conditions alone match 332 of
1,216 stored rows, including the A4 rows recovered on 2026-10-07. A feasibility check on
2026-10-08 showed that the `jev_emulated` decision model (devdoc 2026100502) with a policy from
the decision policy store (devdoc 2026100503) separates them: all 6 open-value rows of record
416 and 6 synthetic edge cases were classified correctly.

## What Changes

- **Soft drop.** A row that `extract_metrics` drops is no longer discarded. It is saved to a
  new table `kb.metrics_dropped` with its drop reason, the stage that dropped it, the decision
  details, and the full row. `kb.metrics` keeps only live rows, so no existing reader changes
  and live `metric_id`s stay contiguous.
- **Every drop is logged.** Each run that drops at least one row writes one `kb.doc_proc_logs`
  entry (`drop_metric_rows`) listing all dropped rows by reason. It replaces the
  `exclude_pure_requirements` entry, which covered only pure requirements. **BREAKING** for the
  `score-extract-metrics` skill, which reads that entry: it switches to `kb.metrics_dropped`
  and keeps reading the old entry for earlier runs.
- **Decision model replaces the word list.** `isAgreedActivitySchedule` and its word lists are
  removed. Rows whose statement kind is `requirement_value_open` (a requirement whose value is
  left open) are judged by a decision-model question: is the open value a quantity of an object,
  an activity's schedule, or not a quantity? Rows that are deterministically metrics (numeric
  criterion or value, definition, test setting) never go to the decision model.
- **Only `activity_schedule` drops, and only above a threshold.** `not_a_quantity` and
  low-confidence answers are recorded on the row for review, not dropped, until evaluated.
- **The policy lives in the decision policy store** as `metric_open_value_kind`, so its wording
  can be revised and versioned without a code change. Each decision records the model,
  `policy_id`, `policy_version` and probabilities.
- **The decision model is set by configuration.** The metrics processor names a `.models.toml`
  profile (`METRIC_DECISION_MODEL`). The Playground's mapping from a profile to a `jev_emulated`
  client (DeepSeek and DashScope settings) moves to a helper both use.
- **Two pages show dropped rows.** System Admin → LLM → Metrics → Benchmark and Knowledge
  System → Metrics can show dropped rows, marked with their reason. All other pages, search,
  indexing, reviewers and the ontology pipeline never see them.
- **Re-extraction clears both tables.** A `force_clear` run deletes the record's dropped rows
  along with its live rows.

## Capabilities

### New Capabilities
- `metric-row-soft-drop`: where dropped `extract_metrics` rows are stored, how they are logged,
  how they are numbered and cleared, and which pages may show them.
- `metric-open-value-decision`: the decision-model check on `requirement_value_open` rows: which
  rows are checked, the question and options, the policy, the drop threshold, what is recorded,
  and the failure behaviour.

### Modified Capabilities
- None in `openspec/specs/`. The unarchived spec `metric-pure-requirement-exclusion` (change
  `exclude-pure-requirements-from-metrics`) says excluded rows are not saved and are logged as
  `exclude_pure_requirements`. This change supersedes those two points: excluded rows are saved
  to `kb.metrics_dropped`, and the log becomes the general drop log. The `metric-row-soft-drop`
  spec states the new behaviour; that change's spec is amended before it is archived.

## Impact

- **Database:** new goose migration creating `kb.metrics_dropped`.
- **Server, `server/api/doc-processing/`:** `extract-metrics.go` (per-batch drop filter,
  `excludePureRequirements`, save paths, `DeleteMetricsByInputRecordID`, logging), removal of
  `isAgreedActivitySchedule`, a new decision step, `metrics_merge.go` (merge mode).
- **Server, other:** `server/api/llmadminhandler/decision_playground_handler.go` (profile →
  `jev_emulated` mapping moves out); metrics read APIs used by the two pages
  (`kbhandler/metrics_handler.go`, `kbhandler/metric_score_runner.go`).
- **Skills (`.agents/`):** `score-extract-metrics/scripts/score_io.py` reads dropped rows from
  `kb.metrics_dropped`.
- **Prompts:** the policy text is kept as `prompts/prompt-metric-open-value-policy-v1.md` and
  seeded into the decision policy store when missing.
- **Web:** `metric-mgmt-view.svelte` and `metric-score-view.svelte` (show dropped rows, with
  Paraglide keys in both message files).
- **Shared:** uses `shared/go/api/llm` (`jev_emulated`) and `shared/go/api/decisionpolicy` as
  they are; no shared-library change expected.
- **Cost:** one single-token LLM call per `requirement_value_open` row (6 of 17 rows in 416).
- **Docs:** extract-metrics spec and impl capsules, devdoc 2026100701 known limitations, ADR
  2026100603 change log.
