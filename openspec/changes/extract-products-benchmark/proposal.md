## Why

`extract_products` has no answer key. Metrics, references, summaries and provisions are each
measured against an independently reviewed gold set (System Admin › Benchmark), but product
extraction is still judged by spot checks and distinct-mention counts. The products spec itself
asks for more: every prompt change "must be A/B'd", and merged-pass mode "has to be re-measured"
for every new model. Neither is possible without a gold set.

## What Changes

- New skill `.agents/skills/extract-products-benchmark`. It writes gold product relations for one
  `kb.inputs` record under frozen, versioned annotation rules (`rules/1.0.0.md`, SHA-256 in
  `rules/versions.json`) and saves an immutable run to new `testbed` tables. It works the same way
  as `extract-metrics-benchmark` and `extract-provisions-benchmark`. Rules 1.0.0 contain the
  user's rulings of 2026-11-08:
  - **Gold unit:** one row per (product, relation_type) pair, like `kb.products`. Rows of the
    same product share a gold product key, so detection can be scored on its own.
  - **Producthood:** waste categories and waste streams (可回收物, 易腐垃圾, 有害垃圾, 废玻璃,
    餐厨垃圾) are products (`product_class` or `material`). This follows the extractor contract
    and the spec's v2 rebalance. Places, facilities and works are not products. A citation line
    is never evidence on its own.
  - **Graded fields:** besides product name, product type, relation type and evidence lines, a
    gold row carries obligation level, requirement text (exact source text), conditions,
    parameters, responsible actor and related products.
- The first gold run is record 416 (DB33/T 2030—2018), the record the products spec's own
  measurements use.
- New project migration for the gold tables: `testbed.product_benchmark_runs`,
  `testbed.products` and `testbed.products_dropped`.
- New skill `.agents/skills/score-extract-products`. It scores a record's `kb.products` rows
  against one gold run, from 1 to 100, using soft F1 over weighted field checks. It also reports
  product-detection precision and recall from the same matches. An agent or LLM pairs the rows;
  the script checks the pairs and does the arithmetic. It uses the compact matching contract that
  the provisions scorer introduced.
- New project migration for `testbed.product_score_runs` (score-run history), plus
  `kb.page_config` access rows (admin/root) for the two new menu entries.
- Backend:
  - `GET /kb/products/gold-options`.
  - `GET /kb/products?source=testbed&skill_version&model_name&run_id` returns one run's gold
    rows and dropped candidates.
  - `gold_kind=products` in the kb inputs list.
  - `/kb/product-scores` (models, gold-runs, list, start, get, artifacts) runs the scorer through
    the shared score runner.
  - Matching prompt `prompts/prompt-score-extract-products-v1.md`.
  - The runner's compact-contract branches, which today test `matchBasis == "provision"`, become
    a `compact` flag on `scoreKind` that both provisions and products set.
- Frontend (development page only): System Admin › Benchmark gets a **Products** folder with two
  entries:
  - **Gold Products**: the existing Products view, read-only, over a chosen gold run, with a
    Gold/Dropped switch.
  - **Benchmark**: a scoring page on `BenchmarkReportViewer`, like Provisions › Benchmark, with a
    product-detection score card.
  - All text goes through Paraglide (en + zh-cn).

## Capabilities

### New Capabilities
- `product-gold-benchmark`: annotating, validating and storing gold product relations under versioned rules (skill and testbed tables).
- `product-benchmark-scoring`: scoring `kb.products` against a gold run (scorer skill, score-run API and table, Benchmark page, Gold Products page).

### Modified Capabilities
<!-- none: no capability in openspec/specs covers benchmarks; extract-provisions-benchmark is not archived yet -->

## Impact

- `.agents` repo: two new skills (Python, with tests).
- ChenWeb:
  - `project_migrations/`: two migrations.
  - `server/api/kbhandler/`: new `gold_products.go` and `product_score_handler.go`; small edits
    to `gold_metrics_options.go` (`goldRunTables`), `products_handler.go` (testbed branch) and
    `metric_score_runner.go` (`compact` flag, table map).
  - `server/api/routes.go`.
  - `prompts/prompt-score-extract-products-v1.md`.
  - `web/src/lib/components/home3/`:
    - `nav-rail.svelte` and `content-panel.svelte`;
    - `products-view.svelte` and `kb-extraction-view.svelte` (optional gold-browser props only);
    - new `product-score-view.svelte` and `product-score-client.ts`.
  - `web/src/lib/services/kbService.ts`.
  - `web/messages/*.json`.
- KnowledgeStore: devdoc for the products benchmark, and a pointer from `extract-products-spec.md`.
- No change to `extract_products`, its prompts or `kb.products`.
