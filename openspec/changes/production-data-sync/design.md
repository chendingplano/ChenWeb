## Context

ChenWeb runs as the same binary on every deployment (Mac dev, the China box, future customer boxes), each against its own Postgres. Reference data curated during dev (e.g. `kb.product_names`, the NMPA classification catalog) currently only reaches a deployed box via a manual DB dump/restore. Deployed boxes must be able to reach out to the dev Mac — never the other way — because a customer box may sit behind a firewall that allows outbound traffic but blocks inbound (confirmed constraint). The Mac has no public IP, so reaching it at all requires a home-router port-forward exposing a Mac-side listener to the internet; that listener is the existing ChenWeb backend, reusing an already-established pattern in this codebase for token-authenticated, non-Kratos "internal" endpoints (`shared/go/api/auth/sms_relay.go`, `server/api/proxytracehandler`, `server/api/agentservicehandler/tools.go`), all registered directly on the root Echo instance ahead of `apiGroup := e.Group("/api/v1")` so they skip both the frontend session-auth gate and `authmiddleware.AuthMiddleware`.

`kb.product_names` is not only touched by the one-time catalog import (`server/cmd/product-names-import`, `status='approved'`) — a deployed box's own doc-processing pipeline independently creates `status='proposed'` rows there (`server/api/doc-processing/product_names_resolve.go`, `extract-products.go`). A sync must not touch those locally-generated rows.

## Goals / Non-Goals

**Goals:**
- Let a deployed box pull specific, registered data items from the dev Mac, admin-triggered, over a token-authenticated connection the box itself initiates.
- Ship one concrete item end-to-end: `kb.product_names` catalog rows, incrementally.
- Keep the item registry pluggable enough that a second Postgres-table item is "add a registry entry," not "write new sync code."

**Non-Goals:**
- No delete propagation (target rows are never removed by a sync; confirmed decision).
- No bidirectional sync — the Mac is always source, the box is always target, for this change.
- No scheduling/automation — sync is a manual admin button; `kb.schedules` is a plausible future hook, not built here.
- No non-table sync-item types (config files, prompts, documents) — the registry shape must not foreclose them, but none are implemented now.
- No multi-tenant/customer data scoping — this change targets the China box; nothing here should make that harder later, but nothing customer-specific is built now.
- No TLS termination code — the pull endpoint is plain HTTP from Echo's point of view, same as `:8090` on the box today; putting TLS in front of the forwarded Mac port is a deploy-time infra decision, not something this change implements.

## Decisions

**1. Sync item = a small Go struct, not an interface hierarchy.** A `datasync.TableSyncItem{ID, Table, CursorCol, NaturalKey, Columns, Filter}` describes everything both sides need — the pull handler and the apply logic both read the same compiled-in `datasync.Registry []TableSyncItem`, since source and target run the same binary. Adding item #2 means adding a struct literal, not new code paths. Rejected a plugin/interface design as premature — nothing today needs per-item custom logic.

**2. Natural key, not `id`, is the upsert conflict target.** `kb.product_names.id` is a per-database `BIGSERIAL` — the box already inserts its own rows (the `proposed` ones) into the same sequence space as anything a sync would insert, so syncing `id` values directly would collide or silently overwrite unrelated local rows. The table already has `UNIQUE(source, seq_no, product_name)`; the `kb_product_names` item uses that as its `NaturalKey`, upserting via `ON CONFLICT (source, seq_no, product_name) DO UPDATE SET ...`. Synced rows get a fresh locally-generated `id` on insert; nothing depends on `id` matching across databases (`kb.products.product_name_id` only needs to be valid within its own database).

**3. `kb_product_names` syncs only catalog rows, via a `Filter`.** `TableSyncItem.Filter` is a raw SQL `WHERE` fragment (`source = 'cn_nmpa_medical_device_classification_catalog'`) ANDed into the source's pull query. This keeps the sync scoped to Mac-authored reference data and structurally excludes each box's own `status='proposed'` rows from ever being touched — those never match the filter, so they're not fetched, and (per Non-Goals) never deleted either way.

**4. Cursor is the source's `update_time`, and the client persists the source-reported cursor, not a locally-recomputed one.** The pull response includes `next_cursor` computed **by the source** as `MAX(update_time)` over the returned page. The target must persist exactly that value, because the target's own `update_time` trigger (Decision 5) will overwrite each upserted row's `update_time` to the target's own `now()` on apply — a target-side value that has no relationship to the source's change history. Storing anything other than the source-reported cursor would desync the "changed since" query.

**5. Add a generic `BEFORE UPDATE` trigger, `public.set_update_time()`, and attach it to `kb.product_names`.** Today `update_time` only defaults on `INSERT`; nothing bumps it on `UPDATE`, so it can't be trusted as a change cursor. The trigger function is written to be reusable by any future syncable table (`NEW.update_time = now(); RETURN NEW;`), attached via a per-table `CREATE TRIGGER ... BEFORE UPDATE ON kb.product_names`.

**6. Cursor + run state lives in a new `kb.data_sync_state` table on the target**, one row per `sync_item_id`: `last_cursor TIMESTAMPTZ`, `last_synced_at TIMESTAMPTZ`, `last_row_count INTEGER`, `last_error TEXT`. This is the only new table; it's deliberately a flat snapshot (not an append-only run log) since nothing today needs sync history beyond "when did this last succeed and how many rows."

**7. Preview/apply mirrors the existing LLM-accounts-import UX**, including the request shape: `POST .../preview` and `POST .../apply` take no body and are independently idempotent-safe — `apply` re-runs the fetch-from-source itself (using the currently stored cursor) rather than trusting whatever the browser saw during `preview`, exactly like `ImportModelsTOMLApply` re-parses `.models.toml` itself rather than trusting the browser's preview payload.

**8. Route split:**
- Source side (Mac), ungated, registered on root `e` before `apiGroup`: `GET /api/internal/data-sync/items/:itemId/changes?since=<RFC3339|empty>&limit=<int>`, header `Authorization: Bearer <DATA_SYNC_SHARED_SECRET>`, checked with `subtle.ConstantTimeCompare` exactly like `sms_relay.go`. Returns `{item_id, next_cursor, has_more, rows: [...]}`, rows ordered by `(cursor_col, natural_key...)` for deterministic pagination boundaries.
- Target side (box), Kratos-authenticated (`apiGroup`): `GET /api/v1/data-sync/items` (list + last-run status), `POST /api/v1/data-sync/items/:itemId/preview` (calls the source, does not write), `POST /api/v1/data-sync/items/:itemId/apply` (calls the source in a `has_more` loop, upserts, advances `data_sync_state`).

**9. Env vars:** `DATA_SYNC_SHARED_SECRET` (same value on both ends — Mac verifies it, box sends it); `DATA_SYNC_SOURCE_URL` (box-only, e.g. `https://<mac-public-host>:8055`, the forwarded address).

## Risks / Trade-offs

- **[Home-router port exposes a new internet-facing surface on the Mac's network]** → Mitigation: the exposed route is read-only (a single `GET .../changes` endpoint, no mutation possible through it), gated by a constant-time-compared shared secret; recommend a non-obvious port and treating the secret like any other production credential (rotate if ever suspected leaked).
- **[Secret sent in a header over what may be plain HTTP if TLS isn't actually terminated in front of the forwarded port]** → Mitigation: this is called out as a deploy prerequisite (see Migration Plan); the code doesn't silently degrade to "secure," so it must be verified before the port-forward goes live, not assumed.
- **[Timestamp-boundary ties: two rows with the identical `update_time` split across a pagination page boundary could cause one to be skipped]** → Mitigation: not solved precisely in this change — `limit` defaults generously high (5000) relative to the table's actual size (hundreds–low-thousands of rows), so pagination essentially never triggers in practice for `kb_product_names`. Flagged as a real gap for any future larger sync item.
- **[Source Mac is asleep/offline/mid-restart when an admin clicks Sync on the box]** → Mitigation: standard HTTP failure surfaces as an error banner in the UI (same pattern as LLM-accounts-import errors); `data_sync_state.last_error` records it; cursor does not advance on failure, so retry is always safe.

## Migration Plan

1. Goose migrations: `public.set_update_time()` trigger function + attach to `kb.product_names`; create `kb.data_sync_state`.
2. Implement `server/api/datasync/`: registry, source pull handler, target preview/apply handlers, natural-key upsert.
3. Wire routes in `server/api/routes.go` (internal group pre-`apiGroup`; admin group inside `apiGroup`).
4. Add the `sysadmin-resources-sync-data` nav entry + `sync-data-view.svelte` + client, mirroring `llm-accounts-view.svelte`.
5. Deploy prerequisites (manual, outside this change): reserve/forward a public port on the Mac's router to the Mac's ChenWeb backend port; confirm TLS is actually terminated in front of it; add `DATA_SYNC_SHARED_SECRET` to the Mac's `mise.local.toml [env]` (restart `mise dev`) and `DATA_SYNC_SHARED_SECRET` + `DATA_SYNC_SOURCE_URL` to the China box's `.env` (restart `chenweb`, per the existing env-var change procedure in the ops devdoc).
6. Verify end-to-end on the box's new admin page: Preview reports the expected changed-row count, Apply syncs, a second Preview reports zero.

**Rollback:** apply is additive-only and the cursor advances only on success, so rollback is simply reverting the deployed binary / not calling apply again — no destructive data step is needed. The trigger and `data_sync_state` migrations can be goose-downed like any other migration if needed.

## Open Questions

1. Exact public port and TLS-termination approach for the Mac's forwarded listener — a deploy-time decision, not a code-design blocker (tentatively port 8055 per the original plan).
2. Confirmed assumption: catalog-imported (`status='approved'`, `source='cn_nmpa_medical_device_classification_catalog'`) rows are never edited locally on a deployed box after import. If that turns out false, the `Filter`-based exclusion of `proposed` rows is still correct, but syncing could clobber a locally-edited catalog row — flagging for awareness, not blocking, since no such local-edit workflow exists today.
