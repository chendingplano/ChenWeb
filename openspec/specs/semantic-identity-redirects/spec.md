# semantic-identity-redirects Specification

## Purpose
TBD - created by archiving change canonical-metric-class-foundations. Update Purpose after archive.
## Requirements
### Requirement: Redirect histories are acyclic and single-target
The system SHALL persist term and assertion redirects append-only, permit at most one active target per source, and reject cycles before commit.

#### Scenario: Cycle is rejected
- **WHEN** a proposed redirect would make a source reachable from itself
- **THEN** the transaction SHALL fail without activating the redirect

### Requirement: Redirect reads are bounded and explainable
Redirect resolution SHALL return the terminal target and traversal history within a configured depth cap, or an explicit unresolved reason.

#### Scenario: Redirect depth cap is reached
- **WHEN** resolution exceeds its configured depth cap
- **THEN** the caller SHALL receive an explicit unresolved result and no inferred target
