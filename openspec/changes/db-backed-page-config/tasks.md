## 1. Database schema (page-config-store)

- [x] 1.1 Add goose migration `project_migrations/<ts>_create_kb_page_def_page_config.sql`: `CREATE SCHEMA IF NOT EXISTS kb;`, `kb.page_def` (`id BIGSERIAL PK`, `page_key VARCHAR UNIQUE NOT NULL`, `route VARCHAR`, `title VARCHAR`, `description TEXT`, audit cols) and `kb.page_config` (`id BIGSERIAL PK`, `page_key VARCHAR NOT NULL`, `entry_key VARCHAR NOT NULL`, `language VARCHAR(10) NOT NULL`, `content JSONB NOT NULL DEFAULT '{}'`, `access_role JSONB`, `accessible BOOLEAN NOT NULL DEFAULT true`, `enabled BOOLEAN NOT NULL DEFAULT true`, audit cols), with `UNIQUE (page_key, entry_key, language)`, index on `(page_key, language)`, and `COMMENT ON TABLE` notes. Include a `-- +goose Down` dropping both tables.
- [x] 1.2 Gather the current `en` default labels: menu ids from `KbSectionId` in `web/src/routes/home3/knowledge/+page.svelte`, and workspace masthead defaults + app tile keys/labels/descriptions from `config/site/site-default.toml` (`[[workspace.apps]]`) and the workspace `+page.svelte`.
- [x] 1.3 Add seed migration `project_migrations/<ts>_seed_page_config_existing_domains.sql`: insert `kb.page_def` rows `home3-knowledge` (route `/home3/knowledge`) and `semos-workspace` (route `/semos/workspace`); insert `kb.page_config` rows (both `en` + `zh-cn`, `access_role='["admin","root","guest","dev","k_engineer","trial"]'::jsonb` — the current `[system].access_roles` set, `accessible=true`, `enabled=true`) for every menu id (en from component defaults, zh-cn from `config/knowledge-content/labels-zh-cn.toml`) and every workspace masthead id + app key (labels/descriptions: en from SiteConfig defaults, zh-cn from `config/workspace-content/labels-zh-cn.toml`). Use `ON CONFLICT (page_key, entry_key, language) DO NOTHING`. Include `-- +goose Down` deleting the seeded `page_key`s.
- [x] 1.4 Confirm both migrations applied (air auto-applies): `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5;` and spot-check `SELECT count(*) FROM kb.page_config GROUP BY page_key, language;`. Re-run seed statements manually if the file was edited after first apply.

## 2. Backend resolution API (page-config-resolution)

- [x] 2.1 Create `server/api/pageconfighandler/` package with a store (`store.go`) exposing: load page def by `page_key`; load all `kb.page_config` rows for a `page_key` grouped by `entry_key` with default-language + requested-language rows.
- [x] 2.2 Implement access evaluation (`access.go`): given the default-language row's `access_role` (parsed JSON), `accessible`, `enabled`, the user's `Roles`, and `appconfig.GetAccessRoles()`, return authorized iff enabled AND accessible AND access_role has ≥1 valid role key (present in `GetAccessRoles()`) AND the user holds ≥1 of those roles (case-insensitive). No wildcard. Unit-test the truth table from `specs/page-config-resolution`.
- [x] 2.3 Implement `GetPageConfig(c echo.Context)` at `GET /api/v1/page-config/:pageKey?lang=<code>`: resolve current user via `rc.IsAuthenticated()`, evaluate access per entry on the default-language row, resolve `content` from requested-lang row else default-lang row (fallback via `appconfig` `[languages].default`), omit disabled/suspended/unauthorized, return `{status,page_key,lang,entries:[{entry_key,content}]}`. Log a diagnostic for unknown `page_key` (empty `entries`, not an error).
- [x] 2.4 Register the route in `server/api/routes.go` under the authenticated `apiGroup`, next to related registrations.
- [x] 2.5 Add `server/api/pageconfighandler/handler_test.go`: unknown page → empty entries + diagnostic; disabled entry omitted; empty/invalid `access_role` suspended (omitted for all); `access_role=["admin"]` omitted for non-admin, present for admin; zh-cn requested with only en row → en content.

## 3. Admin API + pages (page-config-admin)

- [x] 3.1 Add admin CRUD handlers in `pageconfighandler` (`admin.go`): list page defs; list/create/update/delete entries by `page_key + entry_key` (writing both locales together; access/enable/accessible stored on the default-language row). Reads require authenticated user; writes require admin/owner/`admin`-role (mirror `useradminhandler`).
- [x] 3.2 Register admin routes in `server/api/routes.go` under `/api/v1/page-config/admin/...`.
- [x] 3.3 Add `admin_test.go`: non-admin write rejected; admin write persists; delete removes all per-language rows; disabling an entry then re-resolving omits it.
- [x] 3.4 Add admin frontend route `web/src/routes/semos/admin/page-config/+page.svelte` (+ loader): list pages/entries, edit both locales, toggle `enabled`/`accessible`, edit `access_role`. Follow the `/semos/admin/*` pattern from `workspace-lists-live-data`.
- [~] 3.5 Paraglide message keys for admin chrome — intentionally deferred. The `/semos/admin/page-config` page uses plain English chrome (internal admin tool), avoiding coupling the typecheck to the inlang compiler for brand-new keys. Add keys later if the admin UI needs localizing.

## 4. Frontend rewiring (page-config-frontend-integration)

- [x] 4.1 Rewire `web/src/routes/home3/knowledge/+page.svelte` to fetch `GET /api/v1/page-config/home3-knowledge?lang=<getLocale()>`, build an `entry_key → content` map, render a page-owned menu item only if its id is present (using returned `label` else default), keep parent-collapse over the filtered set, and fail open (default menu) on fetch error/pending.
- [x] 4.2 Rewire `web/src/routes/semos/workspace/+page.svelte` (and loader) to fetch `GET /api/v1/page-config/semos-workspace?lang=<getLocale()>`, apply masthead + tile label/description overrides and visibility by presence, fail open on error/pending.
- [~] 4.3 Browser verification — PENDING the user's running Go API + authenticated Kratos session (the deepdoc API server was not running during implementation; only PocketBase + vite were up). Verified instead: seed data matches current ids/labels in both locales (direct DB check), resolver access/fallback logic (unit tests), build + svelte-check. To finish: with the dev stack up and logged in, confirm both pages render identically in `en` and `zh-cn`; disabling an entry via `/semos/admin/page-config` hides it on next load with no restart; disabling all children of a menu section collapses the parent.

- [x] 4.4 Add a nav entry to reach the admin page: `System Admin → Page Content` in `web/src/lib/components/home3/nav-rail.svelte` (leaf child `sysadmin-page-config` + a `selectItem` case opening `/semos/admin/page-config`, matching the existing route-link pattern used by `kb-metrics` etc.).

## 4b. Post-review revisions

- [x] 4b.1 Overlay visibility model: resolver returns `entries` + `hidden` (was: only visible entries, presence-authoritative). Deleting an entry now reverts to the page's built-in default instead of hiding it; hiding is explicit via `enabled=false`. Updated `handler.go`, `pageConfigService.ts`, both pages, unit tests.
- [x] 4b.2 Admin UI opens in the center panel via `content-panel.svelte` (`System Admin → Page Content`, `page-config-admin-view.svelte`), not a new tab. Nav-rail `window.open` case removed; standalone route kept as a thin wrapper.
- [x] 4b.3 Fixed non-working Edit: editor is now a modal dialog (was a below-the-fold inline form). Dark-theme aware.
- [x] 4b.4 Delete confirm dialog explains the fallback ("reverts to built-in default, visible to all; use Enabled=off to hide").
- [x] 4b.5 Added `kb.page_config.entry_desc` (migration `20260720000003`, backfilled) — admin-facing "what is this entry"; surfaced/edited in the admin UI.

## 5. Wrap-up

- [x] 5.1 `cd server && go build ./... && go test ./api/pageconfighandler/...` (and any touched packages).
- [x] 5.2 Frontend typecheck/lint for the touched Svelte files per `ChenWeb/CLAUDE.md`.
- [x] 5.3 Docs updated: spec `2026072001` §9 marked Implemented with implementation notes; ADR `2026072003-adr-db-backed-page-config.md` written (DB-backed decision, `page_key + entry_key` identity, strict `access_role` namespace with no wildcard, default-language-row-authoritative access, dormant file plumbing flagged for follow-up removal).
