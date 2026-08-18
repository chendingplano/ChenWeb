# observed-class-profiles Specification

## Purpose
TBD - created by archiving change canonical-metric-class-foundations. Update Purpose after archive.
## Requirements
### Requirement: Observed profiles retain inclusive evidence-bearing observations
The system SHALL aggregate class observations including represented, malformed, unparsed, missing, nonconforming, conflicting, and outlier instances, with source evidence, frequency, and distribution.

#### Scenario: Outlier remains visible
- **WHEN** a malformed metric is associated with a class
- **THEN** it SHALL appear in that class's observed profile with its state and evidence

### Requirement: Observed profiles do not grant contract authority
Writing or aggregating an observed profile SHALL NOT create, modify, activate, or broaden an authoritative class contract.

#### Scenario: Observation aggregation leaves contract unchanged
- **WHEN** a new profile observation is persisted
- **THEN** no class-contract revision or capability activation SHALL be written
