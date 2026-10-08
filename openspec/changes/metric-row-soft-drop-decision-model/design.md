## Context

`extract_metrics` (`server/api/doc-processing/extract-metrics.go`) drops rows at three points
today, all physically:

| Stage | Where | What it drops |
|---|---|---|
| LLM tag | per enrich batch, `dropRowsTaggedWithDropReason` | rows the LLM tagged with a drop reason (`activity_schedule`, `applicability_scope`, …) |
| Word list | same function, `isAgreedActivitySchedule` (commit `pxwp`, 2026-10-08) | untagged agreed/announced activity schedules |
| Statement kind | after dedup, `excludePureRequirements` | `inspection_requirement`, `delegated_requirement` |

Only the statement-kind stage is audited (`exclude_pure_requirements` log). `kb.metrics` is read
by 149 SQL references in 43 server files, plus `score_io.py` and `benchmark_io.py`.

**Feasibility check (2026-10-08).** Scratch program calling `jev_emulated` with
`deepseek-v4-flash` (`thinking = disabled`, `temperature = 1`), one `choice` question, draft
policy (A4 vs X2 from gold rules 5.1.0):

- Record 416's 6 open-value rows and 6 synthetic edge cases (agreed product strength, agreed test
  time, agreed storage time, announced maintenance plan, …): 12/12 correct, all at p ≥ 0.99. With
  the examples removed from the policy: 11/12, and the miss (product storage time) was 0.57/0.43,
  i.e. flagged as uncertain rather than confidently wrong.
- All 332 stored rows with an open value (326 of them from old v5/v7 prompts): no call failed;
  32 rows had max p < 0.9. `not_a_quantity` wrongly took real quantities (GHG emission
  reductions, 零位误差, USL/LSL). Record 650 (ISO 14068-1) put 11 disclosure periods and dates
  (reporting period, base period, target year) in `activity_schedule`, a domain question the
  current policy does not settle (see Open Questions).

## Goals / Non-Goals

**Goals:**
- No row that `extract_metrics` produces is lost silently: every drop is stored, reasoned and
  logged.
- Dropped rows are invisible to every consumer of `kb.metrics` without changing those consumers.
- The X2-vs-A4 judgement uses a versioned policy and the decision model, not word lists.
- Rows that are deterministically metrics never pay for a decision call.

**Non-Goals:**
- Storing candidates the LLM listed in `dropped_candidates` (they never became rows; they stay in
  the `enrich_metrics` logs).
- A general "is this a true metric?" gate for all rows.
- Dropping on `not_a_quantity` (recorded only, until evaluated).
- Backfilling rows saved before this change; a re-extraction applies it.
- Restoring a dropped row to `kb.metrics` from the UI.

## Decisions

### D1. Dropped rows go to a separate table `kb.metrics_dropped`

`kb.metrics` keeps only live rows. Chosen by the user on 2026-10-08.

Alternatives considered:
- *`drop_reason` column + filter in every reader:* 149 references to change, and any future reader
  can forget the filter.
- *Column + rename table to `kb.metrics_all` + view `kb.metrics`:* readers unchanged, but the two
  startup `CREATE TABLE`/`ALTER TABLE` sites and every future migration must target the base
  table, a trap for migration authors.

Schema (goose migration):

| Column | Type | Notes |
|---|---|---|
| `id` | `BIGSERIAL PRIMARY KEY` | |
| `input_record_id` | `BIGINT NOT NULL` | indexed |
| `drop_id` | `TEXT NOT NULL UNIQUE` | `<record_id>_drp_<seqno>`, seqno from 1 per record |
| `candidate_id` | `TEXT` | Pass 1 lineage |
| `drop_stage` | `TEXT NOT NULL` | `llm_tag` \| `statement_kind` \| `decision_model` |
| `drop_reason` | `TEXT NOT NULL` | tag, statement kind, or decision choice (`activity_schedule`) |
| `decision` | `JSONB` | decision-model details (D4); null for other stages |
| `row_data` | `JSONB NOT NULL` | the full enriched row as it would have been saved |
| `event_id` | `TEXT` | same as live rows |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | |

`row_data` is JSONB, not a copy of `kb.metrics` columns, so the two tables cannot drift when
`kb.metrics` gains columns. Dropped rows get no `metric_id`, so live ids stay contiguous (as spec
`metric-pure-requirement-exclusion` requires) and nothing keyed on `metric_id`
(`kb.assertion_evidence`, `kb.artifact_connections`, `kb.search_artifacts`) can reference them.

### D2. Order of the stages

1. Per enrich batch: rows carrying a drop-reason tag are set aside (`llm_tag`). The word-list
   check is deleted.
2. After dedup and range-type canonicalization: pure requirements set aside (`statement_kind`).
3. Remaining rows of kind `requirement_value_open` go to the decision model (D3). Rows answered
   `activity_schedule` with p ≥ threshold are set aside (`decision_model`).
4. Live rows get `metric_id`s; set-aside rows get `drop_id`s; both are saved in the same save
   step (sequential `HandleEvent` path and chunk-batch `FinalizeChunkBatch` path).

Rows set aside in stage 1 are deduplicated with `dedupeFinalMetricRows` among themselves, since
they come from overlapping chunks like live rows.

### D3. Which rows go to the decision model

Only rows whose `metricStatementKind` is `requirement_value_open`. Every other kind is
deterministic: numeric criteria and values, definitions, test parameters and observations are
metrics; pure requirements are already dropped. Within `requirement_value_open` there is no
further shortcut: a unit does not settle it, because an activity frequency can carry one
(收运频次, 次/日). In record 416 this is 6 of 17 rows.

### D4. The decision request

- Client: `jev_emulated`, built from the `.models.toml` profile named by `METRIC_DECISION_MODEL`.
  The Playground's profile → `ProviderConfig` mapping (`playgroundProviderConfig`: DashScope
  `top_logprobs = 5`; DeepSeek `thinking = disabled`, `temperature = 1`; `omit_temperature`) moves
  to a helper used by both. A profile whose `model_type` is `decision-model` uses
  `jev_compatible` as in the Playground.
- Policy: `decisionpolicy.Store.GetCurrent(ctx, "metric_open_value_kind")`, loaded once per run.
- State: `{"policy": <text>, "row": {metric_name, subject, threshold_or_target, desc, context}}`.
  The policy key sorts first, so it leads every prompt and can be prefix-cached.
- One `choice` question `kind`: `object_quantity`, `activity_schedule`, `not_a_quantity`.
- Drop rule: choice `activity_schedule` and P(`activity_schedule`) ≥
  `METRIC_DECISION_DROP_MIN_P` (default `0.9`).
- Recorded on every judged row as `decision` = `{model, profile, policy_id, policy_version,
  choice, probabilities}`: in `kb.metrics_dropped.decision` for dropped rows, and in
  `kb.metrics.ext_info.open_value_decision` for kept rows. Kept rows with choice
  `not_a_quantity` or max p below the threshold can be listed for review from `ext_info`.
- Usage rows: `PromptName = metric_open_value_kind`, `CallReason = extract_metrics`, with
  `RecordID` and `RunID` set.

Why one request per row, not one per document: `jev_emulated` reads the state from the last user
message, so one row per request keeps each answer independent and the policy prefix shared.

### D5. Failure handling

The decision step never fails the run and never drops on failure. If `METRIC_DECISION_MODEL` is
unset, the policy is missing and cannot be seeded, or a call fails, the row is kept,
`ext_info.open_value_decision` records the error, and a warning is logged with the record and
candidate ids. The policy is seeded from `prompts/prompt-metric-open-value-policy-v1.md` (no
prompt text in code) the first time `GetCurrent` returns not-found; later wordings are new
versions created in the Playground.

### D6. Logging

One `kb.doc_proc_logs` entry per run that sets aside at least one row:
`doc_proc_name = extract_metrics`, `activity_name = drop_metric_rows`, `extra_info` =
`{num_dropped, by_stage, by_reason}`, artifact `dropped` = per row `{drop_id, candidate_id,
drop_stage, drop_reason, kind, metric_name, subject, threshold_or_target, context,
source_line_spans, decision}`. `kind` keeps the field the `exclude_pure_requirements` artifact
had. That entry is no longer written.

`score_io.py` (`score-extract-metrics`) reads `kb.metrics_dropped` rows with
`drop_stage = statement_kind` for the scored run (matched by `event_id`), and falls back to the
`exclude_pure_requirements` entry when the run has none (runs before this change).

### D7. Clearing and merge mode

`DeleteMetricsByInputRecordID` (force_clear) also deletes the record's `kb.metrics_dropped`
rows, so `drop_id`s restart at 1 like `metric_id`s. In merge mode (`force_clear = false`,
`metrics_merge.go`) set-aside rows are appended with `drop_id`s continuing from the record's
current maximum; existing dropped rows are kept, as existing live rows are.

### D8. The two pages

- API: the metrics list used by `MetricMgmtView` (Knowledge System → Metrics) and the metric
  list in `MetricScoreView` (System Admin → LLM → Metrics → Benchmark) accept
  `include_dropped=true`. Dropped rows are returned from `row_data` with `metric_id = drop_id`,
  `dropped = true`, `drop_stage`, `drop_reason` and `decision`. Without the parameter the
  responses are unchanged.
- Knowledge → Metrics: a "Show dropped" toggle, off by default; dropped rows are visibly marked
  and show their reason and, for decision-model drops, the choice and probability.
- Benchmark: dropped rows are shown, marked, but the score excludes them (they were not saved as
  metrics). A gold row matched only by a dropped row is shown as such, so wrong drops are
  visible.
- All labels through Paraglide, in both `en.json` and `zh-cn.json`.

## Risks / Trade-offs

- [Chat-model probabilities are uncalibrated (near 0 or 1)] → the threshold applies only to
  `activity_schedule`; tune it on the gold ledgers of 416 (and 753 once its gold exists); record
  probabilities so a different threshold can be evaluated after the fact.
- [Policy wording sways answers (examples moved one case from 0.99 to 0.43)] → the policy is
  versioned; every decision records `policy_id`/`policy_version`; compare versions on the gold
  set before making one current.
- [Domain disagreement: disclosure dates and periods in ISO 14068-1] → open question below;
  answerable with a policy version, no code change.
- [Extra cost and latency: one call per open-value row] → single-token calls, bounded by
  `jev_emulated` concurrency; DeepSeek needs 15–60 s before a new prefix is cached, so the first
  calls of a run pay full price.
- [Model without logprobs (reasoning models)] → calls fail, rows are kept (D5); the configured
  profile must be a non-reasoning model.
- [`metric-score-view.svelte` and `metric_score_runner.go` have uncommitted edits from other work
  (2026-10-08)] → commit or rebase that work before task group 6.
- [Dropped rows invisible to search may hide a real metric] → they are listed on the two pages
  and in the drop log; a wrong drop costs visibility, not data.

## Migration Plan

1. Apply the goose migration (`mise dev` applies it at start).
2. Deploy server, set `METRIC_DECISION_MODEL` in `mise.local.toml` (e.g. `deepseek-flash-4-1`).
3. Re-extract record 416 with `force_clear`; expect `416_mtc_3` in `kb.metrics_dropped` with stage
   `decision_model`, and the five A4 rows live.
4. Rollback: unset `METRIC_DECISION_MODEL` (decision step skipped, rows kept). The table can stay;
   no reader depends on it except the two pages.

## Open Questions

- Are disclosure dates and periods (ISO 14068-1: reporting period, base period, target year)
  metrics? The gold rules (§2.1 (b)) treat a time slot of an activity as not measurable, but say
  nothing about a declared reporting period. To be settled in the policy and the gold rules,
  together.
- Should `not_a_quantity` drop once evaluated on gold, or move to its own question and policy?
- Should the same mechanism later judge other ambiguous kinds (e.g. `observation` rows)?
