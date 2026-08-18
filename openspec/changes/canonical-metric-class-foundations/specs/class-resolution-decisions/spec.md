## ADDED Requirements

### Requirement: Class resolution is explicit and append-only
The system SHALL record each class-resolution decision with source occurrence, selected class when available, alternatives, evidence, method, confidence, identity state, and supersession history.

#### Scenario: Ambiguous class remains represented
- **WHEN** a metric has multiple unresolved class candidates
- **THEN** the system SHALL persist the ambiguity and its alternatives without selecting an authoritative class

### Requirement: No safe match creates a provisional class
The system SHALL create or select a provisional stable class when no existing class safely matches a source-backed metric, rather than discard the metric or attach it classlessly.

#### Scenario: New metric receives provisional class
- **WHEN** deterministic resolution finds no safe existing class
- **THEN** a provisional class and recorded resolution decision SHALL be available for the metric
