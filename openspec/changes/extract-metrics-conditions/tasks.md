## 1. Schema

- [x] 1.1 Goose migration `project_migrations/<ts>_add_kb_metrics_condition_en.sql`: `ADD COLUMN IF NOT EXISTS condition_en TEXT` (Down drops it)
- [x] 1.2 `ensureMetricsTable` creates `value_min`, `value_max`, `condition`, `condition_en` for fresh test databases

## 2. Write the fields through extract_metrics

- [x] 2.1 Pass 2 batch schema (`buildMetricRelationBatchPrompt`) lists `condition`, `condition_en`, `value_min`, `value_max`
- [x] 2.2 `normalizeMetricList` copies them (endpoints as nullable numbers, never 0 for missing)
- [x] 2.3 `SaveMetrics` and `UpsertMetrics` (INSERT and `ON CONFLICT` update) write the four columns; `condition_en` only for non-English documents
- [x] 2.4 `GetMetricsByInputRecordID` and `loadPersistedMetricArtifactRows` read them back
- [x] 2.5 `mergeResolveDecidedFields` and the merge-resolve payload carry `condition`
- [x] 2.6 Tests: normalize keeps the fields; sqlmock INSERT args include them for Save and Upsert

## 3. Condition in metric identity

- [x] 3.1 `dedupeFinalMetricRows` key includes the condition
- [x] 3.2 `metricsIdentical` (including the concept shortcut) and `metricStaticMatch` require equal normalised conditions; `metricMergeComparedFields` lists `condition`
- [x] 3.3 `prompt-merge-resolve-metrics-v2.md`: a different condition is a different metric; make it the default
- [x] 3.4 Tests for the three scenarios in `specs/metric-condition` (dedupe, concept shortcut, empty vs non-empty)

## 4. Value notation normalization

- [x] 4.1 `normalizeMetricValueNotation`: unit strip, leading comparator strip (+ `strict_bound`), `±x` → `upper_bound`, range endpoints from `a~b` / `a±b`
- [x] 4.2 Call it wherever `canonicalizeMetricValueRangeTypes` runs (kept and tagged Pass 2 rows, `FinalizeChunkBatch`)
- [x] 4.3 Multi-value detection with one `multi_value_rows` warning per run in `kb.doc_proc_logs` (separators 、；;， — not `/`, which is part of units)
- [x] 4.4 Table-driven tests covering every scenario in `specs/metric-value-notation`

## 5. Prompts

- [x] 5.1 `prompt-enrich-metrics-v13.md`: Conditions section (definition, subject vs condition, format, one row per condition); one value per row; property columns of one item; `±x` is `upper_bound`; no unit or comparator in `metric_value`; data sets → `dataset_record`; pass criteria vs test settings; schema gains `condition_en`
- [x] 5.2 `prompt-extract-metric-candidates-v16.md`: `condition_hint`; property columns of one item; data sets are not candidates; a limit stated as a fraction of another limit is a value; requirement values inside narrative table cells are candidates
- [x] 5.3 `dataset_record` in `metricDropReasonTags`; Pass 1 candidate parsing keeps `condition_hint`
- [x] 5.4 `NewMetricsProcessor` defaults → v16 / v13 / merge v2; update any mise/.env pins
- [x] 5.5 Prompt-default tests updated (mise pins are in `mise.local.toml`, which is not tracked)

## 6. Verify

- [x] 6.1 `go test ./api/doc-processing/...` and `go vet` pass
- [x] 6.2 Re-extract record 371 in-process and check that `condition` and `value_min` are filled and that the ± / unit / Table E.1 defects are gone (scoring is left to the user, who runs gold 6.4.0). Result 2026-10-16: 483 rows, 410 with condition/condition_en, 102/103 ranges with endpoints (`1/2~2/3` has none), 77 ±x rows all `upper_bound`, 0 unit repeats, 0 Table E.1 rows, 0 multi-value rows; 6.5.14.1/.2 give 8 rows with distinct conditions (the 6.5.14.2 condition wrongly repeats the device). Not scored.

## 7. Docs

- [x] 7.1 KnowledgeStore devdoc for extract_metrics conditions (what changed, how condition is defined, identity rule)
- [x] 7.2 Note in `extract-metrics-structured-output` that the doc-processor write path is completed by this change
