## ADDED Requirements

### Requirement: Semantic findings retry on a separate dependency-driven path

Execution failures SHALL use the existing failed-processor retry mechanism. Semantic findings SHALL
use a separate dependency-driven retry path and SHALL NOT enter the failed-processor queue.

#### Scenario: Finding does not enter the failed-processor queue
- **WHEN** a stage records a retryable semantic finding
- **THEN** a dependency-driven retry record is created and the failed-processor queue is unchanged

#### Scenario: Execution failure uses the legacy path
- **WHEN** a processor fails with a system failure
- **THEN** the existing failed-processor retry mechanism handles it

### Requirement: Retryable outcomes record a dependency fingerprint

Every retryable semantic outcome SHALL record a dependency fingerprint covering the relevant versions
and decisions, including the mapping table entry/revision, parser/normalizer version, class identity
decision, class-contract revision and validator version, unit/quantity vocabulary release, and
model/prompt version when an LLM participated. Fingerprints SHALL use canonical, versioned
serialization.

#### Scenario: Fingerprint is stable across serialization
- **WHEN** the same dependency set is fingerprinted twice in different map iteration orders
- **THEN** the two fingerprints are byte-equal

#### Scenario: Fingerprint aggregates children
- **WHEN** an outcome's child finding dependency changes
- **THEN** the outcome's aggregate dependency fingerprint changes

#### Scenario: Fingerprint version is explicit
- **WHEN** a fingerprint is persisted
- **THEN** it carries an explicit serialization version prefix

### Requirement: The retry queue is uniquely keyed and transactionally claimed

The retry queue SHALL be unique on `(outcome_id, finding_id, target_dependency_fingerprint)`, where
`finding_id` is nullable for a whole-stage retry. Workers SHALL claim jobs transactionally using the
database's row-lock/skip-locked mechanism and SHALL record a lease or attempt token.

#### Scenario: Concurrent enqueue is idempotent
- **WHEN** two enqueues target the same `(outcome_id, finding_id, target_dependency_fingerprint)`
- **THEN** the second is an idempotent conflict and no duplicate job exists

#### Scenario: Concurrent execution cannot double-activate
- **WHEN** two workers attempt the same job
- **THEN** only one claims it and no two outcomes become active

#### Scenario: Whole-stage retry uses a null finding
- **WHEN** an entire stage must be retried rather than one finding
- **THEN** the queue row carries a null `finding_id`

#### Scenario: Expired lease is reclaimable
- **WHEN** a worker's lease expires without completion
- **THEN** another worker can claim the job without producing duplicate active state

### Requirement: Stale jobs perform no semantic writes

A job whose source input or target dependency no longer matches SHALL record `stale` and SHALL perform
no semantic writes.

#### Scenario: Source changed since enqueue
- **WHEN** a claimed job's source input fingerprint no longer matches
- **THEN** the job records `stale` and writes no assertion, evidence, outcome, or finding

#### Scenario: Dependency moved on since enqueue
- **WHEN** a claimed job's target dependency fingerprint is no longer current
- **THEN** the job records `stale` and performs no semantic writes

### Requirement: Only affected outcomes are scheduled on dependency change

When a dependency changes, the system SHALL schedule only the affected outcomes. An unchanged
fingerprint SHALL reuse the existing outcome and SHALL NOT generate repeated work or alerts. A changed
dependency SHALL produce a superseding outcome.

#### Scenario: Mapping approval retries only affected metrics
- **WHEN** a proposed range-type mapping is approved
- **THEN** only outcomes whose dependency fingerprint includes that mapping revision are scheduled

#### Scenario: Unchanged dependency produces no work
- **WHEN** a retry sweep runs with no dependency change
- **THEN** no jobs are enqueued and no alerts are emitted

### Requirement: Identity-bearing change resolves a different claim, not a revision

If the canonical claim payload remains byte-equal, an assertion-owned non-identity interpretation or
governance change MAY create a new revision under the same `claim_id`. If any identity-bearing field
changes, the system SHALL resolve or create a different `claim_id` and SHALL NOT present the new claim
as a revision of the former one. New evidence, confidence aggregation, or `last_seen` alone SHALL NOT
create an assertion revision.

#### Scenario: Byte-equal payload creates a revision
- **WHEN** a retry changes only interpretation or governance fields and the canonical payload is byte-equal
- **THEN** a new revision is appended under the same `claim_id`

#### Scenario: Identity change creates a new claim
- **WHEN** a retry changes an identity-bearing field
- **THEN** a different `claim_id` is resolved or created, with any required redirect or semantic relation

#### Scenario: New evidence alone creates no revision
- **WHEN** only new evidence, confidence aggregation, or `last_seen` changes
- **THEN** no new assertion revision is created

#### Scenario: Redirect and rebuild are atomic or recoverable
- **WHEN** an assertion redirect, semantic relation, or projection rebuild is required
- **THEN** it occurs in one orchestrated transaction or a recoverable saga with an explicit completion marker

### Requirement: Retry never blocks ordinary pipeline completion

Human involvement SHALL remain optional. Approved mappings and corrections MAY trigger retry, but
ordinary pipeline completion SHALL NOT wait for review.

#### Scenario: Pipeline completes without review
- **WHEN** a record's metrics carry unresolved mappings awaiting human review
- **THEN** the pipeline run completes and does not block on that review

#### Scenario: Approval triggers targeted retry
- **WHEN** an administrator approves a mapping
- **THEN** targeted retry is triggered for the affected outcomes only
