## Why

The Metric Ontology Explorer (`home3/knowledge → Ontology → Metric Ontology Explorer`) already
takes a `?metric_id=` and the source-document pane honours it, but the two other panes do not: the
canvas centre node is a hardcoded `Metric` / `ONTOLOGY CORE`, and the twelve chain-node record tabs
show table-level samples (the first ~25 rows of a `kb.*` table) rather than the rows that belong to
the metric in context. A reader who picks a metric still cannot see *that metric's* object, keyword
concept, governed term, class contract, decision candidates, evidence, assertions, or per-stage
processing outcomes — which is the entire point of a metric-first explorer. Seven of the twelve
chain nodes have no read endpoint at all and render a "schema panel" placeholder.

## What Changes

- **New backend endpoint** `GET /api/v1/kb/metrics/:metric_id/graph` — one aggregate read that
  returns, for a single metric, the related rows for every chain node, keyed by the explorer's
  `model.ts` chain-node ids. It runs joins that already exist in the schema; no new tables, no
  migration. `metric_id` is the canonical `<record_id>_mtc_<seq>` form (legacy `<record_id>_<seq>`
  accepted), consistent with `GET /kb/metrics/:metric_id/wiki`.
- **Canvas centre node reflects the selected metric** — when a metric is in context the centre
  node's label is its `metric_name` (English label where available) and its sub-line is the
  `metric_id`; with no metric selected it stays `Metric` / `ONTOLOGY CORE`. The core rectangle
  widens and long / CJK names are truncated with a full-text tooltip so the node stays legible.
- **All twelve chain-node record tabs become metric-scoped** — each tab renders rows from the
  `metric_graph` payload for its node (Object Mention, Object Node, Keyword Concept, Ontology Term,
  Class Contract Revision, Extract Metrics, Normalize Assertion, Associate Semantics, Project
  Semantics, Assertion Evidence, Decision Candidate, Semantic Assertion). The five that previously
  fetched table-level samples and the seven that were schema-panel placeholders are unified onto
  this one source.
- **Empty state when no metric is selected** — clicking a node with no `?metric_id=` in context
  shows a "pick a metric from the Search tab" prompt in the record tab, not a table-level sample.
  The Search tab and canvas navigation still work with no metric selected.
- **Optional per-satellite count badge** — a satellite may show the count of related rows for the
  metric in context, read from the same payload (e.g. `Object ②`). Cosmetic; behind the same data.
- **Frontend consolidation** — the metric is resolved once in `metric-ontology-explorer-view.svelte`
  and passed to `ontology-canvas.svelte` and `content-viewer.svelte`; the five per-table loaders in
  `metricOntologyExplorerService.ts` collapse into the single `metric_graph` fetch; `model.ts`
  chain nodes keep `table` / `columns` / `description` for display and the empty-state, and drop
  `loaderKey`. No new npm dependency.
- This change **supersedes** the "record tabs read live data where an endpoint exists, otherwise a
  schema panel" behaviour and the fixed centre-node label from the in-progress
  `metric-ontology-explorer-page` change's ADDED deltas. When both changes archive, the promoted
  `metric-ontology-explorer` spec is the merge of the two.

## Capabilities

### New Capabilities
- `metric-scoped-explorer`: the Metric Ontology Explorer bound to a single selected metric — the
  `metric_graph` aggregate read endpoint and its per-node payload, the centre node reflecting the
  selected metric's name and id, every chain-node record tab showing rows scoped to that metric,
  the no-metric-selected empty state for record tabs, and the optional per-satellite count badge.

### Modified Capabilities
<!-- None. The `metric-ontology-explorer` capability is not yet promoted to openspec/specs/ — it
     exists only as ADDED deltas in the in-progress `metric-ontology-explorer-page` change. The
     "What Changes" section records which of those deltas this change supersedes; there is no
     promoted spec file to emit a MODIFIED delta against. -->

## Impact

- **New** `ChenWeb/server/api/kbhandler/metric_graph_handler.go` — `GetMetricGraph` (echo handler),
  its response DTO, and metric-id parsing reused from `metric_wiki_handler.go`
  (`parseMetricID` / `canonicalMetricID`).
- **New** `ChenWeb/server/api/kbhandler/metric_graph_store.go` (or a `store` under
  `server/api/kb...`) — the per-node join queries against `kb.artifact_objects`, `kb.object_nodes`,
  `kb.keyword_concepts`, `kb.ontology_terms`, `kb.ontology_term_headers`,
  `kb.ontology_class_contract_revisions`, `kb.semantic_decision_candidates`,
  `kb.semantic_assertions`, `kb.assertion_evidence`, `kb.semantic_processing_outcomes`
  (+ `kb.semantic_processing_findings`). Read-only; no write path.
- **Modified** `ChenWeb/server/api/routes.go` — one `apiGroup.GET("/kb/metrics/:metric_id/graph",
  kbhandler.GetMetricGraph)` line, next to the existing `/kb/metrics/:metric_id/wiki` route.
- **Modified** `ChenWeb/web/src/lib/services/metricOntologyExplorerService.ts` — replace the five
  `LOADERS` with one `getMetricGraph(metricId)` returning the per-node rows; keep the `Cell` /
  row-shaping helpers.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/model.ts` — drop
  `loaderKey` and the `LoaderKey` type; keep `table` / `columns` / `description`; add a stable
  per-node key if the chain-node `id` is not already the payload key.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer-view.svelte` — resolve
  the metric once (name + id) and thread it to the canvas and content viewer; fetch the graph once
  per `metricId` and pass node rows down (or expose a small store the content viewer reads).
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/ontology-canvas.svelte`
  — centre-node label / sub from the resolved metric; wider core rect; truncation + tooltip;
  optional satellite count badge.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/content-viewer.svelte`
  — record tabs read the graph payload; no-metric empty state; the schema-panel block becomes the
  "no rows for this metric" / "pick a metric" state rather than "endpoint pending".
- **Reused, unchanged** `shared-pdf-viewer.svelte`, `source-pane.svelte` (already metric-aware),
  `metric-search-pane.svelte`, `+page.svelte` (already derives `metricOntologyExplorerId`).
- **No** new npm dependency. **No** database migration. **No** change to the existing
  `Metric Ontology` dashboard or `metricOntologyAnalysisService`.
