## MODIFIED Requirements

### Requirement: Rows holding several values are reported
The system SHALL review unclassified combined Pass 2 values once against original candidate evidence. The response SHALL explicitly decide split, keep or unresolved with a reason per affected candidate. Split replacements SHALL contain distinct independently interpretable assertions, preserving source evidence and every original numeric token with sign and multiplicity. Keep SHALL retain one complete supported structured value for the candidate. Errored, missing, ambiguous or invalid decisions SHALL retain original rows as unresolved. Original non-combined siblings SHALL be preserved. Valid non-scalar structures SHALL NOT trigger a retry or multi-value warning merely for containing several numbers. Remaining unresolved combinations SHALL be reported.

#### Scenario: Merged cell values
- **WHEN** a row has 男300 mm、女250 mm
- **AND** review cannot return a valid explicit split
- **THEN** the original row SHALL remain unresolved and be reported

#### Scenario: Percentage positions and a point count
- **WHEN** a row holds 20%、80%及中间区段任意2~3个
- **AND** review identifies separate position and count properties
- **THEN** the position collection and count range SHALL preserve all values and source evidence

#### Scenario: Keep is successful
- **WHEN** review returns keep with one alternatives structure for 10、20、30
- **THEN** that row SHALL be accepted without requiring two replacements
