## ADDED Requirements

### Requirement: Families that cannot instantiate persist an unresolved semantic occurrence

When a family cannot yet create a semantic assertion, it SHALL persist a
`kb.unresolved_semantic_occurrences` row recording `occurrence_key`, `input_record_id`,
`artifact_type`, `artifact_id`, source revision/fingerprint, raw payload, provenance,
`materialization_state`, `resulting_assertion_id`, `current_outcome_id`,
`supersedes_occurrence_id`, `input_fingerprint`, `dependency_fingerprint`, `active`, lease fields,
`last_seen`, and creation audit fields. `artifact_id` SHALL be required.

#### Scenario: Non-compliant family falls back
- **WHEN** an identifiable artifact belongs to a family with no compliant semantic-instance adapter
- **THEN** one active unresolved semantic occurrence and one outcome envelope are persisted

#### Scenario: Unidentified artifact creates no occurrence
- **WHEN** no artifact can be identified from processor output
- **THEN** the attempt is classified `source_or_output_unrecoverable` and no unresolved occurrence row is fabricated

#### Scenario: Occurrence is generically discoverable
- **WHEN** a consumer queries the generic semantic-discovery API
- **THEN** current unresolved occurrences are returned alongside assertions

### Requirement: Unresolved occurrence identity and active-row cardinality are database-enforced

`occurrence_key` SHALL be deterministic for the source scope and family. The database SHALL enforce
uniqueness on `(occurrence_key, input_fingerprint, dependency_fingerprint)` and at most one active row
per current source occurrence through the partial unique index
`uq_unresolved_semantic_occurrences_active` on `(occurrence_key) WHERE active = true`.

#### Scenario: Identical replay advances last_seen
- **WHEN** an unchanged occurrence is replayed
- **THEN** `last_seen` advances and no duplicate row is appended

#### Scenario: Changed input supersedes transactionally
- **WHEN** the input or dependency fingerprint changes
- **THEN** a superseding row is created and the former row is deactivated in the same transaction

### Requirement: Materialization is transactional or a recoverable saga

Workers SHALL claim materialization with row locking and an expiring lease token. Materialization
SHALL either atomically create or reuse the assertion, evidence, class-resolution decision, outcome
envelope and findings, update `materialization_state`, `resulting_assertion_id`, and
`current_outcome_id`, and supersede the unresolved row; or perform those steps as a recoverable saga
with a deterministic idempotency key and an explicit completion marker. History SHALL remain
append-only.

#### Scenario: Successful materialization supersedes the occurrence
- **WHEN** materialization completes
- **THEN** the assertion, evidence, class decision, outcome, and findings exist and the unresolved row is superseded

#### Scenario: Crash cannot leave both states active
- **WHEN** a worker crashes mid-materialization
- **THEN** reconciliation completes or rolls back the saga before exposing its current projection
- **AND** no materialized assertion is paired with an active unresolved occurrence

#### Scenario: Expired lease is reclaimable
- **WHEN** a materialization lease expires
- **THEN** another worker can claim the occurrence without creating duplicate active state

### Requirement: Fallback existence is enforced by atomic boundary and completeness projection

For a family using the generic fallback, every committed identifiable current artifact SHALL have
either a compliant current assertion path or exactly one active unresolved occurrence. Missing both
SHALL be a failed completeness invariant and SHALL block activation or cutover.

#### Scenario: Artifact with neither path fails completeness
- **WHEN** a committed identifiable current artifact has no compliant assertion path and no active unresolved occurrence
- **THEN** the completeness projection reports a failed invariant and blocks activation

### Requirement: Each artifact family supplies a versioned adapter

Each registered artifact family SHALL supply a versioned adapter defining its raw occurrence identity
and raw payload fields, how an atomic source occurrence is recognized, the minimum raw-preserved
semantic instance shape, provisional class behavior, the value and conformance states it uses,
canonical identity for normalized and raw-preserved cases, capability-aware downstream operations, and
dependency fingerprints and retry triggers. The adapter SHALL also enumerate its required semantic
stages, the decision scopes and dimensions each stage may report, and the disposition/capability
contract for every stage.

#### Scenario: Adapter declaration drives cardinality
- **WHEN** the completeness projection evaluates a family
- **THEN** the required stage set comes from that family's adapter declaration

#### Scenario: Undeclared stage is not required
- **WHEN** a stage is not declared required by the adapter
- **THEN** its absence does not fail completeness

### Requirement: Shared infrastructure owns generic concerns

Shared infrastructure SHALL own outcome and unresolved-occurrence persistence, state vocabulary,
idempotency, retry scheduling, logging, and API query shapes. Artifact semantics SHALL remain
family-owned.

#### Scenario: Family adds no new persistence layer
- **WHEN** a new family adapter is registered
- **THEN** it reuses the shared outcome, occurrence, retry, and discovery infrastructure

### Requirement: Writer activation requires passing the current conformance suite

Family adapters SHALL pass a shared conformance suite before their lossless writer is activated. A
runtime compliance registry SHALL report adapter name/version, enabled writer mode, conformance-suite
version, and last verified result. Activation SHALL be refused when the registered adapter has not
passed the current suite. An unregistered or non-compliant family SHALL use the generic fallback, SHALL
NOT silently skip the artifact, and SHALL NOT advertise full semantic-instance compliance.

#### Scenario: Activation refused without a passing result
- **WHEN** a writer gate is enabled for an adapter that has not passed the current conformance-suite version
- **THEN** activation is refused

#### Scenario: Registry reports current state
- **WHEN** the compliance registry is queried
- **THEN** it returns adapter name, version, writer mode, conformance-suite version, and last verified result

#### Scenario: Non-compliant family cannot advertise compliance
- **WHEN** a family has no registered compliant adapter
- **THEN** it uses the generic fallback and reports itself as non-compliant
