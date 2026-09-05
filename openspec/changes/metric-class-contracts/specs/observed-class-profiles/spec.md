## MODIFIED Requirements

### Requirement: Observed profiles do not grant contract authority
Writing or aggregating an observed profile SHALL NOT itself create, modify, activate, or broaden an authoritative class contract. A contract revision MAY be created only by a separate, explicit synthesis decision that reads accumulated observed-profile evidence under a governed, deterministic rule — never as a direct effect of persisting one observation.

#### Scenario: Observation aggregation leaves contract unchanged
- **WHEN** a new profile observation is persisted
- **THEN** the persistence operation itself SHALL NOT write any class-contract revision or
  capability activation

#### Scenario: A separate synthesis decision may still act on accumulated evidence
- **WHEN** a governed synthesis rule subsequently evaluates a class's accumulated observed-profile
  evidence and finds it eligible under that rule
- **THEN** the synthesis decision, not the act of recording an observation, is what SHALL be
  recorded as the cause of any resulting contract revision
