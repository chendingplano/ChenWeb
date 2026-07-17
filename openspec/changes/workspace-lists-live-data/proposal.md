## Why

The `/semos/workspace` page currently renders three status lists — announcements, recent activities, and alarms/errors — but only "announcements" has a data source (a static, per-locale TOML array), and "recent activities" / "alarms and errors" are hardcoded empty arrays with no backend at all. Operators have no way to publish announcements or activity updates without editing and redeploying config files, and there is no way to see or triage system alarms/errors at all. This change gives all three lists a real, admin-manageable data source.

## What Changes

- Add three new tables: `kb.site_announcements`, `kb.recent_activities` (both row-per-locale for i18n), and `public.alarms_errors` (single language, admin/system-managed).
- Add read APIs so `/semos/workspace` renders all three lists from the database instead of TOML/hardcoded arrays:
  - Announcements: time, importance, announcement text — capped at `[frontend].announcements_max` in `config.local.toml` (default 5).
  - Recent activities: time, type, activity text.
  - Alarms/errors: time, severity, message — no i18n.
- **BREAKING**: Remove the `[workspace].announcements` TOML array (in `config/site/site-default.toml` and `config/site/site-default-zh-cn.toml`) and its `+layout.ts`/`+page.svelte` wiring; announcements are now DB-only.
- Add admin CRUD pages, gated by the existing `authmiddleware.AuthMiddleware` (any authenticated user — no admin role exists yet in this codebase; this is a known, called-out gap):
  - `/semos/admin/announcements` — create/edit/delete, both locales edited together.
  - `/semos/admin/recent-activities` — create/edit/delete, both locales edited together.
  - `/semos/admin/alarms` — read-only for all fields except `status` (`unsolved`/`solved`) and `notes` (an appended, timestamped, user-attributed note list); includes an "unsolved only / all" toggle.
- Add corresponding backend endpoints under `/api/v1/workspace/...` following the existing hand-written-handler pattern (`sitehandler`-style), plus a new `AnnouncementsMax` field/accessor on `FrontendConfigSection`.

## Capabilities

### New Capabilities
- `workspace-announcements`: DB schema, i18n storage, read API (capped, locale-filtered) and admin CRUD API/page for site announcements.
- `workspace-recent-activities`: DB schema, i18n storage, read API (locale-filtered) and admin CRUD API/page for recent activity entries.
- `workspace-alarms-errors`: DB schema, read API (with unsolved/all filter), and admin API/page for triaging alarms/errors (status + append-only notes), surfaced read-only on the workspace page.

### Modified Capabilities
(none — no existing `openspec/specs/` entries cover the workspace page or announcements today)

## Impact

- **DB**: 3 new goose migrations in `ChenWeb/project_migrations/` (`CREATE SCHEMA IF NOT EXISTS kb;` plus the two `kb.*` tables; one `public.alarms_errors` table).
- **Backend**: new handler package(s) under `ChenWeb/server/api/` (e.g. `workspacelists` or split per capability), new routes registered in `ChenWeb/server/api/routes.go` under `/api/v1/workspace/...`, a new `AnnouncementsMax *int` field + `GetAnnouncementsMax()` accessor in `ChenWeb/server/cmd/config/config.go`.
- **Config**: `ChenWeb/config.local.toml` gains `[frontend].announcements_max` (default 5 when unset).
- **Frontend**: `ChenWeb/web/src/routes/semos/workspace/+page.svelte` (and its `+page.ts`/`+layout.ts` data loading) rewritten to fetch all three lists from the new APIs instead of TOML/hardcoded arrays; three new admin routes under `ChenWeb/web/src/routes/semos/admin/`; `[workspace].announcements` removed from `config/site/site-default*.toml`.
- **i18n**: new paraglide message keys for admin page labels (column headers, buttons, empty states) in `web/messages/en.json` / `zh-cn.json`; announcement/activity *content* uses the new row-per-locale DB pattern, not paraglide.
