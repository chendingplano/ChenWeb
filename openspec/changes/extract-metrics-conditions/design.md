## Context

`extract_metrics` runs in two LLM passes. Pass 1 (candidates prompt) finds metric candidates per
chunk. Pass 2 (enrich prompt) turns each batch of candidates into rows. Go code then dedupes,
canonicalises range types, sets rows aside (soft drop), merges with the record's existing rows and
writes `kb.metrics` (`MetricsSQLStore.SaveMetrics` on force-clear, `UpsertMetrics` otherwise).

`condition`, `value_min` and `value_max` exist as columns and in the enrich prompt's own schema
section. They are lost in code:

| Step | Location | Gap |
|---|---|---|
| Pass 2 user message | `buildMetricRelationBatchPrompt` | the "use exactly this schema" block omits them |
| Parse | `normalizeMetricList` | not copied |
| In-run dedupe | `dedupeFinalMetricRows` | key = name, subject, unit, value, lines |
| Write | `SaveMetrics`, `UpsertMetrics`, `ensureMetricsTable` | no columns |
| Read back | `GetMetricsByInputRecordID`, `loadPersistedMetricArtifactRows` | not selected |
| Merge | `metricsIdentical`, `metricStaticMatch`, `metricMergeComparedFields`, merge-resolve payload | condition ignored |

## Goals / Non-Goals

**Goals:**
- Every Pass 2 row carries its condition (source language and English) and, for ranges, its
  numeric endpoints, all the way into `kb.metrics`.
- Rows that differ only in condition stay separate at every stage.
- Prompts and deterministic code fix the record-371 defects: ± comparator, unit inside the value,
  several values per row, data-set tables, and pass criteria tagged as test settings.

**Non-Goals:**
- Backfilling existing rows (user decision 2026-10-16).
- Showing `condition` on the metric pages. Downstream assertion qualifiers come from
  `fix-metric-property-scope-conflation`.
- Splitting multi-value rows in code.

## Decisions

**D1. Condition is free text with labelled qualifiers.** Format: `<kind>：<value>` joined by `；`
(source language), e.g. `适用人群：幼儿；称量≤50 kg`. In English: `<kind>: <value>` joined by `; `.
The gold already uses this format.
- Alternative considered: a JSONB list of `{kind, value}`. Rejected: the column is `TEXT`, the
  assertion normalizer already reads it as a string, and the qualifier kinds are open-ended across
  domains.

**D2. Condition is identity everywhere, compared after normalization.** Equality uses the existing
`metricConditionKey` normalisation (whitespace, case, full/half width). Qualifiers form an
unordered set: order and repeated qualifiers do not affect equality, but both label and value
remain significant. Changes:
- Condition joins the `dedupeFinalMetricRows` key.
- `metricsIdentical` requires equal condition, including on the resolved-concept shortcut.
- `metricStaticMatch` treats condition as a discriminator that must agree even when empty on one
  side: an empty and a non-empty condition do not match.
- The merge-resolve payload carries condition. Prompt `prompt-merge-resolve-metrics-v2.md` says
  that a different condition is a different metric.
- Alternative considered: skip the concept shortcut when conditions differ only for pending groups.
  Rejected: a resolved concept names a *property*, not a metric instance, so 台阶高度 男 and 女
  share it legitimately.

**D3. Deterministic value normalization runs on the Pass 2 rows next to
`canonicalizeMetricValueRangeTypes`** (both the kept and the tagged rows, and again in
`FinalizeChunkBatch`, where canonicalisation already runs). New `normalizeMetricValueNotation(rows)`:
1. Trim a trailing copy of the row's own `unit` from each number in `metric_value`, but only when
   the unit is non-empty and the remainder still parses: `5 s~25 s` (unit s) → `5~25`,
   `1 次` (unit 次) → `1`.
2. Strip one leading comparator (`≥ ≤ > < ⩾ ⩽ 不小于 …`) from a single-number value. The comparator
   is already in `value_range_type`; a strict one adds tag `strict_bound` if missing.
3. A value that is exactly `±x` with `value_range_type` `exact` or `range` becomes `upper_bound`.
   A numeric `a±b` row becomes `range` if the model called it `exact`, `lower_bound` or
   `upper_bound`; derived endpoints replace endpoints attached to that incorrect comparator.
   Asymmetric `+a/−b` is untouched. Non-range numeric rows have NULL endpoints.
4. A `range` row without both endpoints gets them from a plain `a~b` / `a-b` / `a～b` value.
   `a±b` gives `a−b` and `a+b`.
5. A value that still holds more than one number joined by `、 ； ; ，` or 男…女 (not `/`, which is part of units such as 次/min) is not changed.
   One `kb.doc_proc_logs` warning per run lists those rows (`multi_value_rows`), to measure the
   prompt fix.

The rules are notation-only and work the same in any domain. Anything needing judgement stays in
the prompt.

**D4. New prompt versions, never edits.** Current defaults are
`prompt-extract-metric-candidates-v19.md`, `prompt-enrich-metrics-v16.md` and
`prompt-merge-resolve-metrics-v2.md`. The initial condition work used v16/v13; the record-371
scoring follow-up (v18/v15) adds normal-line numeric coverage, candidate population binding, apparatus
condition inheritance, explicit role classification and centred-tolerance rules. v19/v16 make a
number that only qualifies another value of its clause (a paired load, an apparatus grade, a
restated setting) that value's condition instead of a row, keep repeatability limits ("不应大于允许误差的
绝对值" → 1 倍), and keep a one-cell table candidate to its own cell. In code, `groupCandidatesByChunk` no longer cuts an
enrich batch between candidates quoting the same evidence: record 371's Table 1 走平衡木 row split
across two batches, and the batch holding only 计时 re-emitted 分度值/允许误差 (mtc_315/316). Older files
stay so old runs can be reproduced.

**D5. `dataset_record` is a drop reason** like `procedure_count`. It goes into
`metricDropReasonTags`, so a row the model keeps but tags this way is set aside (stage `llm_tag`)
instead of stored.

**D6. `condition_en`** is a new nullable `TEXT` column (goose migration), filled only for
non-English documents, like the other `_en` columns.

## Risks / Trade-offs

- [The model puts the subject's own type or the measured property into `condition`] → The prompt
  gives positive and negative examples. The scorer's new weighted condition check measures it.
- [Stricter identity creates near-duplicates across re-runs when the model words a condition
  differently] → Conditions are normalised before comparison. A wording change is a real change
  of the row's identity, and the merge-resolve LLM still sees overlapping-line groups.
- [The unit-strip rule removes a unit that is part of a composite value] → It removes only an
  exact copy of the row's unit next to a number, and only when the result parses. Table-driven
  tests cover `次/min` vs `次` and `mL/s` vs `mL`.
- [Scores are not comparable with earlier ones] → The scorer records its weight set in
  `score.json`, and the changelog says so.

## Migration Plan

1. Migration adds `kb.metrics.condition_en`; `mise dev` applies it.
2. Deploy code and prompts. Existing rows keep NULL conditions until their record is re-extracted.
3. Rollback: point `ENRICH_METRICS_PROMPT` / `EXTRACT_METRIC_CANDIDATES_PROMPT` back to v12/v15.
   The column is additive and the Down migration drops it.

## Open Questions

- None blocking. Whether the metric pages show `condition` is a follow-up.
