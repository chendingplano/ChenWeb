## Context

Object reconciliation links each object an artifact mentions (metric, provision, inventory item)
to a canonical `kb.object_nodes` row. `FindCandidates` scores each candidate 1.0 (names overlap
and both object types are approved and compatible) or 0.85 (any other name match). With only two
possible scores, ties are common. They usually mean two nodes exist for the same thing, as with
the 管路 nodes created from records 279 and 285.

Current tie path, all in `server/api/doc-processing/`:

1. `ReconcileOne` (`artifact_objects.go`) returns `ambiguous` when `candidates[0].Score ==
   candidates[1].Score`. `FindCandidates` orders by "approved type equal to the object's" and then
   by `id`, not by score, so the check can miss a tie or report a false one, and a lone 0.85
   ahead of a 1.0 creates a new node.
2. `reconcileArtifactObjectsWithLLM` (`object_nodes.go`) sends ties to the LLM adjudicator when
   `RESOLVE_AMBIGUOUS_OBJECT_MODEL_NAME` is set (DR7). Whatever is left logs `WARN object
   reconciliation ambiguous` and is saved with `object_id` NULL.
3. The bulk backfill `ResolveAmbiguousArtifactObjects` (`object_ambiguous_resolution.go`), run by
   `POST /kb/objects/resolve-ambiguous` and by a scheduler job type, can later break remaining
   ties with `pickTieBreakCandidate`. This change leaves it as it is.
4. The Resolve Ambiguous Objects page reads `kb.artifact_objects WHERE reconcile_status =
   'ambiguous'` and recomputes candidates when a row is opened.

The processors save reconciled objects with `ReplaceObjectsForRecord`, which deletes and
re-inserts a record's rows. `kb.artifact_objects` row IDs therefore change on every re-run and
cannot identify an occurrence for long.

## Goals / Non-Goals

**Goals:**
- Every tied artifact object is linked when its record is processed.
- Each distinct tie becomes one attention item that a person can resolve or dismiss. The item
  keeps the evidence seen at detection time.
- `kb.attention_items` is general enough that later problem types need no schema change.
- The admin page works from attention items.

**Non-Goals:**
- Changing `FindCandidates` scoring.
- Merging duplicate nodes automatically. The LLM path already merges when it is confident;
  everything else stays a human decision.
- The bulk backfill and the rows already left `ambiguous`.
- Entity-to-object matching (`matchEntityToExistingObject`) and other review pages.

## Decisions

### D1. One generic table, classified by `item_type`

```sql
CREATE TABLE kb.attention_items (
    id               BIGSERIAL PRIMARY KEY,
    item_type        TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'open',
    dedupe_key       TEXT        NOT NULL,
    title            TEXT        NOT NULL DEFAULT '',
    details          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    input_record_id  BIGINT,
    occurrence_count INTEGER     NOT NULL DEFAULT 1,
    reopen_count     INTEGER     NOT NULL DEFAULT 0,
    first_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_by      TEXT,
    resolved_at      TIMESTAMPTZ,
    resolution       TEXT,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_attention_items_status CHECK (status IN ('open', 'resolved', 'dismissed'))
);
CREATE UNIQUE INDEX uq_kb_attention_items_problem
    ON kb.attention_items (item_type, dedupe_key);
CREATE INDEX idx_kb_attention_items_type_status
    ON kb.attention_items (item_type, status, last_seen DESC);
```

- `item_type` has no CHECK constraint. Types are Go constants in `server/api/attention/` and are
  validated there, so a new type needs no migration. Alternative considered: a CHECK constraint
  or a type registry table. Rejected because every new type would need a migration, for no
  benefit while one application writes the table.
- `input_record_id` is the most recent record that raised the item. It has no foreign key, so
  items survive the record being deleted or re-run.
- `details` holds type-specific evidence (see [D3](#d3-evidence)). Fields common to all types are
  real columns, so lists sort and filter without JSON.
- No column name is a reserved word (`status`, `title` and `resolution` are non-reserved in
  PostgreSQL).

### D2. Dedupe and lifecycle

One row per problem: (`item_type`, `dedupe_key`) is unique across all statuses.
`Record(ctx, item)` upserts with `ON CONFLICT (item_type, dedupe_key)`. A repeat increments
`occurrence_count`, sets `last_seen` and `input_record_id`, and merges the new `details`.

| Existing row | A repeat of the same problem |
|---|---|
| none | inserts an `open` row |
| `open` | updates it |
| `resolved` or `dismissed` | reopens it: `status = 'open'`, `reopen_count + 1`, the closing (status, `resolved_by`, `resolved_at`, `resolution`, plus `reopened_at`) appended to `details.resolution_history`, the closing columns cleared, and the new evidence merged |

A closed problem should not come back: resolving means the cause was fixed, and dismissing means
a person accepted the current data. A recurrence therefore signals that something went wrong, so
it is reopened rather than counted silently. Keeping one row per problem keeps that history in
one place. Alternative considered: a new row for each recurrence. Rejected because the reviewer
would have to find and compare the earlier rows to see that the problem came back.

`Resolve` and `Dismiss` set `status`, `resolved_by` (from the request's user, as
`kb.object_audit_log` does), `resolved_at` and `resolution` (an optional note). Only `open` items
can be resolved or dismissed. There is no reopen action in this change.

<a id="d3-evidence"></a>
### D3. `ambiguous-object-name` items

- **Dedupe key:** the tied `object_id`s, sorted and joined with `|`. The object name is not part
  of the key: two names that tie the same node set have the same cause and the same fix.
- **Title:** the object name. The page builds the displayed heading from it and the details
  through Paraglide, so no server-generated prose needs translating.
- **Details:**

```json
{
  "object_name": "管路",
  "picked_object_id": "obj_279_…",
  "tied_candidates": [
    {"object_id": "obj_279_…", "canonical_name": "管路", "object_type": "component", "score": 1, "method": "exact_name"},
    {"object_id": "obj_285_…", "canonical_name": "管路", "object_type": "component", "score": 1, "method": "exact_name"}
  ],
  "llm_outcome": "not_configured | unresolved | failed",
  "occurrences": [
    {"input_record_id": 383, "artifact_type": "metric", "artifact_id": "…", "object_name": "管路", "seen_at": "…"}
  ]
}
```

`tied_candidates`, `picked_object_id` and `llm_outcome` reflect the latest detection.
`occurrences` keeps the 50 most recent, newest first, de-duplicated on (record, artifact type,
artifact ID, object name). Occurrences store artifact identity, not `kb.artifact_objects` row IDs,
because those change on every re-run.

### D4. Reconciliation: link first, then record

- `ReconcileOne` sorts candidates by score, highest first, keeping the query order among equal
  scores. A tie is two or more candidates with the top score. Other branches keep their
  thresholds and now see the true best candidate. `reconcileArtifactObjectsWithLLM` already
  fetches the candidates of every tied object for the LLM; it now fetches them sorted and takes
  the tied set from the top of that list.
- `reconcileArtifactObjectsWithLLM` keeps its phases. The LLM still adjudicates ties first when
  configured; a confident LLM decision raises no attention item. In the final pass, each object
  still tied after the existing re-check is linked to `pickTieBreakCandidate(obj, tied)` with
  `reconcile_status = 'ambiguous_resolved'` and `ext_info.reconcile_method =
  'tie_break_deterministic'`, and an `ambiguous-object-name` item is recorded. The WARN becomes
  `INFO object reconciliation tie broken` with the item ID. If LLM merges earlier in the batch
  collapsed the tie to one node, the object is matched to it; if they removed every candidate,
  a node is created, as `ReconcileOne` does when nothing matches.
- `pickTieBreakCandidate` receives only the tied candidates, so a lower-scored node with more
  shared names cannot win.
- `ObjectReconciler` gets an optional `Attention attention.Recorder` field. The constructors on
  the tie path set it: the extract metrics, provisions and inventory items processors. The
  others (entity matching in `artifact_postprocess_indexing.go`, the backfill handler, the
  scheduler job, the page's detail handler) never reach the tie path and leave it nil. A nil recorder records nothing, which
  keeps existing unit tests unchanged.
- A failed attention write logs ERROR and does not fail the record: the link is already correct,
  and the ERROR reaches the alert log, so the problem stays visible.
- The bulk backfill (`ResolveAmbiguousArtifactObjects`) is unchanged and records no items.

Alternative considered: keep ties unlinked until a person decides, which was the original DR1.
Rejected by the user: unlinked artifacts are missing from the graph, and the tie-break pick is
usually right because the tied nodes are usually duplicates.

### D5. API

In `server/api/attention/`, registered on the same `apiGroup` as the other knowledge-base admin
routes:

| Route | Purpose |
|---|---|
| `GET /api/v1/kb/attention-items?type=&status=open&limit=&offset=` | list, newest `last_seen` first |
| `GET /api/v1/kb/attention-items/:id` | one item |
| `POST /api/v1/kb/attention-items/:id/resolve` | body `{"resolution": "…"}` |
| `POST /api/v1/kb/attention-items/:id/dismiss` | body `{"resolution": "…"}` |

The `ambiguous-object-name` detail also needs a current `kb.artifact_objects` row for the page's
existing tools. `GET /api/v1/kb/objects/ambiguous-items/:id/artifact-object` (in `kbhandler`,
which already depends on doc-processing) finds the newest occurrence that still has a row,
matched on record, artifact type, artifact ID and object name, and returns its ID. The page then
calls the existing `GET /api/v1/kb/objects/ambiguous/:id` detail endpoint, which loads any
status.

### D6. The admin page

`resolve-ambiguous-objects-view.svelte`:
- The left list shows open `ambiguous-object-name` items: title, occurrence count, last seen.
- Selecting an item loads its representative artifact object through D5 and shows the existing
  detail and candidate panels. An occurrences panel lists every artifact that hit the tie, and
  the tied set seen at detection time is shown next to the live candidates.
- New Resolve and Dismiss buttons, sharing one optional note field. They are disabled while the
  artifact object has unsaved edits. Saving no longer removes anything from the list. The existing actions (use
  candidate, merge, find linked and rebind, create node) still act on artifact objects and nodes.
  After using them, the reviewer closes the item with Resolve.
- If no occurrence has a current row (every source record was deleted), the page shows the
  stored evidence and only Resolve and Dismiss.
- All new text uses Paraglide keys (`resolve_ambiguous_objects_*`) in both message files.

`GET /api/v1/kb/objects/ambiguous` and `ListAmbiguousSummaries` lose their only caller and are
removed.

## Reused modules

Nothing in the catalogue covers a typed review queue, an admin review list or object
reconciliation. Code reused without change: `pickTieBreakCandidate` (given only the tied subset),
`fetchSortedCandidates`, `structureActor`, and the merge, rebind, create-node, artifact-object
detail and update endpoints. `kb.keyword_unresolved` is the closest existing code, but it is keyed
on keyword normal forms and scopes inside the ontology keyword package, so extending it would bind
a general queue to one feature. `server/api/attention/` is built for reuse and is registered with
`create-reusable-module` once it works.

## Risks / Trade-offs

- [The tie-break picks the wrong node] → The item lists every tied candidate and every affected
  artifact. Rebinding or merging fixes all of them at once, and `reconcile_method =
  'tie_break_deterministic'` finds every row linked by a guess.
- [A resolved item reopens because the reviewer resolved it without fixing the cause] → Intended:
  the reopen shows the fix did not hold, and `resolution_history` shows the earlier closing.
- [Items stay open after the cause is fixed, for example after a merge on another page] → Items
  are closed explicitly. Auto-closing on merge can follow once real use shows it is needed.
- [`details` grows for a hot tie] → `occurrences` is capped at 50; `occurrence_count` keeps the
  true total.
- [The DR7 LLM adds latency before the tie-break] → Unchanged from today; the LLM is optional.
- [Concurrent pipelines record the same tie at once] → The partial unique index and `ON
  CONFLICT` make the upsert atomic.

## Migration Plan

1. Deploy the migration and code. New ties are linked and recorded from then on.
2. Close issue #18 once production shows no new `object reconciliation ambiguous` warnings and the
   items are on the page.

Rows already `ambiguous` stay as they are and are no longer listed on the page. The unchanged
backfill can still link them if that is wanted later.

Rollback: revert the code. The table can stay. Rows linked by the tie-break keep their links,
which the old code and page accept.

## Open Questions

- Should the page or the merge endpoint close items automatically when a merge removes every tie
  in the set? Left out for now (see Risks).
