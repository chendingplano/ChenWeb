## Why

Reference data curated on the dev Mac (e.g. the `kb.product_names` NMPA classification catalog) has no repeatable way to reach a deployed box today — only manual DB dump/restore. As ChenWeb is deployed to more boxes (the China box now, customer environments later, some behind firewalls that block inbound connections), we need a self-service, admin-triggered way for a deployed box to pull specific data from the dev machine without a human touching either database directly.

## What Changes

- Add a pluggable **sync item** registry on the ChenWeb backend describing things that can be synced from a source (dev Mac) to a target (deployed box). Start with one item type: Postgres table row sync.
- Add an internal, token-authenticated **pull API** on the source side (`/api/internal/data-sync/...`, added to the existing ChenWeb backend — no new standalone process) that returns rows of a given sync item changed since a cursor.
- Add a **sync client** on the target side that calls the source's pull API and incrementally **upserts** rows (add/update only — never deletes rows locally, even if removed at the source), tracking a per-sync-item cursor.
- Register `kb.product_names` as the first syncable item. Add a `BEFORE UPDATE` trigger (goose migration) so `update_time` is reliably bumped on every row change — it currently only defaults on insert, so it can't yet be trusted as a sync cursor.
- Add a shared-secret bearer-token auth check on the pull API (constant-time compare against an env-configured secret), following the existing `/api/internal/...` pattern used by the mitmproxy-ingest and agent-tools endpoints.
- Add a new admin page, **System Admin → Resources → Sync Data**, listing registered sync items with a per-item "Sync" action (preview candidate/changed-row count, then apply and show a result count) — following the existing two-step preview/apply UX used by LLM Accounts import.

## Capabilities

### New Capabilities
- `production-data-sync`: pull-based, token-authenticated mechanism for a deployed ChenWeb instance to incrementally sync registered data items (starting with the `kb.product_names` table) from a dev-machine source, plus the admin UI to trigger and monitor it.

### Modified Capabilities
(none — no existing `openspec/specs/` capability overlaps with this change)

## Impact

- **Affected code**: new `server/api/datasync/` package (sync-item registry, pull-API handler, sync-client/apply logic); `server/api/routes.go` (new ungated `/api/internal/data-sync/...` group on the source side, new authenticated `apiGroup` routes for the admin UI on the target side); `project_migrations/` (new migration adding the `update_time` trigger to `kb.product_names`, plus whatever cursor-state storage design.md settles on); `web/src/lib/components/home3/nav-rail.svelte` and `content-panel.svelte` (new `sysadmin-resources-sync-data` entry); a new `sync-data-view.svelte` + client following the `llm-accounts-view.svelte` pattern.
- **New env vars**: a shared secret consumed on both ends (source verifies it, target sends it), plus a source-URL setting on the target side pointing at the Mac's publicly reachable sync port. Both the Mac's `mise.local.toml [env]` and the China box's `.env` need updating.
- **Infra, out of repo**: a home-router port-forward on the Mac's network (tentative public port 8055 → the Mac's ChenWeb backend) so the China box can reach it. This is a manual, one-time network step, not a code task in this change.
- **Deployment**: the China box's `.env` gets the new source URL + secret and `chenweb` is restarted; no new systemd unit is needed on either end since this piggybacks on the existing `chenweb` process on both the Mac and the box.
- **Database**: one new migration for the `kb.product_names` trigger, plus cursor-state storage (table/columns TBD in design.md).
