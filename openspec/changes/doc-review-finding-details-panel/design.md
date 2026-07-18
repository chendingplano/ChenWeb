## Context

The doc-review-report page (`web/src/routes/home3/doc-review-report/[id]/+page.svelte`) renders a findings list on the left and embeds `DocStructureView` (`web/src/lib/components/home3/doc-structure-view.svelte`) on the right via `<DocStructureView bind:this={structureView} darkMode={dark} lockedRecordId={inputRecordId} />`. `DocStructureView` is a large (~3000-line), shared component also used unmodified by the standalone Document Structure page and the Knowledge page — those two call sites never pass `lockedRecordId`, so it is a reliable signal that a given `DocStructureView` instance is the doc-review-report embed. `DocStructureView`'s `.structure-sidebar` aside currently renders a "Lines" header, a filter/search bar, and the scrollable `.line-list` (all lines of the source document, editable). Clicking a finding on the left already calls `structureView.focusSourceLines(...)` to scroll/highlight the PDF; the sidebar's LINES list plays no role in that flow inside doc-review-report and just occupies space that would be more useful showing the selected finding's own data.

Three backend gaps block a naive "just render fields we already have" implementation (see change proposal Impact section): the finding's raw `metadata` and `reference_doc` JSONB columns are not exposed in the `GET /api/v1/doc-review/requests/:id` response; the artifact-wiki endpoint (`GET /api/v1/kb/artifacts/wiki`) does not join `kb.object_nodes.canonical_name`; and no endpoint exists to fetch `public.llm_usage_event` rows by id. All three are additive — no schema migration, no breaking change to existing consumers.

## Goals / Non-Goals

**Goals:**
- Replace the LINES list with a Finding Details view **only** in the `DocStructureView` instance embedded by doc-review-report; standalone Document Structure and Knowledge pages are pixel-identical to today.
- Keep `DocStructureView` itself unaware of "findings" — it stays a generic structure/PDF viewer. The finding-details concept lives entirely in a new component owned by the doc-review-report route.
- Surface finding `metadata`/`reference_doc`, artifact + object-node canonical name, the matching `doc_review_logs` row, and its `llm_usage_event` rows, reusing the existing System Admin → Logs → Doc Review Logs dialog pattern (`doc-review-json-dialog.ts`, `doc-review-logs-view.svelte`) for consistency.

**Non-Goals:**
- No changes to the LINES list behavior itself (editing, renumbering, filtering) — it is only conditionally hidden, not modified.
- No backfill of `metadata`/`reference_doc` on existing rows — these columns are already populated going forward wherever the reviewers write them; this change only stops discarding them on read.
- No new admin UI for `llm_usage_event` beyond the read-only list/dialog needed here (existing `llm-usage-logs-view.svelte` / `usage-events-admin` endpoint are untouched).
- No pagination/virtualization for the LLM Calls list — `llm_usage_event_ids` per finding is expected to be small (a handful of calls per artifact review), consistent with existing `Detail` payload sizes in `kb.doc_review_logs`.

## Decisions

### 1. `DocStructureView` sidebar override via a Svelte 5 snippet prop, not a `lockedRecordId` branch inside the component

Add an optional snippet prop, e.g. `sidebarOverride?: Snippet`. In the `.structure-sidebar` aside, render `{#if sidebarOverride}{@render sidebarOverride()}{:else}<!-- existing Lines header + search bar + line-list -->{/if}`. The aside's width/resizer stay outside the `{#if}` so resizing still works either way. Only `doc-review-report/[id]/+page.svelte` passes `sidebarOverride`; the other two call sites are unchanged (they don't pass the prop, so they get today's behavior).

*Alternative considered*: gate the LINES-list removal internally on `lockedRecordId != null` and render the Finding Details content directly inside `DocStructureView`. Rejected — this was the explicit non-preferred option when scoping the change: it would make the shared, generic structure/PDF viewer component know about `FindingItem`, `kb.doc_review_findings`, artifact-wiki fetches, etc., which is a much bigger coupling than a snippet the caller controls.

### 2. New `finding-details-panel.svelte` component owns all data fetching for the four blocks

`+page.svelte` already tracks `activeKey` (the selected finding id) and holds the full `findings: FindingItem[]` array, `requestId`, and `reportRunId`. It passes the currently active `FindingItem | null` (plus `requestId`/`reportRunId`/`dark`) into `<FindingDetailsPanel finding={...} .../>`, rendered inside the `sidebarOverride` snippet. The panel itself:
- Renders the **Finding** block synchronously from the passed-in `finding` prop (no fetch needed — all fields are already in `FindingItem` once widened, see Decision 4).
- On `finding` change, if `finding.artifact_id` is set, fetches the artifact via the artifact-wiki endpoint (Decision 5) for the **Artifact** block.
- On `finding` change, if `finding.artifact_id` is set, fetches the matching `kb.doc_review_logs` row (Decision 6) for the **Doc Review Log** block, and once that resolves, extracts `detail.llm_usage_event_ids` to fetch the **LLM Calls** list (Decision 7).
- Each of the three fetched blocks independently tracks its own loading/error/folded state so a slow or failing fetch doesn't block the others.

*Alternative considered*: fetch everything in `+page.svelte`'s `onFocusFinding` and pass fully-resolved data down. Rejected — `+page.svelte` is already large (1450 lines) and mixes concerns (translation, packages/severity grouping, edit dialogs); keeping the new fetches inside the new panel component matches the existing pattern where `DocStructureView` and `EditToolDialog`/`LlmAutoFixDialog` each own their own data loading.

### 3. Recursive JSON dialog is a new sibling module, not a rewrite of `buildJsonSections`

`doc-review-json-dialog.ts`'s `buildJsonSections`/`flattenRecord` intentionally flatten one level (nested objects become a `JSON.stringify` string in a single row) — that's correct for the existing Doc Review Logs screen (`matched_units`, `findings`, `detail` are naturally flat-ish per-row records). Requirement 2 explicitly asks for a *recursive*, indented tree for `metadata` (arbitrary nesting). Add a new `json-tree-dialog.svelte` (or a `buildJsonTree`/render-recursive helper alongside a new small dialog component) rather than changing `buildJsonSections`'s behavior, since three other call sites (`doc-review-logs-view.svelte`'s Matched/Findings/LLM Call buttons) depend on today's flattened format and must not change.

*Alternative considered*: make `buildJsonSections` recursive and update its one existing caller. Rejected — bigger diff for no behavior the Doc Review Logs screen needs, and risks visually changing an existing, working admin screen.

### 4. Widen `FindingItem` (frontend + backend) additively

Backend `FindingItem` (`server/api/doc-reviews/models.go`) already carries `ArtifactID` and is populated with `RunID` via the `metadata` mirror (from the prior `add-run-id-to-doc-review-findings` change) — both already serialize into the JSON response today. Add two more fields, both `omitempty`:
- `Metadata json.RawMessage `json:"metadata,omitempty"`` — populate from the raw `metadata` bytes already scanned by `GetRequestWithFindings` (`controller.go`), which today are decoded into `applyFindingMetadata` and then dropped. Keep both: decode as today (for `RunID`, `ModelName`, etc.) and also retain the raw bytes on the struct.
- `ReferenceDoc json.RawMessage `json:"reference_doc,omitempty"`` — add `reference_doc` to the `SELECT` list in `GetRequestWithFindings` and scan it into the new field.

Frontend `FindingItem` (`docReviewService.ts`) adds `artifact_id?: string`, `run_id?: number`, `metadata?: unknown`, `reference_doc?: unknown` to match — `finding_type` is already declared.

*Alternative considered*: introduce a separate `GET /api/v1/doc-review/findings/:id` detail endpoint returning the full row. Rejected — `GetRequestWithFindings` already loads every field per finding in one query for the whole request; adding two columns to that one SELECT is strictly simpler than a new endpoint, new route, new handler, and an extra round trip per finding click.

### 5. Join `kb.object_nodes.canonical_name` into the artifact-wiki fetchers, field named to avoid collision

Add `ObjectNodeCanonicalName *string `json:"object_node_canonical_name,omitempty"`` to each of the three record structs in `artifact_wiki_fetch.go` (`metricRecord`, `provisionRecordJSON`, `inventoryItemRecord`), populated via the same join pattern already used by `loadProvisionObjectLinks` (`provision_handlers.go:1151-1178`): `... FROM kb.artifact_objects ao LEFT JOIN kb.object_nodes onode ON onode.object_id = ao.object_id WHERE ao.source_record_id = $1 AND ao.artifact_type = $2 AND ao.artifact_id = $3`, taking the first matched row's `canonical_name` (an artifact can in principle link to multiple objects; showing the first is sufficient for a details panel — this isn't a listing UI).

Named `object_node_canonical_name` (not `canonical_name`) because `inventoryItemRecord` already has its own `canonical_name` column (extraction-time name, distinct from the post-reconciliation `kb.object_nodes.canonical_name`) — reusing the key would silently overwrite or shadow it.

### 6. Doc Review Log lookup: fetch by `run_id` + substring `unit_key`, then filter client-side for an artifact-id-prefixed match

`GET /api/v1/kb/doc-review-logs` already supports combining `run_id` and `unit_key` filters (`doc_review_log_store.go`), but `unit_key` is matched with `ILIKE '%<value>%'` (substring), not equality — passing `artifact_id="244_mtc_1"` would also match a row with `unit_key="244_mtc_10#..."`. Testing against live data also showed `kb.doc_review_logs.unit_key` is not the bare artifact id: the P5 metrics reviewer writes it as `"<artifact_id>#<suffix>"` (e.g. `"244_mtc_1#31082"`, where the suffix disambiguates the specific object/event a metric was compared against). A strict `row.unit_key === finding.artifact_id` equality check therefore never matches. The panel fetches with both filters as today and filters the returned rows client-side for `row.unit_key === artifactId || row.unit_key.startsWith(artifactId + '#')`, taking the first match — the `#` boundary check is also what avoids the `_mtc_1` vs. `_mtc_10` false-positive the substring server filter alone would allow through.

*Alternative considered*: add a new `unit_key_exact`/`unit_key_prefix` query param to the existing endpoint. Rejected as unnecessary backend surface — the expected row count for a given `(run_id, unit_key substring)` pair is small (single digits), so client-side filtering is cheap and keeps the shared endpoint's contract unchanged.

### 7. New `GET /api/v1/llm/usage-events/by-ids` endpoint, reusing `UsageEventAdmin`

Add `Store.ListUsageEventsByIDs(ctx, ids []string) ([]UsageEventAdmin, error)` to `server/api/llmreporthandler/store.go` (`SELECT ... WHERE id = ANY($1)`, same column set as `ListUsageEventsAdmin`) and a handler `GetUsageEventsByIDs` parsing `?ids=a,b,c` (comma-separated, matching how `llm_usage_event_ids` is already stored as a `[]string` in `doc_review_logs.detail`), mounted at `apiGroup.GET("/llm/usage-events/by-ids", ...)` next to the existing `usage-events*` routes in `routes.go`. Reuses the existing `UsageEventAdmin` struct — no new response shape.

*Alternative considered*: reuse `GetUsageEventBody` (`/llm/usage-events/:id/body`). Rejected — that endpoint returns only the archived request/response body blob for one id, not the row's tokens/cost/model/latency fields the panel needs to show, and doesn't support batching by id list.

## Risks / Trade-offs

- [`unit_key` substring matching (Decision 6) could, in pathological cases, return zero rows client-side if the true exact-match row isn't among the first page of results] → `run_id` is always also filtered, and a given `(run_id, artifact_id)` pair should have at most a few `doc_review_logs` rows in practice (one per reviewer pass touching that artifact); acceptable without pagination handling for a details panel. If this proves wrong in practice, revisit with a dedicated exact-match param.
- [Widening `FindingItem.Metadata`/`ReferenceDoc` to raw JSON exposes whatever reviewers have historically written into those columns, unvalidated] → These are read-only display fields in a details panel (rendered through the recursive JSON dialog, not interpreted), so unexpected shapes degrade gracefully to "show what's there."
- [`ObjectNodeCanonicalName` join adds one query per artifact-wiki fetch] → Bounded to a single indexed lookup (`kb.artifact_objects` is keyed by `(source_record_id, artifact_type, artifact_id)`), on an already low-traffic, single-record endpoint (details panel, not a list).
- [Four sequential/parallel fetches (artifact, doc-review-log, then LLM calls once log resolves) per finding click add latency] → All three are scoped to a single artifact/run, small payloads, and each block shows its own loading state independently rather than blocking the whole panel.
