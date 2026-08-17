## ADDED Requirements

### Requirement: Processor outcomes are classified into four categories

The system SHALL classify every processor outcome as `system_failure`,
`source_or_output_unrecoverable`, `semantic_finding`, or `semantic_success`. `system_failure` and
`source_or_output_unrecoverable` SHALL fail the processor. `semantic_finding` and `semantic_success`
SHALL complete the processor and continue semantic processing.

#### Scenario: Infrastructure failure fails the processor
- **WHEN** required input is unreadable, the database is unavailable, a transaction rolls back, a required model or service invocation returns no usable output, or an invariant violation prevents safe persistence
- **THEN** the processor status is `failed` and the affected dependency branch stops for operational retry

#### Scenario: Unrecoverable output fails the processor
- **WHEN** processor output is so malformed that no artifact can be identified or safely preserved
- **THEN** the processor status is `failed` and the invocation and raw output are preserved where possible

#### Scenario: Semantic finding completes the processor
- **WHEN** a mapping is missing, a value is unparsed, a datatype mismatches, a class is provisional or ambiguous, a contract is violated, a value is missing, or sources conflict
- **THEN** the processor status is `completed` with a mandatory finding summary
- **AND** the artifact, instance or outcome, and evidence are persisted and processing continues

#### Scenario: Full success completes the processor
- **WHEN** all required normalization and validation capabilities produce usable output
- **THEN** the processor status is `completed` and processing continues

### Requirement: Execution status is canonically binary

Persisted execution status SHALL be exactly `completed` or `failed`. A completed run SHALL carry a
mandatory `finding_count`, highest finding severity, and finding-summary counts grouped by governed
finding term. "Completed with findings" SHALL exist only as a derived display phrase and SHALL NOT be
a third persisted execution status.

#### Scenario: Only two statuses persist
- **WHEN** a processor run status is written
- **THEN** the persisted value is `completed` or `failed` and no other value is accepted

#### Scenario: Completed run carries a finding summary
- **WHEN** a run completes
- **THEN** its report includes `finding_count`, the highest finding severity, and counts by governed finding term

#### Scenario: Derived label is display-only
- **WHEN** a UI renders a completed run with `finding_count > 0`
- **THEN** it may display "completed with findings" without any such value being persisted

### Requirement: The legacy status projection is preserved

The legacy schema SHALL project `completed` to `success` and `failed` to `failed`. `has_failed_proc`
and failed-processor retry queues SHALL include only `failed` execution.

#### Scenario: Legacy projection maps completed
- **WHEN** a `completed` execution status is written to the legacy `proc_status` field
- **THEN** the persisted legacy value is `success`

#### Scenario: Findings do not set has_failed_proc
- **WHEN** a run completes with content-level findings of any severity
- **THEN** `has_failed_proc` is not set and the record does not enter the failed-processor retry queue

#### Scenario: Failed execution still sets has_failed_proc
- **WHEN** a run fails with a system failure
- **THEN** `has_failed_proc` is set and the record enters the failed-processor retry queue

### Requirement: Finding severity does not convert findings into failures

Content-level findings SHALL be able to carry `error` severity for users. Severity SHALL NOT convert a
finding into an execution failure unless the system cannot safely persist or continue.

#### Scenario: Error-severity finding still completes
- **WHEN** a stage records a finding with `error` severity and all required writes succeed
- **THEN** the processor status is `completed`

### Requirement: "Cannot continue" is defined narrowly

"Cannot continue" SHALL mean that required infrastructure or the required atomic persistence set
cannot complete. Inability to parse, classify, validate, compare, or project a semantic result SHALL
remain a finding when the input, outcome, provenance, and an explicit no-result reason can be durably
committed.

#### Scenario: Unclassifiable input is a finding
- **WHEN** a classifier cannot classify an input but the input, outcome, provenance, and no-result reason commit successfully
- **THEN** the result is a finding and the processor completes

#### Scenario: Optional enrichment failure with declared fallback is a finding
- **WHEN** an optional enrichment service fails and the stage contract declares and persists a deterministic fallback
- **THEN** the result is a finding and the processor completes

#### Scenario: Required service failure is an execution failure
- **WHEN** a service declared required by the stage contract fails
- **THEN** the processor status is `failed`

### Requirement: Runs report findings without producing failure storms

Every processor run SHALL report artifacts examined; normalized and raw-preserved instance counts;
findings by governed finding term, severity, and retry state; new versus reused findings; downstream
operations completed or skipped with explicit reasons; and actual system failures separately.
Operator and admin views SHALL expose findings and affected artifacts without requiring SQL, and SHALL
NOT present the derived "completed with findings" label as a processing failure.

#### Scenario: Run report contains all required sections
- **WHEN** a processor run finishes
- **THEN** its report contains examined counts, instance counts by disposition, findings by term/severity/retry state, new versus reused counts, downstream skip reasons, and system failures as a separate section

#### Scenario: Repeated vocabulary value does not alarm per artifact
- **WHEN** one unresolved vocabulary value recurs across many artifacts
- **THEN** governed occurrence evidence increments and logs summarize per record or run rather than emitting one alarm per artifact

#### Scenario: Operator view does not require SQL
- **WHEN** an operator inspects findings and affected artifacts
- **THEN** the admin view presents them without requiring a database query
