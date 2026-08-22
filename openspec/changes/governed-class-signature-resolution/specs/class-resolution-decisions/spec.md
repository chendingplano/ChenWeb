## ADDED Requirements

### Requirement: Signature resolution is attempted before name-hash or concept fallback
The system SHALL attempt to resolve a metric occurrence's class by matching its resolved
identity-bearing signature dimensions against existing classes before falling back to
`MetricDefinitionTermID`, keyword-concept alignment, or a name-derived hash, and SHALL require at
least one dimension resolved to a non-null term on both the occurrence and the candidate class to
count as a match.

#### Scenario: Zero shared resolved dimensions falls through, not a vacuous match
- **WHEN** a metric occurrence has no identity dimension resolved to a non-null term_id, or shares
  no such dimension with any existing class
- **THEN** the system SHALL NOT select any class by signature and SHALL proceed to the existing
  `MetricDefinitionTermID`/concept/name-hash resolution order unchanged

#### Scenario: A unique best-agreeing class is reused
- **WHEN** exactly one existing class shares the highest count of resolved, non-null, agreeing
  signature dimensions with the occurrence, and no shared resolved dimension disagrees
- **THEN** the system SHALL select that class as the occurrence's `instance_of_term_id` with identity
  state `resolved_existing`, without falling back to concept or name-hash resolution

#### Scenario: A signature-disagreeing class is never selected
- **WHEN** an existing class has a resolved, non-null value for a dimension the occurrence also
  resolved, and the two values disagree
- **THEN** the system SHALL exclude that class from signature-based selection entirely, regardless
  of how many other dimensions might otherwise agree

### Requirement: A tie among equally-agreeing candidates provisions a new class and records alternatives
The system SHALL NOT guess between two or more existing classes that tie on the highest count of
agreeing signature dimensions; it SHALL instead create or select a new provisional class through the
existing fallback path, mark the decision's identity state ambiguous, and record the tied classes as
alternatives on the resolution decision.

#### Scenario: Tied candidates never silently pick a winner
- **WHEN** two or more existing classes tie at the highest non-zero count of resolved, agreeing
  signature dimensions with an occurrence, and neither disagrees with the occurrence on any shared
  resolved dimension
- **THEN** the system SHALL provision a new class rather than select either tied candidate, and SHALL
  record both tied candidates as alternatives on the class-resolution decision

#### Scenario: Ambiguous signature match still yields a governed class, never a classless assertion
- **WHEN** a signature match is ambiguous per the scenario above
- **THEN** the resulting assertion SHALL still reference a real, queryable class term, consistent
  with the existing requirement that no safe match still provisions a class rather than leaving a
  metric classless
