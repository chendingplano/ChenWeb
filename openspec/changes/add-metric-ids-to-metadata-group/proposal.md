## Why

The Metrics page (`home3/knowledge` → Metrics tab, `metric-mgmt-view.svelte`) shows a
"Metadata" attribute group for the selected metric (ID, Document ID, Name, Confidence,
Desc, Formula, Explicit) — the block the user refers to as "Metric Information". Four
columns that already exist on `kb.metrics` are governed identifiers useful for tracing
a metric back to its canonical artifact, keyword concept, and definition term, and for
seeing at a glance whether its `value_range_type` failed governed-vocabulary mapping
(ADR 2026081401 DR6) — but none of the four are queried by the API or rendered in this
block today:

- `metric_id` — the canonical artifact id (`kb.artifact_objects.artifact_id` join key),
  distinct from the internal `id` already shown as "ID"
- `keyword_concept_id` — FK to `kb.keyword_concepts.concept_id` (spec 2026080403 §19)
- `metric_definition_term_id` — governed definition-term identifier (same spec)
- `value_range_type_error` — set when `value_range_type` has no approved governed
  mapping (ADR 2026081401 DR6); already surfaced on the separate "Resolve Metric Range
  Types" admin page, but not here, so a reviewer looking at one metric's detail can't
  tell it has a range-type error without cross-referencing that other page

## What Changes

- Add `keyword_concept_id`, `metric_definition_term_id`, `value_range_type_error` to
  the `SELECT`/`Scan` in `ListMetrics` and `fetchMetricByID`
  (`server/api/kbhandler/metrics_handler.go`) — `metric_id` is already selected/scanned
  there, so no query change needed for it
- Add `KeywordConceptID *string` and `MetricDefinitionTermID *string` fields to the
  `metricRecord` struct (`ValueRangeTypeError` already exists on the struct, populated
  today only by the separate range-type-errors handler)
- Add `metric_id`, `keyword_concept_id`, `metric_definition_term_id`,
  `value_range_type_error` to the frontend `KbMetricRecord` type
  (`web/src/lib/services/kbService.ts`)
- Add four entries to the `metadata` attribute array in `buildMetricGroupAttrs`
  (`web/src/lib/components/home3/metric-mgmt-view.svelte`), rendered in the existing
  "Metadata" petal group alongside ID/Document ID/Name/etc.
- No schema migration — all four columns already exist on `kb.metrics`

## Capabilities

### New Capabilities
- `metric-detail-governed-identifiers`: the Metrics page's per-metric Metadata block
  displays a metric's canonical artifact id, keyword-concept id, definition-term id,
  and value-range-type error (when present), sourced from existing `kb.metrics`
  columns

### Modified Capabilities

<!-- none -->

## Impact

- **Modified files**: `server/api/kbhandler/metrics_handler.go`,
  `web/src/lib/services/kbService.ts`, `web/src/lib/components/home3/metric-mgmt-view.svelte`
- No database migration (all four columns already exist)
- No breaking changes — purely additive fields on existing API responses and one
  existing UI attribute group
