## Why

ADR `2026081401` (governed metric vocabulary / DR6) flags `kb.metrics` rows whose
`value_range_type` doesn't map to an approved `kb.metric_value_range_type_map` entry by
setting `value_range_type_error`, but explicitly deferred building any UI to triage
those rows or the `status='proposed'`/`'ambiguous'` entries behind them — operators
query the tables directly today. That backlog only grows as new documents are
extracted, and the ADR names the existing "Resolve Ambiguous Objects" admin page as
the shape to copy for exactly this workflow. This change closes that gap.

## What Changes

- Add a new System Admin → Database Maintenance page, "Resolve Metric Range Types",
  listing `kb.metrics` rows with a non-empty `value_range_type_error`, searchable by
  input record ID, created-at time range, and offending `value_range_type` ("error
  type").
- Selecting a row shows an Information Block (metric name, description, context,
  value, value data type, value range type, error message) and highlights the source
  location in a PDF Display panel, reusing the existing raw-line/bbox highlight
  mechanism already used by the Metrics page.
- Add a Map Block listing every `kb.metric_value_range_type_map` entry
  (`raw_value`, `canonical_bucket`, `status`); entries with `status != 'approved'` are
  flagged invalid. Each entry has an "Apply" action to set/correct its
  `canonical_bucket` (dropdown of the four known buckets — `lower_bound`,
  `upper_bound`, `exact`, `range` — or free text, since the column has no DB-level
  enum constraint) and a form to add brand-new `raw_value` entries.
- Applying a bucket to an entry sets it to `status='approved'` and cascades: every
  `kb.metrics` row whose normalized `value_range_type` equals that entry's
  `raw_value` and still has `value_range_type_error` set has the error cleared.
- Two small exported additions to the existing `assertions` package so the new
  backend handler can invalidate the in-process governed-table cache and normalize
  raw strings identically to the runtime lookup, instead of duplicating that logic.
- A new partial index on `kb.metrics` to keep the admin list query cheap.

## Capabilities

### New Capabilities
- `metric-range-type-resolution`: admin workflow for listing/reviewing `kb.metrics`
  rows with an unmapped `value_range_type`, and for triaging/correcting
  `kb.metric_value_range_type_map` entries with automatic cascade correction of
  already-flagged metric rows.

### Modified Capabilities
(none — no existing spec covers this table or page)

## Impact

- **Frontend**: new `resolve-metric-range-types-view.svelte` +
  `resolve-metric-range-types-client.ts` under
  `web/src/lib/components/home3/`; one new child nav entry under System Admin →
  Database Maintenance (`nav-rail.svelte`) and one new dispatch branch
  (`content-panel.svelte`).
- **Backend**: new handler file in `server/api/kbhandler/` exposing three read/write
  endpoints on the existing unauthenticated-beyond-login `/api/v1` group (admin-only
  guarantee comes from nav/page-config gating, matching the Resolve Ambiguous Objects
  precedent — no new middleware); two new exported helpers in
  `server/api/ontology/assertions/metric_normalizer.go`.
- **Database**: one new partial index via goose migration on `kb.metrics`
  (`value_range_type_error IS NOT NULL`); no new tables or columns — both
  `kb.metrics.value_range_type_error` and `kb.metric_value_range_type_map` already
  exist.
- **Docs**: follow-up note on ADR `2026081401` §7 marking its deferred admin-UI open
  question as resolved by this page.
