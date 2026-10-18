## MODIFIED Requirements

### Requirement: Metric rows carry an optional provision link
`kb.metrics` SHALL have a nullable `provision_id BIGINT` column that references `kb.provisions(id)`.
It identifies the provision clause whose criterion the metric row states. It is the step link of
the metric's procedure facet. `extract_metrics` MAY set it when a `kb.provisions` row of the same
record covers the row's `frame.procedure.step` and source lines; otherwise it SHALL be null.
Existing rows SHALL NOT be backfilled.

#### Scenario: New metric rows default to no link
- **WHEN** `extract_metrics` saves metric rows for a record that has no provisions
- **THEN** each saved row SHALL have `provision_id` null

#### Scenario: Step with a known provision
- **WHEN** a saved row has `frame.procedure.step` A.2.4 and the record has a provision for clause
  A.2.4 covering the row's lines
- **THEN** the row's `provision_id` is that provision's id

#### Scenario: Link must name an existing provision
- **WHEN** a metric row's `provision_id` is set to a value that is not a `kb.provisions.id`
- **THEN** the database SHALL reject the write
