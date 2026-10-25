## 1. API

- [x] 1.1 Trace response adds `result` (shared column list and scan with the run loader) and `lexical_backend`; handler test updated

## 2. Shell

- [x] 2.1 `dashboard.svelte`: read `analyze` from the loaded URL; clear it and drop it from the URL when another page is chosen; keep the metric-details snippet for the run page's child
- [x] 2.2 `content-panel.svelte`: render the Analyze view for the run page's child when `analyze` is set; no footer, full height

## 3. Analyze tab

- [x] 3.1 `hybrid-search-benchmark-trace-log.svelte` (content panel) from the dialog's markup: header, question search, effort analysis and attempts with verdicts and matching metrics, clickable metric IDs, selected-metric marking, no-trace state
- [x] 3.2 `hybrid-search-benchmark-analyze-view.svelte`: resizable two-panel layout; load the trace; on metric click load metrics + raw lines per record (cached), `PdfViewWindow` with page jump, line and table-row highlights; right panel `MetricDetailVisualizer` through `onMetricDetailsChange`; not-found message
- [x] 3.3 Run page: Analyze becomes a new-tab link; delete the dialog component; service types; `hsba_*` keys in both message files (remove unused); `bun run check`

## 4. Verification and docs

- [x] 4.1 Browser check of a run 21 question (new tab, menu, click metrics in two documents, page jump, highlights, details panel, leave via menu) or, without a login, the endpoint plus `bun run check` and an explicit note
- [x] 4.2 Trace change spec: remove its "Analyze dialog" requirement (replaced here); devdoc `2026102501`, spec `doc-2026060201` §13.2

Notes: no browser login was available to the agent, so 4.1 ran the live trace endpoint against the
dev database (result 674 of run 21: HTTP 200 with result, backend and queries), `bun run check` (0
errors, i18n ok) and the server tests; the in-browser walk-through is left to the user. While
checking the click path, 852 of the 1,271 metric IDs in runs 20/21 traces were not in `kb.metrics`:
5,621 of 8,281 metric index rows belong to 49 records with no `kb.metrics` rows (stale index
entries). Out of scope here; documented in devdoc 2026102501.
