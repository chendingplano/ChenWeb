## Why

The Analyze dialog shows a question's candidate lists, but only as IDs and labels: to see what a
candidate metric actually says, or why the target lost to it, you have to find the metric's
document and page by hand. A dialog also hides the page behind it and cannot sit next to a PDF.

## What Changes

- **Analyze opens a new browser tab** instead of a dialog: the same development page (site header,
  menu tree, Run Benchmark selected), with `analyze=<result id>` in the URL. **BREAKING** for the
  Analyze dialog, which is removed.
- The tab's working area has two resizable panels:
  - **Content panel**: the question's log — its target and ranks, then each query it searched
    (the question; for Medium/High runs the effort analysis and every attempt with its queries,
    the judge's verdict, reason and matching metrics), each query with its BM25 keywords and four
    candidate lists. Every metric ID in the log is clickable.
  - **PDF panel**: the reusable PDF viewer (`pdf-view-window.svelte`), empty until a metric is
    clicked.
- **Clicking a metric** loads its document in the PDF panel, moves to the page of its first source
  line and highlights its source lines (table rows where they were located), and shows the metric
  in the right panel with the Metric Detail Visualizer (module `20261025-01`).
- The trace API also returns the result itself (question, target, ranks, effort trace) and the
  run's lexical backend, so the tab can render from the result ID alone.

## Capabilities

### New Capabilities
- `hybrid-search-benchmark-analyze-page`: the Analyze tab — URL, shell, content log, clickable
  metrics, PDF panel and metric details panel.

### Modified Capabilities
<!-- none in openspec/specs. The unarchived change hybrid-search-benchmark-trace's "Analyze dialog"
     requirement is replaced by this change and removed from that change's spec. -->

## Impact

- Web: `hybrid-search-benchmark-view.svelte` (Analyze link), new
  `hybrid-search-benchmark-analyze-view.svelte` (tab) and `hybrid-search-benchmark-trace-log.svelte`
  (content panel); `hybrid-search-benchmark-analyze.svelte` (dialog) deleted; `content-panel.svelte`
  and `dashboard.svelte` (route the tab, keep the right panel's metric details, drop `analyze` when
  another page is chosen); service types; `web/messages/*.json` (`hsba_*`).
- Reused, unchanged: `pdf-view-window.svelte`, `shared-ui/metric-detail/` (Metric Detail
  Visualizer), `metric-ontology-explorer/metric-source-spans.ts`, `listKbMetrics`, `getRawLines`.
- Server: `hybrid_search_benchmark_trace.go` response gains `result` and `lexical_backend`.
- Docs: devdoc `2026102501`, spec `doc-2026060201` §13.2, the trace change's spec.
