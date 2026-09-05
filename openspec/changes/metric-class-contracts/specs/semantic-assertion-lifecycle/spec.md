## ADDED Requirements

### Requirement: Conformance evaluation never changes assertion status or governance
Setting a metric assertion's conformance state to `semantic:conforms` or `semantic:conformance_contract_violation` SHALL NOT change that assertion's lifecycle `status` or imply governance acceptance. A `represented` assertion that conforms to its class's contract remains `represented`, not `accepted`.

#### Scenario: A conforming assertion stays represented, not accepted
- **WHEN** a `represented` metric assertion's conformance state is set to `semantic:conforms`
- **THEN** the assertion's lifecycle `status` SHALL remain `represented`

#### Scenario: Backfilled conformance re-evaluation preserves the original assertion
- **WHEN** an existing assertion's conformance state is re-evaluated after its class's contract is
  promoted
- **THEN** the system SHALL update that assertion's conformance state in place and SHALL NOT create
  a new assertion or alter its evidence, value, or identity fields
