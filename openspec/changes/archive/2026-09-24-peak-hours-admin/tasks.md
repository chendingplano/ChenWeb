## 1. Database

- [x] 1.1 Add goose migration `project_migrations/<timestamp>_create_peak_hours.sql` creating
      `public.peak_hours` (id, name UNIQUE NOT NULL, hours JSONB NOT NULL, timezone TEXT NOT NULL,
      applicable_days JSONB NOT NULL, exclude_days JSONB NOT NULL DEFAULT '[]', country TEXT NOT
      NULL DEFAULT '', created_at, updated_at), with a `+goose Down` that drops it.
- [x] 1.2 Verify the migration applies cleanly against the dev DB (mise dev / air auto-applies it)
      and is recorded in `project_db_migration`. Confirmed via `psql`: version_id 20260924122346
      is `is_applied = t`, and `public.peak_hours` exists with the expected columns/indexes.

## 2. Backend: store and validation

- [x] 2.1 Create `server/api/peakhourshandler/store.go`: `PeakHours` struct, column list/scan
      helper, `validatePeakHours` (name required, at least one valid `HH:MM-HH:MM` hours entry
      with start<end, timezone validated via `time.LoadLocation`, `applicable_days` mode/content
      validated, each `exclude_days` entry validated as `weekends`/`holidays`/ISO date/ISO date
      range).
- [x] 2.2 Implement `CreatePeakHours`, `GetPeakHoursByName`, `ListPeakHours`, `UpdatePeakHours`,
      `DeletePeakHours` (all by `name`), following the `calendarhandler`/`rewrite_rules_store.go`
      query shape.
- [x] 2.3 Implement holiday-exclusion lookup: given a date and `country`, query
      `public.calendar_holidays` joined to `public.calendars` for
      `(year, country, calendar_type='holidays')`; return "not excluded" (not an error) when
      `country` is empty or no calendar row exists.
- [x] 2.4 Implement `EvaluateActive(record, at time.Time) bool`: convert `at` to the record's
      timezone, check `applicable_days`, then `exclude_days` (weekends via weekday check, holidays
      via 2.3, specific dates/ranges via string comparison), then check `hours` ranges.
- [x] 2.5 Write Go unit tests for validation edge cases and `EvaluateActive`, including the
      DeepSeek worked example (09:00-12:00 & 14:00-18:00, Asia/Shanghai, workdays, exclude
      holidays+weekends) across an active weekday, a weekend, and an excluded holiday date.

## 3. Backend: HTTP handlers and routes

- [x] 3.1 Create `server/api/peakhourshandler/handler.go`: admin-gated `List`, `Create`, `Get`,
      `Update`, `Delete` handlers (reuse the `requireAdmin` pattern from `calendarhandler`) plus
      the public `IsActive` handler (`?at=` query param, RFC3339, defaults to `time.Now()`; 404 on
      unknown name; 400 on unparseable `at`).
- [x] 3.2 Register routes in `server/api/routes.go`: `GET/POST /api/v1/peak-hours`,
      `PUT/DELETE /api/v1/peak-hours/:name`, `GET /api/v1/peak-hours/:name/is-active`.
- [x] 3.3 Write handler-level tests (request/response envelope, admin gating, 404/400 cases).
      Found and fixed a latent bug while writing these: `requireAdmin`'s `calendarhandler`-derived
      pattern returned `c.JSON(...)`'s own result (nil on a successful write) as its "stop here"
      signal, so a denied request's protected logic ran anyway after the 401/403 was written.
      Fixed in this package with a dedicated sentinel error; `calendarhandler` itself was left
      untouched (out of scope here) but likely has the same issue — flagged to the user.

## 4. Frontend

- [x] 4.1 Create `web/src/lib/components/home3/peak-hours-client.ts`: types + `list/create/update/
      delete/isActive` functions against the new REST endpoints, mirroring
      `keyword-rewrite-rules-client.ts`'s envelope handling, plus client-side draft validation.
- [x] 4.2 Create `web/src/lib/components/home3/peak-hours-view.svelte`: table + filters + create/
      edit modal, mirroring `keyword-rewrite-rules-view.svelte`'s structure, with form controls for
      name, a repeatable hours-range list, timezone input, applicable-days mode picker (workdays /
      weekdays / days-of-month) with its sub-editor, a repeatable exclude-days list (weekends /
      holidays checkboxes plus free-form date/range entries), and country input (with a hint when
      "holidays" is excluded but country is blank).
- [x] 4.3 Add nav entry `{ id: 'sysadmin-system-peak-hours', label: 'Peak Hours' }` to the
      `sysadmin-system` group's `children` in `nav-rail.svelte`, alongside `sysadmin-system-calendar`.
- [x] 4.4 Wire `PeakHoursView` into `content-panel.svelte`: import it and add the
      `activeMenu?.childId === 'sysadmin-system-peak-hours'` branch next to the existing
      `sysadmin-system-calendar` branch.

## 5. Verification

- [x] 5.1 Run backend tests (`go test ./server/api/peakhourshandler/...`). All pass.
- [x] 5.2 Run frontend checks (svelte-check / relevant test files) for the new components.
      `bun run check`: 0 errors. `bun test peak-hours-client.test.ts`: 8/8 pass.
- [x] 5.3 Manually exercise the page via `mise dev`: create the DeepSeek example record, confirm
      it lists/edits/deletes correctly, and confirm `GET /api/v1/peak-hours/deepseek/is-active`
      returns the expected `active` value for a few instants (inside a window, outside a window,
      on a weekend, on a configured holiday). Confirmed working by the user in a live browser
      session, including a follow-up UI pass (timezone dropdown, text selection, hours-format
      hint).
