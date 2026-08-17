## Context

`kb.metrics` already has `metric_id`, `keyword_concept_id`, `metric_definition_term_id`,
and `value_range_type_error` as real columns (migrations `20260518000001`,
`20260806000002`, `20260814000004`). The `metricRecord` Go struct
(`server/api/kbhandler/metrics_handler.go`) already has `MetricID` and
`ValueRangeTypeError` fields, but three of the four handler code paths that read
`kb.metrics` (`ListMetrics`, `fetchMetricByID`) don't select `keyword_concept_id`,
`metric_definition_term_id`, or `value_range_type_error` — only the separate
`ListMetricRangeTypeErrors` handler (a different page, "Resolve Metric Range Types")
selects `value_range_type_error`. `metric_id` is already selected by both, so it just
needs the frontend `KbMetricRecord` type and the display block updated.

On the frontend, `buildMetricGroupAttrs` in `metric-mgmt-view.svelte` builds the
`metadata` attribute array (rendered as the "Metadata" petal group) from a
`KbMetricRecord`. That type (`web/src/lib/services/kbService.ts`) doesn't declare
`metric_id`, `keyword_concept_id`, `metric_definition_term_id`, or
`value_range_type_error` today.

## Goals / Non-Goals

**Goals:**
- All four fields are selected by both `ListMetrics` and `fetchMetricByID` so the
  Metrics page's detail view has them regardless of which endpoint populated
  `selectedMetric`.
- All four fields render in the existing "Metadata" petal group, following the same
  `textAttr(key, label, icon, value, hasValue)` pattern as the other metadata entries
  (e.g. only shown/non-empty when the value is present, consistent with every other
  attribute in that group).

**Non-Goals:**
- Not changing the "Resolve Metric Range Types" page or its handler
  (`metric_range_type_errors_handler.go`) — it already has `value_range_type_error`.
- Not adding a link/cross-reference from the new `value_range_type_error` display to
  that other page — out of scope, just surfacing the raw value here.
- Not adding FK-resolved display names for `keyword_concept_id` (→
  `kb.keyword_concepts`) or `metric_definition_term_id` (→ `kb.ontology_terms`) — shown
  as raw ids, matching how every other id-shaped field in this block (`ID`, `Document
  ID`) is already displayed.
- No schema migration — all four columns exist.

## Decisions

- **Add `metric_id` as a new metadata entry rather than replacing the existing "ID"
  entry.** "ID" already displays `m.id` (the internal `kb.metrics.id` primary key,
  used for selection/lookup throughout this component); `metric_id` is a distinct
  governed artifact identifier. Keeping both avoids changing the meaning of an
  existing, already-relied-upon field.
- **Reuse the existing `metricRecord` struct fields where they already exist**
  (`MetricID`, `ValueRangeTypeError`) rather than introducing parallel ones, and only
  add the two genuinely missing fields (`KeywordConceptID`, `MetricDefinitionTermID`).
  Keeps one struct as the single source of truth for a `kb.metrics` row shape across
  all three handlers that read this table.
- **`value_range_type_error` renders like every other text attribute (`hasValue` when
  non-empty), not as a special warning/error-styled entry.** The Metadata group has no
  existing precedent for severity-styled attributes, and adding one is more UI surface
  than this change's stated scope (surfacing the field, not redesigning error
  affordance). A future change can restyle it if reviewers want it to stand out.

## Risks / Trade-offs

- [Duplicating the three-column addition across two near-identical SQL queries
  (`ListMetrics`, `fetchMetricByID`)] → Same pattern already used for every other
  column in this file (see how `metric_id`/`value_range_type` etc. are duplicated
  across all three handlers); not introducing a new inconsistency, just extending the
  existing one. A shared query-fragment helper is out of scope for this change.
- [`metric_definition_term_id` has no DB-level FK (per migration
  `20260806000002`'s comment: `kb.ontology_terms` has no single-column unique
  constraint), so a stale/dangling id could be displayed] → Acceptable; every other
  term-id-reference column in this schema has the same property, and this change only
  displays the raw value, it doesn't validate it.
