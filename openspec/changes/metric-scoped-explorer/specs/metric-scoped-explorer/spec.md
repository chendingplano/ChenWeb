## ADDED Requirements

### Requirement: Metric-graph aggregate read endpoint

The API SHALL expose `GET /api/v1/kb/metrics/:metric_id/graph`, a single read that returns, for one
metric, the related rows for every Metric Ontology Explorer chain node. The endpoint SHALL accept
the canonical `metric_id` form `<record_id>_mtc_<seq>` and the legacy `<record_id>_<seq>` form, and
SHALL reject a missing or malformed value with HTTP 400. It SHALL be read-only and SHALL NOT
require a database migration — every field it returns comes from joins over tables that already
exist (`kb.metrics`, `kb.artifact_objects`, `kb.object_nodes`, `kb.keyword_concepts`,
`kb.ontology_terms`, `kb.ontology_term_headers`, `kb.ontology_class_contract_revisions`,
`kb.semantic_decision_candidates`, `kb.semantic_assertions`, `kb.assertion_evidence`,
`kb.semantic_processing_outcomes`, `kb.semantic_processing_findings`).

The response SHALL contain:

- the resolved metric — at least `metric_id`, `metric_name`, `metric_name_en`, `input_record_id`;
- a per-node map keyed by the explorer's chain-node ids (`object__mention`, `object__node`,
  `keyword__concept`, `mdef__term`, `mdef__contract`, `proc__extract`, `proc__normalize`,
  `proc__associate`, `proc__project`, `ev__ae`, `ev__dc`, `ev__sa`), each entry carrying the rows
  for that node scoped to this metric.

The scoping join for each node SHALL be:

- `object__mention` — `kb.artifact_objects` rows where `artifact_type = 'metric'` and
  `artifact_id = <metric_id>`.
- `object__node` — the `kb.object_nodes` rows those mentions' `object_id` resolve to.
- `keyword__concept` — the `kb.keyword_concepts` row for `kb.metrics.keyword_concept_id`.
- `mdef__term` — the `kb.ontology_terms` row(s) for `kb.metrics.metric_definition_term_id` and the
  unit / quantity-kind / assertion-kind term ids on the metric's assertion.
- `mdef__contract` — the `kb.ontology_class_contract_revisions` row(s) reached from the assertion's
  `instance_of_term_id` via `kb.ontology_term_headers`.
- `ev__dc` — `kb.semantic_decision_candidates` rows where `source_artifact_type = 'metric'` and
  `source_artifact_id = <metric_id>`.
- `ev__sa` — the `kb.semantic_assertions` row(s) referenced by those candidates'
  `resulting_assertion_id`.
- `ev__ae` — the `kb.assertion_evidence` rows for those `resulting_assertion_id`s.
- `proc__extract` / `proc__normalize` / `proc__associate` / `proc__project` — the
  `kb.semantic_processing_outcomes` rows (with their `kb.semantic_processing_findings`) where
  `artifact_type = 'metric'` and `artifact_id = <metric_id>`, grouped by processing stage.

#### Scenario: Graph for a known metric

- **WHEN** a client requests `GET /api/v1/kb/metrics/416_mtc_1/graph` for a metric that exists
- **THEN** the response has HTTP 200, `status: true`, the resolved metric object, and a per-node
  map keyed by the twelve chain-node ids
- **AND** each node entry holds only rows that join to `416_mtc_1` through the paths above

#### Scenario: Legacy metric-id form is accepted

- **WHEN** a client requests the graph with `metric_id` `416_1`
- **THEN** the endpoint normalizes it to `416_mtc_1` and returns the same payload as the canonical
  form

#### Scenario: Malformed metric id

- **WHEN** a client requests the graph with an empty or non-`<record>_mtc_<seq>` `metric_id`
- **THEN** the endpoint responds HTTP 400 with `status: false` and an error message, and runs no
  query

#### Scenario: Metric with no related rows for a node

- **WHEN** a metric has no keyword concept and no class-contract revision
- **THEN** the `keyword__concept` and `mdef__contract` entries are present with an empty row list,
  not omitted and not an error

#### Scenario: No write path

- **WHEN** the endpoint is exercised
- **THEN** it issues only read queries and never inserts, updates, or deletes any row

### Requirement: Canvas centre node reflects the selected metric

The explorer canvas centre node SHALL show the selected metric's identity when a metric is in
context (the page has a non-empty `?metric_id=`): its label SHALL be the metric's name (the
English label where available, otherwise the primary name) and its sub-line SHALL be the
`metric_id`. With no metric in context the centre node SHALL read `Metric` / `ONTOLOGY CORE` as
before. The centre rectangle SHALL size to fit the metric name up to a bound, and a name longer
than that bound SHALL be truncated in the node with the full name available on hover. Changing the
selected metric SHALL update the centre node without a page reload.

#### Scenario: Metric selected

- **WHEN** the explorer is open with `?metric_id=416_mtc_1` and that metric resolves to name
  "易腐垃圾收运频次"
- **THEN** the centre node label reads "易腐垃圾收运频次" (or its English label if present) and the
  sub-line reads `416_mtc_1`

#### Scenario: No metric selected

- **WHEN** the explorer is open with no `metric_id`
- **THEN** the centre node reads `Metric` with sub-line `ONTOLOGY CORE`

#### Scenario: Long name is truncated, not clipped by the node

- **WHEN** the selected metric's name is longer than the centre node's bound
- **THEN** the visible label is truncated with an ellipsis and the full name is shown on hover, and
  the node does not overflow its rectangle

#### Scenario: Switching metric updates the centre node live

- **WHEN** the reader picks a different metric from the Search tab
- **THEN** the centre node label and sub-line update to the newly picked metric with no reload

### Requirement: Chain-node record tabs are scoped to the selected metric

Every chain-node record tab SHALL render rows from the `metric_graph` payload for that node —
scoped to the metric in context — using the node's configured column list for display. This
SHALL apply uniformly to all twelve chain nodes, including the seven that previously rendered a
schema-panel placeholder (Class Contract Revision, the four processor stages, Assertion Evidence,
Decision Candidate). The explorer SHALL fetch the graph once per selected `metric_id` and reuse it
across tabs rather than issuing a separate table-level request per tab. A tab whose node has no
related rows for the metric SHALL show an explicit "no rows for this metric" state, not rows from
another metric and not an error.

#### Scenario: Opening a metric-scoped record tab

- **WHEN** a metric is selected and the reader opens the `Object Mention` chain node
- **THEN** the tab shows the `kb.artifact_objects` row(s) whose `artifact_id` is the selected
  `metric_id`, and no other rows

#### Scenario: Previously-schema-only node now shows metric rows

- **WHEN** a metric is selected and the reader opens the `Decision Candidate` chain node
- **THEN** the tab shows the `kb.semantic_decision_candidates` rows for that metric, not a
  "read endpoint pending" placeholder

#### Scenario: One fetch feeds every tab

- **WHEN** a metric is selected and the reader opens three different chain nodes in turn
- **THEN** the explorer has fetched `GET /kb/metrics/:metric_id/graph` once and each tab reads its
  slice of that payload

#### Scenario: Node with no rows for this metric

- **WHEN** a metric has no class-contract revision and the reader opens `Class Contract Revision`
- **THEN** the tab shows a "no rows for this metric" state, no error, and the rest of the workspace
  keeps working

#### Scenario: Graph fetch fails

- **WHEN** the `metric_graph` request errors
- **THEN** the open record tab shows an error state and the canvas, entry tab, Search tab, and
  source-document pane keep working

### Requirement: Record tabs require a selected metric

When no metric is in context, opening a chain-node record tab SHALL show a "pick a metric from the
Search tab" empty state rather than a table-level sample of the node's `kb.*` table. The canvas,
its unfold/fold and breadcrumb navigation, the entry tab, and the Search tab SHALL remain fully
usable with no metric selected.

#### Scenario: Node click with no metric selected

- **WHEN** the explorer has no `metric_id` and the reader clicks the `Keyword Concept` chain node
- **THEN** the record tab shows a prompt to pick a metric from the Search tab, and shows no rows

#### Scenario: Navigation still works with no metric

- **WHEN** the explorer has no `metric_id`
- **THEN** the reader can still unfold and fold chains, read entry tabs, and use the Search tab to
  pick a metric

#### Scenario: Picking a metric fills the tabs

- **WHEN** a record tab is showing the "pick a metric" state and the reader picks a metric in the
  Search tab
- **THEN** the open record tab re-renders with that metric's rows for its node

### Requirement: Per-satellite related-row count badge

The canvas SHALL support an optional per-satellite badge showing the count of related rows for the
metric in context. When shown, the count SHALL be derived only from the same `metric_graph` payload
the record tabs use. The badge SHALL be absent when no metric is selected and SHALL be absent (not
rendered as a `0` that could read as an error) when the payload could not be loaded. Whether the
badge is displayed at all MAY be decided during implementation, but its data source and its
absence rules are normative.

#### Scenario: Badge shows the related count

- **WHEN** a metric is selected and its `metric_graph` payload has two `object__mention` rows
- **THEN** the `Object` satellite may show a count badge of `2`

#### Scenario: No badge without a metric

- **WHEN** no metric is selected
- **THEN** no satellite shows a count badge

### Requirement: No new frontend dependency and no schema migration

This change SHALL NOT add any npm dependency and SHALL NOT introduce a database migration. The
canvas SHALL remain hand-rolled inline SVG, and the new endpoint SHALL read only from existing
`kb.*` tables.

#### Scenario: Dependency and schema unchanged

- **WHEN** this change is reviewed
- **THEN** `ChenWeb/web/package.json` has no added dependency and no new file appears under the
  goose migrations directory
