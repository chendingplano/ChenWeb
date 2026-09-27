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
is non-empty, and the modal title shows both counts. Clicking a day that is already bound still
removes its binding, as before.

**4. Colors.** Holiday days use indigo while pending and green once saved; these are today's
colors. Adjusted days use an amber outline while pending and a solid amber fill once saved.
Amber stands out from both green and indigo in the light and dark themes. A small legend below
the toolbar names all four states. The tooltip on an adjusted day reads
`<holiday name> (adjusted working day)`.

## Risks / Trade-offs

- [The live `mise dev` server applies migrations as soon as the file is saved] → Write the
  migration in one pass, then confirm with
  `SELECT ... FROM project_db_migration ORDER BY id DESC`.
- [An admin could bind an adjusted day to the wrong holiday] → The tooltip shows the holiday
  name, and clicking the day removes the binding, the same as for holiday days.

## Migration Plan

One goose migration adds the column with a default, and its Down step drops the column. The
change is backward compatible: old clients that don't send `adjusted_dates` get the previous
behavior.
