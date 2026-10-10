# Gold and Dropped Metrics View Design

## Goal

Let users switch between retained gold metrics and metrics recorded in `kb.metrics_dropped`, and optionally show a PDF info card for every metric in the active view.

## Behavior

- Replace the standalone “Show dropped” checkbox with a View selector beneath Order by.
- The selector has “View gold metrics” (default) and “View dropped metrics”. These modes are mutually exclusive.
- “View gold metrics” lists regular metrics only. “View dropped metrics” lists dropped rows only, using the existing include-dropped API response and its `dropped` marker.
- Dropped metrics preserve `source_table_rows` from their saved extraction data, normalized to the same row-reference objects as gold metrics. Selecting a table metric highlights its cited rows and builds its table context through the existing shared path; metrics without row references retain the whole-table fallback.
- Add an independent “Show all metrics info boxes” checkbox, unchecked by default. When enabled, PDF info cards are rendered for all metrics in the active view, subject to the current metric filters, rather than only for the selected metric.
- When the checkbox is off, preserve the existing selected-metric PDF card behavior.
- The Metrics sidebar filter block (name, ID, keyword, description, confidence, Order by, View, and all-info-box controls) sits inside a “Filters” disclosure, folded by default on page load. Opening or folding it preserves all control values and their effects on the current view. The disclosure supports mouse and keyboard interaction.
- Controls and labels use Paraglide messages in English and Chinese.

## Implementation shape

Keep view state and list filtering in `metric-mgmt-view.svelte`. Extend the shared PDF viewer's floating overlay rendering to support multiple cards on a page while retaining its existing single-card behavior for other consumers. In all-card mode, assign each metric to the page of its first resolvable source span; omit cards without a resolvable source page. Place cards in a deterministic stack near the upper-right, ordered by source line and then metric ID. Bound the stack height and allow it to scroll when it exceeds the available page height.

## Validation

Run the ChenWeb web checks and inspect the diff. Verify default mode, switching between modes, checkbox behavior, translations, and that existing single-card viewer consumers keep their behavior.

## Metric Benchmark record selection

- The testbed page header is “Metric Benchmark” (“指标基准” in Chinese).
- Initial load defaults to the newest skill version but shows no input records and displays “Select a model” until a model is chosen.
- Selecting a model lists `kb.inputs` with benchmark metrics for the selected skill version and model.
- Manually changing the skill version clears the model and the selected document, then lists all inputs benchmarked at that version across models. The model selector shows “All models”; selecting a model narrows this list. Gold metrics still require a model selection.
- Each document card shows every distinct matching skill version/model pair. Documents remain unique and pagination counts documents, not benchmark runs.
- The existing `GET /api/v1/kb/inputs?has_gold_metrics=true` response includes an optional `gold_metric_runs` array per record, containing `skill_version` and `model_name`. These pairs honor the same version/model filters as the input list and are fetched only for the authorized page of documents.
- Record-ID retrieval and search selections honor the benchmark filters. Changing the benchmark selection clears the current document and its detail panels; older asynchronous responses cannot restore an obsolete selection.

Validation: run `go test ./server/api/kbhandler` (including version-only and version/model metadata cases) and `bun run check` for Svelte and bilingual message checks. Existing specifications for benchmark scoring document cards describe a different page and remain unchanged.
