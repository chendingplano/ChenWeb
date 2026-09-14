# KB Products Viewer Design

## Goal

Add a Products viewer to `home3/knowledge` under Knowledge System, immediately after Metrics, for browsing `kb.products` records alongside their source documents.

## Design

The existing `ProductsView` and `KbExtractionView` provide the required three-pane interaction: a paginated `kb.inputs` source-record browser, extracted product records for the selected input, and the source document/evidence view. The view uses `listKbProducts(input_record_id)` and does not introduce a second API or database query path.

Products will appear immediately after Metrics in the Knowledge System menu. Its detail groups will cover product metadata, grounding, inputs, actors, and requirements. Related-product navigation/details are excluded; the selected source document remains available for context and evidence.

The `kb-products` page-config entries must remain enabled and accessible in both
supported language rows; an idempotent project migration restores those flags so
the DB-backed menu overlay does not hide the page-owned menu entry.

## Verification

- Verify menu order is Metrics, Products.
- Verify selecting Products renders the existing Products viewer.
- Verify selecting a source input loads `kb.products` records and its source document.
- Verify empty, loading, and error states continue to come from the shared extraction viewer.
