## ADDED Requirements

### Requirement: Every resolved class term is anchored to a stable contract header
Whenever the metric class-resolution path selects or creates a class term — whether newly synthesized, matched by signature, or reused from an existing term created before this requirement existed — the system SHALL ensure that term has a `kb.ontology_term_headers` row and at least one contract revision, without appending a duplicate identity-only revision on repeated resolution of the same class.

#### Scenario: First resolution of a new class creates its header and identity-only revision
- **WHEN** a metric class term is resolved for the first time
- **THEN** the system SHALL create its `kb.ontology_term_headers` row and one `identity_only`
  contract revision

#### Scenario: Repeated resolution of the same class is idempotent
- **WHEN** a metric class term that already has a contract header is resolved again
- **THEN** the system SHALL NOT append another identity-only contract revision

#### Scenario: A pre-existing class term without a header is backfilled on next resolution
- **WHEN** a metric class term created before this requirement existed is resolved again
- **THEN** the system SHALL create its missing `kb.ontology_term_headers` row and identity-only
  contract revision at that time
