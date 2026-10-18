## Why

`kb.metrics` gained `condition`, `value_min` and `value_max` in migration `20260801000014`
(change `extract-metrics-structured-output`), but only the old `kbhandler` import path writes them.
The doc-processor path (`extract-metrics.go`), which produces every current row, loses all three:
the Pass 2 batch schema the model is told to follow omits them, `normalizeMetricList` drops them,
and the `kb.metrics` INSERT has no such columns. On 2026-10-16, 0 of 2,383 rows had a condition and
0 of 430 range rows had `value_min`. Without a condition, two statements of the same property under
different conditions look identical, so the model writes the condition into the subject (e.g.
`身高测试仪(计)（幼儿）`) or merges several values into one row (`男300 mm、女250 mm`).

Scoring record 371 (GB/T 44700—2024) against gold 6.3.0 run `20261015_163536` gave 75/100 and
showed these related defects:
- ± tolerances stored as `exact`/`range` instead of `upper_bound` (about 76 pairs)
- units repeated inside `metric_value` ("5 s~25 s", 34 pairs)
- several values in one row (about 45 missed gold rows)
- an informative example data set turned into 75 column-range rows
- pass criteria of product requirements tagged as test settings

## What Changes

- Pass 2 asks for, keeps and stores `condition`, `condition_en`, `value_min` and `value_max`.
  New column `kb.metrics.condition_en` (goose migration). A `range` row without endpoints gets them
  parsed from `metric_value` when it is a plain `a~b` interval.
- **Condition is part of a metric's identity.** Two rows that differ only in condition are
  different metrics in every merge check, including the resolved-concept shortcut.
- New prompts `prompt-extract-metric-candidates-v16.md` and `prompt-enrich-metrics-v13.md`,
  wired in as the defaults:
  - **Condition definition:** applicability, population, variant, environment, operating state,
    test method or apparatus, location, load or time point under which the value holds. The
    subject names only the object.
  - **Splitting:** one value per row, and one row per condition.
  - **Notation:** `±x` is `upper_bound`; `metric_value` holds no unit or comparator.
  - **Data sets:** tables of records or examples are not metrics (new drop reason
    `dataset_record`).
  - **Kind:** a pass criterion of a product requirement is not a test setting.
  - **Pass 1:** candidates carry a `condition_hint`.
- Deterministic Pass 2 normalization in Go (after range-type canonicalisation): set `±x` to
  `upper_bound`; strip the row's own unit and any leading comparator from `metric_value`; log
  rows that still hold several values.
- No backfill: existing rows get conditions when their documents are re-extracted.

## Capabilities

### New Capabilities
- `metric-condition`: extract_metrics extracts, stores and uses the condition (EN + source
  language) of each metric row, and treats it as part of metric identity.
- `metric-value-notation`: deterministic normalization of Pass 2 values (± tolerance comparator,
  unit and comparator not inside `metric_value`, range endpoints, multi-value detection).
- `metric-row-granularity`: the extraction rules for one value and one condition per row, data
  sets, and pass criteria versus test settings.

### Modified Capabilities
- None. `metric-statement-kind` classification is unchanged; only how rows are tagged changes.

## Impact

- `server/api/doc-processing/extract-metrics.go`: batch schema, `normalizeMetricList`, INSERT and
  its `ON CONFLICT` update, `mergeResolveDecidedFields`, prompt defaults, the normalization step,
  and `dataset_record` in the drop-reason handling.
- `server/api/doc-processing/metrics_merge.go`: `metricsIdentical`, `metricStaticMatch`,
  `metricMergeComparedFields`.
- `project_migrations/`: new migration adding `kb.metrics.condition_en`.
- `prompts/`: two new prompt files; the older versions stay as they are.
- `mise.toml` / `.env` prompt pins, if any point at v15/v12.
- Downstream: `assertions/metric_normalizer.go` already reads `condition`, and the
  `[semantic_assertion_property_map]` work in `fix-metric-property-scope-conflation` will forward
  it to assertion qualifiers. Nothing changes there.
- Outside this repo (separate `.agents` commit): gold rules 6.4.0 in
  `extract-metrics-benchmark` (test method or apparatus is a condition; `condition_en`), and
  `score-extract-metrics` gains a weighted condition check, so new scores are not comparable with
  earlier ones. The user runs the 6.4.0 gold.
