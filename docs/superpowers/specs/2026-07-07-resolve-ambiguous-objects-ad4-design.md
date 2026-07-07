# Design: Resolve Ambiguous Objects — AD4 Refinements

**Date:** 2026-07-07
**Component:** ChenWeb — `web/src/lib/components/home3`, `server/api/kbhandler`
**Parent:** ADR 2026070701 (`KnowledgeStore/doc-repo/adrs/202607/2026070701-adr-object-reconciliation-ambiguous-tie-resolution.md`), Alternative Decision AD4

## Context

DR6 of ADR 2026070701 shipped the **Resolve Ambiguous Objects** admin page
(`resolve-ambiguous-objects-view.svelte`): a left panel listing
`kb.artifact_objects` rows at `reconcile_status = 'ambiguous'`, and a right
panel with two stacked blocks — an editable **Artifact Object** card, and a
**Related Object Nodes** block listing every candidate `kb.object_nodes` row
as a full editable card (each with its own **Use this** button that copies
`object_id` into the artifact object).

After using the page, four refinements were requested (AD4):

1. Split the right panel into two columns instead of one stacked column.
2. Auto-set `reconcile_status = 'matched'` when **Use this** is clicked.
3. Highlight dirty (edited, unsaved) fields visually.
4. Persist an audit record of every write to `kb.object_nodes` and
   `kb.artifact_objects` made through this page.

This spec covers all four. No other page or endpoint changes.

## Goals

- Right panel: Left = Artifact Object (unchanged fields). Right = Top Region
  (compact candidate picker table) + Bottom Region (existing detailed
  candidate cards, editing only — no picker action).
- **Use this** pre-fills both `object_id` and `reconcile_status = 'matched'`
  into the in-memory edit state; still just a form edit, not a save. The
  admin can still change the dropdown before hitting Save.
- Any field whose current value differs from the last-loaded/saved snapshot
  renders with a distinct (red) style, on both the Artifact Object card and
  candidate node cards.
- Every successful `PATCH /kb/objects/artifact-objects/:id` and
  `PATCH /kb/object-nodes/:object_id` writes one row to a new
  `kb.object_audit_log` table recording what changed, on which row, by whom,
  when, and a coarse `action` classification.

## Non-goals

- No changes to DR5's bulk `/kb/objects/resolve-ambiguous` endpoint.
- No UI to browse `kb.object_audit_log` (write-only from this spec's
  perspective; reading it is a future admin-page concern if ever needed).
- No change to candidate ranking/recommendation logic (`RankAmbiguousCandidates`,
  `fetchSortedCandidates`).

## Design

### 1. Two-column right panel

Current DOM: left nav list (`320px` fixed) + right panel, where the right
panel is a single scrolling column containing the Artifact Object card
followed by the Related Object Nodes cards.

New right-panel structure (same outer `320px` left list unchanged):

```
Right panel (flex-1)
├── toolbar (Prev/Next/Cancel/Save/Help) — unchanged
└── content: CSS grid, 2 columns (e.g. grid-template-columns: 1fr 1fr; gap)
    ├── Left column: Artifact Object card — unchanged fields/markup
    └── Right column: flex column, two stacked regions
        ├── Top Region: "Candidates" quick-reference table
        │   columns: Canonical Name | Description | Action
        │   one row per currentNodes[i], Action = "Use this" button
        │   (Recommended badge shown next to the name, reusing node.recommended)
        └── Bottom Region: existing "Related Object Nodes" cards,
            unchanged except the per-card "Use this" button is removed
            (per your answer: Top Region is the only picker control now)
```

At narrow widths the two-column grid can collapse to one column (existing
Tailwind breakpoint conventions in this file — `md:grid-cols-2` or similar);
exact breakpoint left to implementation, not a product requirement here.

The Top Region table is purely a denser view of `currentNodes` — it reads
the same array the Bottom Region renders from, so there is exactly one
source of truth (`currentNodes`) and no risk of the two regions
disagreeing about what a candidate's current (possibly just-edited)
`canonical_name`/`description` is.

### 2. `useCandidate()` sets both fields

```ts
function useCandidate(objectId: string) {
  if (!currentObject) return;
  currentObject.object_id = objectId;
  currentObject.reconcile_status = 'matched';
}
```

Both are plain assignments to the existing `$state` object, identical in
kind to every other field edit already tracked by `isDirty` — nothing new
to wire up for dirty-tracking or Save. The admin can still open the
Reconcile Status dropdown and pick something else before Save (e.g. if they
reconsider and want `rejected` instead) — this is a pre-fill/shortcut, not a
locked value.

### 3. Dirty-field highlighting

Two small comparison helpers, colocated with the existing `isDirty`
`$derived.by`:

```ts
function fieldDirty(current: unknown, snapshot: unknown): boolean {
  if (Array.isArray(current) && Array.isArray(snapshot)) {
    return JSON.stringify(current) !== JSON.stringify(snapshot);
  }
  return current !== snapshot;
}
```

Called inline per input, e.g.:

```svelte
<input
  bind:value={currentObject.object_name}
  style="... ; {fieldDirty(currentObject.object_name, snapshotObject?.object_name)
    ? 'border-color:#F87171; color:#F87171;' : ''}"
/>
```

Applied to every bound field in both the Artifact Object card and the
candidate node cards (`currentNodes[i]` vs `snapshotNodes[i]`, same field
name). No new state — purely a render-time comparison against the snapshot
values that already exist for the overall `isDirty` check. `#F87171` matches
the red already used elsewhere in this file for errors
(`saveError`/`listError`/`detailError` text color), so the dirty color reuses
an existing token rather than introducing a new one.

### 4. Audit log: `kb.object_audit_log`

**Migration** `ChenWeb/project_migrations/20260707000002_create_kb_object_audit_log.sql`:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS kb.object_audit_log (
    id          BIGSERIAL    PRIMARY KEY,
    table_name  TEXT         NOT NULL,
    row_key     TEXT         NOT NULL,
    action      TEXT         NOT NULL,
    changes     JSONB        NOT NULL,
    actor       TEXT,
    create_time TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_object_audit_log_table_name
        CHECK (table_name IN ('kb.artifact_objects', 'kb.object_nodes')),
    CONSTRAINT chk_kb_object_audit_log_action
        CHECK (action IN ('resolve_object_id', 'edit_fields'))
);

CREATE INDEX IF NOT EXISTS idx_object_audit_log_row  ON kb.object_audit_log (table_name, row_key);
CREATE INDEX IF NOT EXISTS idx_object_audit_log_time ON kb.object_audit_log (create_time);

-- +goose Down
DROP TABLE IF EXISTS kb.object_audit_log;
```

`table_name` / `action` are `TEXT` + `CHECK`, matching the existing
`chk_kb_artifact_objects_reconcile_status` convention in this same feature
area — not a Postgres `ENUM` (harder to extend later) and not integer codes
(no meaningful storage win at this table's volume, and it loses
`psql`-readability for no benefit).

**Go helper** (new file `server/api/kbhandler/object_audit_log.go`):

```go
package kbhandler

import (
    "context"
    "database/sql"
    "encoding/json"
)

const (
    objectAuditActionResolveObjectID = "resolve_object_id"
    objectAuditActionEditFields      = "edit_fields"
)

// logObjectAudit is a best-effort insert into kb.object_audit_log — a
// failure here logs a Warn and never fails the caller's PATCH response,
// matching the docactivity.Log precedent used by the doc-structure editor.
func logObjectAudit(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger,
    tableName, rowKey, action, actor string, payload map[string]json.RawMessage) {
    changes, err := json.Marshal(payload)
    if err != nil {
        logger.Warn("marshal object audit changes failed", "table_name", tableName, "row_key", rowKey, "err", err)
        return
    }
    var actorArg any
    if actor != "" {
        actorArg = actor
    }
    _, err = db.ExecContext(ctx,
        `INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)`,
        tableName, rowKey, action, changes, actorArg)
    if err != nil {
        logger.Warn("insert object audit log failed", "table_name", tableName, "row_key", rowKey, "err", err)
        return
    }
    logger.Info("object audit logged", "table_name", tableName, "row_key", rowKey, "action", action)
}
```

**Wiring:**

- `UpdateArtifactObject`: after `affected == 0` check passes (row confirmed
  updated), call
  `logObjectAudit(ctx, db, logger, "kb.artifact_objects", idStr, action, structureActor(rc), payload)`
  where `action = objectAuditActionResolveObjectID` if `settingObjectID` (the
  flag the handler already computes), else `objectAuditActionEditFields`.
- `UpdateObjectNode`: same, `"kb.object_nodes"`, `objectID`, action is always
  `objectAuditActionEditFields` (this table has no object_id-resolution
  concept — `object_id` is the row key, not a settable field), actor via the
  same `structureActor(rc)` helper (already defined in
  `doc_structure_handler.go`, same package — reused, not duplicated).
- `payload` is the already-decoded `map[string]json.RawMessage` from the
  request body — the literal fields the admin submitted, before the
  `ext_info.reconcile_method` stamp (an internal side effect, not something
  the admin directly edited) is layered on.

`actor` resolution: `structureActor(rc)` returns `""` when unauthenticated;
`logObjectAudit` maps that to SQL `NULL` for `actor`.

## Testing

**Frontend** (`resolve-ambiguous-objects-client.test.ts` or a new adjacent
test file):
- `fieldDirty` unit tests: primitive equal/different, array equal/different
  (order-sensitive, matching existing `isDirty` semantics), `undefined`
  snapshot (no row loaded yet).
- Existing 8 client tests remain green (no client.ts contract changes).

**Backend** (`ambiguous_objects_handler_test.go`):
- Of the 7 existing tests, the 2 success-path ones
  (`TestUpdateArtifactObjectSuccessStampsExtInfoWhenObjectIDSet`,
  `TestUpdateObjectNodeSuccess`) get an added
  `mock.ExpectExec(...INSERT INTO kb.object_audit_log...)` expectation, since
  those are the only paths where `affected > 0` and an audit row is written.
  The other 5 (bad-request / not-found paths) are unaffected — no audit
  insert should occur, and sqlmock already fails a test if an unexpected
  query is issued, so their absence of a new expectation is itself the
  assertion.
- New: `TestUpdateArtifactObjectAuditLogsResolveObjectIDWhenObjectIDSet` —
  asserts `action = 'resolve_object_id'` when payload sets a non-empty
  `object_id`.
- New: `TestUpdateArtifactObjectAuditLogsEditFieldsWhenNoObjectIDChange` —
  asserts `action = 'edit_fields'` for a plain field edit.
- New: `TestUpdateObjectNodeAuditLogsEditFields` — same for the node handler.
- Not-found paths (`affected == 0`) must NOT emit an audit insert — covered
  by the existing not-found tests continuing to pass with no new
  `ExpectExec` for the audit table (sqlmock fails the test if an
  unexpected query is issued).

**Manual / Playwright** (matching DR6's precedent of a headless walkthrough):
confirm the two-column layout renders, Top Region "Use this" sets both
fields and the row disappears from the queue after Save (unchanged
resolved-row-leaves-queue behavior, now reliably triggered since
`reconcile_status` is no longer `'ambiguous'` after a Use-this + Save), and
dirty fields visibly turn red before Save and revert after Cancel/Save.

## Migrations

One new migration (above). No changes to the existing
`20260707000001_allow_ambiguous_resolved_reconcile_status.sql`.

## Open follow-ups (explicitly out of scope here)

- No admin UI to view `kb.object_audit_log` — if that's wanted later it's a
  separate small page/endpoint, same shape as `docactivity.List`.
- `ext_info.reconcile_method = "manual_admin"` stamping (DR6, already
  shipped) is unchanged and is not itself audit-logged as a separate
  "changes" entry — it's a side effect of the `resolve_object_id` action,
  visible by reading `kb.artifact_objects.ext_info` directly.
