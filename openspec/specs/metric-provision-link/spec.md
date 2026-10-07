# metric-provision-link Specification

## Purpose
TBD - created by archiving change requirements-metrics-phase1. Update Purpose after archive.
## Requirements
### Requirement: Metric rows carry an optional provision link
`kb.metrics` SHALL have a nullable `provision_id BIGINT` column that references `kb.provisions(id)`.
It identifies the provision clause whose criterion the metric row states. No processor SHALL be
required to fill it in this phase. Existing rows SHALL NOT be backfilled.

#### Scenario: New metric rows default to no link
- **WHEN** `extract_metrics` saves metric rows for a record
- **THEN** each saved row SHALL have `provision_id` null

#### Scenario: Link must name an existing provision
- **WHEN** a metric row's `provision_id` is set to a value that is not a `kb.provisions.id`
- **THEN** the database SHALL reject the write

### Requirement: Provision re-extraction does not delete or block metrics
Deleting a referenced `kb.provisions` row SHALL set `provision_id` to null on the referencing
metric rows. It SHALL NOT delete those rows or fail the provision deletion.

#### Scenario: Provisions of a record are deleted for re-extraction
- **WHEN** `kb.provisions` rows referenced by metric rows are deleted
- **THEN** the deletion SHALL succeed, the metric rows SHALL remain, and their `provision_id` SHALL be null

### Requirement: Metric read APIs expose the provision link
Metric list and detail responses that return `kb.metrics` columns SHALL include `provision_id` as
a number or null.

#### Scenario: Unlinked metric is read
- **WHEN** a client reads a metric row whose `provision_id` is null
- **THEN** the response SHALL contain `"provision_id": null`

