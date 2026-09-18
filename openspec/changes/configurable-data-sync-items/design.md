## Context

`production-data-sync` (shipped 2026-09-15/16, see `openspec/changes/production-data-sync/`) deliberately chose a compiled-in registry (Decision 1: "Sync item = a small Go struct, not an interface hierarchy... Rejected a plugin/interface design as premature") and explicitly scoped out other item shapes (Non-Goal: "No non-table sync-item types... the registry shape must not foreclose them, but none are implemented now"). Both were the right call at the time — there was exactly one item, and per-item custom logic wasn't needed yet. Two things changed: an admin now wants to register a new item without a code deploy, and the next item's data isn't a bare row — `kb.videos` (and similarly-shaped tables) stores bytes on disk (`VIDEO_DIR`, resolved per-deployment) with only a path in the DB row.

Both `TableSyncItem` and the pull/apply pipeline (`pull_handler.go`, `client.go`, `upsert.go`) currently assume every item is fully described by a compiled struct that source and target share simply because they run the same binary. That assumption breaks the moment an item can be created via UI on one instance's own database — the *other* instance in a sync pair has no way to know that item exists, let alone its shape, unless something tells it.

## Goals / Non-Goals

**Goals:**
- Let a sysadmin create, edit, and delete a sync item from the Sync Data page, live, no redeploy — for either kind.
- Add `table_with_files` as a second item kind: copy the file first, then the row, rewriting the row's file column to the new local path.
- Make a source's admin-created items actually usable by a target, without requiring an admin to hand-replicate the same form on every box.
- Ship `kb.videos` as the concrete end-to-end `table_with_files` proof, the same way `kb_product_names` proved the first kind.

**Non-Goals:**
- No edit or delete of a *compiled* item — those still require a code change (`registry.go`) and a redeploy, same as today; the create/edit/delete endpoints only ever touch `kb.data_sync_items`.
- No edit of a *learned* item's shape — a target never authors the shape of an item it only knows about because its source advertised it; see Decision 6.
- No schema introspection / auto-suggested columns or natural-key candidates in the New Data Syncher form — an admin types the same fields a compiled `TableSyncItem` literal already has today. Nice-to-have, not asked for.
- No file de-duplication or change-detection on re-sync — a `table_with_files` row that's re-fetched (e.g. its metadata changed but not its file) re-copies the file every time. Simple and correct, just not bandwidth-optimal; flagged in Risks, not solved here.
- No garbage collection of target-local files whose source row's file changed or disappeared — consistent with the existing "a sync never deletes" rule for rows, extended to files for the same reason (a sync must never be the thing that destroys data unexpectedly).
- No generalization beyond two kinds — no plugin interface, no arbitrary per-item Go code. A `Kind` enum with kind-specific optional columns is enough for two kinds and matches Decision 1's original reasoning.

## Decisions

**1. `kb.data_sync_items` holds admin-created items; compiled `Registry` and this table are unioned, compiled wins on id collision.**
Rather than migrating `kb_product_names` into the database or inventing a plugin abstraction, DB-backed items are purely additive. `ItemByID`/list logic becomes: check `Registry` first (in-memory, zero DB cost, can't be shadowed by a bad UI entry), then fall back to `kb.data_sync_items`. This keeps the existing item working completely unmigrated and keeps "add a new item without deploying" strictly opt-in per item.

Rejected: converting the compiled registry into DB rows at startup (a migration/seed step). Unnecessary churn for one already-working item; "don't refactor things that aren't broken" (`ChenWeb/CLAUDE.md` §1.3).

**2. `TableSyncItem` gains `Kind`, `FileColumn`, `FileDirEnv`, `FileDirDefaultSubdir` — no interface hierarchy.**
```go
type SyncKind string
const (
    KindTable           SyncKind = "table"             // zero value, default
    KindTableWithFiles  SyncKind = "table_with_files"
)

type TableSyncItem struct {
    ID, Table, CursorCol string
    NaturalKey, Columns, JSONColumns []string
    Filter string
    Kind SyncKind // "" behaves as KindTable, so existing compiled items need no edits

    // Only meaningful when Kind == KindTableWithFiles:
    FileColumn           string // one of Columns; holds a server-local absolute path
    FileDirEnv            string // env var naming the target's storage dir, e.g. "VIDEO_DIR"
    FileDirDefaultSubdir  string // fallback subdir under DATA_HOME_DIR, e.g. "Videos" — mirrors videohandler.videoDir()'s own resolution order
}
```
`FileDirEnv`/`FileDirDefaultSubdir` generalize `videohandler.videoDir()`'s exact resolution pattern (env override, else `<DATA_HOME_DIR>/<subdir>`) into item config, rather than datasync importing videohandler or videohandler exporting internals — keeps the two packages decoupled (root `CLAUDE.md` "Project Isolation": avoid tight coupling between non-shared code). The small amount of path-join/sanitize logic this needs is reimplemented locally in `datasync` rather than shared; it's a few lines, and `UploadVideo`'s version isn't exported.

Rejected: a `SyncItem` interface with per-kind implementations (`Fetch()`, `Apply()` methods). Two kinds sharing ~90% of their logic (the row fetch/upsert path is identical; only file materialization is kind-specific) doesn't justify an interface — a single `if item.Kind == KindTableWithFiles` branch ahead of the existing `upsertRows` call is simpler to read and matches Decision 1's original rejection of "plugin/interface design as premature."

**3. Files are fetched by natural key, never by a client-supplied path.**
`GET /api/internal/data-sync/items/:itemId/files?key=<url-encoded JSON array, NaturalKey order>`. The source re-derives the file's actual path server-side: `SELECT <file_column> FROM <table> WHERE [<filter> AND] (<natural key columns>) = (<key values>)`, then streams whatever that query returns. The target never sends — and the source never trusts — a raw filesystem path. This closes an otherwise-obvious path-traversal / filesystem-layout-disclosure hole, and costs nothing extra since the target already has each row's natural key from the changes page.

**4. A target write-through-caches item definitions it learns from its configured source.**
New source endpoint `GET /api/internal/data-sync/items` (shared-secret authed, same as `.../changes`) returns every item the source can act as a source for — compiled ∪ its own `kb.data_sync_items` — as plain definitions (id, kind, table, cursor col, natural key, columns, json columns, filter, file config). On the target, `HandleListSyncItems` calls this (best-effort — a failure just means the list falls back to what's already known, not a hard error) and **upserts** each returned definition into its own `kb.data_sync_items` with `origin = 'learned'` (create if new, overwrite the stored shape if the source's definition changed since the last refresh — see Decision 6 for why this is genuinely an upsert, not insert-if-missing), then lists the union of `Registry` ∪ its own table (which now includes freshly-learned items). `HandlePreviewSync`/`HandleApplySync` then resolve `itemId` the same way (`Registry` ∪ local table) — no separate "is this known" round-trip needed at sync time, since list already persisted it.

This is the piece that makes "create it once on the source" actually usable on a target, instead of requiring an admin to open the same form twice with byte-identical field values (a guaranteed-eventually-inconsistent workflow). Without it, a target's Sync Data page would either never show a source-only item, or would need to fail Preview/Sync with "unknown item" the first time, before anyone has thought to hand-copy the definition — a broken-feeling feature.

Trade-off accepted: if two different upstream sources both define an item with the same id but different shapes (unlikely, but possible in a future multi-source setup), the target's cached copy is simply whatever it last learned, last-writer-wins on each list call. No identity/ownership system is built to prevent this — not asked for, and today every target has exactly one configured source.

**5. Admin-submitted SQL-shaped fields are validated as identifiers before being persisted, not just trusted like compiled Go source is.**
A compiled `TableSyncItem` is written by a developer and reviewed like any other code change; a `kb.data_sync_items` row comes from an HTML form filled in by a sysadmin, then gets string-interpolated into generated SQL (`fetchChangesPage`, `upsertRows`, and the new per-file lookup) exactly like compiled items' fields already are. The create endpoint validates: `Table` matches `^[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*$` (schema.table); every entry in `Columns`, `NaturalKey`, and `CursorCol`/`FileColumn` matches `^[a-z_][a-z0-9_]*$` and (for `NaturalKey`/`FileColumn`) is present in `Columns`; `Filter` is accepted as free-text SQL (it's already a raw `WHERE` fragment by design in the compiled case — no available identifier-only grammar covers it) but only reachable by someone who could already run arbitrary SQL as a signed-in sysadmin, so this is fat-finger protection, not a trust-boundary fix. `Kind` must be one of the two known values; `table_with_files` additionally requires `FileColumn` and at least one of `FileDirEnv`/`FileDirDefaultSubdir` set.

**6. Every `kb.data_sync_items` row carries an `origin` of `local` or `learned`; edit and delete are gated on it.**
`local` = created through this instance's own New Data Syncher form (`created_by` a real user). `learned` = write-through-cached from this instance's configured source via discovery (Decision 4). The gating:
- **Create** always produces `origin = 'local'` — there's no other way to author a shape.
- **Edit** (`PUT /api/v1/data-sync/items/:itemId`) is only allowed on `origin = 'local'` items. A `learned` item's shape is owned by whichever source advertised it; allowing a local edit would just get silently overwritten by the next list refresh (Decision 4), which is confusing, not useful. A compiled item isn't in this table at all, so `PUT` 404s the same way `ItemByID` already 404s for an unknown id today, plus a clearer message when the id *is* a compiled one ("edit `registry.go` and redeploy instead").
- **Delete** (`DELETE /api/v1/data-sync/items/:itemId`) is allowed on both `local` and `learned` items — deleting a `learned` item only ever removes this instance's local cache row (harmless: it's re-learned on the next successful discovery refresh if the source still offers it, and simply stays gone if the source deleted it too). Deleting a `local` item also deletes its `kb.data_sync_state` row in the same request (application-level cascade — no FK constraint, since `kb.data_sync_state` also holds rows for compiled items that have no `kb.data_sync_items` parent to reference). A compiled item can't be deleted.
- **Editing a `local` item resets its sync state** (clears `kb.data_sync_state` for that id) rather than trying to determine whether the specific fields that changed were "safe" (e.g. only `Filter`) versus structural (`CursorCol`, `NaturalKey`, `Kind`). A stale cursor evaluated against a changed shape is a correctness bug (Decision 4/5 of `production-data-sync/design.md` already establish how load-bearing the cursor's meaning is); resetting unconditionally is simple, always safe, and the cost — one full re-sync after an edit — is cheap for an admin-triggered, infrequent action.

Rejected: letting a target edit a `learned` item's local copy as a manual override (e.g. "I want files to land in a different directory on this box than the source suggests"). `FileDirEnv`/`FileDirDefaultSubdir` are already resolved per-instance against that instance's own environment (Decision 2), so this need doesn't actually arise — two boxes can share an item definition and still store files in different real directories, because the env var itself differs per box. No override mechanism needed.

**7. `kb.videos` needs a migration before it can be a sync item: `update_time` + `kb.set_update_time()` trigger, and `UNIQUE(stored_path)`.**
`kb.videos` today has only `created_at` (insert-time, never bumped) and no unique constraint besides the surrogate `id` — neither a valid `CursorCol` nor a valid `NaturalKey` per the existing rules (Decision 4 and 2 of `production-data-sync/design.md`, unchanged by this change). `stored_path` is unique in practice by construction (`UploadVideo` names it `<UnixNano>_<sanitized-filename>`), so `UNIQUE(stored_path)` is safe to add without a backfill conflict. Reuses the existing trigger function — no new trigger code.

**8. `kb.videos` references its cover image by a stable natural key (`kb.images.uid`), not the surrogate `id`.**
While registering `kb.videos` as the `video` item (task 6.1), we found it carries a second file-shaped reference beyond `stored_path`: `image_id BIGINT`, a nullable FK to `kb.images.id` added in `20260722000004_alter_kb_videos_metadata.sql` ("soft reference — no cascade, a deleted cover just 404s"). `kb.images` is synced independently via its own `table_with_files` item (`kb-images`, local-origin, created via the admin UI). Two designs were considered and rejected before landing on this one:

- *Add a second `FileColumn` (or a new `table-with-two-files` kind) to `table_with_files`.* Rejected: both frame `image_id` as "another column that already holds a local path, fetch that blob too," which isn't what it is — the image's bytes live one hop away, in `kb.images.stored_path` on the source. Neither option touches the actual defect: `image_id` is a surrogate id copied verbatim from source to target by the ordinary `table`/`table_with_files` row-copy path (`upsertRows`), and source/target `BIGSERIAL` sequences are independent (this is exactly why `NaturalKey` is documented and validated to exclude surrogate keys everywhere else in this system). A raw copy of `image_id` has no guaranteed relationship to the correct row in the target's `kb.images` — it silently points at the wrong row, or none.
- *Store the referenced row's `stored_path` instead of its `id`.* Rejected: `materializeFiles`/`fetchAndSaveFile` (Decision 2) always mint a fresh `<UnixNano>_<sanitized-name>` path when saving a `table_with_files` row's file on a new instance — the same construction `storeImage`/`UploadVideo` use on create. `kb.images.stored_path` is therefore guaranteed to differ between source and target for the same logical row, for the same structural reason `image_id` is wrong: it's local-instance-generated, not portable.

**Decision:** add `kb.images.uid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE`, assigned once at row creation and never touched by `materializeFiles` (which only ever rewrites `FileColumn`, i.e. `stored_path`) — so it's identical on source and target for the same logical row. Replace `kb.videos.image_id` with `kb.videos.image_uid UUID`, a plain data column holding a copy of the referenced `kb.images.uid`. Redefine the `kb-images` item's `NaturalKey` as `[uid]` (replacing `stored_path`), so its own upsert identity is stable across re-syncs too, not just its cross-referenceability. No `REFERENCES` constraint is added — `image_id` never had one either (the migration's comment describes a reference; nothing in SQL enforced it), so `image_uid` stays faithful to that same soft-reference, tolerate-a-missing-row style rather than introducing new enforcement that wasn't asked for.

This needs zero `datasync`/Go changes: `image_uid` is an ordinary `Columns` entry on the `video` item, exactly like `stored_path` already is for a table's own row identity — not a `FileColumn`, nothing to materialize. `video`'s `Kind`/`FileColumn` are unchanged from Decision 2. This is why neither of the two options first considered is needed: once the FK is represented as a portable natural-key value instead of an instance-local surrogate id, there's nothing left for a second file-kind or a second `FileColumn` to do.

Accepted caveat: if `video` is applied before `kb-images` in a given sync run, `image_uid` on the target briefly names a `uid` not yet present in the target's `kb.images` — the same "soft reference, tolerate a missing row" behavior the column already had via `id`, just scoped to a transient window (until `kb-images` next syncs) instead of a permanent mismatch.

## Risks / Trade-offs

- **[Re-syncing a `table_with_files` item re-copies files whose content hasn't changed, just because some other column on the row changed]** → Mitigation: none built; accepted for now given video files sync manually and infrequently (admin-triggered, not scheduled). If this becomes a real cost, a follow-up could skip the file fetch when a source-provided content hash/size matches what's already at the target path — not built here (Non-Goals).
- **[A `table_with_files` sync can be interrupted mid-row: file copied, row upsert not yet run (or vice versa)]** → Mitigation: `upsertRows` still runs inside its existing transaction per page; a row is only upserted after its file fetch succeeds, so a crash between "file saved" and "row upserted" leaves at worst an orphaned file (never a row pointing at a missing file). The cursor still only advances on full page success, so a retry re-fetches and re-upserts that row — an idempotent overwrite of the same file plus a no-op orphan of the first copy. Consistent with the existing "safe to just click Sync again" property; not zero-waste, but never incorrect.
- **[Learning item definitions from a source and caching them locally means a target's `kb.data_sync_items` can grow entries it never explicitly asked for]** → Mitigation: this is the intended behavior (Decision 4) and mirrors how the target already implicitly "knows" every compiled item without asking; unlike a first draft of this design, an admin *can* now remove an unwanted learned entry (Decision 6's delete path) — it just isn't automatic, so a stale learned item lingers until someone notices and deletes it.
- **[SQL-identifier validation on the create/edit endpoints (Decision 5) is defense-in-depth, not a hard trust boundary]** → Already-noted: creating or editing a sync item requires the same sysadmin session that could already run arbitrary SQL through other admin tooling in this codebase. Validation prevents fat-fingered breakage of the sync feature itself, not a privilege escalation.
- **[Editing a `local` item always resets its sync state (Decision 6), even for a purely cosmetic change like tightening `Filter`]** → Accepted: admin-triggered edits are expected to be rare, and a full re-sync is a bounded, idempotent, upsert-only operation (never destructive) — cheaper than building field-level change detection to avoid it.
- **[`kb.videos.image_uid` can transiently reference a `uid` the target's `kb.images` doesn't have yet (Decision 8), if `video` is synced before `kb-images`]** → Mitigation: none built; accepted as the same "soft reference, missing cover shows a placeholder" tolerance the column already had via `image_id`, just self-healing once `kb-images` next syncs instead of being permanently wrong. An admin who wants covers correct immediately syncs `kb-images` first.

## Migration Plan

1. Goose migrations: create `kb.data_sync_items`; add `update_time` (+ attach `kb.set_update_time()`) and `UNIQUE(stored_path)` to `kb.videos`.
2. Extend `datasync.TableSyncItem` with `Kind`/`FileColumn`/`FileDirEnv`/`FileDirDefaultSubdir`; add DB-backed CRUD (`InsertItem`, `dbItemByID`, `dbListItems`) and union them with `Registry` in `ItemByID`/list.
3. Source side: add the per-file streaming handler and the item-definitions handler; register both routes next to the existing `.../changes` route (same ungated, shared-secret-authed placement).
4. Target side: add `POST`/`PUT`/`DELETE /api/v1/data-sync/items[/:itemId]` (create/edit/delete, origin-gated per Decision 6, with identifier validation and sync-state reset on edit / cascade on delete); extend `HandleListSyncItems` to fetch+cache (upsert) the source's advertised items; add file materialization ahead of `upsertRows` for `table_with_files` items in `HandleApplySync`.
5. Admin UI: "New Data Syncher" button opens a form (kind selector; shared fields; file-only fields shown when kind is `table_with_files`) on `sync-data-view.svelte`; each `local`/`learned` row gets Edit (local only) and Delete actions; all post through new `sync-data-client.ts` functions.
6. Use the new UI to create the `kb_videos` item on the Mac (source) after its migration lands; verify on a target (or a second local instance standing in for one, as `production-data-sync`'s own verification did): the item appears via discovery, Preview reports the expected row count, Apply copies files and rows, a re-Preview reports zero.
7. Once end-to-end verification (step 6) passes, rewrite `2026091601-devdoc-data-sync.md` as the source-of-truth runbook (tasks.md §8) — this is the last step, not done alongside the code.
8. Decision 8 addendum, discovered while doing step 6: goose migrations add `kb.images.uid` and replace `kb.videos.image_id` with `image_uid` (backfilled via a join on the existing `image_id`→`kb.images.id` relationship before it's dropped); re-edit the already-created `kb-images` item's `NaturalKey` to `[uid]`; create the `video` item's `Columns` with `image_uid` (not `image_id`). No `datasync` code changes.

**Rollback:** everything here is additive — new table, new nullable-by-default columns on the compiled struct, new routes, new `kb.videos` columns/constraint. Reverting the binary is sufficient; the new goose migrations can be downed independently like any other migration. No existing item, route, or data is altered.

## Resolved Questions

Both questions raised in the first draft of this design were resolved by the requester rather than left as assumptions:

1. ~~Should `production-data-sync` be archived into `openspec/specs/`?~~ **Resolved: no.** Instead, `KnowledgeStore/doc-repo/devdocs/202609/2026091601-devdoc-data-sync.md` becomes the single source-of-truth document for how data sync works operationally (setup, usage, troubleshooting, the full registry/CRUD/discovery/file-kind picture), referencing both `production-data-sync/` and this change for design rationale rather than duplicating it. Updated as the last step of implementation (tasks.md §8), once the feature is verified end-to-end — so the devdoc describes the as-built system, not a proposal.
2. ~~Is create-only sufficient?~~ **Resolved: no — full create/edit/delete is required.** See Decision 6.
