## Why

On the doc-review-report page, the embedded Document Structure panel shows a raw LINES list (all extracted lines of the source document) next to the PDF. When a reviewer clicks a finding on the left, the LINES list doesn't help them evaluate it — they need the finding's own fields (severity, confidence, metadata, reference doc), the artifact it flags (metric/provision/inventory item), and the review-run diagnostics (matched units, LLM calls) that produced it. Today that data is either not shown at all, scattered across separate admin screens (System Admin → Logs → Doc Review Logs), or not exposed by the API yet.

## What Changes

- Remove the LINES list from the Document Structure panel **only when it is embedded in the doc-review-report page** (the standalone Document Structure page and the Knowledge page keep the LINES list unchanged); the panel itself (header, PDF pane) stays.
- In its place, show a "Finding Details" panel for the currently selected finding, with four foldable name-value blocks:
  1. **Finding** (default unfolded): `artifact_id`, `aspect`, `severity`, `finding_type`, `title`, `description`, `suggestion`, `confidence`, `metadata` (button → recursive JSON dialog), `location`, `reference_doc`.
  2. **Artifact** (default unfolded, shown only when `artifact_id` is set): the `kb.metrics`/`kb.provisions`/`kb.inventory_items` record via the existing artifact-wiki endpoint, plus `kb.object_nodes.canonical_name` when the artifact is linked to an object node.
  3. **Doc Review Log** (default folded): the `kb.doc_review_logs` row matched by `unit_key = artifact_id` and `run_id`, showing `unit_key`, `matched_units`, `findings`, `outcome`, `detail` (the last three as dialog buttons, reusing the System Admin → Logs → Doc Review Logs dialog pattern).
  4. **LLM Calls** (default folded): a list of `public.llm_usage_event` rows referenced by `doc_review_logs.detail.llm_usage_event_ids`; clicking a list entry opens a dialog with that row's fields.
- **BREAKING**: none — this only adds fields to existing API responses and adds a new endpoint; no existing field or endpoint is removed or renamed.

## Capabilities

### New Capabilities
- `doc-review-finding-details-panel`: the doc-review-report page's embedded Document Structure panel replaces its LINES list with a foldable, name-value "Finding Details" view of the selected finding, its source artifact, its review-log record, and the LLM calls that produced it.

### Modified Capabilities

<!-- none — no existing spec files exist for doc-review-report or doc-structure-view yet -->

## Impact

- **Frontend**: `web/src/routes/home3/doc-review-report/[id]/+page.svelte`, `web/src/lib/components/home3/doc-structure-view.svelte` (new optional sidebar-override snippet prop), new `web/src/lib/components/home3/finding-details-panel.svelte`, new recursive JSON tree dialog (extends `doc-review-json-dialog.ts`), `web/src/lib/services/docReviewService.ts` (widen `FindingItem` type), `web/src/lib/services/kbService.ts` or a new service module (artifact-wiki / doc-review-logs / llm-usage-events-by-ids fetchers).
- **Backend**: `server/api/doc-reviews/models.go` + `controller.go` (expose raw `metadata` and `reference_doc` on `FindingItem`), `server/api/kbhandler/artifact_wiki_fetch.go` (join `kb.object_nodes.canonical_name` for metric/provision/inventory_item), `server/api/llmreporthandler/store.go` + `handler.go` + `server/api/routes.go` (new `GET /api/v1/llm/usage-events/by-ids` endpoint).
- No database migration — all referenced columns (`metadata`, `reference_doc`, `run_id`, `object_nodes.canonical_name`) already exist.
- Documentation: change-log entry in `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md`.
