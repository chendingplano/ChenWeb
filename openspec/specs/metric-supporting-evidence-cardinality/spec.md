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
