## Why

ChenWeb has no way to define holiday calendars used elsewhere in the system (e.g. business-day
calculations, scheduling). Admins need a page to define reusable holiday definitions per country
and bind them to specific years, viewed and edited on an actual year calendar grid.

## What Changes

- New "Calendar" leaf page under Development → System Admin → System, admin/root only.
- Admins can pick a year, country, and calendar type ('holidays' initially) and see a full
  12-month calendar grid for that year.
- Admins can select one or more day cells and attach a holiday (existing or newly-created
  "holiday info") to those dates.
- "Holiday info" (name, country, description, note) is year-independent and reusable across
  years; "calendar" (year + country + calendar type) is the year-specific container that binds
  holiday infos to specific dates within that year.
- Admins can edit or remove a holiday binding on a given date, and delete an entire calendar
  (year + country + type).
- Country is chosen from a fixed dropdown list (ISO country list), not free text.

## Capabilities

### New Capabilities
- `holiday-calendar-admin`: Admin CRUD for year-independent holiday definitions ("holiday info")
  and year-specific holiday calendars that bind those definitions to dates within a given year,
  country, and calendar type, presented as an interactive year-calendar grid.

### Modified Capabilities
(none)

## Impact

- New Postgres tables in `miner` database, schema `public`: `holiday_info`, `calendars`,
  `calendar_holidays` (goose migration under `ChenWeb/project_migrations/`).
- New Go backend handlers + routes under `/api/v1/calendars/...` (admin-gated, following the
  `keyword_handlers.go` / `pageconfighandler` pattern).
- New Svelte admin view + API client under `web/src/lib/components/home3/`.
- New nav leaf entry in `nav-rail.svelte` and render branch in `content-panel.svelte`.
