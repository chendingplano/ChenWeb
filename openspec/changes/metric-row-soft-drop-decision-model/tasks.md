## 1. Storage

- [x] 1.1 Goose migration creating `kb.metrics_dropped` (columns per design D1, index on `input_record_id`, unique `drop_id`); use the `db-migration` skill
- [x] 1.2 Add `SaveDroppedMetrics` and `NextDropSeq` to the metrics store interface, `MetricsSQLStore` and `ResolvingMetricsStore`; make `DeleteMetricsByInputRecordID` also delete the record's dropped rows
- [x] 1.3 Store tests: save, `drop_id` numbering from 1, delete clears both tables

## 2. Soft drop in the pipeline

- [x] 2.1 Delete `isAgreedActivitySchedule`, its word lists, `containsAny` and its test (commit `pxwp`); `dropRowsTaggedWithDropReason` returns set-aside rows with `drop_stage = llm_tag` and the matching tag as reason
- [x] 2.2 Collect `llm_tag` rows across batches and dedupe them with `dedupeFinalMetricRows`
- [x] 2.3 `excludePureRequirements` returns set-aside rows with `drop_stage = statement_kind` and the kind as reason
- [x] 2.4 Assign `drop_id`s and save set-aside rows in both save paths (sequential `HandleEvent`, chunk-batch `FinalizeChunkBatch`) and in merge mode (`metrics_merge.go`, continuing from the record's highest `drop_id`)
- [x] 2.5 Replace `logExcludedPureRequirements` with one `drop_metric_rows` log entry (design D6)
- [x] 2.6 Tests: each stage stores its rows; live ids contiguous; merge-mode numbering; one log entry with per-stage and per-reason counts; no entry when nothing dropped

## 3. Decision model

- [x] 3.1 Move the Playground's profile → `ProviderConfig` mapping (`playgroundProviderConfig`) to a helper shared by the Playground and the metrics processor; keep the Playground's tests passing
- [x] 3.2 Write `prompts/prompt-metric-open-value-policy-v1.md` from the feasibility draft (A4 vs X2 vs not-a-quantity, with examples)
- [x] 3.3 Load policy `metric_open_value_kind` via `decisionpolicy.Store.GetCurrent`; seed it from the prompt file on not-found
- [x] 3.4 Decision step after the statement-kind stage: judge `requirement_value_open` rows only, one `choice` question, state `{policy, row}`; set aside `activity_schedule` with p ≥ `METRIC_DECISION_DROP_MIN_P` (default 0.9) as `decision_model`
- [x] 3.5 Record the decision in `kb.metrics_dropped.decision` or `kb.metrics.ext_info.open_value_decision`; usage events with `PromptName = metric_open_value_kind`, `CallReason = extract_metrics`, record and run ids
- [x] 3.6 Failure handling (design D5): unset model, missing policy, failed call → keep row, record error, warn, continue
- [x] 3.7 Tests with a fake decision client: only open-value rows judged; threshold boundary; `not_a_quantity` kept; failure keeps the row; unset model makes no calls
- [x] 3.8 Add `METRIC_DECISION_MODEL` and `METRIC_DECISION_DROP_MIN_P` to `mise.local.toml` (quoted values; see the mise boolean trap) and to the env docs

## 4. Read APIs

- [x] 4.1 Metrics list API used by `MetricMgmtView`: `include_dropped=true` returns dropped rows from `row_data` with `metric_id = drop_id`, `dropped`, `drop_stage`, `drop_reason`, `decision`; default response unchanged
- [x] 4.2 Benchmark/score API: return dropped rows, marked, and exclude them from scoring; report a gold row matched only by a dropped row. Done in `score_io.py` (the runner runs it): scoring reads `kb.metrics` only; `extraction.dropped_rows` and a "Dropped rows" report section flag drops on the lines of a missed gold row. `metric_score_runner.go` unchanged
- [x] 4.3 Handler tests for both parameters

## 5. Skill

- [x] 5.1 `score-extract-metrics/scripts/score_io.py`: read dropped pure requirements from `kb.metrics_dropped` (by run `event_id`), fall back to the `exclude_pure_requirements` entry; update its tests and SKILL.md
- [x] 5.2 Note the log change in `extract-metrics-benchmark` SKILL.md and CHANGELOG (frozen rules files unchanged)

## 6. Pages

- [x] 6.1 Before starting: confirm the uncommitted edits to `metric-score-view.svelte` and `metric_score_runner.go` are committed. User decision 2026-10-08: build now, commit the page files after the other work is committed (`metric-score-view.svelte` and `metric_score_runner.go` ended up unchanged by this change)
- [x] 6.2 Knowledge System → Metrics (`metric-mgmt-view.svelte`): "Show dropped" toggle, off by default; dropped rows marked with stage, reason and decision probability; keep `select-text`. Built, NOT committed yet (with `kbService.ts`): waits for the other uncommitted work in the message files
- [x] 6.3 Benchmark: dropped rows shown and marked, not counted. Done without editing `metric-score-view.svelte`: the page shows `report.md`, whose "Dropped rows" section (score_io.py) lists them and flags any on the lines of a missed gold row
- [x] 6.4 Paraglide keys in `web/messages/en.json` and `zh-cn.json`; `bun run check` passes (0 errors, i18n ok). NOT committed yet, same reason as 6.2

## 7. Verify live

- [x] 7.1 Restart the doc-processor (no stale `mise doc-processor-run`), re-extract record 416 with `force_clear`. Done 2026-10-08 in-process (temporary env-gated test calling `HandleEvent`, deleted after), because the running `mise doc-processor-run` (started 06:22) has the old code and was left alone; restart it before the next pipeline run
- [x] 7.2 Confirm `416_mtc_3`'s row is in `kb.metrics_dropped` (stage `decision_model`), the five A4 rows are live, and one `drop_metric_rows` log entry exists. Result: this run split the agreed schedule into 餐厨垃圾收运频次 and 餐厨垃圾收运时间, both `416_drp_2/3` at P(activity_schedule) ≈ 1.00; 比能耗, 发酵周期, 单室体积 kept as `object_quantity` (1.00); one `llm_tag` drop (沼气池容积, applicability_scope); 34 live rows; one `drop_metric_rows` entry; policy `metric_open_value_kind` v1 seeded (id 3). The two 配备数量 rows were not extracted as open-value rows in this run. `score_io.py export` lists the 3 dropped rows
- [ ] 7.3 Score 416 against gold 5.1.0 with `score-extract-metrics`; record the score and the decision probabilities here
- [ ] 7.4 Check both pages in English and Chinese

## 8. Docs

- [x] 8.1 Coding capsule `extract-metrics-spec.md`: 3.4.2 (soft drop, decision step, word list removed), 3.7 logging, workflow, 5.1 storage
- [x] 8.2 Coding capsule `extract-metrics-impl.md`: stages, store methods, decision client
- [x] 8.3 Amend spec `metric-pure-requirement-exclusion` in change `exclude-pure-requirements-from-metrics` (stored as dropped, new log) before that change is archived
- [x] 8.4 Devdoc 2026100701 known limitations; ADR 2026100603 change-log entry
- [x] 8.5 Devdoc 2026100502: the emulated decision model is now used by `extract_metrics`
