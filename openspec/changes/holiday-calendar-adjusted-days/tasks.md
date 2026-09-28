## 1. Database

- [x] 1.1 Add goose migration `project_migrations/<ts>_add_calendar_holidays_day_kind.sql` adding `day_kind TEXT NOT NULL DEFAULT 'holiday' CHECK (day_kind IN ('holiday','adjusted'))`, Down drops it
- [x] 1.2 Verify the column exists in `miner` and existing rows read `holiday`

## 2. Backend

- [x] 2.1 `store.go`: add `DayKind` to `CalendarDate`, select it in `getCalendar`
- [x] 2.2 `store.go`: `upsertCalendarDates` takes holiday and adjusted date lists, writes `day_kind`, overwrites it on conflict
- [x] 2.3 `handler.go`: accept `adjusted_dates`, require at least one list non-empty, reject overlap with 400
- [x] 2.4 Unit test for the overlap/validation helper; `go build ./... && go test ./api/calendarhandler/`

## 3. Frontend

- [x] 3.1 `calendar-admin-client.ts`: `day_kind` on `CalendarDate`, `adjustedDates` param on `upsertCalendarDates`
- [x] 3.2 `calendar-admin-view.svelte`: Set Holidays / Set Adjusted Days mode buttons before Attach Holiday, two pending sets, mode-aware click toggle
- [x] 3.3 Distinct colors for pending/saved holiday vs adjusted days, legend, adjusted tooltip
- [x] 3.4 `svelte-check` passes

## 4. Docs

- [x] 4.1 Update devdoc `2026092401-devdoc-holiday-calendar-admin.md` (schema, API, frontend, verification)

## 5. Modify saved days (added 2026-09-28)

- [x] 5.1 `calendar-admin-view.svelte`: clicking a saved day stages a kind change or removal in `pendingEdits` instead of deleting at once
- [x] 5.2 Modify button, disabled with no staged edits; saves through the existing upsert/delete endpoints and reloads; Clear Selection discards edits
- [x] 5.3 Pink dashed outline + legend entry for saved days with unsaved edits
- [x] 5.4 `svelte-check` has no errors in the calendar files
- [x] 5.5 Update devdoc and design/spec
- [x] 5.6 Modify also saves new selections, each bound to the holiday of its nearest saved day (no dialog)
- [ ] 5.7 Logged-in browser click-through

## 6. Create calendar before editing (added 2026-09-28)

- [x] 6.1 `POST /calendars` (`CreateCalendar`, idempotent) + route + `createCalendar` client
- [x] 6.2 Page: `keyValid` guard; grid and Set/Attach disabled while the calendar doesn't exist; lower panel shows Create instead of the holiday list
- [x] 6.3 `go build ./api/...`, `go test ./api/calendarhandler/`, `svelte-check` clean for calendar files
- [x] 6.4 Update devdoc, design, spec
- [ ] 6.5 Logged-in browser click-through of Create
