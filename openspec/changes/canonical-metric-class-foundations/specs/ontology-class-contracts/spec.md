## ADDED Requirements

### Requirement: Class contracts are append-only and capability-specific
The system SHALL store class-contract revisions append-only and SHALL record each enabled validation or comparison capability with its named validator, version, result, and evidence.

#### Scenario: Identity-only class has no unsupported capability
- **WHEN** a class has no passing validator for a capability
- **THEN** the class MAY receive instances but SHALL not advertise or execute that capability

### Requirement: Contract revision does not arise from raw observation union
The system SHALL create authoritative contract revisions only through a governed synthesis decision, not by copying every observed attribute or value form.

#### Scenario: Malformed observed value does not broaden a numeric contract
- **WHEN** an observed instance contains a raw string incompatible with a numeric contract
- **THEN** the observation remains preserved with its state and the numeric contract remains unchanged
