# Merge Related Object Nodes — Design

**Date:** 2026-07-08
**Component:** Resolve Ambiguous Objects admin page
**Files:** `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`,
`web/src/lib/components/home3/resolve-ambiguous-objects-client.ts`,
`server/api/doc-processing/object_nodes.go`

## Problem

On the Resolve Ambiguous Objects admin page, the "Related Object Nodes" list
shows the candidate `kb.object_nodes` for one ambiguous artifact object. Reviewers
need to collapse duplicate/near-duplicate object nodes into a single canonical
node by hand: pick one node as the **master** (survivor), pick one or more other
nodes to **merge into it**, and merge.

Merging means: every `kb.artifact_objects` row that points to a merged node is
repointed to the master, and the merged node is effectively removed from the
knowledge base — it must never again be considered when resolving artifact
objects.

Four concrete UI/behavior requirements:

1. The scrollbar currently spanning the whole right panel should apply to the
   "Related Object Nodes" list only.
2. Each record in the list gets two checkboxes, **Master** and **Select**.
   Selecting **Master** on a record disables that record's **Select** checkbox.
3. A **Merge** button sits on top of the list and does not scroll with it.
4. **Merge** is enabled only when exactly one record's **Master** is checked and
   at least one other record's **Select** is checked. It merges all Selected
   nodes into the Master node, then removes the Selected nodes.

## Decisions

- **Soft-merge, not hard delete.** Reuse the existing
  `POST /api/v1/kb/objects/merge` endpoint (ADR 2026070101 DR3). Per (loser →
  survivor) pair it repoints `kb.artifact_objects.object_id` to the survivor and
  marks the loser `reconcile_status='merged'`, `canonical_object_id=survivor`.
  The node row is retained for identity redirect, but is **effectively deleted**
  from resolution (see next decision). This is reversible and ADR-consistent.
- **Merged nodes are excluded from all future resolution.**
  `ObjectNodeSQLStore.FindCandidates` is the single candidate-finding path for
  object-node resolution — used by both the automated
  `reconcileArtifactObjects` pipeline and the admin `RankAmbiguousCandidates`
  detail/ranking path. Its `WHERE reconcile_status <> 'rejected'` filter is
  widened to `WHERE reconcile_status NOT IN ('rejected', 'merged')`. After this,
  a merged node is never returned as a candidate again, in either path, so it is
  effectively deleted for resolution purposes and disappears from the "Related
  Object Nodes" list on reload.
- **Merge commits immediately, with confirmation.** Clicking Merge shows a
  confirmation modal, then commits server-side (one endpoint call per Selected
  node), then reloads the detail. Merge is independent of the Save button.
- **Every merge is audit-logged.** The merge endpoint already writes one
  `merge_nodes` row to `kb.object_audit_log` per call (loser object_id,
  survivor_object_id, repointed_mentions count). Looping one call per Selected
  node yields one audit entry per merged node. No audit-schema change.

## Backend changes

### 1. `object_nodes.go` — exclude merged nodes from candidates

`FindCandidates` query (currently line 47):

```
WHERE reconcile_status <> 'rejected'
```

becomes

```
WHERE reconcile_status NOT IN ('rejected', 'merged')
```

This is correct beyond the admin page: a merged/redirected node should never be
a match target during automated reconciliation — the survivor is. Update the
one affected unit test in `object_nodes_test.go` that asserts the query
shape/behavior.

### 2. No new endpoint

Reuse `POST /api/v1/kb/objects/merge` unchanged. Existing request shape:

```json
{ "loser_object_id": "<selected>", "survivor_object_id": "<master>" }
```

It validates both ids exist and differ, repoints mentions, marks the loser
merged in a transaction, and audit-logs `merge_nodes`.

## Frontend changes

### 3. Client — `resolve-ambiguous-objects-client.ts`

Add:

```ts
export function mergeObjectNodes(
  survivorObjectId: string,
  loserObjectId: string
): Promise<{ status: boolean; survivor_object_id: string; repointed_mentions: number }>;
```

POSTs to `/api/v1/kb/objects/merge` with the two ids.

### 4. View — `resolve-ambiguous-objects-view.svelte`

**State**

- `masterNodeId: string | null` — the object_id chosen as survivor.
- `selectedNodeIds: Set<string>` — object_ids chosen to merge into master
  (reassigned on change so Svelte 5 reactivity fires).
- `merging: boolean`, `mergeError: string`, `mergeConfirm: boolean` — mirror the
  existing `saving` / `saveError` / `cancelConfirm` patterns.
- Reset `masterNodeId`, `selectedNodeIds`, `mergeError` inside `selectRow` so
  merge selection never leaks across records.

**Scrollbar scoping (req 1 & 3)**

In the "Related Object Nodes" card:
- Card `<h2>` header and the Merge button form a fixed (non-scrolling) header row.
- The `{#each currentNodes}` node cards move into a wrapper with
  `overflow-y:auto` and a `max-height`, giving the list its own scrollbar.
- The outer right-panel `overflow-y-auto` stays as a small-viewport safety net;
  on normal viewports the bounded list owns the scrolling.

**Per-node checkboxes (req 2)**

Each node card's header row gets two labeled checkboxes:
- **Master** — radio-like single select. Checking a node's Master sets
  `masterNodeId` to that node (clearing any previous master) and removes that id
  from `selectedNodeIds`. Unchecking clears `masterNodeId`.
- **Select** — toggles membership in `selectedNodeIds`. `disabled` when this node
  is the current master.

**Merge button + action (req 3 & 4)**

- Button in the fixed header. `disabled` unless
  `masterNodeId !== null && selectedNodeIds.size >= 1`.
  (Master is never in `selectedNodeIds`, so "at least one other Select" holds.)
- Click → `mergeConfirm = true` (confirmation modal, same overlay/modal styling).
- Confirm → for each id in `selectedNodeIds`, sequentially call
  `mergeObjectNodes(masterNodeId, id)`. On any failure, stop and show
  `mergeError`. On success of all, `await selectRow(selectedId)` to reload the
  now-shorter candidate list and reset merge state.

## Error handling

- Merge validation (missing/equal ids, node missing) is enforced server-side and
  surfaced through `mergeError`, mirroring `saveError`.
- Partial failure: merges run sequentially; if call N fails, calls 1..N-1 have
  already committed (each is its own transaction). The error banner reports the
  failure and the reload reflects whatever succeeded — the reviewer can retry the
  remainder. This is acceptable for an admin tool and matches the per-node
  transactional design of the existing endpoint.

## Testing

- **Backend:** update the `FindCandidates` unit test for the widened
  `reconcile_status NOT IN ('rejected', 'merged')` filter; assert a `'merged'`
  node is not returned as a candidate.
- **Frontend:** the enable/disable logic (exactly-one-master + ≥1-select, master
  disables its own select) is pure and unit-testable if a helper is extracted;
  otherwise verify via the running page.
- **Manual:** on the running app, pick a master + one or more selects, merge,
  confirm the artifact objects repoint, the merged nodes leave the list, and a
  `merge_nodes` audit row is written per merged node.

## Out of scope

- No change to `kb.object_audit_log` schema or the merge endpoint itself.
- No batch/single-call multi-loser endpoint (looping the existing per-pair
  endpoint keeps one audit row per merged node and avoids new backend surface).
- No un-merge / redirect-following UI.
