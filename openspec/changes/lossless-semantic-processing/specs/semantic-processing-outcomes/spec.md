## ADDED Requirements

### Requirement: Structured semantic outcomes are persisted per artifact and stage

The system SHALL persist an append-only `kb.semantic_processing_outcomes` store recording
`outcome_key`, `input_record_id`, `artifact_type`, `artifact_id`, `assertion_id`, `stage_term_id`,
`disposition_term_id`, `finding_count`, `highest_severity_term_id`, `raw_fragment`,
`dependency_fingerprint`, processor name/version, extraction run, model/prompt version,
`supersedes_outcome_id`, `input_fingerprint`, `active`, `last_seen`, and creation audit fields. Every
required semantic-stage attempt, including success, SHALL have exactly one outcome envelope or reuse
an identical existing envelope.

#### Scenario: Successful stage still records an envelope
- **WHEN** a required semantic stage completes with no findings
- **THEN** one outcome envelope exists with `disposition_term_id = 'semantic:normalized'` and `finding_count = 0`

#### Scenario: Outcome key is deterministic
- **WHEN** an outcome is written
- **THEN** `outcome_key` is the deterministic hash of `input_record_id`, `artifact_type`, `artifact_id`, and `stage_term_id`

#### Scenario: Table is append-only
- **WHEN** any outcome field other than `last_seen` or the active projection must change
- **THEN** a new row is inserted and the prior row is marked superseded rather than updated

### Requirement: Outcome identity and active-row cardinality are database-enforced

The database SHALL enforce uniqueness on `(outcome_key, input_fingerprint, dependency_fingerprint)`
and SHALL enforce at most one active outcome per stable source/stage scope through the partial unique
index `uq_semantic_processing_outcomes_active` on `(outcome_key) WHERE active = true`. Activation of a
superseding outcome and deactivation of the former row SHALL occur in the same locked transaction.

#### Scenario: Duplicate identical write conflicts
- **WHEN** two writers attempt the same `(outcome_key, input_fingerprint, dependency_fingerprint)`
- **THEN** the database rejects the second insert as a uniqueness conflict

#### Scenario: Two concurrent workers produce one active row
- **WHEN** two workers race on the same occurrence and dependency
- **THEN** exactly one outcome row remains active

#### Scenario: Supersession is atomic
- **WHEN** a changed input or dependency produces a superseding outcome
- **THEN** the new row becomes active and the former row becomes inactive in the same transaction

#### Scenario: Active row carries the current fingerprint
- **WHEN** an outcome row is active
- **THEN** its `input_fingerprint` is the current raw occurrence revision

### Requirement: Completed artifact-stage outcomes identify their artifact

A completed semantic-stage outcome SHALL have a non-null `artifact_id`. A null `artifact_id` SHALL be
permitted only for a failed invocation-level outcome classified as `source_or_output_unrecoverable`,
and SHALL NOT create an unresolved semantic occurrence.

#### Scenario: Completed outcome without artifact is rejected
- **WHEN** a completed semantic-stage outcome is written with a null `artifact_id`
- **THEN** the write is rejected

#### Scenario: Unidentified failed invocation
- **WHEN** processor output is so malformed that no artifact can be identified
- **THEN** a failed invocation-level outcome with null `artifact_id` is recorded
- **AND** no `kb.unresolved_semantic_occurrences` row is created

### Requirement: Typed findings are append-only children of an outcome

The system SHALL persist `kb.semantic_processing_findings` with `outcome_id`, `finding_key`,
`dimension_term_id`, `finding_term_id`, `severity_term_id`, `retry_state_term_id`, `error_code`,
`details`, `dependency_fingerprint`, `supersedes_finding_id`, `active`, `last_seen`, and creation
audit fields. One stage SHALL be able to report multiple independent findings without duplicating its
outcome envelope. Two artifacts SHALL NEVER share an outcome envelope or a finding row.

#### Scenario: Multiple independent findings on one stage
- **WHEN** a stage discovers a datatype mismatch, a contract violation, and a source conflict
- **THEN** one outcome envelope holds three distinct active finding rows

#### Scenario: Findings are per-artifact
- **WHEN** two artifacts exhibit the same unresolved vocabulary value
- **THEN** each has its own outcome envelope and its own finding row

#### Scenario: Finding uniqueness is enforced
- **WHEN** a finding is written
- **THEN** the database enforces uniqueness on `(outcome_id, finding_key, dependency_fingerprint)`
- **AND** enforces one active finding per `(outcome_id, finding_key)` via `uq_semantic_processing_findings_active`

### Requirement: Finding summaries are transactionally derived

`finding_count` and `highest_severity_term_id` SHALL be derived from the current child finding set
within the same transaction that writes that set. Reports SHALL count child `finding_term_id` values
rather than the envelope disposition.

#### Scenario: Summary matches children
- **WHEN** an outcome's child finding set is written or superseded
- **THEN** `finding_count` equals the number of active children and `highest_severity_term_id` equals their maximum severity

#### Scenario: Reports count finding terms
- **WHEN** a run report groups findings
- **THEN** it groups by governed `finding_term_id`, not by outcome disposition

### Requirement: Outcome supersession deactivates child findings

When an outcome is superseded, all of its active child findings SHALL be deactivated in the same
transaction. A deferred constraint trigger SHALL reject commit if an active finding references an
inactive outcome.

#### Scenario: Children deactivate with the parent
- **WHEN** an outcome envelope is superseded
- **THEN** every active child finding of that outcome becomes inactive in the same transaction

#### Scenario: Orphan active finding is rejected at commit
- **WHEN** a transaction would commit an active finding referencing an inactive outcome
- **THEN** the deferred constraint trigger rejects the commit

### Requirement: Unchanged replay is idempotent

An unchanged replay SHALL reuse the existing outcome row and advance only `last_seen`. It SHALL NOT
append a new row or re-alert. A changed input or dependency SHALL insert a new row and mark the former
active row superseded in the same transaction.

#### Scenario: Replay advances last_seen only
- **WHEN** a stage is re-run with identical input and dependency fingerprints
- **THEN** the existing outcome row's `last_seen` advances and no new row or alert is produced

#### Scenario: Logs are not the source of truth
- **WHEN** an outcome must be determined
- **THEN** it is read from `kb.semantic_processing_outcomes`, never reconstructed from log text

### Requirement: Disposition, dimension, and finding vocabularies are governed ontology terms

Disposition, dimension, and finding terms SHALL be extensible governed ontology terms, not closed
database enums. The initial governed disposition terms SHALL be `semantic:normalized`,
`semantic:raw_preserved`, `semantic:not_applicable`, and `semantic:no_result`. The initial governed
finding terms SHALL include `semantic:mapping_unresolved`, `semantic:mapping_ambiguous`,
`semantic:unparsed`, `semantic:value_missing`, `semantic:value_unknown`, `semantic:datatype_mismatch`,
`semantic:contract_violation`, `semantic:class_provisional`, `semantic:class_ambiguous`,
`semantic:identity_evidence_conflict`, `semantic:source_conflict`, and `semantic:no_verdict`. Stable
machine-readable `error_code` values SHALL identify programmatic cases and human details SHALL NOT
participate in identity.

#### Scenario: New finding term needs no schema change
- **WHEN** a new governed finding term is introduced
- **THEN** it is added as an ontology term and no database enum or CHECK constraint is altered

#### Scenario: Governed identifiers are used exactly
- **WHEN** a disposition or finding is persisted or returned through an API
- **THEN** the exact governed underscore identifier is used, and hyphenated aliases are rejected

#### Scenario: error_code is identity-stable
- **WHEN** a finding's human-readable details change
- **THEN** its `error_code` and `finding_key` are unchanged

### Requirement: Completeness is verified independently of uniqueness

The unique indexes SHALL enforce at most one row, not row existence. The system SHALL provide a
transactional completeness projection comparing each current occurrence with its adapter-declared
required stage set; missing outcomes SHALL make the attempt or cutover incomplete and retryable.

#### Scenario: Missing required stage outcome is reported
- **WHEN** a current occurrence lacks an outcome for an adapter-declared required stage
- **THEN** the completeness projection reports the attempt as incomplete and retryable

#### Scenario: Completeness blocks cutover
- **WHEN** the completeness projection reports missing required outcomes
- **THEN** writer activation or cutover is refused
