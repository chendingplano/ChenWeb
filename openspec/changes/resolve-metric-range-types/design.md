## Context

`kb.metrics.value_range_type_error` is set by `MetricsProcessor.checkValueRangeTypeMappings`
(`server/api/doc-processing/extract-metrics.go:803-859`) whenever a row's normalized
`value_range_type` resolves, via `assertions.ValueRangeTypeMapper.Lookup`, to a
`kb.metric_value_range_type_map` entry with `status='proposed'` (including
newly-auto-inserted ones). It is never cleared once set — the flag is a point-in-time
record, and nothing in the codebase today updates it after a human later approves the
mapping. `MetricNormalizer.Normalize` recomputes derived fields (`value_form`,
`comparator`, `assertion_kind`, numeric bounds) fresh on every run from
`kb.metrics.value_range_type` + `canonical_bucket`, and never writes to `kb.metrics`
itself — so approving a mapping doesn't require recomputing anything on the metric row
except clearing the stale error flag.

There is no admin UI for either table today (`grep` for
`kb.metric_value_range_type_map` finds only the assertions package and its tests). ADR
`2026081401` §7 explicitly names the existing "Resolve Ambiguous Objects" page
(`resolve-ambiguous-objects-view.svelte` + `-client.ts`, System Admin → Database
Maintenance) as the template to copy for this workflow.

Key existing mechanics this design reuses rather than reinvents:
- **Admin nav**: no SvelteKit routes — `nav-rail.svelte` defines menu tree,
  `content-panel.svelte` dispatches by `activeMenu.childId` to a self-fetching leaf
  component. Admin-only access is enforced by nav/page-config gating, not backend
  role middleware (`apiGroup` only requires `authmiddleware.AuthMiddleware`).
- **PDF highlight**: `kb.metrics.input_record_id` + `source_line_spans` (line-number
  JSONB) resolve, via `GET /kb/raw-lines?input_record_id=N`, to per-line
  `{page_number, coords}` (bbox normalized to 0-1000). `metric-mgmt-view.svelte`
  already implements span→page/line resolution and canvas-overlay rendering
  (`:284-380`, `:855-921`) against `PdfViewWindow`.
- **Governed cache**: `assertions.defaultValueRangeTypeMapCache` caches
  `kb.metric_value_range_type_map` in-process for 30s, invalidated on write — but only
  from inside the `assertions` package today.

## Goals / Non-Goals

**Goals:**
- Let an operator find `kb.metrics` rows with an unmapped `value_range_type`, see
  enough context (including PDF source) to judge the correct bucket, and fix the
  underlying governed mapping.
- Make that fix immediately visible: the mapping becomes `approved`, the in-process
  lookup cache stops serving the stale `proposed` classification, and every
  already-flagged `kb.metrics` row sharing that raw value gets its error cleared in
  one action.
- Let an operator pre-seed `kb.metric_value_range_type_map` with new `raw_value`
  entries before they ever cause an error (matching requirement 5's "add more
  entries").

**Non-Goals:**
- Recomputing/re-deriving `kb.semantic_decision_candidates` or
  `kb.semantic_assertions` payloads. Those are produced by `normalize_assertions` /
  `associate_semantics` on their own reprocessing schedule (backlog-drain), which will
  pick up the now-approved bucket on its next run against the affected record(s) —
  this page only clears the `kb.metrics.value_range_type_error` flag, it does not
  trigger reprocessing.
- Setting `status='ambiguous'` from the UI. Today that status is set outside this
  workflow (direct DB/other tooling) to mean "no bound direction is inferable, stop
  asking" for strings like `"threshold"`/`"tolerance"`. This page's Apply action only
  ever produces `approved` (a bucket was supplied) — an "mark ambiguous, no bucket"
  action is a plausible follow-up but wasn't asked for and is deferred.
- New backend role/permission middleware — matches the Resolve Ambiguous Objects
  precedent of nav-only + page-config gating.
- An audit-log table for map edits — `kb.metric_value_range_type_map.note` /
  `modify_by` / `modify_time` already capture who changed what, when, matching how the
  table already tracked `first_seen_record_id`/`last_seen_record_id` provenance.

## Decisions

**D1 — Single upsert endpoint handles both "Apply" and "Add Entry".**
`POST /api/v1/kb/metric-value-range-type-map` with body `{raw_value, canonical_bucket,
note?}` does `INSERT ... ON CONFLICT (raw_value) DO UPDATE`, always normalizing
`raw_value` via the new `assertions.NormalizeValueRangeTypeRaw` before use so it's keyed
identically to the runtime lookup. When `canonical_bucket` is non-empty the row is set
to `status='approved'`; an empty bucket is rejected (400) — there is no product need
today to insert a bare `proposed` row by hand, since the runtime lookup already
auto-inserts those. Alternative considered: separate POST (create) and PATCH (apply)
endpoints — rejected as pure duplication, since "add a new approved entry" and
"correct an existing invalid entry" are the same write (upsert-by-raw_value) from the
DB's point of view, and requirement 7's cascade must fire in both cases (a freshly
added entry might match rows that already errored before the entry existed).

**D2 — Cascade correction matches on a SQL-side port of the same normalization, not an
app-side loop.**
```sql
UPDATE kb.metrics
   SET value_range_type_error = NULL
 WHERE value_range_type_error IS NOT NULL
   AND lower(regexp_replace(trim(value_range_type), '[- ]', '_', 'g')) = $1
```
`$1` is the already-normalized `raw_value`. The regex is a faithful port of
`normalizeValueRangeTypeRaw` (lowercase, trim, hyphen/space → underscore) — a single
pass over one character class is equivalent to Go's two sequential `ReplaceAll` calls
since neither introduces a new hyphen or space. This is a set-based `UPDATE` instead of
loading candidate rows into the handler and filtering in Go, which is both simpler and
avoids an extra round trip for what is otherwise a straightforward equality match.
Trade-off: the normalization logic now exists in two places (Go and SQL) that must stay
in sync — mitigated by a code comment in the handler cross-referencing
`normalizeValueRangeTypeRaw`, and a Go test that runs the same fixture strings through
`assertions.NormalizeValueRangeTypeRaw` and asserts the SQL predicate (executed via
sqlmock/integration test) agrees.

**D3 — Two small exported additions to `assertions`, not a new package.**
```go
// InvalidateValueRangeTypeMapCache clears the in-process kb.metric_value_range_type_map
// cache so the next Lookup re-reads the table instead of serving a stale entry for up
// to the 30s TTL. Callers outside this package that write to the table directly (e.g.
// an admin correction handler) must call this after committing.
func InvalidateValueRangeTypeMapCache() { defaultValueRangeTypeMapCache.invalidate() }

// NormalizeValueRangeTypeRaw exposes normalizeValueRangeTypeRaw so callers outside
// this package can key kb.metric_value_range_type_map.raw_value identically to the
// runtime lookup (see ValueRangeTypeMapper.Lookup).
func NormalizeValueRangeTypeRaw(raw string) string { return normalizeValueRangeTypeRaw(raw) }
```
Alternative considered: let the new handler manage its own cache-invalidation signal
(e.g. a package-level callback registry) — rejected as overengineering for two
one-line wrappers around already-existing unexported functions; this is the smallest
change that removes the up-to-30s staleness window and the normalization-drift risk.

**D4 — New handler file, reusing the existing `metricRecord` DTO.**
`server/api/kbhandler/metric_range_type_errors_handler.go` adds three handlers:
- `ListMetricRangeTypeErrors` (`GET /kb/metrics/range-type-errors`) — filters:
  `input_record_id` (exact), `date_from`/`date_to` (on `created_at`), `value_range_type`
  (exact, the "error type" filter — see D5). Returns `[]metricRecord` (the same DTO
  `ListMetrics`/`UpdateMetric` already use), scoped with `WHERE value_range_type_error
  IS NOT NULL` plus the optional filters, `ORDER BY created_at DESC`, capped at a fixed
  500-row limit (matches current scale — see Risks).
- `ListValueRangeTypeMapEntries` (`GET /kb/metric-value-range-type-map`) — all rows,
  `ORDER BY (status <> 'approved') DESC, occurrence_count DESC` so invalid entries
  surface first.
- `UpsertValueRangeTypeMapEntry` (`POST /kb/metric-value-range-type-map`) — D1/D2/D3
  combined; response is `{entry, corrected_count}` so the frontend can show "3 metrics
  corrected."
No new middleware; registered on the existing `apiGroup` in `routes.go` next to the
other `/kb/...` routes.

**D5 — "Error type" filter is sourced from the Map Block's own non-approved rows, not
a separate endpoint.**
Today there is exactly one error message shape
(`unmapped value_range_type: "<raw>"`), so the only meaningful "type" axis is which raw
`value_range_type` triggered it — which is exactly `kb.metric_value_range_type_map`
rows with `status != 'approved'`. The frontend reuses the Map Block's already-fetched
list to populate the search dropdown (label: the `raw_value`, e.g. `threshold`), rather
than adding a `DISTINCT value_range_type` endpoint that would return the same
information a second way.

**D6 — `canonical_bucket` editor is a combobox seeded from the four known buckets.**
`lower_bound`, `upper_bound`, `exact`, `range` (the only values `CanonicalMetricValueRangeType`
and `guessCanonicalBucketFromCues` ever produce, confirmed against live data — no other
value has ever been approved). Since the column has no DB `CHECK` constraint,
free-text entry stays available for a future fifth bucket without a code change; the
backend does not validate `canonical_bucket` against this list (only checks
non-empty), matching the column's actual DB-level looseness.

**D7 — PDF highlight logic is duplicated, not extracted into a shared util.**
`metric-mgmt-view.svelte` is 4,291 lines and is the primary, heavily-used Metrics page;
extracting its span→page/line resolution and highlight-overlay rendering (~150 lines
across two regions) into a shared module is a refactor of already-shipped code for the
sake of a new admin page, which risks regressing the primary page for no functional
gain here. The new view duplicates the minimal needed slice
(`normalizeMetricSpans`/`selectedLinesByPage`/`renderMetricHighlights` equivalents)
against the same `GET /kb/raw-lines` + `PdfViewWindow` contract. Flagged as a candidate
for extraction if a third page ever needs the same mechanism.

**D8 — One new partial index.**
```sql
CREATE INDEX IF NOT EXISTS idx_kb_metrics_range_type_error
    ON kb.metrics (input_record_id, created_at)
    WHERE value_range_type_error IS NOT NULL;
```
Covers both the exact-match record-ID filter and the time-range filter for the (small,
error-only) partial row set at negligible write/storage cost. Added via goose
migration per workspace convention.

## Risks / Trade-offs

- **[Risk]** SQL-side normalization (D2) silently drifts from `normalizeValueRangeTypeRaw`
  if that Go function is ever changed. → **Mitigation**: code comment cross-reference in
  both places + a test asserting agreement on a fixture set (see D2).
- **[Risk]** Clearing `value_range_type_error` does not itself produce an accepted
  assertion — an operator could read "corrected" as "done" when `normalize_assertions`
  still needs to re-run for that record. → **Mitigation**: the corrected-count response
  and its UI copy explicitly say "error cleared on N metric rows; assertions will
  reflect this after the next reprocessing pass," not "fixed."
- **[Risk]** 500-row cap on the error list (D4) could hide rows during a large backlog
  spike. → **Mitigation**: filters (record ID / time / error type) make this
  workable for now; acceptable given current scale (dozens of `proposed` map entries,
  low-thousands of metric rows total). Revisit with real pagination if the backlog
  regularly exceeds 500.
- **[Trade-off]** Duplicating PDF-highlight logic (D7) instead of extracting a shared
  util costs ~150 lines of near-duplicate code. Accepted to avoid touching the
  high-traffic existing Metrics page.

## Migration Plan

- Additive only: one new index, one new handler file, two new exported functions on an
  existing package (backward compatible — nothing existing calls them, nothing
  existing changes signature), one new nav entry, one new Svelte view. No feature
  flag needed; ships live once merged (`mise dev`/air hot-reloads, migration
  auto-applies per workspace convention).
- Rollback: revert the commit(s); the index drop is safe (`DROP INDEX
  idx_kb_metrics_range_type_error`) and no other code depends on it.

## Open Questions

None blocking — the one real ambiguity (where a governance "Map Block" belongs
relative to a record-scoped Left/Right split) is resolved in specs as: a full-width
section below the Left/Right panels, since it operates across all records rather than
the currently-selected one.
