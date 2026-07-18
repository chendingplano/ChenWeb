## 1. Backend: expose `metadata` and `reference_doc` on findings

- [x] 1.1 Add `Metadata json.RawMessage \`json:"metadata,omitempty"\`` to `FindingItem` (`server/api/doc-reviews/models.go:69-101`)
- [x] 1.2 Add `ReferenceDoc json.RawMessage \`json:"reference_doc,omitempty"\`` to `FindingItem` (same struct)
- [x] 1.3 In `GetRequestWithFindings` (`server/api/doc-reviews/controller.go:321-348`), add `reference_doc` to the `SELECT` list (`COALESCE(reference_doc, '{}'::jsonb)::text` or similar) and scan it into a new local var
- [x] 1.4 In the same scan loop, set `f.Metadata = []byte(metadata)` (the raw metadata string already scanned into `metadata` at line ~340-346, currently only passed to `applyFindingMetadata`) and `f.ReferenceDoc = []byte(referenceDoc)`
- [x] 1.5 Confirm `f.ArtifactID` and `f.RunID` (via `applyFindingMetadata`) are already populated on this path (no change needed — verify only)

## 2. Backend: join `kb.object_nodes.canonical_name` into artifact-wiki fetchers

- [x] 2.1 Add `ObjectNodeCanonicalName *string \`json:"object_node_canonical_name,omitempty"\`` to `metricRecord` (`server/api/kbhandler/metrics_handler.go:21`), `provisionRecordJSON` (`server/api/kbhandler/provision_handlers.go:963`), `inventoryItemRecord` (`server/api/kbhandler/inventory_items_handler.go:17`)
- [x] 2.2 In `fetchMetricByMetricID` (`server/api/kbhandler/metric_wiki_compile.go:64-100`), after loading the metric row, run a second query mirroring `loadProvisionObjectLinks` (`provision_handlers.go:1151-1178`): `SELECT onode.canonical_name FROM kb.artifact_objects ao LEFT JOIN kb.object_nodes onode ON onode.object_id = ao.object_id WHERE ao.source_record_id = $1 AND ao.artifact_type = 'metric' AND ao.artifact_id = $2 AND onode.canonical_name IS NOT NULL AND onode.canonical_name != '' LIMIT 1`, set `ObjectNodeCanonicalName` when found
- [x] 2.3 Do the same in `fetchProvisionByArtifactID` (`artifact_wiki_fetch.go:464-523`, `artifact_type = 'provision'`)
- [x] 2.4 Do the same in `fetchInventoryItemBySearchID` (`artifact_wiki_fetch.go:409-462`, `artifact_type = 'inventory_item'`)

## 3. Backend: `GET /api/v1/llm/usage-events/by-ids`

- [x] 3.1 Add `ListUsageEventsByIDs(ctx context.Context, ids []string) ([]UsageEventAdmin, error)` to `server/api/llmreporthandler/store.go` (near `ListUsageEventsAdmin`, `store.go:384`), `SELECT ... FROM public.llm_usage_event WHERE id = ANY($1)`, same column set/scan as `ListUsageEventsAdmin`
- [x] 3.2 Add `GetUsageEventsByIDs(c echo.Context) error` handler in `server/api/llmreporthandler/handler.go` (near `ListUsageEventsAdmin`, `handler.go:185`): parse `?ids=a,b,c` (comma-separated, split + trim, empty → `[]`), call the store method, return `{status:true, usage_events:[...]}`
- [x] 3.3 Add the interface method to the store interface used by the handler (`handler.go:25` area, alongside `ListUsageEventsAdmin`)
- [x] 3.4 Register route `apiGroup.GET("/llm/usage-events/by-ids", llmreporthandler.GetUsageEventsByIDs)` in `server/api/routes.go` near line 323
- [x] 3.5 Add/update the stub store in `handler_test.go` and a handler test for the new route (empty ids, unknown ids, found ids)

## 4. Backend verification

- [x] 4.1 `cd server && go build ./... && go vet ./...`
- [x] 4.2 `cd server && go test ./api/doc-reviews/... ./api/kbhandler/... ./api/llmreporthandler/...`

## 5. Frontend: widen types and add fetchers

- [x] 5.1 Add `artifact_id?: string`, `run_id?: number`, `metadata?: unknown`, `reference_doc?: unknown` to `FindingItem` (`web/src/lib/services/docReviewService.ts:87-100`)
- [x] 5.2 Add `getArtifactWiki(artifactType: string, artifactId: string): Promise<Record<string, unknown>>` wrapping `GET /api/v1/kb/artifacts/wiki?artifact_type=...&artifact_id=...&include_article=0`, and an `artifactTypeFromKey(key: string): string | null` helper (regex `^[1-9][0-9]*_(mtc|prv|inv)_[1-9][0-9]*$` → metric/provision/inventory_item), in a new shared module `web/src/lib/components/home3/artifact-key.ts`; update `doc-review-logs-view.svelte`'s local `artifactPattern`/`artifactType` (lines 85-90) to import from it instead of duplicating
- [x] 5.3 Add `getDocReviewLogsFor(unitKey: string, runId: number): Promise<LogRow | null>` to `docReviewService.ts` (or a new small module) wrapping `GET /api/v1/kb/doc-review-logs?unit_key=...&run_id=...`, filtering the response client-side for `row.unit_key === unitKey` and returning the first match or `null`
- [x] 5.4 Add `LLMUsageEventDetail` type (mirrors backend `UsageEventAdmin`: id, model_name, prompt_name, call_reason, call_loc, request_started_at, input/output/total tokens, prompt_cache_hit/miss_tokens, latency_ms, error_message, metadata_json, etc.) and `getLLMUsageEventsByIds(ids: string[]): Promise<LLMUsageEventDetail[]>` wrapping the new endpoint, in `web/src/lib/components/home3/llm-activities-client.ts`

## 6. Frontend: recursive JSON tree dialog

- [x] 6.1 Add a `buildJsonTree(value: unknown): JsonTreeNode[]` recursive builder (each node: `{label, value, children?}`, objects/arrays recurse into `children`, primitives are leaves) as a new export in `doc-review-json-dialog.ts`, separate from the existing flat `buildJsonSections`/`buildMatchedUnitsSections`
- [x] 6.2 Create `web/src/lib/components/home3/json-tree-dialog.svelte`: a modal (reuse the overlay/focus-trap/Escape pattern from `doc-review-logs-view.svelte:62-81,124`) that renders a `JsonTreeNode[]` recursively with indentation per nesting level

## 7. Frontend: `finding-details-panel.svelte`

- [x] 7.1 Create `web/src/lib/components/home3/finding-details-panel.svelte` accepting props `{ finding: FindingItem | null; requestId: number | null; runId: number | null; dark: boolean }`
- [x] 7.2 Render an empty/placeholder state when `finding` is `null`
- [x] 7.3 Render the **Finding** block (default unfolded): `artifact_id`, `aspect`, `severity`, `finding_type`, `title`, `description`, `suggestion`, `confidence`, `location`, `reference_doc` as name-value rows; `metadata` as a button opening the `json-tree-dialog` from task 6
- [x] 7.4 Render the **Artifact** block (default unfolded, shown only when `finding.artifact_id` is set): on `finding` change, resolve artifact type via `artifactTypeFromKey`, call `getArtifactWiki`, show the returned record (including `object_node_canonical_name` when present) as name-value rows; loading/error states scoped to this block
- [x] 7.5 Render the **Doc Review Log** block (default folded, shown only when `finding.artifact_id` is set): on `finding` change (or on first expand — match the lazy-load pattern already used for reviewer translation in `+page.svelte`'s `ensureReviewerLocalized`), call `getDocReviewLogsFor(finding.artifact_id, finding.run_id)`, show `unit_key`, `outcome` as rows and `matched_units`, `findings`, `detail` as dialog buttons reusing `buildJsonSections`/the existing flat dialog pattern from `doc-review-json-dialog.ts` + a shared small modal (extract or reuse from `doc-review-logs-view.svelte`'s inline modal)
- [x] 7.6 Render the **LLM Calls** block (default folded): once the Doc Review Log fetch resolves, parse `detail.llm_usage_event_ids` (array of strings) — if non-empty, call `getLLMUsageEventsByIds` and show a clickable list (id + model_name + created time); clicking an entry opens a dialog with that row's fields via `buildJsonSections`
- [x] 7.7 Add fold/unfold state per block (Finding + Artifact default open, Doc Review Log + LLM Calls default folded), matching the chevron/count-badge visual pattern already used for packages/reviewers in `+page.svelte`

## 8. Frontend: wire into `DocStructureView` and the doc-review-report page

- [x] 8.1 Add an optional `sidebarOverride?: Snippet` prop to `DocStructureView` (`web/src/lib/components/home3/doc-structure-view.svelte`)
- [x] 8.2 In the `.structure-sidebar` aside (`doc-structure-view.svelte:1123-1396`), wrap the "Lines" header + search bar + `.line-list` block in `{#if sidebarOverride}{@render sidebarOverride()}{:else} ... {/if}`, keeping the aside's width/resizer outside the conditional
- [x] 8.3 In `web/src/routes/home3/doc-review-report/[id]/+page.svelte`, compute the currently selected `FindingItem` from `activeKey` + `findings`, and pass a `{#snippet sidebarOverride()}<FindingDetailsPanel finding={selectedFinding} {requestId} runId={reportRunId} {dark} />{/snippet}` into `<DocStructureView>` (near line 970)

## 9. Verification

- [x] 9.1 `cd web && npm run check` (or the project's svelte-check task) and `npm run build`
- [x] 9.2 Manually load a doc-review-report page with `mise dev`/`air`: confirm the LINES list is gone and the Finding Details panel appears; click a finding with an `artifact_id` and confirm the Artifact, Doc Review Log, and LLM Calls blocks populate; open the `metadata` dialog and confirm nested objects render indented, not stringified
- [x] 9.3 Load the standalone Document Structure page and the Knowledge page; confirm the LINES list still renders exactly as before

## 10. Documentation

- [x] 10.1 Add a change-log entry to `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md` noting the Finding Details panel and the three additive backend exposures (finding `metadata`/`reference_doc`, artifact-wiki `object_node_canonical_name`, `llm_usage_event` by-ids endpoint), referencing this openspec change
