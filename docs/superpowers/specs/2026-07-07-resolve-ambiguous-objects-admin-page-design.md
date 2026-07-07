# Resolve Ambiguous Objects Admin Page Design

**Date:** 2026-07-07  
**Project:** `ChenWeb`  
**Route Family:** `web/src/lib/components/home3/content-panel.svelte` (System Admin → Database Maintenance)  
**Scope:** New admin page for manually resolving `kb.artifact_objects` rows stuck at `reconcile_status = 'ambiguous'`

## Goal

Add a "Resolve Ambiguous Objects" page under System Admin → Database Maintenance that lets an admin manually resolve `kb.artifact_objects` rows left at `reconcile_status = 'ambiguous'` (per ADR 2026070701) by reviewing each row alongside its candidate `kb.object_nodes`, editing fields on either side, and assigning a resolved `object_id`.

This is a human-in-the-loop complement to the existing automated `POST /kb/objects/resolve-ambiguous` backfill (ADR 2026070701, DR3–DR5): the automated pass applies a deterministic tie-break heuristic; this page lets an admin override that heuristic per row when the data itself needs correction (e.g. merging near-duplicate object nodes, fixing a misspelled name) rather than just picking blindly between ties.

## Non-Goals

- Not a general-purpose `kb.object_nodes` browser/editor — only nodes returned as candidates for the currently selected ambiguous row are shown.
- Not a replacement for the automated backfill endpoint; both remain available.
- No creation of new `kb.object_nodes` rows from this page (the automated backfill already handles the "no candidates remain" case by creating one).
- No merge/delete of `object_nodes` rows.

## Backend API

Four new endpoints, following the existing conventions in `kbhandler` (`resolve_ambiguous_objects_handler.go`'s `EchoFactory.NewFromEcho` + `ApiTypes.ProjectDBHandle` pattern, and `UpdateMetric`'s partial-update-via-field-whitelist pattern).

### `GET /api/v1/kb/objects/ambiguous`

Lightweight list for the left panel. Query: `SELECT id, artifact_type, artifact_id, object_name, object_name_en, confidence FROM kb.artifact_objects WHERE reconcile_status = 'ambiguous' ORDER BY id`. No pagination in this phase (ADR 2026070701 reports ~40 affected rows; revisit if the backlog grows materially).

Response:
```json
{"status": true, "rows": [{"id": 101, "artifact_type": "provision", "artifact_id": "...", "object_name": "...", "object_name_en": "...", "confidence": 0.85}]}
```

### `GET /api/v1/kb/objects/ambiguous/:id`

Full detail for one row, used both on card click and on Prev/Next navigation.

- Loads the full `ArtifactObject` row by `id` (reuse the scan shape from `ArtifactObjectSQLStore.LoadAmbiguous`, but by primary key instead of by status+limit).
- Calls `ObjectNodeSQLStore.FindCandidates` for that object to get the candidate `object_nodes` (this naturally returns the broader "similarly named" set, not just the two tied top scorers — satisfies the "show more than the bare tie" requirement without a new query).
- Sorts candidates by score descending and marks the `pickTieBreakCandidate` winner with `"recommended": true`.

Response:
```json
{
  "status": true,
  "artifact_object": { "id": 101, "object_name": "...", "object_id": null, "reconcile_status": "ambiguous", "...": "..." },
  "candidates": [
    { "object_id": "obj_...", "canonical_name": "...", "score": 0.85, "method": "lexical_name", "recommended": true, "...": "..." },
    { "object_id": "obj_...", "canonical_name": "...", "score": 0.85, "method": "lexical_name", "recommended": false, "...": "..." }
  ]
}
```

### `PATCH /api/v1/kb/objects/artifact-objects/:id`

Partial update, same style as `UpdateMetric` (`metrics_handler.go:727`): decode `map[string]json.RawMessage`, whitelist fields, build a dynamic `SET` clause.

Whitelisted fields: `object_name`, `object_name_en`, `object_name_zh`, `language`, `object_type`, `object_role`, `aliases`, `acronyms`, `description`, `evidence_quote`, `object_id`, `reconcile_status`, `reconcile_confidence`.

`reconcile_status` is validated against the existing constants (`ObjectReconcilePending`, `ObjectReconcileMatched`, `ObjectReconcileNew`, `ObjectReconcileAmbiguous`, `ObjectReconcileAmbiguousResolved`, `ObjectReconcileRejected` — `artifact_objects.go:15-21`). When the update sets a non-empty `object_id`, also stamp `ext_info.reconcile_method = "manual_admin"` (merged into existing `ext_info`, not overwritten) so provenance distinguishes manual resolutions from `tie_break_deterministic` / `exact_name` / `lexical_name` / `new_node`.

### `PATCH /api/v1/kb/object-nodes/:object_id`

Partial update of one `kb.object_nodes` row. Whitelisted fields: `canonical_name`, `canonical_name_en`, `canonical_name_zh`, `primary_language`, `object_type`, `aliases`, `acronyms`, `description`.

### Routes registration

In `routes.go`, alongside the existing `/kb/objects/resolve-ambiguous`:
```go
apiGroup.GET("/kb/objects/ambiguous", kbhandler.ListAmbiguousObjects)
apiGroup.GET("/kb/objects/ambiguous/:id", kbhandler.GetAmbiguousObjectDetail)
apiGroup.PATCH("/kb/objects/artifact-objects/:id", kbhandler.UpdateArtifactObject)
apiGroup.PATCH("/kb/object-nodes/:object_id", kbhandler.UpdateObjectNode)
```

## Frontend

### Nav wiring

- `nav-rail.svelte`: add a third child under the existing `sysadmin-db` group (`nav-rail.svelte:196-201`):
  ```js
  { id: 'sysadmin-db-resolve-ambiguous', label: 'Resolve Ambiguous Objects' }
  ```
- `content-panel.svelte`: add a dispatch branch next to the existing `sysadmin-db-consistency` / `sysadmin-db-maint-log` branches (`content-panel.svelte:202-205`):
  ```svelte
  {:else if activeMenu?.childId === 'sysadmin-db-resolve-ambiguous'}
      <ResolveAmbiguousObjectsView {darkMode} />
  ```

New component: `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`, self-fetching (owns its own data loading, no props beyond `darkMode`), matching the styling conventions of `db-consistency-view.svelte` / `db-maint-log-view.svelte` (design tokens, dark/light derived colors).

### Layout

Two-panel, full height:

**Left panel** — scrollable card list, loaded once on mount from `GET /kb/objects/ambiguous`. Each card: `object_name` (+ `object_name_en` if present), `artifact_type`, `confidence`. Clicking a card calls `GET /kb/objects/ambiguous/:id` and populates the right panel. Selected card is highlighted. A resolved row (see Save behavior) is removed from this list without a refetch of the whole list.

**Right panel**, top to bottom:

1. **Artifact Object block** (editable): `object_name`, `object_name_en`, `object_name_zh`, `language`, `object_type`, `object_role`, `aliases` (chip/tag input), `acronyms` (chip/tag input), `description`, `evidence_quote`, `object_id` (text input), `reconcile_status` (dropdown, six values above), `reconcile_confidence` (number input). Read-only context strip: `id`, `source_record_id`, `artifact_type`, `artifact_id`. `source_line_spans` and `ext_info` are intentionally not shown (kept out of the form to stay focused on the fields relevant to resolving the tie).
2. **Related Object Nodes block** (editable): one card per candidate from the detail response. Each card shows read-only `object_id`, `score`, `method`, and a **Recommended** badge on the `recommended: true` entry; editable `canonical_name`, `canonical_name_en`, `canonical_name_zh`, `object_type`, `aliases`, `acronyms`, `description`; and a **"Use this"** button that copies the candidate's `object_id` into the Artifact Object block's `object_id` field (client-side only, folds into the same dirty-edit set — does not save by itself).

### State model

```ts
type FormState = {
  artifactObject: ArtifactObjectEditable;
  nodes: Record<string /* object_id */, ObjectNodeEditable>;
};
```

- `snapshot: FormState | null` — set on load/save, used for dirty comparison and Cancel/discard.
- `current: FormState | null` — bound to form inputs.
- `isDirty = current !== null && !deepEqual(current, snapshot)`.
- `ambiguousIds: number[]` — full ordered id list from the left-panel load, used for Prev/Next index math without refetching the list.
- `selectedIndex: number` — position of the current row within `ambiguousIds`.

### Button behavior

- **Prev / Next**: move `selectedIndex` by ±1 within `ambiguousIds`. If `isDirty`, show a confirm dialog worded for navigation — *"Save changes before moving to the previous/next record?"* — with **Save & Continue / Discard & Continue / Stay** options. If not dirty, navigate immediately (fetch detail for the new id).
- **Cancel**: if `isDirty`, show a confirm dialog worded for in-place discard — *"Discard your edits to this record?"* — Confirm resets `current = structuredClone(snapshot)`. If not dirty, no-op (no dialog).
- **Save**: PATCH the artifact_object (`/kb/objects/artifact-objects/:id`) with only the fields that changed vs. `snapshot.artifactObject`; then PATCH each node in `current.nodes` that changed vs. `snapshot.nodes[objectId]`. On success: re-snapshot (`snapshot = structuredClone(current)`); if the saved `reconcile_status` is no longer `'ambiguous'`, remove this id from `ambiguousIds` and the left-panel list, then auto-select the next remaining id (or show an empty-queue state if none remain). On failure: show the error inline near the field/block that failed; keep edits; do not navigate.
- **Help**: opens a static modal (no server call) explaining: what `ambiguous` means, what the Recommended badge means, how "Use this" + Save resolves a row, and what each `reconcile_status` value means — using the same language as ADR 2026070701 DR3/DR4 so the on-page help stays consistent with the ADR the team already treats as the source of truth for this behavior.

## Testing Strategy

Backend (Go, `server/api/kbhandler`, `server/api/doc-processing`):
- `ListAmbiguousObjects` returns only rows at `reconcile_status = 'ambiguous'`.
- `GetAmbiguousObjectDetail` returns candidates sorted by score with exactly one `recommended: true` (matching `pickTieBreakCandidate`'s existing tie-break logic).
- `UpdateArtifactObject` rejects unknown fields, accepts a partial field set, validates `reconcile_status` against the known constants, and merges (not overwrites) `ext_info` when stamping `reconcile_method = "manual_admin"`.
- `UpdateObjectNode` rejects unknown fields, accepts a partial field set.

Frontend (manual verification via `webapp-testing` / dev server, since this is UI-driven):
- Left panel lists all ambiguous rows and updates after a resolving Save.
- Selecting a card loads both blocks; "Use this" fills `object_id`.
- Prev/Next with unsaved edits prompts before navigating; Discard/Save/Stay each behave as specified.
- Cancel prompts before discarding, no-ops when not dirty.
- Save persists both blocks, surfaces field-level errors on failure, and removes a resolved row from the left panel.
- Help opens and closes without a network call.

## Tradeoffs

- Candidate list per row is whatever `FindCandidates` returns (typically small, 2–5 nodes) — if a row's true match isn't in that set, this page cannot currently reach it; broadening beyond `FindCandidates` would require a separate node-search UI, deferred as out of scope.
- Two independent PATCH calls on Save (not one transactional endpoint) means a partial failure (e.g. artifact_object saves, one node PATCH fails) can leave the two tables slightly out of sync until the admin retries; acceptable since each PATCH is idempotent and the row stays visible with its error until it fully succeeds.
