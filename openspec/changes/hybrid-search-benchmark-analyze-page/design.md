## Context

The development page is one SvelteKit route (`/development`) rendering `dashboard.svelte`: site
header, `nav-rail.svelte` (menu tree), `content-panel.svelte` (working area, picked by the `view`
and `child` URL parameters) and `context-shelf.svelte` (right panel). A view can fill the right
panel by passing a snippet to `onMetricDetailsChange`; the Metrics page does this with the Metric
Detail Visualizer. The menu restores its selection from `window.location` on mount and only
resolves children that exist in the tree. SvelteKit's `replaceState` (used on every menu click)
updates `window.location` but not `page.url`.

Each question's trace (openspec `hybrid-search-benchmark-trace`) is served by
`GET /api/v1/kb/hybrid-search-benchmark/results/:id/trace`; today it is drawn in a dialog.

## Goals / Non-Goals

**Goals:** a new tab per Analyze click with the same shell; a log of the question's search with
clickable metric IDs; the metric's document at the right page with its lines highlighted; the
metric's details in the right panel; reuse the existing PDF viewer and Metric Detail Visualizer.

**Non-Goals:** no new menu item; no editing; no new PDF viewer or highlight engine; no change to
how traces are stored.

## Decisions

### 1. URL: the run page's child plus `analyze`

The Analyze button is a link (`<a target="_blank" rel="noopener">`, so middle-click and "open in
new tab" work too) to the current URL with `analyze=<result id>`:
`/development?view=system-admin&child=sysadmin-hybrid-search-benchmark-run&analyze=42`.

- The menu tree resolves `child` as usual, so the header and menu are the real ones, with Run
  Benchmark selected.
- `dashboard.svelte` reads `analyze` once from the loaded URL into state and passes it to
  `content-panel.svelte`, which renders the Analyze view instead of the run view for that child.
- Choosing another page in the menu clears the state and removes `analyze` from the URL, so
  coming back shows the run page. Choosing Run Benchmark itself keeps the analysis (the tab exists
  for it; closing the tab returns to the original one).

*Alternatives:* a separate route `/development/benchmark-analyze/[id]` would need the dashboard to
take content from outside `content-panel` and to navigate away on menu clicks; a hidden menu child
is not supported by the menu tree.

### 2. Working area: content log + PDF panel

`hybrid-search-benchmark-analyze-view.svelte` owns the layout: two panels side by side with a
draggable, keyboard-resizable divider (as in `benchmark-report-viewer.svelte`), stacked on narrow
screens. It fills the viewport (no footer) and each panel scrolls on its own.

`hybrid-search-benchmark-trace-log.svelte` is the content panel, built from the dialog's markup:

1. Header: question, target (clickable), hop type, language, the four ranks, run and search run ID.
2. *Question search*: the question's query (seq 1): keywords, error, four lists.
3. Medium/High runs: *Effort analysis* (strategy, inferred need), then each *attempt*: its queries
   (each collapsible with keywords and four lists), the judge's verdict (found or not), reason and
   matching metrics (clickable), then the effort rank. Attempts come from the result's
   `effort_trace`; queries are matched to attempts by the query rows' `attempt`. The fallback
   search after a failed analysis appears as its own entry.

Every metric ID (target, list rows, matching IDs) is a button; the selected metric's rows are
marked in every list, separately from the target highlight.

### 3. Clicking a metric

The input record comes from the metric ID (`recordIdFromMetricId`, canonical
`<record>_mtc_<n>`; the Metric Ontology Explorer uses the same rule). The view loads, once per
record and cached for the tab, `listKbMetrics(record)` and `getRawLines(record)`, then:

- **PDF panel**: `PdfViewWindow` with `fileUrl=/api/v1/kb/inputs/<record>/file`,
  `tableReferences = metric.source_table_rows`, and a `renderHighlights` that draws the metric's
  source lines (`lineNumbersFromSpans(metric.source_line_spans)`) from the raw lines' coordinates,
  skipping lines whose table rows the viewer located itself — the same drawing as the Metrics and
  Review Metrics pages. The page moves to the first source line's page (`resolveMetricSpans`). The
  viewer is keyed by record, so switching documents reloads it and switching metrics within a
  document only repaints.
- **Right panel**: a snippet passed through `onMetricDetailsChange` renders
  `MetricDetailVisualizer` with the metric and the record's raw lines (so Grounding is filled),
  styled through its CSS variables. Before any click it shows a hint. `dashboard.svelte` keeps the
  snippet for the run page's child as it does for the Metrics pages.

A metric ID with no record, or no longer in `kb.metrics`, shows a message in the PDF panel and no
details.

### 4. Trace API returns the result

`GET …/results/:id/trace` adds `result` (the same object as in a run's `results`, including
`effort_trace`) and `lexical_backend` (from the run), so the tab needs one request. The result
loader shares its column list and scan with the run loader.

## Risks / Trade-offs

- [Pages that hold `analyze` in a bookmark] → still work: the parameter is read on load.
- [Large documents: `listKbMetrics` returns every metric of the record] → loaded once per record
  per tab; the Metrics page loads the same list.
- [Metric IDs outside the canonical format] → no document opens; the panel says so.
