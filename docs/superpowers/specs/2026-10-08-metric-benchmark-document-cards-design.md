# Metric Benchmark Document Cards

## Goal

Make document search results in the Metrics Benchmark picker easier to scan and adapt to the available width.

## Design

Render results in an equal-width responsive grid. Each card shows `kb.inputs.title`, `kb.inputs.doc_no`, and `kb.inputs.file_name` on separate single lines. Long values truncate with an ellipsis and expose their full value on hover. The existing document selection behavior remains unchanged. Use the current English and Chinese Paraglide messages for any user-facing fallback text.

## Implementation and verification

The change belongs in `web/src/lib/components/home3/metric-score-view.svelte`; `InputRecordSummary` already includes all three fields. Run the ChenWeb frontend checks (`bun run check`) after the UI change.
