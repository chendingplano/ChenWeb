## 0. Prerequisite

- [x] 0.1 Archive `extract-metrics-conditions` (its `metric-condition` spec is narrowed by this change)

## 1. Schema

- [x] 1.1 Goose migration: `kb.metrics.frame jsonb` (nullable, GIN index) and `testbed.metrics.frame jsonb`
- [x] 1.2 Allow `drop_reason = resource_spec` in `kb.metrics_dropped` (no check constraint exists — nothing to change)
- [x] 1.3 Apply on `miner`. Never on a TEST_DATABASE_URL that points at `miner`.

## 2. Frame model and label map

- [x] 2.1 `server/api/doc-processing/metric_frame.go`: frame types, normaliser (role list, empty → nil), uses signature
- [x] 2.2 `conditionLabelFacet` map for the labels in design.md Context (zh and en), plus `splitLegacyCondition`
- [x] 2.3 `药物浓度` → spec of the single agent; log the zero-agent and multi-agent cases
- [x] 2.4 Unit tests: every Context-table label, unknown label untouched, idempotent, revert round-trip

## 3. Extractor wiring

- [x] 3.1 Pass 2 batch schema and `normalizeMetricList` accept `frame`; `SaveMetrics` and `UpsertMetrics` write it
- [x] 3.2 Apply `splitLegacyCondition` to Pass 2 output before notation and dedupe steps
- [x] 3.3 Add the uses signature and method to the dedupe key, `metricsIdentical`, `metricStaticMatch` and the merge-resolve input
- [x] 3.4 Move `inheritClauseConditions` / `inheritServedQuantityConditions` targets to the frame facets; update their tests
- [x] 3.5 `dropResourceSpecRows`, with the `kb.metrics_dropped` write (`drop_stage = enrich`, `resource_spec`, host `metric_id`)
- [x] 3.6 Set `provision_id` when a provision of the record covers `procedure.step` and the row's lines
- [x] 3.7 Tests: the 364 A.2.4 fixture (one dose row per agent, no strength rows, mtc_33 gone), the apparatus case and the product-standard exception

## 4. Prompts

- [x] 4.1 `prompts/prompt-extract-metric-candidates-v29.md`: the resource-spec rule, the specified-object exception, amount-less steps → X16
- [x] 4.2 `prompts/prompt-enrich-metrics-v28.md`: the `frame` output, facet definitions, condition narrowed, names without strength
- [x] 4.3 Pin the defaults in `extract-metrics.go` and `mise.local.toml`; update the prompt-ref tests (`extract_metrics_pure_requirements_test.go`, `metric_multi_value_recovery_test.go`)
- [x] 4.4 Live probe on the production chunk for 364 A.2.4 and 371 6.5.14 (readAllLines + canonicalChunkInputText + buildDocContextLine) — done with the hand-built-line live harness (`TestMetricExtractionQualityLive`, 12/12 pass, one run)

## 5. Backfill

- [x] 5.1 Backfill command (`--dry-run`, `--revert`, per-label counts) over `kb.metrics` and `testbed.metrics`
- [ ] 5.2 Dry-run on `miner`, review the counts with the user, then run it

## 6. Ontology vocabulary

- [x] 6.1 Seed `mea:resource`, `mea:uses_resource`, `mea:resource_role` (7 role terms) and `mea:has_spec` in the measurement module through the normal term seeding path

## 7. API and UI

- [x] 7.1 `metrics_handler.go`, `metrics_dropped.go` and `metric_wiki_compile.go` return `frame`; handler tests
- [x] 7.2 `kbService.ts` types; `metric-detail-groups.ts` and the detail panel show Conditions / Uses / Method / Procedure, with empty groups hidden
- [x] 7.3 Paraglide keys in `en.json` and `zh-cn.json` (条件 / 使用 / 方法 / 程序, role names); `bun run check` passes; the pages allow text selection

## 8. Benchmark (`.agents`)

- [x] 8.1 `extract-metrics-benchmark/rules/9.0.0.md` (default) and frame validation in the gold tooling
- [x] 8.2 `score-extract-metrics` scoring v4 (weights, uses soft match, version guard)
- [ ] 8.3 The user runs the 9.0.0 gold for 364 and 371. Do not run it unless asked.
- [ ] 8.4 Re-extract 364, 371 and one non-clinical, non-test-equipment document; score with v4; report per-facet accuracy

## 9. Docs and commits

- [x] 9.1 Devdoc in KnowledgeStore (create-devdoc skill); update `metric-ontology-v1.0-en.md` §4.1 for the uses facet; note the stale parts of devdoc 2026101601 (condition scope)
- [ ] 9.2 Commit via jj: ChenWeb (this change), `.agents` (rules and scorer) and KnowledgeStore separately; check `jj log`
