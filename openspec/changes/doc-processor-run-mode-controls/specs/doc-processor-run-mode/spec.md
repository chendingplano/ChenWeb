## ADDED Requirements

### Requirement: Run Mode selection
The Manual Launch panel SHALL provide a radio button group with four mutually exclusive run modes that determine which records from the current selection are eligible for dispatch.

#### Scenario: Default run mode
- **WHEN** the Manual Launch panel is first rendered
- **THEN** the "Run Unfinished & Failed" mode SHALL be selected by default

#### Scenario: Run Unfinished Only filters eligible records
- **WHEN** run mode is "Run Unfinished Only" and the user clicks Launch
- **THEN** only records that have at least one processor stage with status `pending` or `in-progress` SHALL be dispatched

#### Scenario: Run Failed Only filters eligible records
- **WHEN** run mode is "Run Failed Only" and the user clicks Launch
- **THEN** only records that have at least one processor stage with status `failed` SHALL be dispatched

#### Scenario: Run Unfinished & Failed filters eligible records
- **WHEN** run mode is "Run Unfinished & Failed" and the user clicks Launch
- **THEN** only records that have at least one processor stage with status `pending`, `in-progress`, or `failed` SHALL be dispatched

#### Scenario: Force Run dispatches all selected records
- **WHEN** run mode is "Force Run" and the user clicks Launch
- **THEN** all selected records SHALL be dispatched regardless of their processor stage statuses

### Requirement: Force flag in event payload reflects run mode
The `force` field in the dispatched `kb.line-file-generated` event payload SHALL be `true` only when the run mode is "Force Run"; for all other modes it SHALL be `false`.

#### Scenario: Force Run sets force flag true
- **WHEN** run mode is "Force Run"
- **THEN** the dispatched event payload SHALL contain `"force": true`

#### Scenario: Non-force modes set force flag false
- **WHEN** run mode is any of "Run Unfinished Only", "Run Failed Only", or "Run Unfinished & Failed"
- **THEN** the dispatched event payload SHALL contain `"force": false`

### Requirement: Max Records cap
The Manual Launch panel SHALL provide a numeric input labeled "Max Records to Run" that limits how many records are dispatched in a single launch session.

#### Scenario: Default max records value
- **WHEN** the Manual Launch panel is first rendered
- **THEN** the "Max Records to Run" input SHALL default to `5`

#### Scenario: Max Records caps dispatch count
- **WHEN** the run-mode filter yields more eligible records than the Max Records value
- **THEN** only the first N eligible records (up to Max Records) SHALL be dispatched

#### Scenario: Invalid max records disables Launch
- **WHEN** the "Max Records to Run" input value is not a positive integer (zero, negative, or non-numeric)
- **THEN** the Launch button SHALL be disabled

#### Scenario: Valid positive integer enables Launch
- **WHEN** the "Max Records to Run" input contains a positive integer and at least one processor is selected
- **THEN** the Launch button SHALL be enabled
