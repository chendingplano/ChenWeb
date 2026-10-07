# metric-supporting-evidence-cardinality Specification

## Purpose
TBD - created by archiving change canonical-metric-class-foundations. Update Purpose after archive.
## Requirements
### Requirement: Metric current support is singular after audited cleanup
After an auditable duplicate cleanup, the database SHALL enforce at most one active `supports` evidence link for each metric occurrence `(artifact_type, artifact_id, input_record_id)`.

#### Scenario: Duplicate metric support is rejected
- **WHEN** a second active supporting link is inserted for the same metric occurrence
- **THEN** the database SHALL reject it while retaining historical/superseded evidence rows

### Requirement: Non-metric evidence fan-out remains supported
The metric support cardinality rule SHALL not restrict non-metric artifacts or non-supporting roles.

#### Scenario: Non-metric multiple supports remain valid
- **WHEN** a non-metric artifact has multiple active supporting relationships allowed by its family
- **THEN** the cardinality rule SHALL not reject them

### Requirement: Deleting metric occurrences retires their supporting evidence
The system SHALL soft-delete every active `kb.assertion_evidence` row with `artifact_type = 'metric'` for an `input_record_id` before it deletes that record's `kb.metrics` rows for re-extraction (the forced and `force_clear` paths, which reuse `metric_id`). Each row SHALL be retired through the
single-row evidence deletion path, with a reason naming the cause. A `metric_id` reused by later
extraction SHALL NOT inherit evidence from a deleted occurrence.

#### Scenario: Re-extraction retires old metric evidence
- **WHEN** metrics for record R are deleted for re-extraction while R has active metric evidence
- **THEN** every such evidence row SHALL be marked deleted with reason "metric rows deleted for re-extraction" before the metric rows are deleted

#### Scenario: Last support removed makes the assertion unsupported
- **WHEN** a retired evidence row was the last active `supports` link of an assertion whose status allows evidence-loss transition
- **THEN** that assertion SHALL transition to `unsupported` with the retirement recorded in its reason

#### Scenario: Reused metric_id starts without evidence
- **WHEN** a re-extraction saves a new row with a `metric_id` that a deleted row used before
- **THEN** no active evidence SHALL reference that `metric_id` until the lossless writer links the new row

#### Scenario: Other records and artifact types are untouched
- **WHEN** metrics for record R are deleted
- **THEN** evidence for other records, and evidence with any `artifact_type` other than `metric`, SHALL remain active

