## 1. Database

- [x] 1.1 Add goose migration `20260815000002_add_kb_metrics_range_type_error_index.sql`
      creating `idx_kb_metrics_range_type_error ON kb.metrics (input_record_id,
      created_at) WHERE value_range_type_error IS NOT NULL` (use the `db-migration`
      skill; confirm it auto-applies via the running `mise dev`/air dev server, per
      `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5`).

## 2. Backend: assertions package

- [x] 2.1 In `server/api/ontology/assertions/metric_normalizer.go`, add exported
      `InvalidateValueRangeTypeMapCache()` wrapping
      `defaultValueRangeTypeMapCache.invalidate()`.
- [x] 2.2 In the same file, add exported `NormalizeValueRangeTypeRaw(raw string)
      string` wrapping `normalizeValueRangeTypeRaw`.
- [x] 2.3 Add/extend a test in `metric_normalizer_test.go` asserting both new exported
      functions behave identically to their unexported counterparts.

## 3. Backend: handler + routes

- [x] 3.1 Create `server/api/kbhandler/metric_range_type_errors_handler.go` with
      `ListMetricRangeTypeErrors` (`GET`): filters `input_record_id`, `date_from`,
      `date_to`, `value_range_type`, all optional and AND-combined; `WHERE
      value_range_type_error IS NOT NULL`; `ORDER BY created_at DESC LIMIT 500`;
      returns `[]metricRecord` (reuse the existing DTO/scan helpers from
      `metrics_handler.go`).
- [x] 3.2 Add `ListValueRangeTypeMapEntries` (`GET`) returning all
      `kb.metric_value_range_type_map` rows (`raw_value`, `canonical_bucket`,
      `status`, `occurrence_count`, `first_seen_record_id`, `last_seen_record_id`,
      `note`, `create_time`, `create_by`, `modify_time`, `modify_by`), `ORDER BY
      (status <> 'approved') DESC, occurrence_count DESC`.
- [x] 3.3 Add `UpsertValueRangeTypeMapEntry` (`POST`): body `{raw_value,
      canonical_bucket, note}`; validate both `raw_value` and `canonical_bucket`
      non-empty (400 otherwise); normalize `raw_value` via
      `assertions.NormalizeValueRangeTypeRaw`; `INSERT ... ON CONFLICT (raw_value) DO
      UPDATE SET canonical_bucket = EXCLUDED.canonical_bucket, status = 'approved',
      note = EXCLUDED.note, modify_time = now(), modify_by = <session user>`; then run
      the cascade `UPDATE kb.metrics SET value_range_type_error = NULL WHERE
      value_range_type_error IS NOT NULL AND lower(regexp_replace(trim(value_range_type),
      '[- ]', '_', 'g')) = $1` (comment cross-referencing
      `normalizeValueRangeTypeRaw`, per design D2); call
      `assertions.InvalidateValueRangeTypeMapCache()`; respond `{entry,
      corrected_count}`.
- [x] 3.4 Register all three routes on the existing `apiGroup` in
      `server/api/routes.go`, next to the other `/kb/...` routes (no extra
      middleware): `GET /kb/metrics/range-type-errors`, `GET
      /kb/metric-value-range-type-map`, `POST /kb/metric-value-range-type-map`.
- [x] 3.5 Unit tests (`metric_range_type_errors_handler_test.go`, sqlmock-based
      matching the style of `ambiguous_objects_handler_test.go`): each filter
      combination on the list endpoint; upsert-creates-new-row case; upsert-updates-
      existing-invalid-row case; cascade clears only matching+errored rows and leaves
      others untouched; missing `canonical_bucket` returns 400.
- [x] 3.6 SQL-vs-Go normalization agreement test: run a fixture list of raw strings
      (including hyphens, spaces, mixed case, unicode) through
      `assertions.NormalizeValueRangeTypeRaw` and through the SQL predicate against a
      throwaway row, assert equal (per design D2 mitigation).

## 4. Frontend: API client

- [x] 4.1 Create `web/src/lib/components/home3/resolve-metric-range-types-client.ts`
      (mirroring `resolve-ambiguous-objects-client.ts`): typed `MetricRangeTypeError`
      and `ValueRangeTypeMapEntry` interfaces; `listMetricRangeTypeErrors(filters)`,
      `listValueRangeTypeMapEntries()`, `upsertValueRangeTypeMapEntry(patch)` API
      calls.
- [x] 4.2 Unit tests for the client's pure helpers (filter query-string building,
      response shape mapping).

## 5. Frontend: view

- [x] 5.1 Create `web/src/lib/components/home3/resolve-metric-range-types-view.svelte`
      with Left panel (search controls per spec: record ID, date range, error-type
      dropdown sourced from non-approved map entries; results list) and Right panel
      (Information Block: name/description/context/value/value_data_type/
      value_range_type/error message).
- [x] 5.2 Add PDF Display to the Right panel: on record selection, fetch `GET
      /kb/raw-lines?input_record_id=N`, resolve the metric's `source_line_spans` to
      page/line/coords, render via `PdfViewWindow` + a highlight-overlay callback
      (duplicated minimal slice per design D7 — do not modify
      `metric-mgmt-view.svelte`).
- [x] 5.3 Add the Map Block (full-width section below the Left/Right split): entry
      list with invalid entries flagged/sorted first; `canonical_bucket` combobox
      (options: `lower_bound`, `upper_bound`, `exact`, `range`, plus free text) per
      entry; "Apply" button calling the upsert client method and showing the
      `corrected_count` result; an "Add Entry" form (raw_value + canonical_bucket)
      using the same upsert call.
- [x] 5.4 Wire Left-panel selection to Right-panel state (selected record drives both
      Information Block and PDF highlight), matching the interaction pattern in
      `resolve-ambiguous-objects-view.svelte`.

## 6. Frontend: nav wiring

- [x] 6.1 Add `{ id: 'sysadmin-db-resolve-metric-range-types', label: 'Resolve Metric
      Range Types' }` to the `sysadmin-db` group's `children` in `nav-rail.svelte`.
- [x] 6.2 In `content-panel.svelte`: import the new view, add the
      `activeMenu?.childId === 'sysadmin-db-resolve-metric-range-types'` dispatch
      branch, and add the same childId to the no-footer exclusion set used by
      `sysadmin-db-resolve-ambiguous`.

## 7. Verification

- [x] 7.1 `cd server && go build ./... && go vet ./... && go test ./...`
- [x] 7.2 `cd web && bun run check` (or project's equivalent typecheck) and relevant
      `bun run test` unit tests for the new client.
- [x] 7.3 Manual walkthrough with `mise dev` running: open Resolve Metric Range Types,
      search by record ID/time/error type, select a record, confirm Information Block
      + PDF highlight, apply a correction on an invalid map entry, confirm the
      corrected-count response and that the corrected row(s) no longer appear in the
      error list on refresh, add a brand-new map entry.

## 8. Docs

- [x] 8.1 Append a short "Resolved" note to ADR `2026081401` §7 in
      `KnowledgeStore/doc-repo/adrs/202608/2026081401-adr-governed-metric-vocabulary-and-phase-d-failure-reporting.md`
      pointing to this page as closing the deferred admin-UI open question.

## 9. Map Block tabs + Apply-to-metrics (follow-up)

- [x] 9.1 In `metric_range_type_errors_handler.go`, add `ApplyValueRangeTypeMapEntry`
      (`POST /kb/metric-value-range-type-map/apply`): body `{raw_value}`; normalize via
      `assertions.NormalizeValueRangeTypeRaw` (400 if empty); load the entry (404 if
      absent); refuse with 409 unless `status = 'approved'` with a non-empty
      `canonical_bucket`; then `UPDATE kb.metrics SET value_range_type = $2,
      value_range_type_error = NULL WHERE lower(regexp_replace(trim(value_range_type),
      '[- ]', '_', 'g')) = $1 AND (value_range_type IS DISTINCT FROM $2 OR
      value_range_type_error IS NOT NULL)`; respond `{entry, applied_count}`.
- [x] 9.2 Register the route in `server/api/routes.go` next to the other two.
- [x] 9.3 sqlmock tests: success (applied_count reported), 409 for a `proposed`
      entry, 409 for an approved entry with a NULL `canonical_bucket`, 404 for an
      unknown `raw_value`, 400 for a blank `raw_value`.
- [x] 9.4 Client: `applyValueRangeTypeMapEntryToMetrics(rawValue)` plus the pure
      `splitMapEntriesByStatus(entries)` helper that produces the two tab lists;
      shelf-store wrapper `applyRangeTypeMapEntryToMetrics` that reloads the map and
      refreshes the errored-metrics list.
- [x] 9.5 Client tests: the split helper's partitioning/ordering, the apply call's
      request shape and `applied_count`, and the 409 refusal surfacing.
- [x] 9.6 `context-shelf.svelte`: replace the single sorted list with a
      "Needs Triage (n)" / "Approved (n)" tab pair, and give each approved entry an
      "Apply" button (separate from the existing approve/save check button) that shows
      the updated-row count or the refusal.
- [x] 9.7 Verification: `go build ./server/... && go vet ./server/api/kbhandler/`,
      `go test ./server/api/kbhandler/ -run 'ValueRangeTypeMap|RangeTypeError'`,
      `bun test src/lib/components/home3/resolve-metric-range-types-client.test.ts`,
      `bun run check` (no new errors).
- [x] 9.8 Keep the approve confirmation readable after the tab split: a
      tab-independent notice in the Map Block, since approving moves the entry (and
      its per-entry message) out of the tab the operator is looking at.
