## 1. Table grid parser

- [x] 1.1 Add `server/api/doc-processing/table_grid.go` with `ParseTableGrid(html)` using `golang.org/x/net/html` (promote to a direct dependency in `go.mod`), expanding `rowspan`/`colspan` into a full grid
- [x] 1.2 Implement header detection (`<th>` rows; else row 0; plus the next row while header rows contain a `colspan > 1` cell), row IDs `h0…`/`r1…`, and column labels joined top-down with `/`
- [x] 1.3 Implement NFKC-normalized row hashing (SHA-1 over cells joined with `\x1f`, first 12 hex)
- [x] 1.4 Unit tests: record 416 Table 1 (rowspan rows carry 序号 `1` and 垃圾类型 `易腐垃圾`), a colspan multi-level header, determinism, hash change on cell edit, malformed HTML returns an error

## 2. Row-numbered chunk input

- [x] 2.1 In `input_lines.go`, render `table` lines as `<line>#<row id>: cell | cell | …` rows inside `markedLinesToJSON`, `blockLinesToJSON` and `rawLinesToJSON`, falling back to raw HTML on parse errors
- [x] 2.2 Test that serialization is byte-identical across calls and that line numbers and non-table lines are unchanged
- [x] 2.3 Search `prompts/` for guidance that assumes HTML tables (`<table>`, `HTML`, `<td>`) and update wording in the prompts in active use

## 3. Storage

- [x] 3.1 Goose migration in `project_migrations/`: `ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS source_table_rows JSONB NULL` (use the `db-migration` skill)
- [x] 3.2 Add `source_table_rows` to `ensureMetricsTable`, `SaveMetrics`, `UpsertMetrics`, `GetMetricsByInputRecordID` and `loadPersistedMetricArtifactRows` in `extract-metrics.go`
- [x] 3.3 Union `source_table_rows` in the metric merge path (the path that writes `merge_log`)

## 4. Prompt v6 and parsing

- [x] 4.1 Create `prompts/prompt-enrich-metrics-v6.md` from v5: describe the numbered-row table rendering, add `source_table_rows: ["116#r1"]`, add `context`/`context_en` guidance for tables
- [x] 4.2 Add `source_table_rows` to the schema in `buildMetricRelationBatchPrompt` and switch the prompt reference in `extract-metrics.go` to v6
- [x] 4.3 Parse `"<line>#<row id>"` strings in `normalizeMetricList` into `{line, rows}`, dropping malformed entries

## 5. Context builder

- [x] 5.1 Implement `buildTableMetricContext` with the D5 selection order (valid refs → single-winner evidence match → LLM row IDs → whole table if ≤5 data rows → keep LLM text and log a warning via `CreateDefaultLogger`)
- [x] 5.2 Render caption + `[row id] label: value | …` rows with 400-rune cell and 2000-rune total caps; set `metric_context_en` to the same text when the document language is English
- [x] 5.3 Call the builder before persistence for metrics whose spans cover a `table` line, and write computed hashes into `source_table_rows`
- [x] 5.4 Unit tests: 416 比能耗 → row r1 with full 技术要求/适用范围 cells; evidence tie across 易腐垃圾 rows is not a single winner; bad LLM row ID dropped; small-table fallback; length caps; non-table metric untouched

## 6. Read-time neighbor rows

- [x] 6.1 Add `tableContextWindow(grid, refs, radius=1)` returning header + matched rows + ±1 data rows with a `matched` flag, clipped at table edges
- [x] 6.2 In `hydrateMatchedMetricContexts` (`review-metrics.go`), replace a table span line's raw HTML with the window in numbered-row form when `source_table_rows` is present
- [x] 6.3 Return `source_table_rows` and `table_context` from `ListMetrics` (the list the PDF popup reads) and `fetchMetricByID` in `metrics_handler.go`
- [x] 6.4 Show `table_context` as a small table with the matched row highlighted in the metric tooltip/detail UI (`web/src/lib/components/home3/`); add a component test
- [x] 6.5 Test that `search_document` for `416_mtc_31` does not contain text from rows r2/r3

## 7. Backfill

- [x] 7.1 Add `server/cmd/table-metric-context-backfill` (pattern of `metric-contract-backfill`) with `--dry-run` and `--record-id`; load each record's lines once, run the builder, update `metric_context`, `source_table_rows`, `search_document`; leave `metric_context_en` alone
- [x] 7.2 Print matched / whole-table / unmatched counts; verify the second run changes 0 rows
- [x] 7.3 Dry-run on record 416, check `416_mtc_31`, then run on all records

## 8. Verification and docs

- [x] 8.1 `go test ./server/api/doc-processing/... ./server/api/doc-reviews/... ./server/api/kbhandler/...` and the web tests pass; `mise build-server` succeeds
- [ ] 8.2 Re-run extract_metrics on record 416 and confirm the new metric has `source_table_rows` and a row-complete `metric_context`
- [x] 8.3 Write a devdoc on table row addressing (`create-devdoc` skill) and record which docs are now stale
