# semantic-assertion-lifecycle Specification

## Purpose
TBD - created by archiving change canonical-metric-class-foundations. Update Purpose after archive.
## Requirements
### Requirement: Assertions preserve independent lifecycle and class identity
Semantic assertions SHALL preserve their lifecycle status and independent class identity, mapping-resolution, value, and conformance states. A `represented` assertion is source-admitted but not accepted, and an instance-to-class reference SHALL not imply governance endorsement.

#### Scenario: Represented class-resolved assertion remains unendorsed
- **WHEN** a source-backed assertion has status `represented` and a resolved stable class reference
- **THEN** governance and accepted-only profile consumers SHALL not treat it as accepted
