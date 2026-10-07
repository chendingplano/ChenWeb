# Benchmark Results and PDF Viewer Design

## Goal

In the Metric Score benchmark page, show the current benchmark result content beside the PDF for the associated `kb.inputs` record. Selecting a result entry navigates the PDF to its cited page and highlights its cited source content.

## Layout

- Keep the existing benchmark result content in the left panel.
- Add a right panel using the existing reusable `PdfViewWindow` component used by Gold Metrics.
- Give each panel its own vertical scrolling container so scrolling through results does not move the PDF and scrolling the PDF does not move the results.
- Keep the PDF panel available for the active input record even when a benchmark run has no score, or the score contains no metric rows. Use the selected input record when no result run is open, and the open run's `input_record_id` when a result is open.
- When no input record is selected, show a localized empty state in the PDF panel.
- On narrow viewports, stack the two panels while retaining independent scrolling where space permits.
- When the active input record changes, load the new PDF, reset the PDF page to page 1, and clear the selected entry and its highlights. When the open benchmark run changes for the same input record, also clear the selected entry and highlights.

## Source navigation and highlighting

- Make each matched pair, missed metric, and false-positive entry selectable from the left panel.
- Resolve a matched pair's `gold` and `pred` IDs from `detail.input.gold` and `detail.input.predictions`; resolve a missed entry's `gold` ID from `detail.input.gold`, and a false-positive entry's `pred` ID from `detail.input.predictions`. Match IDs against each record's `metric_id`. If a referenced ID has no corresponding record, skip that record and use any other resolvable record for the entry.
- Read `source_line_spans` from each resolved record. Accept the existing formats: individual line numbers, numeric strings, inclusive `start:end` / `start-end` / `start,end` ranges, and objects containing `line_number`, `line`, `line_no`, or `lineNo`. Expand ranges with a 200-line safety cap, then map line numbers to pages using `getRawLines(activeInputId)`; retain only line numbers present in the raw lines.
- Reuse the shared PDF viewer and its existing `renderHighlights` and `highlightVersion` props. Raw-line `coords` are `[x1, y1, x2, y2]` bounding boxes in a 1000-by-1000 top-left-origin coordinate space. For each resolved line with at least four finite coordinate values, scale x coordinates by `viewport.width / 1000` and y coordinates by `viewport.height / 1000`, then draw the resulting rectangle with the established `.pdf-highlight` style. When selecting an entry, navigate to the page of its first resolvable source line; highlight all resolvable source lines, including those on other pages.
- For a matched pair, combine available source lines from the gold and prediction records and sort them by line number; use the first resolvable line in that order for navigation. For missed and false-positive entries, sort the record's source lines by line number and use its first resolvable line for navigation.
- Every entry selection replaces the prior highlight set. If the selected entry has no usable source spans, unresolved IDs, or raw-line lookup fails, clear prior highlights and keep the current PDF page instead of navigating to an unrelated page. A raw-line lookup failure must not affect score rendering.

## Data flow and component boundary

`MetricScoreView` owns the active input record, selected result entry, PDF page, raw-line lookup, and highlight selection. It passes `inputId`, the file URL (`/api/v1/kb/inputs/{id}/file`), bound `page`, and a stable selection-based `highlightVersion` to the existing `PdfViewWindow`, along with a `renderHighlights` callback. `highlightVersion` is already an input on `PdfViewWindow` and `SharedPdfViewer`; no new PDF viewer API is required. `PdfViewWindow` forwards page binding and highlighting to `SharedPdfViewer`, which scrolls to the bound page and repaints overlays when the highlight version changes. The existing components remain the rendering and navigation implementation; this feature does not create a second PDF renderer.

The viewer's existing PDF loading state and error presentation handle a missing or unreadable PDF. If the active input ID is unavailable, show the localized empty state. If raw lines cannot be fetched, keep the PDF available, clear highlights, and leave the current page unchanged.

## Internationalization and accessibility

- Route all new visible labels, empty-state text, and accessible names through Paraglide with English and Chinese messages.
- Keep text selectable in both panels.
- Give selectable result entries keyboard-accessible button behavior and a visible focus state.

## Acceptance criteria

1. The benchmark detail appears in the left panel and the input record's PDF appears in the right panel using the existing PDF viewer.
2. Each panel scrolls independently.
3. The PDF remains visible when a run has no metrics, and uses the active input record.
4. Selecting a matched, missed, or false-positive entry moves the PDF to the page containing its source and highlights the corresponding available content.
5. Missing source spans or line data do not break the benchmark view or move the PDF to the wrong page.
6. New UI text is localized in English and Chinese and all panel content remains selectable.
7. Changing the active input record resets the viewer to page 1 and clears highlights; changing runs for the same record clears stale highlights.

## Scope

This change is limited to the benchmark results viewer and the existing PDF viewer integration. It does not change score generation, scoring semantics, or PDF rendering internals beyond what is needed to support the existing `PdfViewWindow` interface.
