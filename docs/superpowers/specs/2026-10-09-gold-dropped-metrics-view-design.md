# Gold and Dropped Metrics View Design

## Goal

Let users switch between retained gold metrics and metrics recorded in `kb.metrics_dropped`, and optionally show a PDF info card for every metric in the active view.

## Behavior

- Replace the standalone “Show dropped” checkbox with a View selector beneath Order by.
- The selector has “View gold metrics” (default) and “View dropped metrics”. These modes are mutually exclusive.
- “View gold metrics” lists regular metrics only. “View dropped metrics” lists dropped rows only, using the existing include-dropped API response and its `dropped` marker.
- Add an independent “Show all metrics info boxes” checkbox, unchecked by default. When enabled, PDF info cards are rendered for all metrics in the active view, subject to the current metric filters, rather than only for the selected metric.
- When the checkbox is off, preserve the existing selected-metric PDF card behavior.
- Controls and labels use Paraglide messages in English and Chinese.

## Implementation shape

Keep view state and list filtering in `metric-mgmt-view.svelte`. Extend the shared PDF viewer's floating overlay rendering to support multiple cards on a page while retaining its existing single-card behavior for other consumers. In all-card mode, assign each metric to the page of its first resolvable source span; omit cards without a resolvable source page. Place cards in a deterministic stack near the upper-right, ordered by source line and then metric ID. Bound the stack height and allow it to scroll when it exceeds the available page height.

## Validation

Run the ChenWeb web checks and inspect the diff. Verify default mode, switching between modes, checkbox behavior, translations, and that existing single-card viewer consumers keep their behavior.
