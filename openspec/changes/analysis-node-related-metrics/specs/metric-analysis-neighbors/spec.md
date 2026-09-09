## ADDED Requirements

### Requirement: Analysis satellite unfolds a two-node chain

The Metric Ontology Explorer canvas SHALL give the `Analysis` satellite a downstream chain of
exactly two nodes, `Metrics of Same Class` and `Metrics of Similar Classes`, in that order. The
satellite SHALL unfold, fold, and show a breadcrumb the same way the other chained satellites do,
and each node SHALL open a record tab in the content viewer. No other satellite's chain SHALL
change, and the `Analysis` entry-tab prose SHALL be unchanged.

#### Scenario: Analysis unfolds two nodes

- **WHEN** the reader clicks the `Analysis` satellite on the canvas
- **THEN** it unfolds a chain of two nodes labelled `Metrics of Same Class` and
  `Metrics of Similar Classes`
- **AND** clicking either node opens a record tab for it

#### Scenario: Other satellites unaffected

- **WHEN** this change is reviewed
- **THEN** the twelve existing chain nodes and their record tabs are unchanged, and
  `GET /api/v1/kb/metrics/:metric_id/graph` and its payload are unchanged

### Requirement: Related-metrics read endpoint

The API SHALL expose `GET /api/v1/kb/metrics/:metric_id/related-metrics`, a read-only endpoint
returning a ranked list of metrics related to the metric identified by `:metric_id` through one of
two cohorts, selected by a required `scope` query parameter with the values `same_class` and
`similar_class`. It SHALL accept the canonical `metric_id` form `<record_id>_mtc_<seq>` and the
legacy `<record_id>_<seq>` form, reject a missing or malformed `metric_id` with HTTP 400, respond
HTTP 404 when the metric does not exist, and reject a missing or unrecognised `scope` with HTTP
400. It SHALL accept an optional `limit` query parameter bounded to 1..200; when absent the limit
SHALL default to 20 for `similar_class` and 200 for `same_class`. It SHALL issue only read queries.

The response body SHALL contain `status`, the resolved `metric_id`, the requested `scope`, the
selected metric's resolved governed class term as `class_term_id` (or `null` when the metric has
no resolved class), and `results` — an array of metric rows. Each row SHALL carry at least
`metric_id`, `metric_name`, `metric_name_en`, `input_record_id`, that row's own `class_term_id`,
a `class_label` (falling back to the term id when no better label exists), `metric_value`,
`metric_unit`, and `source_filename`. For `scope=similar_class` each row SHALL additionally carry
`matched_class_term_id` and a numeric `score`; for `scope=same_class` those two fields SHALL be
omitted.

#### Scenario: Same-class cohort

- **WHEN** a client requests `.../416_mtc_1/related-metrics?scope=same_class` for a metric whose
  resolved governed class term is `T`
- **THEN** `results` contains every other metric whose resolved governed class term is `T`, each
  appearing once, up to the limit (default 200), and `class_term_id` on the response and on every
  row is `T`

#### Scenario: Similar-class cohort

- **WHEN** a client requests `.../416_mtc_1/related-metrics?scope=similar_class`
- **THEN** `results` contains at most 20 metrics (the default limit), none of them in the selected
  metric's own class, each carrying `matched_class_term_id` and `score`
- **AND** the rows are ordered by their matched class's similarity score, highest first

#### Scenario: Legacy metric-id form

- **WHEN** a client requests the endpoint with `metric_id` `416_1`
- **THEN** it is normalised to `416_mtc_1` and the same response is returned as for the canonical
  form

#### Scenario: Malformed metric id or scope

- **WHEN** a client requests the endpoint with an empty `metric_id`, a non-`<record>_mtc_<seq>`
  `metric_id`, or a `scope` other than `same_class` / `similar_class`
- **THEN** the endpoint responds HTTP 400 with `status: false` and an error message and runs no
  cohort query

#### Scenario: No write path

- **WHEN** the endpoint is exercised for either scope
- **THEN** it issues only read queries and never inserts, updates, or deletes any row

### Requirement: Selected metric with no resolved class

The endpoint SHALL treat a selected metric that has no resolved governed class — no resulting
semantic assertion, or an assertion carrying no `instance_of_term_id` — as a valid empty result,
not an error: it SHALL respond HTTP 200 with `class_term_id: null` and `results: []` for either
scope. The corresponding record tab SHALL present this as a distinct state, worded to say the
metric has no resolved governed class yet, separate from the "no rows" and error states.

#### Scenario: Deferred metric has no peers

- **WHEN** a metric's decision candidate is deferred with no `resulting_assertion_id` and a client
  requests either scope
- **THEN** the response is HTTP 200 with `class_term_id: null` and an empty `results` array

#### Scenario: Record tab distinguishes "no class" from "no rows"

- **WHEN** the `Metrics of Same Class` tab is opened for a metric with no resolved class
- **THEN** the tab shows a message that the metric has no resolved governed class yet, not the
  generic "no rows for this metric" message and not an error

### Requirement: Same-class cohort is the exact class peer group

For `scope=same_class` the endpoint SHALL return every metric — other than the selected metric —
whose resolved governed class term equals the selected metric's, reached through
`kb.semantic_decision_candidates` (`source_artifact_type = 'metric'`) →
`resulting_assertion_id` → `kb.semantic_assertions.instance_of_term_id`. A metric that reaches the
class through more than one assertion SHALL appear once. The result SHALL be ordered
deterministically (by `input_record_id`, then `metric_id`), not by a relevance score, and SHALL be
capped at the effective limit.

#### Scenario: Deduped peer

- **WHEN** a peer metric has two resulting assertions that both carry the class term `T`
- **THEN** it appears exactly once in the `same_class` results

#### Scenario: Selected metric excluded

- **WHEN** the `same_class` cohort is computed for `416_mtc_1`
- **THEN** `416_mtc_1` itself is never in `results`

### Requirement: Similar-class cohort is matched contracts expanded to their instances

For `scope=similar_class` the endpoint SHALL: (1) hybrid-match class contracts similar to the
selected metric's class contract via the class-contract hybrid-search capability, taking up to a
bounded number of candidate classes (excluding the selected metric's own class); (2) collect the
metrics whose resolved governed class term is one of those matched classes; (3) order those
metrics by their matched class's similarity score (then `input_record_id`, `metric_id`), dedupe by
`metric_id` keeping the highest-scoring match, exclude any metric in the selected metric's own
class, and return the top `limit` (default 20). Each returned row SHALL identify which matched
class it came from (`matched_class_term_id`) and carry that class's similarity `score`.

#### Scenario: Rows trace to a matched class

- **WHEN** the `similar_class` cohort returns a metric row
- **THEN** its `matched_class_term_id` is one of the classes the hybrid match returned, and its
  `class_term_id` equals that `matched_class_term_id`

#### Scenario: Selected metric's own class is excluded from similar

- **WHEN** the `similar_class` cohort is computed
- **THEN** no returned metric has the same resolved governed class term as the selected metric
  (those metrics belong only to the `same_class` cohort)

#### Scenario: Closest class ranks first

- **WHEN** matched class `A` has a higher similarity score than matched class `B`
- **THEN** every returned metric from `A` sorts ahead of every returned metric from `B`

### Requirement: Analysis record tabs render a navigable metric table

Each of the two `Analysis` record tabs SHALL render its `results` as a table using the node's
configured column list, reusing the content viewer's existing table shell, loading state, empty
state, and error state. The tab SHALL fetch its data lazily when opened — not as part of the
`GET /api/v1/kb/metrics/:metric_id/graph` payload — and SHALL cache it per `(metric_id, scope)` so
re-selecting the tab for the same metric does not refetch. Every table row SHALL be activatable
(click or keyboard) and activating a row SHALL recenter the whole explorer on that row's metric by
deep-linking `?metric_id=` to it, the same navigation the Search tab uses. The `Analysis`
satellite SHALL NOT show a related-row count badge.

#### Scenario: Lazy fetch on open

- **WHEN** a metric is selected and the reader opens the `Metrics of Similar Classes` tab for the
  first time
- **THEN** the explorer issues `GET /api/v1/kb/metrics/:metric_id/related-metrics?scope=similar_class`
  at that moment, and the `metric_graph` fetch is not re-issued

#### Scenario: Row click recenters the explorer

- **WHEN** the reader activates a row in either `Analysis` tab
- **THEN** the URL's `metric_id` becomes that row's metric and the canvas centre node, the record
  tabs, and the source-document pane all update to the newly selected metric with no full reload

#### Scenario: Cached per metric and scope

- **WHEN** the reader opens `Metrics of Same Class`, switches to another tab, and returns to
  `Metrics of Same Class` for the same selected metric
- **THEN** the tab shows the cached rows without issuing a new request

#### Scenario: No metric selected

- **WHEN** the explorer has no `metric_id` and the reader opens either `Analysis` node
- **THEN** the tab shows the "pick a metric from the Search tab" state and issues no
  related-metrics request

#### Scenario: Fetch fails

- **WHEN** the related-metrics request errors
- **THEN** the open `Analysis` tab shows a contained error state and the canvas, other tabs, and
  the source-document pane keep working
