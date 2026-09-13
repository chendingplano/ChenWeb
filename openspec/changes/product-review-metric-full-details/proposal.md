## Why

The Product Review page's METRIC DETAILS panel (`product-metric-review-view.svelte`) shows a
flattened, review-scoped summary of a matched metric (artifact id, name, subject, value,
threshold, score, inclusion reason, etc.). The richer, curated record — full metric fields,
metadata, source context, grounding line spans, and reasoning tags — already exists in the
Knowledge System's Metrics workspace (`metric-mgmt-view.svelte`, reached via Knowledge Base →
Metrics), but a reviewer has no way to see it for the metric they're currently looking at without
leaving Product Review and manually re-locating the same record and metric by hand. Adding an
in-place "Full Details" view closes that gap without interrupting the reviewer's session.

## What Changes

- Add a "Full Details" button at the end of the METRIC DETAILS panel's field list in
  `product-metric-review-view.svelte`, shown whenever a result row is selected.
- Clicking it opens an in-page modal dialog (same overlay pattern as the existing "document
  record" dialog) showing the metric's full Metric / Metadata / Context / Grounding / Reasoning
  fields — the same curated field set the Metrics workspace shows — without navigating away from
  Product Review or opening a new tab.
- Extract the field-grouping logic behind that curated view (`buildMetricGroupAttrs`,
  `normalizeMetricSpans`, `confidencePct`, previously private to `metric-mgmt-view.svelte`) into a
  shared module (`metric-detail-groups.ts`) so both views stay in sync on what fields appear for a
  metric. `metric-mgmt-view.svelte`'s own rendering is otherwise unchanged.
- The dialog fetches the metric's full record via the existing `listKbMetrics(record_id)` call
  (matching by `metric_id`) and reuses the raw lines already loaded for the selected result's PDF
  view for the Grounding section — no new backend endpoint.
- Add new `pmr_*` i18n strings (English + Chinese) for the button, dialog group labels, and
  loading/not-found states.

## Capabilities

### New Capabilities
- `metric-full-details-dialog`: an in-page dialog on Product Review showing a selected result's
  metric in the same curated Metric/Metadata/Context/Grounding/Reasoning detail as the Metrics
  workspace, sourced via `listKbMetrics` + the already-loaded document raw lines.

### Modified Capabilities
(none — no existing spec covers this frontend area)

## Impact

- `web/src/lib/components/home3/product-metric-review-view.svelte` — new button + modal dialog
  in the detail pane; fetches the full `KbMetricRecord` and renders it grouped, reusing the
  existing `.doc-dialog-*` overlay styling.
- `web/src/lib/components/home3/metric-detail-groups.ts` (new) — shared pure logic for building
  the Metric/Metadata/Context/Grounding/Reasoning attribute groups from a `KbMetricRecord`.
- `web/src/lib/components/home3/metric-mgmt-view.svelte` — refactored to import the shared logic
  instead of defining it locally; no behavior change.
- `web/messages/en.json`, `web/messages/zh-cn.json` — new message keys for the button, dialog
  group labels, and loading/not-found states.
- No backend/API changes — the dialog fetches through the existing `listKbMetrics` endpoint and
  reuses raw lines already loaded for the selected result's PDF view.

Superseded from the initial version of this proposal: a "new tab" deep-link to the standalone
`/home3/metrics` route was implemented first, then replaced with the in-page dialog above per
explicit feedback that opening a new tab interrupts the reviewer's viewing — see design.md.
