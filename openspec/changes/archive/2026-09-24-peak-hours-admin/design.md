## Context

ChenWeb already has a System Admin → System → Calendar page
(`holiday-calendar-admin`) backed by `public.holiday_info` (year-independent
holiday definitions, scoped by `country`) and `public.calendars` /
`public.calendar_holidays` (which years+country+calendar_type have which
dates bound). Peak Hours needs to exclude "holidays" from its active windows,
so it must resolve against that existing data rather than duplicating it.

There is also an established CRUD-admin-page pattern in this codebase:
`kb.keyword_rewrite_rules` (store in `server/api/ontology/keywords`, handler
registered directly on `apiGroup`, single-file Svelte view +
`*-client.ts` talking to a JSON REST envelope `{status, results/record,
error_msg}`). `calendarhandler` shows the same shape for `public.*` tables,
plus the `requireAdmin` gate this change should reuse verbatim.

## Goals / Non-Goals

**Goals:**
- Store named, reusable peak-hours definitions with hour ranges, timezone,
  applicable-day rules, and excluded-day rules (weekends / holidays /
  specific dates, ORed).
- Let admins manage these definitions through a System Admin page.
- Let other backend code answer "is `<name>` active at `<instant>`?" via a
  small evaluation endpoint, without re-implementing the day/hour/exclusion
  logic.

**Non-Goals:**
- No scheduler, cron, or push notifications when a window opens/closes —
  evaluation is pull-only (a single instant, on request).
- No UI or API changes to the holiday-calendar-admin tables themselves;
  peak-hours only reads from them.
- No multi-tenant/ownership model for peak-hours records — same admin-only
  access model as the other System Admin pages.
- No caching layer for evaluation; it's a handful of small queries and pure
  Go logic per call, not a hot path.

## Decisions

**Table shape: one `public.peak_hours` row per named definition, JSONB for
the three rule fields (`hours`, `applicable_days`, `exclude_days`).**
These fields are small, always read/written as a whole, and never queried by
their internal structure (no "find all records active on Tuesdays" query is
needed) — a normalized schema (separate child tables like
`calendar_holidays`) would add joins and migration complexity for no
querying benefit. JSONB keeps validation in Go (one place) and keeps the
admin form a single read/write round-trip, matching how `keyword_rewrite_rules`
keeps its rule fields as plain columns on one row.

```sql
CREATE TABLE public.peak_hours (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    hours             JSONB NOT NULL,   -- ["09:00-12:00", "14:00-18:00"]
    timezone          TEXT NOT NULL,    -- IANA zone, e.g. "Asia/Shanghai"
    applicable_days   JSONB NOT NULL,   -- {"mode":"workdays"} | {"mode":"weekdays","days":[...]} | {"mode":"days_of_month","days":[...]}
    exclude_days      JSONB NOT NULL DEFAULT '[]', -- ["weekends","holidays","2026-12-25","2026-12-26..2026-12-31"]
    country           TEXT NOT NULL DEFAULT '',    -- holiday_info.country to resolve "holidays" exclusion; '' if unused
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**`hours` as `"HH:MM-HH:MM"` strings, not `{start,end}` objects.** Simple to
validate with one regex (`^\d{2}:\d{2}-\d{2}:\d{2}$`) plus a start<end check,
easy to render in the admin form as one text input per range, and matches
the worked DeepSeek example directly (`"09:00-12:00"`, `"14:00-18:00"`).

**`applicable_days` as a discriminated object, not a sniffed array.** The
three modes (`workdays`, `weekdays`, `days_of_month`) have different
validation rules (weekday names vs. 1–31 integers) and the admin form needs
to know which sub-editor to show; an explicit `mode` key avoids content-
sniffing on both the Go and Svelte sides.

**`exclude_days` as one flat array of strings, ORed.** `"weekends"` and
`"holidays"` are keywords; anything else must parse as either an ISO date
(`YYYY-MM-DD`) or an ISO date range (`YYYY-MM-DD..YYYY-MM-DD`). One array
keeps the admin form to a single repeatable text-input list, and OR
semantics fall out naturally (a date is excluded if it matches *any* entry).

**Holiday resolution: `country` + the date's year, `calendar_type = 'holidays'`, fixed.**
`public.calendars` is keyed by `(year, country, calendar_type)`. Peak-hours
records don't need to choose a `calendar_type` — they always mean the
default holiday calendar — so evaluation always queries
`calendar_type = 'holidays'` for `(year(date), country)`. If `country` is
`''` or no matching calendar row exists, `"holidays"` simply excludes
nothing for that record (fail-open, not an error) — a record that doesn't
care about holidays shouldn't be forced to pick a country.

**Evaluation endpoint: `GET /api/v1/peak-hours/{name}/is-active?at=<RFC3339>`.**
`at` defaults to server `time.Now()`. The handler loads the record, converts
`at` into the record's `timezone`, checks `applicable_days` first (cheap,
no DB), then `exclude_days` (the `"holidays"` case is the only one needing a
DB lookup), then checks `hours` ranges. Returns `{"status":true,"active":bool}`.
A 404 (`{"status":false,"error_msg":"peak hours not found"}`) if `name`
doesn't exist, a 400 if `at` fails to parse.

**Timezone validation via `time.LoadLocation` at write time**, not against a
hardcoded list — Go's tzdata is the source of truth and stays current
without maintenance here.

**Route/package placement:** `server/api/peakhourshandler` (new package,
mirrors `calendarhandler`'s one-purpose-per-package convention), registered
directly on `apiGroup` in `routes.go` next to the `calendars/*` routes
(reads from the same tables) rather than nested under `kb/*` (which is for
knowledge-base/ontology concepts, not admin config).

**Frontend: mirror `keyword-rewrite-rules-view.svelte` structure exactly**
(single view file + `*-client.ts`, same envelope handling, same modal-form-
with-preview layout), swapping the rule-specific fields for peak-hours
fields (name, hours list editor, timezone input, applicable-days mode
picker, exclude-days list editor, country input). No new shared UI
components — this one page doesn't justify extracting the modal/table shell
yet (only two pages would use it).

## Risks / Trade-offs

- **JSONB means no DB-level structural validation** → Mitigated by
  validating all three JSONB fields in Go on every write (create and
  update), matching how `validateRewriteRule` centralizes validation for
  `keyword_rewrite_rules`.
- **`country` defaulting to `''` silently excludes nothing for "holidays"**
  → Mitigated by the admin form flagging (client-side hint, not a hard
  block) when `exclude_days` includes `"holidays"` but `country` is blank,
  so the gap is visible without blocking legitimate no-holiday-exclusion
  records.
- **Ambiguous day-of-month semantics near month boundaries** (e.g. day 31 in
  a 30-day month) → `days_of_month` entries that don't exist in a given
  month simply never match that month; no rollover/clamping behavior. This
  matches the plain reading of "day-of-month" and avoids surprising
  end-of-month shifting.
- **No test coverage for DST transitions in the record's timezone** → Go's
  `time` package handles zone offset conversion correctly per-instant, and
  hour-range comparisons happen after conversion, so this isn't expected to
  need special-casing; flagged here rather than adding untested complexity
  upfront.

## Migration Plan

Additive only: one goose migration creating `public.peak_hours`
(`project_migrations/<timestamp>_create_peak_hours.sql`, `+goose Up`/`+goose
Down` with `DROP TABLE`). No existing table or endpoint changes. Safe to
apply and roll back independently of the holiday-calendar-admin tables it
reads from.

## Open Questions

None outstanding — the earlier requirements pass with the user resolved
storage shape, evaluation-API shape, and holiday-country linkage.
