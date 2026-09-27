## Why

A real holiday is usually a run of consecutive days off plus, in some countries, one or more
"adjusted" days: weekend days that become working days to make up for the time off. China's
2026 New Year is an example: Jan 1–3 are days off and Sunday, Jan 4 is a working day. The
Holiday Calendar admin can only bind dates to a holiday as plain days off. That makes it
impossible to record adjusted working days, or to tell them apart in the grid.

## What Changes

- Each date bound to a holiday gets a **day kind**: `holiday` (a day off) or `adjusted` (an
  adjusted working day that belongs to that holiday).
- The calendar toolbar gains two selection-mode buttons, placed before **Attach Holiday**:
  **Set Holidays** first, then **Set Adjusted Days**. Days clicked while a mode is active are
  added to that mode's pending selection. **Attach Holiday** then binds the chosen holiday to
  both sets at once, each date with its day kind.
- Holiday days and adjusted days are shown in different colors, both while pending and once
  saved, and a small legend explains the colors.
- `PUT /api/v1/calendars/dates` accepts an optional `adjusted_dates` array.
  `GET /api/v1/calendars` returns `day_kind` for each date. Existing callers keep working:
  omitting `adjusted_dates` behaves exactly as before.
- Schema: `calendar_holidays.day_kind` column (goose migration). Existing rows default to
  `holiday`.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `holiday-calendar-admin`: date bindings carry a day kind (holiday vs adjusted working day).
  Selection uses two modes, one per day kind, and the grid shows the two kinds in different
  colors.

## Impact

- DB: new migration in `project_migrations/` adding `calendar_holidays.day_kind`.
- Backend: `server/api/calendarhandler/store.go` and `handler.go` (payload and response shape;
  the change is additive).
- Frontend: `web/src/lib/components/home3/calendar-admin-view.svelte` and
  `calendar-admin-client.ts`.
- Docs: `KnowledgeStore/doc-repo/devdocs/202609/2026092401-devdoc-holiday-calendar-admin.md`
  and `openspec/specs/holiday-calendar-admin/spec.md` (at archive).
