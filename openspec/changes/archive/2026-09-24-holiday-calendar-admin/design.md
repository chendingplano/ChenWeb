## Context

Admin-only CRUD pages in ChenWeb follow an established pattern (`keyword_handlers.go` +
`keyword-rewrite-rules-view.svelte`, and the `pageconfighandler` admin CRUD API): a Go handler
package with `require<Feature>Admin(c, loc)` role checks, plain REST routes registered in
`server/api/routes.go`, and a Svelte view + thin `fetch`-based API client under
`web/src/lib/components/home3/`, wired into the nav via `nav-rail.svelte` (leaf entry) and
`content-panel.svelte` (render branch). No SvelteKit route is needed — this is a client-side SPA
(`/home3`) driven by `activeMenu` state.

There is no existing `calendars`/holiday table, and no existing full-year 12-month
multi-select calendar-grid component (`calendar-01/02.svelte` are single-month date pickers,
not reusable for this).

## Goals / Non-Goals

**Goals:**
- Let an admin/root user define reusable, year-independent "holiday info" records
  (name, country, description, note) and, separately, bind them to specific calendar dates for a
  given (year, country, calendar type) "calendar".
- Render a full-year (12-month) grid for a selected year, with holiday dates visually marked, and
  let the admin click/select one or more day cells to attach a holiday.
- Support edit and delete of both holiday info and calendar/date bindings.

**Non-Goals:**
- No public/non-admin consumption API for other features to query holidays (e.g. business-day
  calculators) — this change only builds the admin management surface. Consumption is a future
  change if/when a consumer exists.
- No recurring-rule engine (e.g. "4th Thursday of November" auto-computed each year). Each year's
  dates are entered explicitly by the admin, since "holiday on a given year" is explicitly
  year-dependent per the requirement.
- No timezone handling beyond plain calendar dates (`date` column, no time-of-day).

## Decisions

**Data model — three tables, schema `public`, database `miner`:**

1. `public.holiday_info` — year-independent holiday definitions.
   - `id bigserial PK`, `country text NOT NULL`, `name text NOT NULL`, `description text`,
     `note text`, `created_at`, `updated_at`.
   - Unique on `(country, name)` — a given country doesn't define the same named holiday twice.
2. `public.calendars` — the year-specific container, matching the requirement that calendars are
   "identified by year, country, and calendar type".
   - `id bigserial PK`, `year int NOT NULL`, `country text NOT NULL`,
     `calendar_type text NOT NULL DEFAULT 'holidays'`, `created_at`, `updated_at`.
   - Unique on `(year, country, calendar_type)`.
3. `public.calendar_holidays` — binds a `holiday_info` to one specific date within one `calendars`
   row ("holiday on a given year").
   - `id bigserial PK`, `calendar_id bigint NOT NULL REFERENCES calendars(id) ON DELETE CASCADE`,
     `holiday_info_id bigint NOT NULL REFERENCES holiday_info(id) ON DELETE RESTRICT`,
     `holiday_date date NOT NULL`, `created_at`, `updated_at`.
   - Unique on `(calendar_id, holiday_date)` — a given date in a given calendar maps to exactly
     one holiday. Selecting multiple day cells and attaching one holiday info creates one row per
     selected date, all sharing the same `holiday_info_id`.
   - `ON DELETE RESTRICT` on `holiday_info_id` so deleting a holiday info that's still bound to a
     date requires removing the bindings first (surfaced as a 409 in the API) — avoids silent data
     loss without adding cascade complexity.

   Alternative considered: store holiday dates as a `date[]`/JSONB column directly on
   `calendars` keyed by `holiday_info_id`. Rejected — a real join table is simpler to query
   ("what's on this date"), update (add/remove one date), and matches existing relational
   conventions in the codebase (no JSONB blobs used for structured per-row data elsewhere in
   admin CRUD tables).

**Country list:** fixed dropdown, sourced from a small static list embedded in the frontend
(no existing shared country-list module in the codebase; adding one is unnecessary for a single
dropdown). Values are ISO 3166-1 alpha-2 codes with display names.

**Backend routes** (admin-gated, mirrors `keyword_handlers.go`'s
`requireKeywordRewriteAdmin` pattern — new `requireCalendarAdmin(c, loc)`):
- `GET /api/v1/calendars/holiday-info?country=` — list holiday infos (optionally filtered)
- `POST /api/v1/calendars/holiday-info` — create
- `PUT /api/v1/calendars/holiday-info/:id` — update
- `DELETE /api/v1/calendars/holiday-info/:id` — delete (409 if still bound to a date)
- `GET /api/v1/calendars?year=&country=&calendar_type=` — get one calendar with its bound dates
  (creates the row on first save rather than requiring a separate "create calendar" step, since a
  calendar is just the (year, country, type) key)
- `PUT /api/v1/calendars/:id/dates` — upsert a batch of `{date, holiday_info_id}` bindings
  (handles "select N cells, attach one holiday" in one call)
- `DELETE /api/v1/calendars/:id/dates/:date` — remove one date binding
- `DELETE /api/v1/calendars/:id` — delete the whole calendar (cascades bindings)

**Frontend:** one new view component `calendar-admin-view.svelte` (year/country/type selector +
12-month grid, using plain CSS grid per month rather than a third-party calendar library — the
existing `calendar-01/02.svelte` widgets are single-month pickers and not a fit for a
multi-select year grid) plus `calendar-admin-client.ts` for the fetch calls, following the
`keyword-rewrite-rules-*` naming/envelope convention (`{status, results?, record?, error_msg?}`).
A small modal/side-panel lets the admin pick an existing holiday info or create a new one when
binding selected dates.

**Nav wiring:** add `{ id: 'sysadmin-calendar', label: 'Calendar' }` to the `system-admin`
children in `nav-rail.svelte`, and a matching `{:else if activeMenu?.childId === 'sysadmin-calendar'}`
branch in `content-panel.svelte`.

**Per-deployment default country (added after initial implementation review):** different
ChenWeb deployments want the page to open on a different country by default — a China-only
deployment shouldn't land on US holidays. A 4th table, `public.calendar_default_country`, holds
at most one row (`id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1)`, `country TEXT NOT NULL`,
`updated_at`); its singleton shape makes "only one default at a time" structural rather than
something the application has to enforce by clearing other rows. `GET
/api/v1/calendars/default-country` returns `{country}` or `{country: null}`; the frontend uses
that (falling back to the hardcoded `'US'` only when no row exists) as the initial `country`
state instead of always starting on `COUNTRIES[0]`. `PUT`/`DELETE` on the same path set/clear it
from a "Set as default country" checkbox in the Holiday Definitions panel header.

Alternative considered: a boolean `is_default` column on `holiday_info` itself (per the initial
phrasing of "flag on a holiday definition"). Rejected — the default is a property of a *country*,
not of an individual holiday, and a country with zero holiday_info rows yet should still be
settable as the default; a flag column would have no row to live on until the first holiday is
created for that country.

## Risks / Trade-offs

- [Risk] Selecting many day cells across month boundaries and attaching a holiday in one action
  could be a large payload → Mitigation: batch endpoint takes a plain array of dates, no
  practical size concern at this scale (a year has ≤366 days).
- [Risk] Deleting a `holiday_info` still referenced by other calendars is a foot-gun →
  Mitigation: `ON DELETE RESTRICT` + explicit 409 response listing why deletion was blocked.
- [Trade-off] No recurring-holiday computation means admins must re-enter dates every year →
  accepted per the explicit requirement that "holiday on a given year" is year-dependent data,
  not a rule engine.

## Migration Plan

Additive only: new tables via a single goose migration in `ChenWeb/project_migrations/`, new
routes, new nav leaf. No existing data or behavior changes. Rollback is dropping the migration
(goose down) plus reverting the route/nav additions.

## Open Questions

None outstanding — data model and API surface are settled by the decisions above.
