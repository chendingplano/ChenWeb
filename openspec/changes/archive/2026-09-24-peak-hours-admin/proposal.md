## Why

Several features (e.g. routing/throttling decisions around providers like DeepSeek)
need a reusable, admin-editable definition of "peak hours" — a named window of
hours, on certain days, in a certain timezone, with exclusions for weekends and
holidays. No such concept exists today; admins have no way to define or inspect
these windows, and no code has a way to query them.

## What Changes

- Add `public.peak_hours` table storing named peak-hours definitions: hours
  (time-of-day ranges), timezone, applicable days (workdays / specific weekdays /
  specific days-of-month), and excluded days (weekends / holidays / specific
  dates or date ranges, ORed together).
- Add a `country` field on each record so "exclude holidays" can resolve against
  the existing `public.holiday_info` / `public.calendars` / `public.calendar_holidays`
  tables from the holiday-calendar-admin change.
- Add backend CRUD endpoints (`server/api/peakhourshandler`) for admins to
  create/list/update/delete peak-hours definitions, following the
  `calendarhandler` pattern (admin-only via `requireAdmin`).
- Add an evaluation endpoint, `GET /api/v1/peak-hours/{name}/is-active`, that
  resolves whether a given instant (default: now) falls within a named
  peak-hours window, so other backend code/features can consume the concept.
- Add a System Admin page ("System Admin → System → Peak Hours") for
  managing peak-hours definitions, following the `keyword-rewrite-rules-view`
  frontend pattern (single Svelte view + REST client).
- Add a nav entry under the existing `sysadmin-system` group, sibling to
  `sysadmin-system-calendar` (Calendar).

## Capabilities

### New Capabilities
- `peak-hours-admin`: named peak-hours definitions (CRUD storage, day/hour/
  exclusion rules, timezone- and holiday-aware evaluation) plus the admin page
  to manage them.

### Modified Capabilities
(none — this does not change requirements of any existing capability; it adds
a new consumer of the existing holiday-calendar data, not a change to it)

## Impact

- New table: `public.peak_hours` (goose migration in `project_migrations/`).
- New backend package: `server/api/peakhourshandler` (store.go, handler.go).
- New routes registered in `server/api/routes.go` under `/api/v1/peak-hours`.
- New frontend files: `web/src/lib/components/home3/peak-hours-view.svelte`,
  `peak-hours-client.ts`, wired into `content-panel.svelte` and `nav-rail.svelte`.
- Read-only dependency on `public.holiday_info` / `public.calendars` /
  `public.calendar_holidays` (holiday-calendar-admin change) for holiday
  exclusion lookups — no changes to those tables.
