# ontology-class-identity Specification

## Purpose
TBD - created by archiving change canonical-metric-class-foundations. Update Purpose after archive.
## Requirements
### Requirement: Stable ontology class identity is independent of contract revision
The system SHALL maintain one stable ontology class `term_id` independent of every append-only term or class-contract revision, and SHALL expose current class state through `kb.ontology_terms_current`.

#### Scenario: Contract evolution preserves class identity
- **WHEN** a class receives a new validated contract revision
- **THEN** every existing instance retains its stable class `term_id` and the current view resolves the new revision

### Requirement: Current-term reader migration is verified before base reshaping
The system SHALL inventory and migrate application current-term readers to the compatibility view or stable terms API before it prohibits base-table reads.

#### Scenario: Unmigrated reader blocks reshaping
- **WHEN** the read audit finds an application consumer reading the base term table for current state
- **THEN** base-table reshaping SHALL remain blocked and the consumer SHALL be reported
