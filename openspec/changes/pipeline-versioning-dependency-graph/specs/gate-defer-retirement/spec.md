## ADDED Requirements

### Requirement: `defer` is not a storable gate effect

The system SHALL reject, at pipeline-version creation time, any `kb.pipeline_rules` row whose `effect` is `defer`. `defer` SHALL NOT be a value any successfully created pipeline version can contain.

#### Scenario: Authoring a rule with effect=defer is rejected

- **WHEN** a pipeline version is authored with a rule whose `effect = 'defer'`
- **THEN** the system SHALL reject the pipeline version creation, naming the offending gate.

#### Scenario: Gate-fact validation subsumes the old defer justification

- **WHEN** a pipeline version passes the gate-fact-availability check (see
  `pipeline-dependency-graph` capability)
- **THEN** every gate in that version SHALL have a provable upstream producer for every fact its
  predicate references, so authoring guidance for an unresolvable gate SHALL be to add the missing
  processor as a dependency or rewrite/remove the gate — not to mark it `defer`.

### Requirement: Runtime indeterminate gate resolution is a hard failure, not a fallback

If the gate resolver ever produces an indeterminate or defer-shaped outcome at runtime against an already-validated pipeline version, the system SHALL treat it as a hard processing failure for that record's run (raising an alarm and failing the run), regardless of conflict-handling mode. The system SHALL NOT fall open to `enable` or `skip` for an indeterminate gate.

#### Scenario: Indeterminate gate resolution fails the run

- **WHEN** the gate resolver reaches an indeterminate resolution for a processor during a document
  run
- **THEN** the system SHALL raise an alarm and fail that record's run, and SHALL NOT silently
  resolve the gate to `enable` or `skip`.

#### Scenario: This path is a safety net, not an expected outcome

- **WHEN** every live pipeline version has passed creation-time gate-fact-availability validation
- **THEN** the indeterminate-resolution failure path SHALL NOT be reachable in normal operation;
  its presence in the code SHALL be treated as defense-in-depth, not a designed retry mechanism.

### Requirement: No deferred-retry scheduling exists

The system SHALL NOT retry a processor run later based on a previously deferred gate decision. `DeferredPaths`/`DeferFingerprint`-based retry plumbing SHALL be removed.

#### Scenario: No retry is scheduled after a hard gate failure

- **WHEN** a run fails due to an indeterminate gate resolution
- **THEN** the system SHALL NOT schedule an automatic later retry keyed on the gate's missing
  facts; recovery requires the pipeline version to be fixed (a new version authored) or the
  document's facts to be corrected before a fresh run.
