## Context

`public.calendar_holidays` binds a date in a calendar `(year, country, calendar_type)` to one
`holiday_info`, and allows at most one binding per `(calendar_id, holiday_date)`. Every binding
is implicitly a day off. The admin page (`calendar-admin-view.svelte`) has a single multi-select
set of dates, and **Attach Holiday** binds one holiday to that whole set.

Some countries, China in particular, pair a holiday with *adjusted working days*: weekend days
that become working days. These days belong to the holiday, since they exist because of it, but
they mean the opposite of a day off.

## Goals / Non-Goals

**Goals:**
- Record, for each bound date, whether it is a day off or an adjusted working day of its holiday.
- Let an admin select both kinds of date and bind them to one holiday in a single action.
- Show the two kinds in different colors, both while selecting and once saved.

**Non-Goals:**
- A read API for other features, such as a business-day calculator. This is still out of scope,
  as in the original change.
- Enforcing that holiday days are consecutive, or that adjusted days fall on weekends. Real
  calendars have exceptions, and the admin is trusted.
- Adjusted working days that don't belong to any holiday.

## Decisions

**1. A `day_kind` column on `calendar_holidays`, not a second table.**
`day_kind TEXT NOT NULL DEFAULT 'holiday' CHECK (day_kind IN ('holiday','adjusted'))`.
A date can't be both a day off and a working day in the same calendar, so the existing
`UNIQUE (calendar_id, holiday_date)` already enforces the right invariant. A separate
`calendar_adjusted_days` table would need a cross-table uniqueness check.
Alternative rejected: a boolean `is_adjusted`. A text value with a CHECK constraint reads
better in SQL and can grow later (for example a `bridge` day) without another rename. Existing
rows backfill to `holiday` through the column default, so the migration needs no data step.

**2. An additive API: `adjusted_dates` alongside `dates`.**
`PUT /calendars/dates` takes `{..., dates: [...], adjusted_dates: [...], holiday_info_id}`.
The endpoint upserts both lists in one transaction; `ON CONFLICT (calendar_id, holiday_date)`
now also overwrites `day_kind`. At least one of the two lists must be non-empty. A date that
appears in both lists is rejected with 400, because the request contradicts itself.
Alternative rejected: `dates: [{date, kind}]`. That is a breaking change to the payload shape,
and it offers nothing extra, since there are only two kinds.
`GET /calendars` adds `day_kind` to each date.

**3. The frontend uses a selection mode, not a modifier key.**
The toolbar reads: `Set Holidays` · `Set Adjusted Days` · `Attach Holiday` · `Clear Selection`.
The two Set buttons work as a toggle group that picks the current *selection mode*; the default
is Holidays, which matches today's behavior. Clicking an unbound day toggles it in the current
mode's set, and removes it from the other set if it was there. The page keeps two `Set<string>`
values, `selectedHolidays` and `selectedAdjusted`. **Attach Holiday** is enabled when either set
is non-empty, and the modal title shows both counts.

**3a. Saved days are edited in place and saved with Modify (added 2026-09-28).**
Clicking a saved day no longer deletes it at once. It stages an edit: in the active mode the day
toggles between that mode's kind and removed. The day keeps its holiday. A **Modify** button,
placed after Attach Holiday, saves every change to the active calendar, meaning the one chosen
by year, country and calendar type: staged edits and new holiday/adjusted selections. It is
disabled when there are no changes. No dialog asks for a holiday. Each new day joins the
holiday of its nearest saved day, because adjusted days, and days added to a holiday, sit next
to that holiday. The button is also disabled when there are only new selections and the
calendar has no saved days. Modify uses the existing endpoints: one `PUT /calendars/dates` per
holiday, then one `DELETE` per removed date. Clear Selection discards staged edits.
Alternative rejected: a new bulk endpoint that applies all edits in one transaction. It would
make the save atomic, but a partial failure here only leaves some edits saved; the page reloads
and shows the true state. We can add the endpoint later if that proves to be a problem.

**3b. A calendar must be created before its days can be edited (added 2026-09-28).**
Year, country and calendar type identify one holiday calendar. If it has no `calendars` row,
the day grid is disabled and the lower panel shows a **Create** button instead of the holiday
list. A new endpoint, `POST /calendars`, creates the empty row; it is idempotent.
`PUT /calendars/dates` still creates the row when it is missing, so older clients keep working.
The page just never relies on that any more.

**3c. Holiday definitions belong to (country, calendar type) (added 2026-09-28).**
Country + calendar type define the list of specific holidays, reused across years but not
across calendar types. For example, a future "Product A Promotion Days" type has its own list.
`holiday_info` gets a `calendar_type` column; existing rows backfill to `holidays`. Name and
display-order uniqueness become per `(country, calendar_type)`. Saving dates checks that the
holiday's country and calendar type match the calendar's, and returns 400 otherwise. The
Calendar Type field becomes a dropdown fed by `calendar-types.ts`, with one entry, `holidays`,
for now.
Alternative rejected: a foreign key from `calendar_holidays` to a composite key on
`holiday_info`. It would enforce the match in the database, but it needs duplicated columns on
`calendar_holidays`; one check in the only write path is enough for now.

**4. Colors.** Holiday days use indigo while pending and green once saved; these are today's
colors. Adjusted days use a solid amber fill while pending and a solid light blue fill once saved.
Both stand out from green and indigo in the light and dark themes. A saved day with a staged
edit gets a pink dashed outline. A small legend below
the toolbar names all four states. The tooltip on an adjusted day reads
`<holiday name> (adjusted working day)`.

## Risks / Trade-offs

- [The live `mise dev` server applies migrations as soon as the file is saved] → Write the
  migration in one pass, then confirm with
  `SELECT ... FROM project_db_migration ORDER BY id DESC`.
- [An admin could bind an adjusted day to the wrong holiday] → The tooltip shows the holiday
  name, and clicking the day stages its removal, the same as for holiday days.

## Migration Plan

One goose migration adds the column with a default, and its Down step drops the column. The
change is backward compatible: old clients that don't send `adjusted_dates` get the previous
behavior.
