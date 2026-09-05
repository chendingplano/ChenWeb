## ADDED Requirements

### Requirement: can_instantiate is available to every class with a contract header
The system SHALL declare and pass the `semantic:can_instantiate` capability for any metric class that has a `kb.ontology_term_headers` row, regardless of its contract's `definition_state`.

#### Scenario: Identity-only class can still instantiate
- **WHEN** a metric class exists only as an `identity_only` contract
- **THEN** `semantic:can_instantiate` SHALL be declared and recorded as passing for that class

### Requirement: can_validate_value requires a non-identity-only contract
The system SHALL NOT declare the `semantic:can_validate_value` capability as passing for a class whose contract is `identity_only`. It SHALL declare it passing only once the contract is `partially_defined` or `validated` and specifies a value type and at least one permitted unit.

#### Scenario: Identity-only class cannot validate values
- **WHEN** a capability check for `semantic:can_validate_value` is attempted against an
  `identity_only` contract
- **THEN** the system SHALL reject the check rather than record a passing result

#### Scenario: Partially-defined class can validate values
- **WHEN** a metric class's contract is `partially_defined` with a declared value type and unit
- **THEN** `semantic:can_validate_value` SHALL be declared and recorded as passing for that class

### Requirement: Per-instance conformance is evaluated against the class's current contract
The system SHALL set a metric assertion's conformance state by comparing that specific instance's resolved value type and unit against its class's current contract, independently of the contract's capability declarations. An `identity_only` contract SHALL always result in `semantic:not_evaluated`.

#### Scenario: Conforming instance under a partially-defined contract
- **WHEN** a metric assertion's resolved unit and value type match its class's `partially_defined`
  contract
- **THEN** the assertion's conformance state SHALL be `semantic:conforms`

#### Scenario: Non-conforming instance under a partially-defined contract
- **WHEN** a metric assertion's resolved unit does not match its class's `partially_defined`
  contract's permitted units
- **THEN** the assertion's conformance state SHALL be `semantic:conformance_contract_violation`

#### Scenario: Identity-only contract is always not evaluated
- **WHEN** a metric assertion's class contract is `identity_only`
- **THEN** the assertion's conformance state SHALL be `semantic:not_evaluated`
