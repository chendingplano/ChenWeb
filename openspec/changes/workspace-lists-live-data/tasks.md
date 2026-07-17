## 1. Database Migrations

- [x] 1.1 Add goose migration creating `kb.site_announcements` (`CREATE SCHEMA IF NOT EXISTS kb;`, columns: `id BIGSERIAL PK`, `group_id BIGINT NOT NULL`, `lang VARCHAR(10) NOT NULL`, `occurred_at TIMESTAMPTZ NOT NULL`, `importance VARCHAR(20) NOT NULL DEFAULT 'normal'`, `announcement_text TEXT NOT NULL`, `created_at`, `updated_at`, `created_by`, `updated_by`, `UNIQUE (group_id, lang)`) in `ChenWeb/project_migrations/`
- [x] 1.2 Add goose migration creating `kb.recent_activities` (same shape: `group_id`, `lang`, `occurred_at`, `activity_type VARCHAR(50)`, `activity_text TEXT`, timestamps, `UNIQUE (group_id, lang)`)
- [x] 1.3 Add goose migration creating `public.alarms_errors` (`id BIGSERIAL PK`, `occurred_at TIMESTAMPTZ NOT NULL`, `severity VARCHAR(20) NOT NULL`, `message TEXT NOT NULL`, `status VARCHAR(20) NOT NULL DEFAULT 'unsolved' CHECK (status IN ('unsolved','solved'))`, `notes JSONB NOT NULL DEFAULT '[]'::jsonb`, `created_at`, `updated_at`)
- [x] 1.4 Verify all three migrations apply cleanly on server startup via `sharedgoose.RunProjectMigrations` (local run against staging DB)

## 2. Backend Config

- [x] 2.1 Add `AnnouncementsMax *int` field (`mapstructure:"announcements_max"`) to `FrontendConfigSection` in `ChenWeb/server/cmd/config/config.go`
- [x] 2.2 Add `GetAnnouncementsMax() int` accessor returning 5 when unset, following the `GetEnableLoginWithGithub` nil-pointer idiom
- [x] 2.3 Add `announcements_max = 5` (or leave unset to test the default) under `[frontend]` in `ChenWeb/config.local.toml`

## 3. Backend API — Announcements

- [x] 3.1 Create handler package (e.g. `ChenWeb/server/api/workspacelists/announcements.go`) with: `ListAnnouncements` (GET, locale + `announcements_max`-capped, hard cap enforced), `CreateAnnouncement` (POST, writes one row per configured language sharing a new `group_id`), `UpdateAnnouncement` (PUT, transactionally updates all rows for a `group_id`), `DeleteAnnouncement` (DELETE, removes all rows for a `group_id`)
- [x] 3.2 Register routes in `ChenWeb/server/api/routes.go` under `apiGroup` (`/api/v1/workspace/announcements`, `/api/v1/workspace/announcements/:group_id`)
- [x] 3.3 Manual verification: create/edit/delete an announcement via curl/HTTP client against a running local server, confirm both locale rows behave correctly

## 4. Backend API — Recent Activities

- [x] 4.1 Create handler package (e.g. `ChenWeb/server/api/workspacelists/activities.go`) with `ListRecentActivities` (GET, locale-filtered, fixed cap 20), `CreateActivity`, `UpdateActivity`, `DeleteActivity` (group_id semantics mirroring announcements)
- [x] 4.2 Register routes in `routes.go` (`/api/v1/workspace/recent-activities`, `/api/v1/workspace/recent-activities/:group_id`)
- [x] 4.3 Manual verification: create/edit/delete a recent activity entry via curl/HTTP client

## 5. Backend API — Alarms/Errors

- [x] 5.1 Create handler package (e.g. `ChenWeb/server/api/workspacelists/alarms.go`) with `ListAlarms` (GET, fixed cap 20 for workspace view, `?unsolved_only=true` filter for admin view), `UpdateAlarm` (PATCH, accepts `{status?, note?}`; appends server-stamped `{time, user, note}` to `notes` via `notes || jsonb_build_array(...)`; ignores any other fields in the request body)
- [x] 5.2 Register routes in `routes.go` (`/api/v1/workspace/alarms`, `/api/v1/workspace/alarms/:id`)
- [x] 5.3 Manual verification: seed a test row directly in the DB, confirm status toggle and note-append behave correctly and read-only fields can't be changed via the API

## 6. Frontend — Workspace Page

- [x] 6.1 Remove `[workspace].announcements` array from `ChenWeb/config/site/site-default.toml` and `site-default-zh-cn.toml`
- [x] 6.2 Remove the `cfg.workspace.announcements` derivation and the hardcoded `recentActivity`/`alarms` empty arrays in `ChenWeb/web/src/routes/semos/workspace/+page.svelte`
- [x] 6.3 Add data loading for all three lists (via `+page.ts`/`+page.server.ts` or client fetch, matching existing `+layout.ts` conventions) calling the three new list endpoints with the active locale
- [x] 6.4 Update the BULLETIN/DESK rendering to show three columns per list (time/importance/announcement, time/type/activity, Time/Severity/Alarms-Errors), preserving existing empty-state messages
- [ ] 6.5 Manual verification in browser: confirm all three lists render real data, empty states still show correctly when a list is empty, and locale switching changes announcement/activity text

## 7. Frontend — Admin Announcements Page

- [x] 7.1 Create `ChenWeb/web/src/routes/semos/admin/announcements/+page.svelte` (+ `+page.ts` for data loading) with a list view and create/edit/delete forms covering both configured languages in one form
- [x] 7.2 Wire the page to the `/api/v1/workspace/announcements` endpoints
- [ ] 7.3 Manual verification in browser: create, edit, and delete an announcement; confirm it appears/disappears on `/semos/workspace`

## 8. Frontend — Admin Recent Activities Page

- [x] 8.1 Create `ChenWeb/web/src/routes/semos/admin/recent-activities/+page.svelte` (+ `+page.ts`) mirroring the announcements admin page
- [x] 8.2 Wire the page to the `/api/v1/workspace/recent-activities` endpoints
- [ ] 8.3 Manual verification in browser: create, edit, and delete a recent activity entry; confirm it appears/disappears on `/semos/workspace`

## 9. Frontend — Admin Alarms Page

- [x] 9.1 Create `ChenWeb/web/src/routes/semos/admin/alarms/+page.svelte` (+ `+page.ts`) with a read-only table (time, severity, message, status, notes) plus an "unsolved only / all" toggle
- [x] 9.2 Add inline controls to change `status` and append a `note` (text input, no time/user fields — those are server-set)
- [x] 9.3 Wire the page to `/api/v1/workspace/alarms` (list with filter) and the `PATCH` endpoint
- [ ] 9.4 Manual verification in browser: toggle unsolved/all filter, mark an alarm solved, append a note, confirm read-only fields aren't editable in the UI

## 10. i18n

- [x] 10.1 Add paraglide message keys for any new static admin-page labels (column headers, buttons, filter toggle, empty states) to `web/messages/en.json` and `web/messages/zh-cn.json`
- [x] 10.2 Confirm announcement/activity *content* uses the DB row-per-locale values directly (no paraglide involved) and alarms/errors content is rendered with no locale switching applied

## 11. Data Carry-Over & Cleanup

- [x] 11.1 Before deleting the TOML `[workspace].announcements` arrays, seed `kb.site_announcements` with their current contents (one-off script or manual `INSERT` alongside the migration) so existing announcement text isn't lost
- [x] 11.2 Confirm `site-default.toml` and `site-default-zh-cn.toml` no longer reference `[workspace].announcements` and the app still starts cleanly

## 12. Final Verification

- [x] 12.1 Run `go vet ./...` and `go test ./...` from the workspace root (or scoped to ChenWeb) to confirm no regressions
- [ ] 12.2 Run through the full golden path in a browser: view `/semos/workspace` with all three lists populated, then use each admin page to add/edit/delete/triage an entry and confirm the workspace view updates
- [x] 12.3 Confirm logging is in place for the new create/update/delete/status-change operations per `shared/Documents/Logs.md`
