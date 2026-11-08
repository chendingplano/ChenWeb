## 1. Gold tables

- [x] 1.1 Project migration `create_testbed_products`: create `testbed.product_benchmark_runs`, `testbed.products` and `testbed.products_dropped`. Include vocabulary checks for `product_type`, `relation_type` and `obligation_level`, the run-key FK, unique (run key, `product_rel_id`), indexes and a down migration.
- [x] 1.2 Apply it to the dev database and confirm the columns.

## 2. Gold skill `extract-products-benchmark` (.agents)

- [x] 2.1 `rules/1.0.0.md` and `rules/versions.json`. The rules cover:
  - the unit (D1) and producthood A/X/D rules, with the user's waste-class ruling (D2);
  - naming and elided coordinations;
  - the relation decision order;
  - the enrichment labels (D3);
  - the inventory, labelling and audit procedure.
- [x] 2.2 `scripts/benchmark_io.py` with `prepare`, `validate` and `save`. It checks:
  - spans in the forms `N`, `N-M`, `N#rK` and `N#hK`, against existing lines and table rows;
  - the vocabularies;
  - product-key consistency, and no repeated relation within a product;
  - `related_products` ⇔ the four inter-product relations;
  - exact `product_name`, evidence and requirement-text parts;
  - coverage, the ledger and rule IDs;
  - English companions.

  `save` runs in one transaction with a run-key guard.
- [x] 2.3 `scripts/test_benchmark_io.py`: validation refusals (one per spec scenario) and the save SQL shape.
- [x] 2.4 `SKILL.md`, `references/benchmark-contract.md`, `USER_MANUAL.md`, `CHANGELOG.md` and `agents/openai.yaml`.

## 3. Scorer skill `score-extract-products` (.agents)

- [x] 3.1 `scripts/score_io.py`:
  - `export`: gold-run selection, predictions from `kb.products`, ledger, suggestions, warnings (mixed extractions, Pass-1-only rows), and exit codes 3/4;
  - `project`: the compact model view;
  - the compact decoder, with cause `r` and field overrides;
  - `score`: match validation, the weighted applicable checks, soft F1, product detection, groups, `report.md`, and expanded notes in en or zh-cn.
- [x] 3.2 `weights.json`, `scripts/test_score_io.py` and `scripts/test_compact_io.py`. Test perfect, wrong relation, extra relation, detection counts, invalid overrides and line gates.
- [x] 3.3 `SKILL.md`.

## 4. Backend (ChenWeb)

- [x] 4.1 Project migration `create_product_score_runs`: create `testbed.product_score_runs` (with `llm_run_id`) and the page_config rows (admin/root) for `sysadmin-llm-products-gold` and `sysadmin-llm-products-benchmark`.
- [x] 4.2 Add `goldRunTables["products"]` and `ListGoldProductOptions`, and route `GET /kb/products/gold-options`.
- [x] 4.3 Add `listGoldProducts` for `GET /kb/products?source=testbed…` (gold rows and dropped candidates), with a test.
- [x] 4.4 Write `prompts/prompt-score-extract-products-v1.md`.
- [x] 4.5 Replace the runner's `matchBasis == "provision"` branches with a `compact` flag on `scoreKind`; the existing provision score tests must still pass.
- [x] 4.6 `product_score_handler.go` (`productScoreKind` plus gold-runs, start, list, get and artifacts), routes, the runner table map and the validation-message allowlist, with a test.

## 5. Frontend (ChenWeb)

- [x] 5.1 kbService: `GoldKind` adds `products`; `listKbProducts` takes gold params; add the gold row and dropped types.
- [x] 5.2 Nav: a Products folder (Gold Products, Benchmark) on the development page, and the content-panel routes.
- [x] 5.3 `KbExtractionView`: optional record-browser params, paused message, filter snippet, and the selected record passed to `loadItems`. `ProductsView` uses them for `source="testbed"` with GoldRunFilter and a Gold/Dropped switch, read-only.
- [x] 5.4 `product-score-client.ts` and `product-score-view.svelte` on `BenchmarkReportViewer`, with a product-detection card.
- [x] 5.5 Messages in en and zh-cn. Run `bun run check` and `svelte-check`, and confirm text is selectable.

## 6. First gold run and verification

- [x] 6.1 Gold run of record 416 under rules 1.0.0: prepare → inventory → label → audit → validate → save → read back.
  — run `20261108_110147` (claude-opus-5-5): 188 rows, 160 products, 134 dropped candidates; read-back matched every field. Draft and payload in `Artifacts/0/416/benchmark-products/1.0.0/claude-opus-5-5/20261108_110147/`.
- [x] 6.2 Score record 416's existing `kb.products` rows with the scorer skill (agent matching), and record the score and main defects.
  — score 66 (soft P 77.7%, R 57.0%); product detection P 94.6%, R 76.9%, F1 84.8%. Extraction: deepseek-flash, prompt-enrich-product-mention-v4, 2026-11-07, 138 rows. Main defects: 37 gold products missed (umbrella waste classes, the 不包括 exemptions, waste streams such as 废水/渗滤液/飞灰/炉渣/沼渣/沼液); hazardous-waste examples typed regulated_object (15 rows); storage duties found in 2 of 12 rows. Artifacts in `Artifacts/0/416/extract-products-scores/1.0.0/claude-opus-5-5/20261108_110147/20261108_score1/`.
- [x] 6.3 Gold Products and Benchmark pages: run handler SQL against the dev DB, and check the pages in en and zh-cn. Run one live LLM score run from the API if a scoring model is available.
  — handler SQL on the saved 416 run returns 188 rows and 134 dropped; both pages checked with mocked APIs in en and zh-cn (`web/scripts/test-product-benchmark-pages.py`, screenshots reviewed). A live LLM score run was not done: it needs a logged-in admin session. The agent-matched score of 416 (task 6.2) is 66.
- [x] 6.4 Go tests, Python tests and `bun run check`.
  — new Go tests pass; the 4 kbhandler failures (TestMetricScoreRunnerEndToEnd, TestMetricScoreScorerParityAndInvalidMatches, TestPlanInputDeletePaths×2) predate this change. Python: 26 gold-helper and 21 scorer tests pass; provisions scorer tests unchanged. `bun run check`: no errors in this change's files; 4 errors remain in `wpi-editor.svelte`, `wpi-component-tree.test.ts` and `routes/home3/knowledge/+page.svelte` (another session's uncommitted work). The i18n check passes.

## 7. Docs and commits

- [x] 7.1 Devdoc for the products benchmark (KnowledgeStore), and a pointer in `extract-products-spec.md`.
- [x] 7.2 Commit `.agents`, ChenWeb and KnowledgeStore separately with explicit paths, then check `jj log`.
  — ChenWeb `xrtq` (split by hunk: the other session's uncommitted edits in the message files, `kb-extraction-view.svelte` and `products-view.svelte` stay in the working copy), `.agents` `nqkz`, KnowledgeStore `uquu`.
