## Context

`/semos/workspace` (`ChenWeb/web/src/routes/semos/workspace/+page.svelte`) renders three lists in a "DESK"/"BULLETIN" layout. Today: announcements come from a static per-locale TOML array (`config/site/site-default*.toml`, loaded via `+layout.ts` → `GET /api/site-config`); recent activities and alarms/errors are hardcoded `[]` with a comment (ADR 2026071102) explicitly rejecting fake demo data. There is no DB table, handler, or admin UI for any of the three today (confirmed via full-workspace grep).

The codebase has no admin/role concept — `authmiddleware.AuthMiddleware` only distinguishes logged-in vs anonymous. Introducing a role system is out of scope for this change (see Non-Goals); the write endpoints are gated the same as any other `/api/v1` endpoint.

i18n content in this codebase has three existing precedents (paraglide compile-time strings, per-locale companion TOML/files keyed by content-id, and a JSONB locale-map column for LLM-translated review findings). This change adopts **row-per-locale** for the two translatable tables, per user decision — closest to the existing per-locale-file convention and the simplest to query (`WHERE lang = $1`) without adopting the heavier JSONB-translation machinery built for doc-review findings.

## Goals / Non-Goals

**Goals:**
- Replace the TOML/hardcoded lists on `/semos/workspace` with DB-backed, capped, locale-aware reads.
- Give admins (any authenticated user, see below) full CRUD over announcements and recent activities, and a status/notes triage flow for alarms/errors.
- Follow existing ChenWeb conventions: hand-written handler package + explicit route registration (not the generic `RequestHandlers` CRUD layer), viper/mapstructure config, goose migrations in `project_migrations/`.

**Non-Goals:**
- No new role/permission system. Every authenticated user can create/edit/delete announcements and recent activities, and change alarm status/notes. This is a known gap, explicitly accepted by the user for this change; a follow-up change should add an admin role check once one exists elsewhere in the codebase.
- No automatic ingestion of alarms/errors from logs/monitoring — this change only builds the schema + read/status-update API. Something else (out of scope) is expected to `INSERT` rows; the admin page only edits `status`/`notes` on existing rows.
- No pagination UI beyond a simple max-rows cap; admin list pages fetch a bounded page (see Decisions) with no infinite scroll/cursor paging in this pass.
- No navigation/menu wiring beyond adding the three new routes — no new global nav entry is added (out of scope; can be a follow-up).

## Decisions

**1. Row-per-locale schema, tied by `group_id`.**
Each logical announcement/activity is one or more rows sharing a `group_id` (an app-generated ID, distinct from each row's own `id`), one row per supported language (`en`, `zh` per `[languages].languages` in `config.local.toml`). Non-text fields (`occurred_at`, `importance`/`activity_type`) are duplicated per locale row rather than normalized into a parent table, so the table name stays exactly `kb.site_announcements` / `kb.recent_activities` (matching the task's literal table names) instead of splitting into parent+translation tables. `UNIQUE (group_id, lang)` prevents duplicate locale rows within a group. Admin edit/delete operate on `group_id` (updates/deletes all locale rows in the group together); create always writes one row per configured language in a single transaction.
- *Alternative considered:* parent table + child translations table — more normalized, avoids duplicating `occurred_at`/`importance`, but introduces a second table per capability and doesn't match the single-table names given in the task. Rejected for this change; can revisit if duplicated-field drift becomes a real problem.

**2. `public.alarms_errors` is single-row, no `group_id`/`lang`.**
No i18n per the task. Columns: `id`, `occurred_at`, `severity`, `message`, `status` (`unsolved`|`solved`, CHECK constraint), `notes` (JSONB array of `{time, user, note}`, default `'[]'`), `created_at`. Rows are expected to be inserted by future system/monitoring code, not by this change's API (no create/delete endpoint is built here — see Non-Goals).

**3. Alarm notes are appended server-side, not client-overwritten.**
`PATCH /api/v1/workspace/alarms/:id` accepts `{status?: "unsolved"|"solved", note?: string}`. If `note` is present, the handler appends `{"time": now, "user": <authenticated user id/email>, "note": note}` to the existing JSONB array via `notes = notes || jsonb_build_array(...)` rather than accepting a full array from the client — avoids lost-update races between concurrent admins and keeps `user`/`time` server-trustworthy.

**4. Severity/status use CHECK constraints; `importance`/`activity_type` stay free-text.**
`alarms_errors.status` and (recommended) `severity` get fixed value sets via `CHECK` since they drive UI behavior (unsolved/all toggle). `site_announcements.importance` and `recent_activities.activity_type` stay plain `VARCHAR` with no `CHECK` — these are open-ended, admin-authored categories the frontend doesn't branch on, so a fixed enum would just create migration friction if the category set grows. Frontend renders whatever string is stored.

**5. Config: `AnnouncementsMax *int` on `FrontendConfigSection`, TOML key `announcements_max`.**
Follows the existing `EnableLoginWithGithub *bool` idiom (nullable pointer so "unset" is distinguishable from an explicit low value) and the codebase's snake_case TOML key convention (`default_knowledge_store`, not kebab-case, despite the task text using a hyphen). `GetAnnouncementsMax()` returns 5 when nil. The read endpoint applies `LIMIT LEAST($configured_max, hard_cap)` server-side (hard cap e.g. 100) so a misconfigured value can't return unbounded rows.
- Recent activities and alarms/errors have no user-specified cap; the read endpoints apply a fixed internal `LIMIT 20` (not configurable) to keep the workspace page bounded — flagged as a default, easy to make configurable later if needed.

**6. API shape follows the `sitehandler`/`docactivity` hand-written pattern, not the generic `RequestHandlers` layer.**
Consistent with every recent feature-specific endpoint in this codebase (site-config, workspace-content, doc-review-activities). New handler package(s) under `ChenWeb/server/api/` with `db.QueryContext`/manual `Scan` loops, registered explicitly in `routes.go` under `apiGroup` (`/api/v1`, `AuthMiddleware`-protected) for all read+write endpoints in this change (workspace-page reads for these three lists require login too, consistent with the rest of the workspace page).

**7. Removing the TOML announcements source is a breaking config change.**
`[workspace].announcements` is deleted from both `config/site/site-default.toml` and `config/site/site-default-zh-cn.toml`, and the corresponding `+layout.ts`/`+page.svelte` derivation (`cfg.workspace.announcements`) is removed. Anyone relying on the TOML array loses that content until it's re-entered via the new admin page — acceptable per user decision ("replace entirely") and per CLAUDE.md noting this is a staging server where destructive/breaking changes are acceptable.

## Risks / Trade-offs

- **[No admin role]** → any logged-in user can create/edit/delete announcements, recent activities, and alarm status/notes. Mitigation: explicitly called out in proposal/design as accepted scope; recommend a fast-follow change once a role system exists anywhere in the codebase.
- **[Duplicated non-text fields per locale row]** → editing `occurred_at`/`importance` for one locale and not the other is possible if the admin update path has a bug (should always update all rows in `group_id` together). Mitigation: admin update endpoint is transactional and always writes all locale rows for a `group_id` in one `UPDATE ... WHERE group_id = $1` statement.
- **[Breaking TOML removal]** → any current announcement content in the TOML files disappears until manually re-entered via the new admin UI. Mitigation: call this out during PR review; optionally seed the new table from the current TOML content as part of the migration's data-carry-over step (see tasks.md).
- **[Unbounded `notes` JSONB growth]** → an alarm could accumulate many notes over time with no size cap. Accepted for this change (staging server); revisit if it becomes a real issue.

## Migration Plan

1. Add 3 goose migrations (`project_migrations/`): `kb.site_announcements`, `kb.recent_activities` (both with `CREATE SCHEMA IF NOT EXISTS kb;`), `public.alarms_errors`.
2. Migrations run automatically at ChenWeb server startup via the existing `sharedgoose.RunProjectMigrations` call — no manual migration step needed in deployment.
3. Optional data carry-over: a one-time seed (either in the migration's `-- +goose Up` or a short one-off script) inserting the current `config/site/site-default*.toml` `[workspace].announcements` entries into `kb.site_announcements` before the TOML array is deleted from the config files, so existing announcement text isn't silently lost.
4. Rollback: standard goose `Down` migrations drop the three tables; frontend/backend code revert is a normal git revert (no data migration needed for rollback since this is additive).

## Open Questions

- None blocking — all prior ambiguities (write-path scope, i18n storage, TOML replacement, admin auth, admin route location, alarms scope/notes shape) were resolved with the user during proposal. Remaining defaults (fixed `LIMIT 20` for activities/alarms, no `CHECK` on `importance`/`activity_type`) are called out above as decisions, not open questions, and can be revisited if requirements surface.
