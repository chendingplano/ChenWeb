## Context

Spec `2026072001-spec-page-content-configurability-i18n` §1–8 is implemented and file-based. Two page domains are configurable today:

- `/home3/knowledge` — Wiki sidebar. Visibility from `[knowledge-content]` (startup viper), labels from `config/knowledge-content/labels-<lang>.toml` (per request). Served by `kbhandler.GetKbMenuConfig` at `GET /api/v1/kb/menu-config`. Base menu tree + ids owned by `web/src/routes/home3/knowledge/+page.svelte`.
- `/semos/workspace` — masthead + app tiles. Visibility from `[workspace-content]`, labels/descriptions from `config/workspace-content/labels-<lang>.toml`. Served by `sitehandler.GetWorkspaceContentConfig` at `GET /api/v1/workspace/content-config`. Base content owned by `SiteConfig`; tiles identified by `WorkspaceApp.key`.

Spec §9 asks to move this to DB-backed configuration keyed by stable ids, with role-based access and explicit enable/disable, while keeping the same architectural split (page owns structure; DB owns per-entry overrides).

Established building blocks this design reuses:
- Row-per-locale i18n pattern from `workspace-lists-live-data` (`kb.site_announcements` uses `group_id + lang` unique index).
- Canonical role set: `[system].access_roles` in `config.local.toml` → `appconfig.GetAccessRoles(...)`. User roles available in a handler via `rc := EchoFactory.NewFromEcho(c, loc)` then `rc.IsAuthenticated()` → `*ApiTypes.UserInfo{Roles, Admin, IsOwner}`.
- Language config: `[languages]` in `config.local.toml` (`languages = ["en","zh-cn"]`, `default`).
- goose migrations in `project_migrations/`, `kb` schema, applied automatically by the running `mise dev`/air server.

## Goals / Non-Goals

**Goals:**
- Persist configurable page content as application data in `kb.page_def` / `kb.page_config`, keyed by the stable pair `page_key + entry_key`.
- Resolve a page's entries per (locale, user) with role-based access (fail closed) and locale fallback to `[languages].default`.
- Rewire `/home3/knowledge` and `/semos/workspace` onto the resolution API with **no visual regression** (seeded data reproduces today's rendering).
- Provide admin CRUD to manage page defs and entries.

**Non-Goals:**
- Removing the file-based plumbing (`GetKnowledgeContentConfig`, `GetWorkspaceContentConfig`, the label loaders and `config/*/labels-*.toml` files) — left dormant, flagged for a follow-up cleanup change.
- Defining page structure (menu tree, tile layout, icons, routes) in the DB — structure stays page-owned per §4.1. The DB governs which entries are visible and their label/description text.
- Cross-session language persistence (still an open concern per §8).
- An admin role system beyond the existing `[system].access_roles` keys.

## Decisions

### D1 — Two tables, per-language rows, entry identity on `page_key + entry_key`
`kb.page_def` holds one row per configurable page. `kb.page_config` holds one row per entry **per language**. Unique constraint `(page_key, entry_key, language)`. This matches §9.1/§9.5/§9.6 and mirrors the proven `group_id + lang` shape from `kb.site_announcements`. Frontend, backend, and admin reference entries by `page_key + entry_key`, never by display text.

`kb.page_def`:
- `page_key VARCHAR` UNIQUE — stable identity (e.g. `home3-knowledge`, `semos-workspace`)
- `route VARCHAR` — the frontend route, for admin/lookup
- `title VARCHAR`, `description TEXT` — admin-facing metadata
- audit columns (`created_by/updated_by/created_at/updated_at`)

`kb.page_config`:
- `page_key VARCHAR` (FK-by-value to `kb.page_def.page_key`)
- `entry_key VARCHAR` — stable entry identity (existing menu id / `WorkspaceApp.key` / masthead id)
- `language VARCHAR(10)` — paraglide locale code (`en`, `zh-cn`)
- `content JSONB` — resolved payload: `{ "label": "...", "description": "..." }` (description optional)
- `access_role JSONB` — array of role tokens (see D3)
- `accessible BOOLEAN NOT NULL DEFAULT true` — explicit accessibility switch (§9.2)
- `enabled BOOLEAN NOT NULL DEFAULT true` — explicit enable/disable (§9.2)
- audit columns
- UNIQUE `(page_key, entry_key, language)`; index on `(page_key, language)` for the resolver.

*Alternative considered:* a normalized 3-table split (entry-level controls vs. per-language content). Rejected — §9.1 explicitly calls for two tables and the payload is small; the per-language duplication of control columns is handled by D2.

### D2 — The default-language row is authoritative for access + enable
Because access controls physically live on every per-language row but conceptually belong to the entry, the resolver evaluates `accessible`, `enabled`, and `access_role` **from the entry's default-language row** (`language = [languages].default`), which §9.6 requires to exist for every active entry. Only `content` is taken from the requested-language row (falling back to the default row's `content`). The admin UI edits access/enable on the entry and writes them to the default-language row, keeping non-default rows' control columns irrelevant (they carry translated `content` only).

*Alternative considered:* evaluate access on whichever row matches the requested locale. Rejected — an entry's visibility could then flip per language, which is surprising and error-prone.

### D3 — `access_role` namespace = `[system].access_roles` keys (strict, no wildcard)
`access_role` is a JSON array of role keys drawn from `appconfig.GetAccessRoles(...)`, matched case-insensitively against the user's `Roles`. There is no wildcard token — accessibility follows §9.2 strictly and fails closed.

Accessibility (per §9.2), evaluated on the default-language row:
- accessible **iff** `accessible = true` AND `access_role` is non-null/non-empty AND it contains at least one *valid* role key (present in `GetAccessRoles()`) AND the user holds at least one of those roles.
- `accessible = false` → inaccessible to all.
- `access_role` null/empty/no-valid-role → entry is *suspended*, inaccessible to all.

*Seeding the two existing pages:* they follow these strict rules like any other entry. The user has already granted the needed roles to existing users, so the seed assigns each existing entry the full `[system].access_roles` key list (`["admin","root","guest","dev","k_engineer","trial"]`) so today's users retain access. Operators can later trim these to scope specific entries. *Alternative considered:* a `"*"` "any authenticated user" token — rejected per the user's decision to have the existing pages follow the spec's accessibility rules exactly.

### D4 — Resolution response is authoritative for visibility; structure stays page-owned
`GET /api/v1/page-config/:pageKey?lang=<code>` returns exactly the entries that are enabled AND authorized for the current user, each with resolved `content`. Disabled/suspended/unauthorized entries are **omitted** (no client-side hide flag), per §9.4.

The frontend keeps owning structure (menu tree, tile layout, ids, icons, routes) but renders a page-owned item **only if** its `entry_key` appears in the response, using the returned `label`/`description` as overrides (falling back to the component's hardcoded defaults when a field is absent). Because the seed creates a row for every current item, rendering is unchanged. Parent-collapse logic (hide a parent when all children are hidden) continues to run frontend-side over the filtered set.

This is a deliberate shift from the file-based "absent id ⇒ visible (fail-open)" posture to "DB is authoritative for what shows," which is what §9.4/§9.5 describe for the DB-backed design. New page-owned items must be given a `kb.page_config` row (via admin or migration) to appear.

Response shape:
```json
{
  "status": true,
  "page_key": "home3-knowledge",
  "lang": "zh-cn",
  "entries": [
    { "entry_key": "kb-metrics", "content": { "label": "指标" } }
  ]
}
```

### D5 — Locale fallback
For each entry authorized via its default-language row: if a row exists for the requested `lang`, use its `content`; otherwise use the default-language row's `content`. Missing optional fields inside `content` fall back to the frontend's built-in defaults. This preserves the current fail-open translation posture with `[languages].default` as the explicit, centrally-configured fallback (§9.6).

### D6 — Admin gating
Admin list/read endpoints require an authenticated user. Admin write endpoints (create/update/delete page defs and entries) require admin/root, following `useradminhandler`'s check (`currentUser.Admin || currentUser.IsOwner || slices.Contains(Roles,"admin")`). Admin pages live under `web/src/routes/semos/admin/page-config` like the `workspace-lists-live-data` admin pages, edit both locales together, and expose access_role / accessible / enabled controls.

### D7 — Migrations: schema file + seed file
Two goose migrations in `project_migrations/`:
1. `..._create_kb_page_def_page_config.sql` — `CREATE SCHEMA IF NOT EXISTS kb;`, both tables, constraints, indexes, table comments.
2. `..._seed_page_config_existing_domains.sql` — insert `kb.page_def` rows for `home3-knowledge` and `semos-workspace`, and `kb.page_config` rows (both `en` + `zh-cn`, `access_role = ["admin","root","guest","dev","k_engineer","trial"]` (the current `[system].access_roles` set), `accessible = true`, `enabled = true`) for every current menu id and workspace key, with labels/descriptions carried over from today's `config/knowledge-content/labels-*.toml`, `config/workspace-content/labels-*.toml`, and the component/SiteConfig defaults. Idempotent (`ON CONFLICT (page_key, entry_key, language) DO NOTHING`). Kept separate so it can be iterated without touching the schema migration (air auto-applies; a seed edited after apply must be re-run manually per the workflow note).

## Risks / Trade-offs

- **Regression on rewired pages** → Seed reproduces every current id/label/description in both locales and grants the full current role set, so any user holding at least one of those roles retains access; validate `/home3/knowledge` and `/semos/workspace` render identically before/after in both locales.
- **Users with no matching role lose access (strict §9.2)** → Intended per the user's decision; the user has already granted the needed roles to existing users. A user whose `Roles` is empty or disjoint from `[system].access_roles` will see nothing on these pages, by design.
- **Fail-open → fail-closed visibility shift (D4)** → Documented; seed guarantees parity for existing items. Unknown/absent entry_keys no longer auto-appear, which is the intended DB-authoritative behavior.
- **Air auto-applies migrations mid-edit** → Author the seed migration completely before saving; if effects are missing, check `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5` and re-run seed statements manually rather than editing an applied file.
- **Two label sources during transition (file + DB)** → The rewired frontends read only the DB API; the file endpoints are left inert to keep the change surgical, with removal deferred to a follow-up.

## Migration Plan

1. Add schema migration; air applies it. Verify tables exist.
2. Add seed migration; air applies it. Verify seeded rows match current ids/labels.
3. Land backend resolution + admin handlers and routes; `go build ./... && go test`.
4. Rewire the two frontends to the resolution API; verify no visual diff in both locales.
5. Add admin pages; verify CRUD round-trips.

**Rollback:** goose `Down` drops the two tables (data is deployment config, reproducible from the seed). Reverting the frontend commits restores the file-based endpoints, which remain intact.

## Open Questions

- **Public-access model (D3): resolved** — no wildcard token; the two existing pages follow the strict §9.2 rules, seeded with the full `[system].access_roles` key list (the user has granted existing users the needed roles).
- Should the now-dormant file-based config sections/loaders be removed in a follow-up change, or retained as a documented fallback? (Proposal assumes follow-up removal.)
