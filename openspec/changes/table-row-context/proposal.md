## Why

Metrics taken from tables get a `metric_context` that is too thin to understand on its own. Example: metric `416_mtc_31` (比能耗, record 416) has context `表1 易腐垃圾及其他垃圾主要处理模式 易腐垃圾-机器成肥`. The source row's 技术要求 and 适用范围 cells are missing, so a reader cannot tell what is actually required or where it applies.

The cause is not retrieval. MinerU emits a whole table as **one input line** of HTML (record 416, line 116), so `source_line_spans: ["116"]` already covers the full table. `metric_context` is free text written by the pass-2 LLM, and `prompt-enrich-metrics-v5.md` gives it no guidance, so the model writes a short breadcrumb. There is also no way to point at a single row: the smallest address we have is the whole table line.

## What Changes

- **Table grid parsing.** A new parser turns a table line's HTML into a normalized grid: `rowspan`/`colspan` are expanded so every row is self-contained, and rows get stable logical IDs (`h0, h1…` for header rows, `r1, r2…` for data rows).
- **Row-numbered LLM input.** `canonicalChunkInputText` renders table lines as caption plus numbered rows (`116#h0: 序号 | 垃圾类型 | …`, `116#r1: 1 | 易腐垃圾 | …`) instead of raw HTML. Line numbers do not change. Every chunk-based processor shares this serialization, so they all see the new form and cross-processor cache reuse is kept.
- **Row references on metrics.** A new field `source_table_rows` (e.g. `[{"line":116,"rows":["r1"],"row_hash":"…"}]`) records which rows a metric came from. `source_line_spans` and `line_range` are **unchanged**, so the existing span consumers are unaffected.
- **Deterministic `metric_context` for table metrics.** For a metric whose source is a table line, code builds `metric_context` as caption + header row(s) + matched row(s), rendered as `header: value` pairs, and replaces the LLM's text. Rows are found from `source_table_rows`, then by matching evidence strings against cells, then from an LLM-reported row index. If the table has ≤5 data rows, the whole table is used.
- **Neighbor rows at read time.** The ±1 data rows around the matched row are **not** stored in `metric_context`, which feeds `search_document` and search ranking. They are added when context is read (doc-review hydration and the metric detail UI).
- **Prompt update.** The enrich prompt (new `prompt-enrich-metrics-v6.md`) asks the model to cite rows it used as `source_table_rows` and gives explicit guidance for `context` / `context_en`.
- **Backfill.** A one-off command rebuilds `metric_context` and `source_table_rows` for existing table-sourced metrics from stored lines, without LLM calls.

## Capabilities

### New Capabilities
- `table-row-addressing`: parsing single-line HTML tables into a normalized row grid, logical row IDs, row hashes, and the row-numbered rendering of tables in chunk input.
- `table-metric-context`: how `metric_context` is built for table-sourced metrics, how rows are matched, what is stored vs. added at read time, and the backfill of existing metrics.

### Modified Capabilities
<!-- None: no existing spec in openspec/specs/ covers metric extraction or chunk serialization. -->

## Impact

- **Code:** `server/api/doc-processing/input_lines.go` (chunk serialization), `extract-metrics.go` (normalization, persistence, context builder), new table-grid package or file in `doc-processing`, `server/api/doc-reviews/review-metrics.go` (read-time neighbor rows), metric detail/tooltip components in `web/src/lib/components/home3/`.
- **Database:** new nullable column `kb.metrics.source_table_rows JSONB` (goose migration). `search_document` is rebuilt for affected rows because `metric_context` changes.
- **Prompts:** new `prompts/prompt-enrich-metrics-v6.md`; `extract-metrics.go` switches to it.
- **LLM cost/caching:** the chunk prefix changes for every chunk containing a table, so the first run of every chunk processor after deploy gets about zero DeepSeek cache hits. Tokens per table go **down**, because row text replaces HTML tags.
- **Other processors** (provisions, inventory items, entities, …) see the new table rendering but do not store row references yet; that is left for later changes.
- **Not affected:** `source_line_spans` format, `line_range` / GiST overlap edges, the span parsers, and PDF highlighting (MinerU only gives a table-level bbox, so highlighting stays table-level).
