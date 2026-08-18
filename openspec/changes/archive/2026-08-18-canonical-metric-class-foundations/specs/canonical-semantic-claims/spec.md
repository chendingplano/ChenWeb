## ADDED Requirements

### Requirement: Claim identity is versioned and concurrency-safe
The system SHALL store canonical claim identities under a registered canonical-key version and SHALL enforce uniqueness for find-or-create under concurrent writers.

#### Scenario: Equivalent claims converge
- **WHEN** two occurrences produce byte-equal canonical keys under the active key version
- **THEN** they SHALL resolve to one `claim_id` with independent evidence

### Requirement: Assertion logical identity uses claim ID
For claim-registry managed assertions, `semantic_assertions.logical_identity_key` SHALL equal the registry `claim_id`; occurrence-derived keys SHALL not be used as canonical claim identity.

#### Scenario: Identity-bearing change creates a different claim
- **WHEN** an assertion change alters a canonical identity-bearing field
- **THEN** it SHALL resolve to a different claim rather than revise the prior claim

### Requirement: Instances reference stable classes
Ontology object instances SHALL store a stable `instance_of_term_id` and optional normalization-contract revision reference independently of lifecycle and value state.

#### Scenario: Class contract revision does not rewrite instances
- **WHEN** a class contract revision is activated
- **THEN** existing instances SHALL retain their original stable class reference
