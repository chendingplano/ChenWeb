## 1. Database migrations

- [ ] 1.1 Use the `db-migration` skill to add a goose migration creating a reusable `public.set_update_time()` trigger function and attaching a `BEFORE UPDATE` trigger to `kb.product_names` that sets `update_time = now()`.
- [ ] 1.2 Use the `db-migration` skill to add a goose migration creating `kb.data_sync_state` (`sync_item_id TEXT PRIMARY KEY`, `last_cursor TIMESTAMPTZ`, `last_synced_at TIMESTAMPTZ`, `last_row_count INTEGER NOT NULL DEFAULT 0`, `last_error TEXT`).
- [ ] 1.3 Confirm both migrations apply cleanly against the local dev DB (via `mise dev`'s auto-migrate) and check `project_db_migration` for the new version ids.

## 2. Sync item registry (shared by source and target)

- [ ] 2.1 Create `server/api/datasync/registry.go` with a `TableSyncItem{ID, Table, CursorCol, NaturalKey, Columns, Filter}` struct and a `Registry []TableSyncItem` containing one entry for `kb_product_names` (table `kb.product_names`, cursor `update_time`, natural key `(source, seq_no, product_name)`, columns per the current schema excluding `id`, filter `source = 'cn_nmpa_medical_device_classification_catalog'`).
- [ ] 2.2 Add a small lookup helper (`ItemByID(id string) (TableSyncItem, bool)`) for use by both the pull handler and the admin handlers.

## 3. Source pull API (Mac side)

- [ ] 3.1 Implement the pull handler in `server/api/datasync/pull_handler.go`: `GET /api/internal/data-sync/items/:itemId/changes?since=&limit=` — looks up the item, runs `SELECT <columns> FROM <table> WHERE [<filter> AND] <cursor_col> > $1 ORDER BY <cursor_col>, <natural key...> LIMIT $2`, and returns `{item_id, next_cursor, has_more, rows}` (`next_cursor` = max `cursor_col` in the returned page, or the incoming `since` if the page is empty).
- [ ] 3.2 Add shared-secret auth: read `DATA_SYNC_SHARED_SECRET` from env, compare the `Authorization: Bearer <token>` header with `crypto/subtle.ConstantTimeCompare`, returning 500 if unconfigured and 401 on mismatch — following `shared/go/api/auth/sms_relay.go`.
- [ ] 3.3 Return a 404-class error for an unknown `itemId`.
- [ ] 3.4 Register the route in `server/api/routes.go` directly on root `e`, before `apiGroup := e.Group("/api/v1")` is created, so it's exempt from both the frontend session gate and `authmiddleware.AuthMiddleware` (mirror the `/api/internal/mitmproxy/ingest` / `/api/internal/agent-tools` placement).
- [ ] 3.5 Default `limit` to a generous value (e.g. 5000) when not supplied, sized well above `kb_product_names`'s expected row count so pagination normally never triggers.

## 4. Target admin API (box side)

- [ ] 4.1 Implement `server/api/datasync/admin_handler.go`: `GET /api/v1/data-sync/items` returning each registry item joined with its `kb.data_sync_state` row (or a "never synced" default).
- [ ] 4.2 Implement `POST /api/v1/data-sync/items/:itemId/preview`: reads the item's stored cursor, calls the configured source's pull endpoint once (or loops on `has_more`) with `DATA_SYNC_SOURCE_URL` + `DATA_SYNC_SHARED_SECRET`, returns `{item_id, changed_row_count}` — no writes, no cursor advance.
- [ ] 4.3 Implement `POST /api/v1/data-sync/items/:itemId/apply`: same fetch loop as preview, but for each page upserts rows via `INSERT ... ON CONFLICT (<natural key>) DO UPDATE SET <non-key columns>`, then on full success writes the source-reported `next_cursor`, `last_synced_at`, `last_row_count` to `kb.data_sync_state`; on any failure leaves the stored cursor untouched and records `last_error`.
- [ ] 4.4 Register these three routes under `apiGroup` (Kratos-session-authenticated) in `server/api/routes.go`.
- [ ] 4.5 Read `DATA_SYNC_SOURCE_URL` / `DATA_SYNC_SHARED_SECRET` from env at request time; return a clear configuration error if either is unset (matches how the source handler fails closed when its own secret is unset).

## 5. Admin UI

- [ ] 5.1 Add a `sysadmin-resources-sync-data` entry to the Resources submenu in `web/src/lib/components/home3/nav-rail.svelte`, alongside the existing `sysadmin-resources-videos` / `sysadmin-resources-external-terminology` entries.
- [ ] 5.2 Wire the new `childId` into `web/src/lib/components/home3/content-panel.svelte`'s render branches.
- [ ] 5.3 Create `web/src/lib/components/home3/sync-data-view.svelte` + a `sync-data-client.ts`, mirroring `llm-accounts-view.svelte` / `llm-accounts-client.ts`: list registered items with last-synced time/row count, a "Preview" button per item showing the changed-row count, and a "Sync" (apply) button that appears after preview, showing the synced-row result. Surface source-unreachable / config errors in the existing error-banner style.

## 6. Local verification

- [ ] 6.1 Unit-test the pull handler's cursor/pagination query (empty cursor, non-empty cursor, `has_more` boundary) and the natural-key upsert query (insert path, update path, filtered-out row is never touched).
- [ ] 6.2 Exercise preview/apply end-to-end locally against a second local Postgres database standing in for the "target," pointed at the dev Mac's own pull endpoint as "source," confirming: first preview shows the full catalog row count, apply upserts them all and advances the cursor, second preview shows zero.
- [ ] 6.3 `cd ChenWeb && mise build-server-linux` (or the relevant build task) still succeeds with the new package.

## 7. Deployment (tracked here, executed outside this code change)

- [ ] 7.1 Reserve and configure the home-router port-forward on the Mac's network (tentatively public port 8055 → the Mac's ChenWeb backend port) and confirm TLS is actually terminated in front of it before enabling.
- [ ] 7.2 Add `DATA_SYNC_SHARED_SECRET` to the Mac's `mise.local.toml [env]` and restart `mise dev`.
- [ ] 7.3 Add `DATA_SYNC_SHARED_SECRET` (same value) and `DATA_SYNC_SOURCE_URL` to the China box's `.env`, then `systemctl restart chenweb` there.
- [ ] 7.4 On the box's new Sync Data admin page, confirm Preview reports the expected `kb_product_names` changed-row count, Apply syncs successfully, and a follow-up Preview reports zero.
