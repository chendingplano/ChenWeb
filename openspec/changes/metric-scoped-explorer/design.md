## Context

The Metric Ontology Explorer shipped in the in-progress `metric-ontology-explorer-page` change as
a three-pane workspace (canvas orrery, tabbed content viewer, source-document pane). It already
threads a `?metric_id=` from the Search tab through `+page.svelte`
(`metricOntologyExplorerId`) into `metric-ontology-explorer-view.svelte`, and `source-pane.svelte`
uses it to load the right PDF and highlight the metric's source lines. Two panes ignore it:

- `ontology-canvas.svelte` builds the centre node from a hardcoded literal
  `{ label: 'Metric', sub: 'ONTOLOGY CORE' }` and never receives `metricId`.
- `content-viewer.svelte` opens record tabs that call `LOADERS[loaderKey]()` in
  `metricOntologyExplorerService.ts` with **no argument** — each loader fetches the first ~25 rows
  of a whole `kb.*` table. Five chain nodes have a loader; the other seven render a
  "schema · read endpoint pending" placeholder.

Investigation of the schema (see the proposal) confirmed every metric→node link already exists,
and that four of the node queries collapse to the same anchor:

```
kb.artifact_objects        WHERE artifact_type='metric' AND artifact_id=<metric_id>
kb.assertion_evidence      WHERE artifact_type='metric' AND artifact_id=<metric_id>
kb.semantic_processing_outcomes  WHERE artifact_type='metric' AND artifact_id=<metric_id> AND active
kb.semantic_decision_candidates  WHERE source_artifact_type='metric' AND source_artifact_id=<metric_id>
```

the rest are FK lookups:

```
artifact_objects.object_id                -> kb.object_nodes
kb.metrics.keyword_concept_id             -> kb.keyword_concepts
kb.metrics.metric_definition_term_id      -> kb.ontology_terms   (+ unit / quantity-kind / kind term ids on the assertion)
decision_candidates.resulting_assertion_id-> kb.semantic_assertions
   semantic_assertions.instance_of_term_id-> kb.ontology_term_headers -> kb.ontology_class_contract_revisions
```

`kb.semantic_decision_candidates` already has a store (`DecisionCandidateStore.ListAdmin`) that
filters on `source_artifact_type` / `source_artifact_id`; `kb.assertion_evidence` has
`EvidenceStore` with an `assertion_id` filter; `kb.semantic_processing_outcomes` keeps history
behind an `active` flag. Metric-id parsing (`parseMetricID` / `canonicalMetricID`, canonical
`<record_id>_mtc_<seq>` with the legacy `<record_id>_<seq>` accepted) lives in
`metric_wiki_handler.go`.

Constraints: ChenWeb `CLAUDE.md` — minimum code, surgical edits, match existing style, no
speculative abstraction; no new npm dependency; no goose migration; the existing `Metric Ontology`
dashboard and `metricOntologyAnalysisService` are untouched.

## Goals / Non-Goals

**Goals:**

- One backend read, `GET /api/v1/kb/metrics/:metric_id/graph`, returning the related rows for all
  twelve explorer chain nodes for a single metric, keyed by the `model.ts` chain-node ids, plus the
  resolved metric (`metric_id`, `metric_name`, `metric_name_en`, `input_record_id`).
- Canvas centre node shows the selected metric's name (label) and `metric_id` (sub-line), widened
  and truncated so a long or CJK name stays inside the rectangle; `Metric` / `ONTOLOGY CORE` when
  none is selected.
- Every chain-node record tab renders rows from that payload, scoped to the metric — the five
  ex-loader nodes and the seven ex-schema-panel nodes unified onto one source, one fetch per
  `metric_id`.
- Node clicks with no metric selected show a "pick a metric from the Search tab" state, not a
  table-level sample.
- Optional per-satellite count badge from the same payload.
- Net simplification of `metricOntologyExplorerService.ts` (five loaders → one fetch) and `model.ts`
  (drop `loaderKey`). No new dependency, no migration.

**Non-Goals:**

- No pagination / history view for a node's rows (a metric relates to a handful of rows per node;
  a defensive per-node cap is enough). Processor-stage history beyond the `active` outcome row is
  out of scope.
- No change to `source-pane.svelte`'s own metric resolution — it needs raw lines regardless, and
  rewiring it is not required to meet the spec (listed as an Open Question).
- No change to the Search tab, `+page.svelte` (already derives the id), `shared-pdf-viewer`, or the
  existing `Metric Ontology` dashboard.
- No editing / curation actions; the endpoint is read-only.
- No deep-linking to a canvas state, no graph search, no minimap.

## Decisions

### D1. One aggregate endpoint, not per-endpoint `?metric_id=` filters

`GET /api/v1/kb/metrics/:metric_id/graph` returns the whole per-node map in one response.

- **Chosen** because the scoping anchor is shared (`artifact_type='metric' AND artifact_id=<id>`
  covers four tables) and the remainder are cheap FK lookups, so a single handler is small; it is
  one round-trip for the whole canvas; it lets a satellite show a count badge without extra
  requests; it keeps `metricOntologyExplorerService.ts` to one function; and it mirrors the
  existing `GET /kb/metrics/:metric_id/wiki` shape (one metric → one composed payload).
- **Alternative — add a `metric_id` filter to each existing list endpoint** (`/kb/objects/search`,
  `/kb/keyword-concepts`, `/kb/ontology/terms`, `/kb/semantic-assertions`) plus three new endpoints
  for the schema-panel tables. Rejected: more HTTP surface, several round-trips per canvas, and a
  metric's assertion subject is an `object_node` (not the metric) so `/kb/semantic-assertions`
  could not be filtered by metric without a new column or join anyway.
- **Alternative — extend the metric wiki payload.** Rejected: the wiki page is heavyweight,
  LLM-generated, and cached as prose; it is the wrong artifact for raw record rows.

### D2. Response keyed by chain-node id; rows projected on the client

```
{
  "status": true,
  "metric": { "metric_id": "...", "metric_name": "...", "metric_name_en": "...", "input_record_id": 416 },
  "nodes": {
    "object__mention": { "rows": [ { ...typed fields... } ] },
    "object__node":     { "rows": [ ... ] },
    ...
    "proc__project":    { "rows": [ ... ] }
  }
}
```

Each node's `rows` are typed row objects with documented field names (not pre-flattened
`Cell[][]`). The frontend keeps `columns` authoritative in `model.ts` and adds a tiny per-node
`project(raw) => Cell[]` in `metricOntologyExplorerService.ts`, so display formatting and column
order stay in one place on the client and the server stays free of presentation concerns.
`content-viewer.svelte` stays a generic loading / rows / empty / error renderer.

- **Alternative — server returns `Cell[][]` already projected to the node's columns.** Rejected:
  moves the column list into Go, splitting the display contract across two languages; the
  client-side projector is ~10 lines per node and testable in `model.test.ts` / a service test.

### D3. Fetch once in the view composer; pass data down

`metric-ontology-explorer-view.svelte` gains one `$effect` keyed on `metricId` that calls
`getMetricGraph(metricId)` and holds `{ metric, nodes }` in `$state`. It passes:

- `metric` (name + id) and per-satellite counts to `ontology-canvas.svelte` (new props);
- the resolved `metric` and the `nodes` map to `content-viewer.svelte` (replacing the `metricId`
  prop's role for record tabs).

Record tabs read `nodes[nodeId].rows` synchronously — no per-tab fetch, no per-tab loading state
beyond the single graph fetch. The centre-node identity comes from `graph.metric`, so no extra
`listKbMetrics` call is made for the canvas.

### D4. Centre-node sizing and truncation

In `ontology-canvas.svelte` the core node is currently `w: 178, h: 66` with `label` / `sub` text.
With a metric selected:

- `label` = metric display name (`metric_name` — the source-language name, matching
  `metric-search-pane.svelte`'s `metricName()` precedent), `sub` = `metric_id`.
- Core width becomes `clamp(120 + visibleLen * charW, 178, 320)`; a name past the width bound is
  truncated to a fixed visible length with an ellipsis. CJK is counted at ~1.6× a Latin char for
  the width/really-truncate heuristic.
- The full metric name is placed in an SVG `<title>` on the core `<g>` for hover.
- `fitView()` already frames the bounding box of all nodes, so a wider core is handled without
  further layout math.

With no metric: unchanged literal `Metric` / `ONTOLOGY CORE`, `w: 178`.

### D5. Three explicit record-tab states

`content-viewer.svelte`'s record-tab body renders exactly one of:

1. **no metric selected** — a "Pick a metric from the Search tab to see its records" prompt
   (`metricId` empty). The `Search` tab and canvas navigation still work.
2. **metric selected, `nodes[nodeId].rows` empty** — "No <table> rows for this metric."
3. **graph fetch failed** — a contained error line; canvas, entry tab, Search tab and source pane
   keep working.

Otherwise it renders the rows table using `model.ts` `columns`. The old
"schema · read endpoint pending" block and its `activeNode.loaderKey` branch are removed.

### D6. `model.ts` and service cleanup

- `model.ts`: remove `loaderKey` from every `ChainNode` and delete the `LoaderKey` type. Keep
  `table`, `columns`, `description` (used by the rows table header, the empty state, and the entry
  copy). Keep `evidenceSpans` on `ev__ae`. The chain-node `id` is already unique and becomes the
  payload key — no new field needed.
- `metricOntologyExplorerService.ts`: delete the five `load*` functions and the `LOADERS` map;
  export `getMetricGraph(metricId): Promise<MetricGraph>` and the per-node `project` helpers; keep
  `Cell`, `dash`, `clip`.

### D7. Backend layout and reuse

- `server/api/kbhandler/metric_graph_handler.go` — `GetMetricGraph(c echo.Context)`: parse +
  canonicalise `metric_id` (reuse `parseMetricID` / `canonicalMetricID`), 400 on malformed, then
  call the store, then return the DTO. Error-code prefix in the `EchoFactory` style used by
  `metric_wiki_handler.go` (`CWB_KB_MGRAPH_0xx`).
- `server/api/kbhandler/metric_graph_store.go` — one `metricGraphStore{ DB *sql.DB }` with a
  `Load(ctx, recordID int64, metricID string)` that runs the node queries. Reuse
  `assertions.DecisionCandidateStore.ListAdmin` (filter `source_artifact_type='metric'`,
  `source_artifact_id=metricID`) for `ev__dc`; reuse `assertions.EvidenceStore` list (or a direct
  `WHERE artifact_type='metric' AND artifact_id=$1` on `kb.assertion_evidence`) for `ev__ae`; a
  direct `active`-filtered query on `kb.semantic_processing_outcomes` (+ findings) grouped by
  `stage_term_id` for the four `proc__*` nodes; direct queries for the object / keyword / term /
  contract lookups. Each node capped at a defensive `LIMIT` (e.g. 200).
- `server/api/routes.go` — add `apiGroup.GET("/kb/metrics/:metric_id/graph", kbhandler.GetMetricGraph)`
  directly after the existing `/kb/metrics/:metric_id/wiki` line.
- Read-only: the store opens no transaction and issues only `SELECT`s.

### D8. Stage-outcome rows are the `active` set

`kb.semantic_processing_outcomes` supersedes by `outcome_key` behind an `active` boolean. The
`proc__*` nodes query `AND active = true` so they show the current outcome per stage, with its
`kb.semantic_processing_findings`. History is out of scope (Non-Goal).

### D10. Processor nodes carry honest per-metric facts, not a run log

The explorer's Processor chain has four nodes (`Extract Metrics → Normalize Assertion →
Associate Semantics → Project Semantics`), but the recorded data does not line up 1:1:
`kb.semantic_processing_outcomes` only has three semantic stages
(`semantic:stage_normalize`, `semantic:stage_class_resolution`, `semantic:stage_associate`),
and extraction / the lossless projection write no outcome row at all. Rather than change the
canvas chain topology, the four nodes are repurposed to the honest per-metric facts that do
exist:

| Node | Rows (metric-scoped) | Columns |
| --- | --- | --- |
| `proc__extract` | one row from `kb.metrics` | model, explicit?, confidence, location, extracted |
| `proc__normalize` | the `stage_normalize` outcome (+ findings) | stage, disposition, status, category, findings |
| `proc__associate` | the `stage_class_resolution` and `stage_associate` outcomes (+ findings) | stage, disposition, status, category, findings |
| `proc__project` | one row per resulting `kb.semantic_assertions` | assertion, status, value state, conformance, contract rev |

`model.ts`'s `columns` for these four nodes are updated to match; the `table` label stays a short
description. No spec scenario changes — each node still shows "metric-scoped rows for that node".

### D9. Spec lineage with the sibling change

`metric-ontology-explorer` is not in `openspec/specs/` yet — it exists only as ADDED deltas in the
still-open `metric-ontology-explorer-page` change. This change is a new capability
`metric-scoped-explorer` whose ADDED requirements supersede the sibling's "record tabs read live
data where an endpoint exists, otherwise a schema panel" behaviour and its fixed centre-node label.
At archive time the two must be reconciled so the promoted `metric-ontology-explorer` spec does not
carry the stale schema-panel wording (see Migration Plan).

## Risks / Trade-offs

- **[A metric that never completed Phase D has empty `proc__*`, `ev__sa`, `mdef__contract`, and
  `ev__ae` nodes]** — its decision candidate may be `deferred` with no `resulting_assertion_id`. →
  Mitigation: the spec's "node with no rows for this metric" state covers it; the tab says so
  plainly rather than erroring. The `ev__dc` node still shows the deferred candidate, which is the
  useful signal.
- **[Column contract split: server field names vs `model.ts` columns]** a rename on one side
  silently drops a column. → Mitigation: a service/model test pins each node's `project` output
  length to `columns.length` and checks the field keys the projector reads exist in a sample
  payload fixture.
- **[Spec reconciliation at archive]** if `metric-ontology-explorer-page` archives after this
  change, its ADDED "schema panel" requirement would land in the promoted spec alongside this
  change's superseding requirement. → Mitigation: archive `metric-ontology-explorer-page` first, or
  edit its delta to drop the superseded requirement before archiving either; called out here and in
  tasks.
- **[`ontology-canvas.svelte` is bespoke SVG; a wider variable-width core touches layout]** →
  Mitigation: only `w` and the label text change; `fitView()` already reads node extents, so
  framing adapts; no change to the ellipse or chain-column math.
- **[Per-node `LIMIT` could hide rows for an unusually connected metric]** → Mitigation: 200 is far
  above any observed per-metric fan-out; if it is ever hit, that node's tab shows the capped set
  and a follow-up can add paging. Not expected in the pilot corpus.
- **[Two `?metric_id=` consumers now — the graph fetch and `source-pane`'s own resolve]** minor
  duplicate work on metric change. → Mitigation: acceptable; unifying them is an Open Question, not
  required by the spec.

## Migration Plan

Additive, front-end + one read endpoint, no flag (consistent with how `home3/knowledge` sections
and the sibling explorer change shipped). Deploy via the normal `cd ChenWeb && mise dev` / build;
the Go route is picked up on server restart (air reload in dev). No data migration, no schema
change.

Rollback: revert the new `metric_graph_handler.go` / `metric_graph_store.go`, the one-line
`routes.go` hunk, and the `metricOntologyExplorerService.ts` / `model.ts` /
`metric-ontology-explorer-view.svelte` / `ontology-canvas.svelte` / `content-viewer.svelte` hunks.
The explorer returns to table-level record tabs and the fixed centre node.

Spec reconciliation: before archiving either this change or `metric-ontology-explorer-page`, ensure
the superseded requirements (schema-panel record-tab rule; fixed centre-node label) are dropped
from `metric-ontology-explorer-page`'s delta so the promoted `metric-ontology-explorer` spec is
consistent. Preferred order: finish/accept `metric-ontology-explorer-page`'s remaining
mise-dev verification and archive it, then archive this change.

## Open Questions

- **Locale for the centre-node label** — prefer `metric_name_en` when the knowledge view is in
  English? The explorer has no locale prop today (only `darkMode`). Current decision: show
  `metric_name` and put `metric_name_en` in the hover title when it differs. Revisit if a locale
  prop is added.
- **Fold `source-pane.svelte` into the shared fetch** — let it consume `graph.metric` /
  `graph.nodes.ev__ae` instead of its own `listKbMetrics` + `getRawLines`. Deferred; it still needs
  `getRawLines` for bbox highlighting, so the saving is one call.
- **Evidence bbox highlighting** — `ev__ae` rows carry `source_line_spans`; wiring them into the
  source pane's highlight layer (today driven only by the metric's own `source_line_spans`) is a
  natural follow-up but not in this change's scope.
- **Processor-stage history** — a later change could expose superseded (`active = false`) outcome
  rows as a per-stage timeline; v1 shows only the current row.
