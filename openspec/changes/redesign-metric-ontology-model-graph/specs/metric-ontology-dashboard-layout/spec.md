## ADDED Requirements

### Requirement: Central Metric Ontology Model graph
The dashboard-mode view of `/home3/ontology-metric-analysis` SHALL render a "Metric Ontology Model"
graph as the visual center of the page: a full-width band ahead of the KPI/panel/table content in
DOM order, carrying an accent treatment that distinguishes it from the surrounding panels.

The graph SHALL present the model of `metric-ontology-v1.0-en.md` §5 as three labeled lanes, one per
population of §5.1 — ontology-born, corpus-level identity, and record-born — each in its own colour,
with a legend stating what reprocessing a document does to each. Entity names SHALL match the
spelling used in §5.2–5.4.

#### Scenario: Three population lanes render
- **WHEN** a user opens the dashboard-mode view (no `?view=` query param, or `?view=dashboard`)
- **THEN** the page shows a graph section with an ontology-born lane containing at minimum
  `kb.ontology_terms`, `kb.ontology_modules`, module release, `kb.ontology_candidates`,
  `kb.ontology_term_labels`, and `kb.metric_value_range_type_map`; a corpus-level lane containing
  `kb.keyword_concepts`, `kb.object_nodes`, and `kb.semantic_claim_identities`; and a record-born
  lane containing `kb.inputs`, `kb.metrics`, `kb.artifact_objects`,
  `kb.semantic_decision_candidates`, `kb.semantic_assertions`, `kb.assertion_evidence`, and
  `kb.semantic_processing_outcomes`
- **AND** `kb.metrics` carries an emphasis treatment marking it as the point of contact between the
  three populations

#### Scenario: Governed term kinds are shown inside the terms shell
- **WHEN** the Metric Ontology Model graph is rendered
- **THEN** the `kb.ontology_terms` shell groups the term kinds a metric reaches under metric
  identity (`metric_definition`, metric `class`), measurement science (the
  `quantity_kind → dimension → unit` chain of §4.2), and claim frame (assertion kinds of §4.3 and
  the binding properties of §4.1), and names the six measurement classes of §4.1

#### Scenario: The pipeline reads left to right
- **WHEN** the Metric Ontology Model graph is rendered
- **THEN** the record-born lane runs `kb.inputs` → `kb.metrics` → `kb.semantic_decision_candidates`
  → `kb.semantic_assertions` → `kb.assertion_evidence` left to right, with the `extract_metrics`,
  `normalize_assertions`, and `associate_semantics` stage names shown on that path

#### Scenario: The §5.5 joins are labeled edges
- **WHEN** the Metric Ontology Model graph is rendered
- **THEN** a labeled edge connects `kb.metrics` to its keyword concept and on to a governed
  `metric_definition` term (`metric_name`, `core:aligns_to_term`); a labeled path connects
  `kb.metrics` through its object mention to an object node and on to the assertion's subject
  (`subject_object_id`); a labeled edge connects `kb.semantic_assertions` to the terms shell
  carrying `instance_of`, assertion kind, unit, and quantity kind; a labeled edge connects
  `kb.semantic_decision_candidates` to the terms shell for value-range-type, unit, and quantity-kind
  lookup; and a labeled edge connects `kb.assertion_evidence` back to `kb.inputs`

### Requirement: Existing dashboard content grouped by model entity
The dashboard-mode view SHALL group its existing KPI cards, four panels (error presence, coverage
state, mapping inventory, errors by type), and recent-occurrences table into three entity clusters
placed beneath the model band, instead of the prior flat KPI-row-then-2x2-grid-then-table stack. No KPI, panel, or table row is added, removed, or rebound to different data as part of
this grouping.

#### Scenario: Ontology-terms cluster
- **WHEN** the dashboard-mode view is rendered
- **THEN** the "Ontology metrics" KPI, "Metric classes" KPI, and the mapping-inventory panel appear
  together in the cluster below the model band on the `ontology_terms` side

#### Scenario: Semantic-assertions cluster
- **WHEN** the dashboard-mode view is rendered
- **THEN** the "Current instances" KPI, the coverage-state panel, and the errors-by-type panel appear
  together in the cluster below the model band on the `semantic_assertions` side

#### Scenario: Evidence/Metrics cluster
- **WHEN** the dashboard-mode view is rendered
- **THEN** the "Occurrences", "With errors", and "No detected errors" KPIs, the error-presence donut
  panel, and the recent-occurrences table appear together in a full-width cluster beneath the other
  two

### Requirement: Layout preserves existing data behavior and responsiveness
The redesigned layout SHALL NOT change any existing data-loading, filtering, search, or error/loading
state behavior of the dashboard-mode view, and SHALL remain usable on narrow viewports.

#### Scenario: Live data and fixture fallback unaffected
- **WHEN** `getMetricOntologyAnalysis` resolves or rejects
- **THEN** the KPI values, panel contents, and recent-occurrences table populate or fall back to the
  last safe fixture exactly as before this change, only their position on the page has moved

#### Scenario: Narrow viewport collapses to a single column
- **WHEN** the viewport is at or below the existing `1050px` breakpoint
- **THEN** the graph and the three clusters stack into a single reading-order column instead of the
  multi-column grid used at wider viewports

#### Scenario: The diagram scrolls rather than widening the page
- **WHEN** the viewport is too narrow for the diagram to render at a legible scale
- **THEN** the diagram scrolls horizontally within its own container
- **AND** the page itself does not acquire a horizontal scrollbar

### Requirement: Unaffected views and routes
This change SHALL NOT alter the Document Metrics mode (`?view=document`), the Ontology Metrics mode,
or the `metricOntologyAnalysisService` API contract.

#### Scenario: Document Metrics mode unchanged
- **WHEN** a user opens `/home3/ontology-metric-analysis?view=document`
- **THEN** the `DocumentMetricsAnalysis` component renders exactly as it did before this change
