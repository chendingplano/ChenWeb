## Context

- MinerU returns each table as one block: `table_body` (HTML), `table_caption`, `bbox`, `img_path`. There is **no per-row or per-cell bbox**. The line builder writes this as three consecutive lines: `table-caption`, `table-image`, `table`. The `table` line's content is the whole `<table>…</table>` string.
- MinerU uses `<td>` everywhere; header rows are not marked with `<th>`. `rowspan` and `colspan` are common (record 416, Table 1 has `rowspan="2"`).
- Every chunk processor sends `canonicalChunkInputText` (`input_lines.go`), a JSON array of `{flag, line_number, page_number, line_type, content}`. It is byte-identical across processors so they can reuse the DeepSeek cache.
- `metric_context` / `metric_context_en` are free text from the pass-2 enrich LLM (`prompt-enrich-metrics-v5.md`), stored in `kb.metrics`, and folded into `search_document`. That document feeds the full-text ranking and `ILIKE` matching in `metric_search_handler.go`.
- `source_line_spans` is read by about 57 Go files and 26 frontend files. `kb.search_artifacts.line_range` is an integer multirange with a GiST index, used for artifact overlap edges.
- `hydrateMatchedMetricContexts` (`review-metrics.go`) already loads ±10 lines around a matched metric for review prompts. For a table metric that includes the whole raw HTML table.

## Goals / Non-Goals

**Goals:**
- A table metric's `metric_context` can be understood alone: caption, column headers and the complete matched row.
- A metric can reference individual table rows, in a way that stays valid if a table is reparsed.
- Nothing that reads `source_line_spans` or `line_range` needs to change.
- Existing metrics can be fixed without LLM calls.

**Non-Goals:**
- Row-level PDF highlighting (no row bboxes from MinerU).
- Storing row references for provisions, inventory items, entities etc. They get the new table rendering in their input, but persisting their row references is a later change.
- Changing how non-table lines are serialized.
- Handling tables that MinerU fails to emit as HTML (image-only tables).

## Decisions

### D1. Address rows inside the line; do not split tables into lines

A row is addressed as (line number, logical row ID). The physical line stays one line.

*Alternative rejected: one line per row in the `.txt`.* This would renumber every line after the first table in every document, invalidating all stored spans across all artifact types unless everything is reprocessed. It would also give no highlighting gain, because each row line would carry the same table bbox, and it would force `rowspan` cells to be duplicated or dropped.

*Alternative rejected: span syntax `"116#r1"` inside `source_line_spans`.* The three span parsers (`parseArtifactSpan`, `parseLineSpanRange`, `normalizeSourceLineSpans`) and the frontend parse spans as integers. They would fail or silently drop the suffix, and those failures are hard to notice.

### D2. Normalized grid with stable row IDs

New file `server/api/doc-processing/table_grid.go`: `ParseTableGrid(html string) (*TableGrid, error)` using `golang.org/x/net/html` (already an indirect dependency; becomes direct).

- `rowspan`/`colspan` are expanded: a spanned cell's text is copied into every grid position it covers. Every row then has the full column count and reads on its own.
- **Header rows.** Rows containing `<th>` are header rows. If there are none, row 0 is a header row. Any immediately following row is also a header row while the header rows contain a `colspan > 1` cell (multi-level headers). Header rows are `h0, h1…`; data rows are `r1, r2…` in document order.
- **Column labels.** Each column's label joins its header cells top-down with `/`, skipping repeats (e.g. `性能指标/温度`).
- **Row hash.** `row_hash` is the first 12 hex digits of SHA-1 over the row's NFKC-normalized cell texts joined with `\x1f`. A stored reference whose hash no longer matches is treated as stale (D5).
- Parse failures return an error. Callers fall back to today's behaviour: raw HTML in input, LLM text for context.

### D3. Row-numbered table rendering in `canonicalChunkInputText`

For `line_type == "table"`, `content` becomes numbered rows instead of HTML:

```
116#h0: 序号 | 垃圾类型 | 处理模式 | 技术要求 | 适用范围
116#r1: 1 | 易腐垃圾 | 机器成肥 | 采用机械成肥设备，… | 人口密度高，…
116#r2: 1 | 易腐垃圾 | 太阳能辅助堆肥 | …
```

Rows are joined with `\n`. The `line_number`, `line_type` and JSON envelope do not change. This lives in the shared serializer, so every processor sends the same bytes and cross-processor cache reuse survives. The rendering is fully determined by the HTML, so repeat runs send identical prefixes.

*Why render spans expanded (repeating `1 | 易腐垃圾` on r2)?* The model sees each row complete, the same way it will be stored. The few extra tokens are more than repaid by dropping the HTML tags.

### D4. `source_table_rows` field

New nullable column `kb.metrics.source_table_rows JSONB`:

```json
[{"line": 116, "rows": ["r1"], "row_hash": {"r1": "3fa1c09b7e22"}}]
```

- Filled from the LLM's cited rows (D6) and checked against the grid: unknown IDs are dropped, and the hash is computed by code, never taken from the LLM.
- Metric merge (the `merge_log` path) takes the union of row references, as it already does for spans.
- A column, not an `ext_info` key, because the context builder, backfill and read APIs all query it, and `ext_info` is already crowded.

### D5. Deterministic context builder

`buildTableMetricContext(lines, metric) (ctx string, refs []TableRowRef, ok bool)` runs just before persistence, for each metric that has at least one span covering a `table` line. Rows are chosen in this order:

1. `source_table_rows` present and its hashes match → use it.
2. Evidence match: score each data row by how many of the metric's `metric_name`, `metric_value`, `threshold_or_target` and `condition` (NFKC, lower-cased, whitespace removed, ≥2 runes) occur in its cells. A single top-scoring row wins; any tie falls through to step 3 (a metric that really appears in several rows is covered by the LLM's row citations).
3. LLM-cited row IDs that exist in the grid, even without a hash match.
4. If the table has ≤5 data rows → all data rows.
5. Otherwise → no match. Keep the LLM's context and log a warning (`table metric context: no row match`) with metric_id, line and table size.

Output format, in the document's language:

```
表1 易腐垃圾及其他垃圾主要处理模式
[r1] 序号: 1 | 垃圾类型: 易腐垃圾 | 处理模式: 机器成肥 | 技术要求: 采用机械成肥设备，… | 适用范围: 人口密度高，…
```

- `label: value` pairs make each row self-describing, so no separate header line is needed.
- Each cell is capped at 400 runes and the whole context at 2000 runes, with `…` marking truncation.
- The builder **replaces** `metric_context`. `metric_context_en` comes from the LLM (D6). For English documents the store already writes no `_en` columns, so nothing extra is needed.
- If the spans also cover prose lines, the LLM context is kept and the table block appended after it; a context that already contains the block is left as is, so re-runs are idempotent.
- Caption = every `table-caption` line directly above the table (skipping `table-image`), joined in order. MinerU often emits a second caption line such as `单位为毫米`.
- A row that is one cell spanning all columns (footnotes, section rows) renders once without a column label; other `colspan` copies are skipped in `label: value` pairs. The numbered LLM rendering (D3) collapses full-width rows the same way.

### D5a. Row suffixes in spans

Because every chunk processor now sees `116#r1:` labels, any of them may echo `116#r1` into `source_line_spans`. `stripTableRowRefs` removes `#h…`/`#r…` suffixes at the top of each LLM-output span parser (metrics, provisions, entities, products, summaries), so spans stay plain line numbers.

### D6. Prompt v6

`prompts/prompt-enrich-metrics-v6.md` = v5 plus:

- A description of the row-numbered table rendering.
- A new output field `source_table_rows: ["116#r1"]`, required when a metric comes from a table line.
- `context` guidance: for tables, cite rows rather than paraphrase them; the system rebuilds the source-language context. `context_en` must be an English rendering of the caption plus the cited rows' `header: value` pairs.

`normalizeMetricList` parses `"116#r1"` strings into `{line, rows}`.

### D7. Neighbor rows are added at read time, never stored

`metric_context` feeds `search_document`. If neighbor rows were stored there, a search for 太阳能辅助堆肥 would return the 机器成肥 metric 比能耗. So:

- Stored: caption + matched row(s) only.
- Read time: a helper `tableContextWindow(grid, refs, radius=1)` returns header + matched rows + ±1 data rows with the matched rows marked.
  - `hydrateMatchedMetricContexts`: for a span line of type `table` with row refs, the raw HTML line is replaced by this window in row-numbered form.
  - `GET /kb/metrics?input_record_id=N` (`ListMetrics`, the list the PDF metric popup reads) and the post-update reload (`fetchMetricByID`) return `source_table_rows`; `ListMetrics` also returns `table_context: [{line, caption, columns, rows[{id, cells, full_width, header, matched}]}]`, reading the record's line file once and only when some metric has row refs. The popup in `metric-mgmt-view.svelte` shows it as a small table with the matched row highlighted.
  - Review hydration fetches `source_table_rows` for the matched metrics in one extra query rather than widening the three matched-metric queries. Table lines without row refs are shown as whole numbered tables instead of raw HTML (this also applies to the provisions and inventory reviewers, which share `artifactSourceContextLines`).

### D8. Backfill command

A one-off subcommand (`server/cmd`, same pattern as other maintenance commands):

- Selects metrics whose spans cover a `table` line, loads each record's lines once, runs the D5 builder, and writes `metric_context`, `source_table_rows` and a rebuilt `search_document`.
- Leaves `metric_context_en` alone, because there is no LLM call.
- Rewrites `search_document` by replacing the old context text inside it with the new one, so object/category/English parts written at extraction time are kept (appends when the old text is not found). `trg_refresh_metric_search_columns` refreshes `search_vector`.
- Has a `--dry-run` that prints before/after for a sample, a `--record-id` filter, and a summary count of matched / whole-table / unmatched metrics.

## Risks / Trade-offs

- [Header detection is wrong for key-value tables with no header row] → row 0 becomes a header, which still shows as `label: value` pairs. The table stays readable, only the labels are odd. Unmatched and whole-table counts from the backfill dry run show how often this happens.
- [Prompt prefix changes for every table chunk] → the first run of every chunk processor after deploy gets about zero DeepSeek cache hits. Deploy once, not in stages, so the cost is paid once.
- [Processors whose prompts assume HTML tables] → search the prompts for "HTML"/"<table>" guidance and update wording in the same change. The row format is plainer text, so models read it at least as well.
- [Evidence matching picks the wrong row when rows share text] → a single row must win on score; ties only count if they contain the exact same evidence; otherwise fall through to the LLM row ID or no match. Unit tests use Table 1 of record 416, where 易腐垃圾 appears in three rows.
- [`metric_context` and `metric_context_en` disagree after backfill] → known and temporary: the English text stays the old breadcrumb until the record is re-run. Listed as an open question.
- [Table split across pages is two `table` lines] → row IDs are per line and each line is handled on its own. Borrowing the header of the first part for a headerless continuation was considered but **not implemented**; revisit if continuation tables show up with bad labels.

## Migration Plan

1. Goose migration adds `kb.metrics.source_table_rows JSONB NULL`; the live dev server applies it on restart.
2. Ship the grid parser, serializer change, builder, prompt v6 and read-time window together.
3. Run the backfill with `--dry-run` on record 416 and check `416_mtc_31`, then run it for all records.
4. Rollback: revert the code; the column is nullable and can stay. Backfilled `metric_context` values remain better than before, so there is no need to revert data.

## Open Questions

- Should the backfill also regenerate `metric_context_en` with a small, cheap translation call, or wait for records to be re-run?
- Should other processors (provisions first) persist row references in a follow-up change?
