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

## 8. Gold condition_en and pages (follow-up request, 2026-10-16)

- [x] 8.1 Migration `20261016000002_add_testbed_metrics_condition_en.sql`; `extract-metrics-benchmark` validates/saves `condition_en` (required from rules 6.4.0 for a non-English condition)
- [x] 8.2 Metrics API (`metrics_handler.go`: list, gold list, single fetch; `metrics_dropped.go`) returns `condition` / `condition_en`; PATCH accepts them
- [x] 8.3 Metric detail panel (Metrics workspace, Product Review "Full Details") and the production-vs-gold comparison shelf show Condition and Condition (English); Paraglide keys in both message files
- [x] 8.4 Metric wiki infobox shows the condition in the page language (new or refreshed pages; cached pages keep their old infobox)
- [x] 8.5 In-app scorer prompt v2 (`prompt-score-extract-metrics-v2.md`): condition pairing and condition overrides, matching the skill; scorer tests expect scoring-version-2 scores

## 9. Record 371 scoring follow-up (2026-10-16)

- [x] 9.1 Trace misses to saved candidate/enrichment logs for run 220; preserve the existing gold run and scoring rules
- [x] 9.2 Candidates v18: normal-line numeric coverage (including apparatus dimensions/accuracy), per-population table coverage, apparatus capabilities, adjustment-trigger exclusion
- [x] 9.3 Enrich v15: preserve candidate population, independent settings, qualified property names, exact counts versus minima, product criteria versus test setup, bare ±x versus a±b
- [x] 9.4 Repair numeric a±b rows to ranges and derive endpoints; clear endpoints on non-range values; regressions reproduce the original failures before the fix
- [x] 9.5 Compare condition qualifiers independently of order/repetition while preserving labels, values and distinct populations/methods
- [x] 9.6 Update defaults and local mise pins; add an opt-in real-model quality regression with isolated output (no database metric writes)
- [x] 9.7 Verify live prompt cases and update the conditions devdoc with results: six cases pass (27 returned rows, 16 expected assertions plus trigger exclusion); normal processor/handler suites pass (1,729 checks, four skips), go vet, doc-processor build and strict OpenSpec validation pass. Full-document re-extraction/rescoring was not performed.

## 10. Combined timing test values (run 223, 2026-10-16)

- [x] 10.1 Reproduce the two combined percentage-position/point-count values from record 371, lines 286/287; add failing regression tests before the fix
- [x] 10.2 Enrich v19 separates mixed units/properties and defines corrective enrichment; update both code defaults and the local mise pin
- [x] 10.3 Retry affected candidate evidence groups once, retaining unrelated and non-combined sibling rows; validate distinct replacement rows, source containment, signed numeric preservation and completeness before replacing originals
- [x] 10.4 Preserve originals and the existing warning on failed/invalid retries; test omitted counts, duplicates, unrelated evidence, changed signs and incomplete multiple-row replacements
- [x] 10.5 Verify the complete doc-processing suite, go vet for processors/handlers and strict OpenSpec validation; the live two-clause check returns all six expected assertions among ten metrics without database metric writes. Two scoring-handler tests also fail on the unmodified baseline (expected score 75, actual 90); leave those outside this fix
- [x] 10.6 Update the conditions devdoc and contract. Existing stored rows and full-document scores are not changed; re-extraction applies the new behavior

## 11. Condition conventions of gold 20261016_162743 (run 226, 2026-10-17)

- [x] 11.1 Classify run 226's 234 failed condition pairs (53.7%): prompt-vs-gold conventions (~76), clause-wide qualifiers not inherited (~85), gold inconsistencies (~25), the rest synonyms and single misses
- [x] 11.2 Candidates v22 / enrich v20: apparatus by name only, table column label as condition, `检验项目`, no `取值区段`, clause/list/sibling-subclause apparatus and medium, `适用条件` exemptions; update code defaults, tests and the local mise pins
- [x] 11.3 `inheritClauseConditions` (design D4a) with tests for list items, sibling clauses, table lines, two methods in one clause, the same apparatus under another label and a qualifier outside the clause lead; replay on run 226 changes 9 rows, all toward gold, none away
- [x] 11.4 Live quality cases pass with v22/v20 (7 cases); the simulation-method case now expects the apparatus name and rejects capabilities
- [x] 11.5 Corrected gold copy `20261017_060405` (6.6.0 / claude-opus-5-5): 24 rows of 5.4.3.3, 6.4.2.3, 6.4.2.6 and Table C.1 made consistent (category as condition, every item qualifier on every row of the item)
- [ ] 11.6 Restart the doc-processor, re-extract record 371 and score against gold `20261017_060405`

## 12. Definitional numbers (benchmark rules 6.7.0, 2026-10-17)

- [x] 12.1 Candidates v23 / enrich v21: a category boundary of the people or things the object is for (age bands) and a counting, naming or unit convention (360° per turn) are `term_definition`, not metrics; a threshold classifying the document's object by a measured property and a value required of a person or sample stay rows. Code defaults, tests and the local mise pins updated
- [x] 12.2 Live quality case for 371 lines 97–102 and 291 (no 周岁 row, no 360° row, 1 min kept); all 8 cases pass with v23/v21. The isolated case also passes with v22/v20, so it guards regressions; the full-document effect needs 11.6

## 13. Prescribed-value tables versus data sets (2026-10-17)

- [x] 13.1 Root cause of the intermittent Table 3 miss (371, 20 resistor values; runs 221, 223, 227, 228): the v16 data-set bullet ("parameter library … reference data table") makes the candidates pass skip the table. On the production chunk 17 input: v15 6/6, v16–v23 0–1/6, v23 without the bullet 6/6 (but Table E.1 leaks ~35 records)
- [x] 13.2 Candidates v24 / enrich v22: a data set is a table whose rows are records of individual cases (表 E.1); a table prescribing test, apparatus or reference-standard values per model/grade/setting (表 3) is kept cell by cell with the row as condition. Probe: Table 3 6/6 with 20 cells each, Table E.1 0 candidates 6/6. Defaults, tests and local mise pins updated
- [x] 13.3 Live quality case "prescribed-value table versus data-set records" (20 Table 3 rows, no Table E.1 row): passes 4/4 with v24/v22, fails 1 of 2 with v23/v21. All 9 live cases pass

## 14. Product sizes (user ruling, 2026-10-17)

- [x] 14.1 371 mtc_3/4 (加载面长度 250 / 宽度 200) and mtc_267/268 (底座长度 / 宽度 300) split a size written as a product, as v24/v22 told them to. Candidates v25 / enrich v23: "250 mm×200 mm" and "长300 mm×宽 300 mm" are one row each (加载面尺寸 `250×200` mm, 底座尺寸 `300×300` mm, exact), and a tolerance serving the size is one row. Separately stated dimensions stay split. No Go change: unit stripping already yields `250×200` and `×` is not a list separator
- [x] 14.2 Live quality case "size written as a product is one metric" (lines 191–192 and the 119 obstacle row): passes 3/3 with v25/v23, fails with v24/v22 (the four split rows). All 10 live cases pass; defaults, tests and local mise pins updated
