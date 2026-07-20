## Why

ChenWeb's configurable, language-aware page content (spec `2026072001-spec-page-content-configurability-i18n`, §1–8) is fully implemented but **file-based**: visibility lives in `config.toml`/`config.local.toml` boolean maps loaded at startup, and per-language text lives in `config/*/labels-<lang>.toml` files read per request. That means a restart is needed to change visibility, there is no per-role access control, and operators must edit and redeploy deployment files rather than manage content as application data. Spec §9 defines the planned next step: move this capability to database-backed page configuration keyed by stable ids, with role-based access control and explicit enable/disable.

## What Changes

- Add two tables that hold configurable page content as application data:
  - `kb.page_def` — one row per configurable frontend page (stable `page_key`, `route`, metadata).
  - `kb.page_config` — one row per configurable entry **per language** (`page_key` + `entry_key` + `language`), carrying the content payload, `access_role` (JSON array), an explicit `accessible` flag, and an explicit `enabled` flag.
- Add an authenticated **resolution API** that, for a `page_key` + requested locale + current user, returns only entries that are enabled AND authorized, with content resolved for the locale and falling back to `config.local.toml::[languages].default`. Disabled, suspended, or unauthorized entries are omitted (not returned with a client-side hide flag). Unknown page/entry keys surface diagnostics in logs.
- Standardize entry identity on the pair `page_key + entry_key` (unique per language) and standardize the `access_role` namespace on `[system].access_roles` keys (validated via `appconfig.GetAccessRoles()`, matched case-insensitively against the user's `Roles`).
- Implement the accessibility model from §9.2: an entry is accessible iff its `accessible` flag is on, `access_role` is non-empty, contains at least one *valid* role, and the user holds at least one of those roles; otherwise the entry is inaccessible to all (fail closed).
- **Rewire the two existing domains** to consume the DB-backed resolution API instead of the file-based endpoints:
  - `/home3/knowledge` (Wiki sidebar menu — currently `GET /api/v1/kb/menu-config`, `[knowledge-content]` + `config/knowledge-content/labels-<lang>.toml`).
  - `/semos/workspace` (masthead + app tiles — currently `GET /api/v1/workspace/content-config`, `[workspace-content]` + `config/workspace-content/labels-<lang>.toml`).
- Seed the existing menu ids and workspace app keys into `kb.page_def`/`kb.page_config` (both locales, `en` + `zh-cn`) via the migration so the two pages render unchanged after rewiring (no visual regression).
- Add admin CRUD tooling to manage page defs and entries, gated like the existing `/semos/admin/*` pages (authenticated; admin/root for writes), following the `workspace-lists-live-data` pattern.

Note: this change does **not** remove the file-based config plumbing (`GetKnowledgeContentConfig`, `GetWorkspaceContentConfig`, the `labels-<lang>.toml` loaders) in the same step — the DB becomes the source of truth for these two pages, and the now-unused file path is called out for a follow-up removal to keep this change surgical.

## Capabilities

### New Capabilities
- `page-config-store`: the `kb.page_def` / `kb.page_config` schema, stable `page_key + entry_key` identity, per-language rows with an explicit default-language row, `access_role` / `accessible` / `enabled` columns, goose migrations, and the seed of the two existing domains.
- `page-config-resolution`: the authenticated read API that resolves a page's entries for a given locale and user — access evaluation (fail closed), locale fallback via `[languages].default`, omission of disabled/suspended/unauthorized entries, and unknown-key diagnostics.
- `page-config-admin`: admin API + `/semos/admin/page-config` pages to create/read/update/delete page defs and entries (per-language content, access roles, enable/disable, accessibility).
- `page-config-frontend-integration`: `/home3/knowledge` and `/semos/workspace` consume the resolution API in place of their file-based config endpoints, preserving current rendering via the seeded data.

### Modified Capabilities
<!-- No existing openspec/specs/ entries cover the knowledge menu or workspace page today. -->
(none)

## Impact

- **DB**: new goose migrations in `ChenWeb/project_migrations/` — `kb.page_def`, `kb.page_config` (with unique `(page_key, entry_key, language)` and supporting indexes), plus a data-seed migration for the current knowledge-menu ids and workspace app keys in `en` + `zh-cn`.
- **Backend**: new handler package (e.g. `server/api/pageconfighandler/`) for the resolution API and admin CRUD; routes registered in `server/api/routes.go` under `/api/v1/page-config/...`; reuse of `appconfig.GetAccessRoles()` and `[languages]` accessors. Two existing handlers (`kbhandler.GetKbMenuConfig`, `sitehandler.GetWorkspaceContentConfig`) are superseded as the frontend data source but left in place.
- **Frontend**: `web/src/routes/home3/knowledge/+page.svelte` and `web/src/routes/semos/workspace/+page.svelte` (and their loaders) fetch the resolution API; new admin route(s) under `web/src/routes/semos/admin/`; new paraglide message keys for admin UI chrome (`web/messages/en.json` / `zh-cn.json`).
- **Config**: no new sections required; `[system].access_roles` and `[languages]` already exist and are reused. The `[knowledge-content]` / `[workspace-content]` visibility sections and `config/*/labels-<lang>.toml` files become dormant for these two pages.
- **Docs**: spec `2026072001` §9 becomes implemented; a short ADR records the DB-backed decision, the `page_key + entry_key` identity contract, and the `access_role` namespace decision.
