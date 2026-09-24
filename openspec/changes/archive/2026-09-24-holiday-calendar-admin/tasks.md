## 1. Database migration

- [x] 1.1 Add goose migration in `project_migrations/` creating `public.holiday_info`
      (id, country, name, description, note, created_at, updated_at; unique on
      (country, name))
- [x] 1.2 Same migration: create `public.calendars` (id, year, country, calendar_type
      default 'holidays', created_at, updated_at; unique on (year, country, calendar_type))
- [x] 1.3 Same migration: create `public.calendar_holidays` (id, calendar_id FK →
      calendars(id) ON DELETE CASCADE, holiday_info_id FK → holiday_info(id) ON DELETE
      RESTRICT, holiday_date, created_at, updated_at; unique on (calendar_id, holiday_date))
- [x] 1.4 Write the goose Down section dropping all three tables in reverse order
- [x] 1.5 Verify migration applies cleanly against the `miner` dev database — confirmed all
      three tables (with FKs/uniques) exist via `\d` in psql after the live `mise dev`/air
      server auto-applied it on rebuild. (Down was not exercised against the live shared dev
      DB to avoid disturbing its migration-tracking state; it is a straightforward
      `DROP TABLE IF EXISTS` in dependency order, reviewed but not executed.)

## 2. Backend: holiday info endpoints

- [x] 2.1 Add `server/api/calendarhandler/handler.go` with `requireAdmin(c, loc)` helper
      mirroring `requireKeywordRewriteAdmin`
- [x] 2.2 Implement `ListHolidayInfo` (GET, optional `country` filter)
- [x] 2.3 Implement `CreateHolidayInfo` (POST) with (country, name) uniqueness check
- [x] 2.4 Implement `UpdateHolidayInfo` (PUT `:id`)
- [x] 2.5 Implement `DeleteHolidayInfo` (DELETE `:id`), returning 409 if bound to any date
- [x] 2.6 Register routes in `server/api/routes.go` under `/api/v1/calendars/holiday-info...`

## 3. Backend: calendar + date-binding endpoints

- [x] 3.1 Implement `GetCalendar` (GET, query params year/country/calendar_type) — returns the
      calendar row (or empty shape if none exists yet) plus its date bindings joined with
      holiday info name
- [x] 3.2 Implement `UpsertCalendarDates` (PUT `/calendars/dates`, body carries year/country/
      calendar_type) — creates the `calendars` row if missing, then upserts a batch of dates
      sharing one holiday_info_id, replacing any existing binding on a given date
- [x] 3.3 Implement `DeleteCalendarDate` (DELETE `:id/dates/:date`)
- [x] 3.4 Implement `DeleteCalendar` (DELETE `:id`) cascading date bindings
- [x] 3.5 Register routes in `server/api/routes.go` under `/api/v1/calendars...`

## 4. Frontend: API client and view

- [x] 4.1 Add `web/src/lib/components/home3/calendar-admin-client.ts` with typed fetch
      wrappers for all endpoints above, following the `{status, results?, record?,
      error_msg?}` envelope convention
- [x] 4.2 Add a small static ISO country list module used for the country dropdown
      (`country-list.ts`)
- [x] 4.3 Add `web/src/lib/components/home3/calendar-admin-view.svelte`: year/country/type
      selectors + 12-month CSS-grid calendar rendering the selected year
- [x] 4.4 Implement multi-cell selection on day cells (click to toggle; click-drag was
      dropped as unnecessary per simplicity — a click-to-toggle multi-select satisfies the
      requirement without added complexity)
- [x] 4.5 Implement "attach holiday" action: modal/panel to pick existing holiday info or
      create a new one, then call the batch upsert endpoint for all selected dates
- [x] 4.6 Implement per-date "remove binding" action and "delete calendar" action with a
      confirmation step
- [x] 4.7 Implement holiday info management UI (list/create/edit/delete) within the same view

## 5. Nav wiring

- [x] 5.1 Add a new "System" group with a "Calendar" leaf (`sysadmin-system-calendar`) under
      the `system-admin` children in `nav-rail.svelte`
- [x] 5.2 Add the matching `{:else if activeMenu?.childId === 'sysadmin-system-calendar'}`
      branch in `content-panel.svelte` rendering `CalendarAdminView`

## 6. Verification

- [x] 6.1 Exercised the schema/query logic directly against the live `miner` dev DB (same SQL
      the store functions run): create holiday info, unique-constraint rejection on a
      duplicate (country, name), create a calendar and bind two dates, re-bind one date to a
      different holiday info (replacement works), delete the calendar (bindings cascade to
      zero). Test rows cleaned up afterward. **Not done:** clicking through the actual
      `calendar-admin-view.svelte` UI in a browser as a logged-in admin — this workspace has
      no bootstrap admin credentials or saved Playwright session available to this agent.
      `svelte-check` and `go build` both pass with no errors/warnings in the new files.
- [x] 6.2 Verified `GET /api/v1/calendars/holiday-info` against the live dev server with no
      session cookie returns 401 "Authentication required" (blocked by the pre-existing
      `authmiddleware.AuthMiddleware` on `apiGroup`, before reaching the handler's own
      admin/role check). Did not separately verify the 403 role-check path (authenticated
      non-admin) or the nav entry's visibility, since that also requires a logged-in session.
- [x] 6.3 Verified via direct SQL: deleting a `holiday_info` row still referenced by
      `calendar_holidays` raises the FK `RESTRICT` violation that `deleteHolidayInfo`
      pre-checks for and maps to `ErrHolidayInfoInUse` → HTTP 409.

## 7. Per-deployment default country

Follow-up requested after initial review: different ChenWeb deployments want different (or no)
default country on page load, not always hardcoded 'US'.

- [x] 7.1 Add goose migration creating singleton `public.calendar_default_country`
      (id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1), country TEXT NOT NULL, updated_at) —
      applied live and confirmed via `\d` in psql
- [x] 7.2 Backend: `GetDefaultCountry` (GET `/api/v1/calendars/default-country`) → `{country:
      string|null}`, admin-gated like the rest of this handler
- [x] 7.3 Backend: `SetDefaultCountry` (PUT `/api/v1/calendars/default-country`) — upserts the
      singleton row to the given country (singleton table makes "only one default" automatic)
- [x] 7.4 Backend: `ClearDefaultCountry` (DELETE `/api/v1/calendars/default-country`) — removes
      the row, reverting to the hardcoded 'US' fallback
- [x] 7.5 Register the three routes in `routes.go`
- [x] 7.6 Frontend: on mount, fetch the default country and use it as the initial `country`
      state (falling back to 'US' if none configured) instead of always starting on
      `COUNTRIES[0]`
- [x] 7.7 Frontend: add a "Set as default country" checkbox to the "Holiday Definitions
      ({country})" panel header, checked when the selected country matches the stored default;
      toggling calls the PUT/DELETE endpoints and refreshes state
- [x] 7.8 Verified via direct SQL against the live dev DB (equivalent to what the store
      functions run): setting country A then country B as default leaves exactly one row
      (B); deleting the row (clear) reverts to zero rows, which the client maps to the 'US'
      fallback. `go build ./server/...` and `svelte-check` both pass with no new
      errors/warnings. UI click-through as a logged-in admin still not independently done
      (same limitation noted in section 6).
