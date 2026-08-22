## ADDED Requirements

### Requirement: Identity-bearing properties distinguish unresolved from not-applicable
For every artifact-type field configured as `identity = true`, the system SHALL write its resolved
signature entry onto `kb.ontology_terms.properties` as `{"raw": <value>, "term_id": <term_id-or-
null>}`, and SHALL omit the key entirely for a field not configured for the occurrence's artifact
type.

#### Scenario: Unresolved identity field is distinguishable from an unconfigured one
- **WHEN** an identity-bearing field is configured for an artifact type but its raw value has not
  yet resolved to a governed term
- **THEN** its `properties` entry SHALL have `term_id: null`, distinct from a field key that is
  absent because it is not configured at all

#### Scenario: Non-identity fields keep their existing plain value shape
- **WHEN** a configured field has `identity = false` or omits `identity`
- **THEN** the system SHALL write it to `properties` as a plain value, unchanged from prior behavior

### Requirement: Signature composition is configured per artifact type via a typed field list
The system SHALL read which fields participate in the class-identity signature, and which governed
dimension each resolves against, from operator configuration scoped per artifact type, rather than
from a hardcoded field list.

#### Scenario: A field with no configured entry is never treated as identity-bearing
- **WHEN** an artifact type has no configuration entry for a given field
- **THEN** that field SHALL NOT participate in class-identity signature matching

#### Scenario: Misconfiguration is surfaced as a warning, not silently ignored
- **WHEN** an artifact type known to require signature-based resolution has zero `identity = true`
  configured entries
- **THEN** the system SHALL log a warning at configuration load time without refusing to start

### Requirement: Occurrence value and context fields are one property bag regardless of storage column
The system SHALL NOT rely on `object_literal` versus `qualifiers` as a meaningful functional
distinction for occurrence fields; any code that reads occurrence properties SHALL treat both as one
combined bag.

#### Scenario: A field's presence in either column is equally readable
- **WHEN** an occurrence field is written to either `object_literal` or `qualifiers`
- **THEN** a reader of the occurrence's properties SHALL be able to find it without depending on
  which column it was written to
