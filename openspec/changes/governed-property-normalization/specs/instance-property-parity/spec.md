## ADDED Requirements

### Requirement: An occurrence's qualifiers carry every property its resolved class carries, matching the class side's own resolution treatment
The system SHALL configure `[semantic_assertion_property_map]` in the same typed, per-artifact-type
entry format as `[ontology_term_property_map]`, so any field resolved on the class side can also be
configured on the instance side with the same treatment, and SHALL apply that configuration so
`kb.semantic_assertions.qualifiers` is never missing a property that `kb.ontology_terms.properties`
carries for the occurrence's resolved class.

#### Scenario: subject appears on both the class and the instance, resolved the same way
- **WHEN** a metric occurrence resolves `subject` onto its class's `properties`
- **THEN** the same occurrence's `qualifiers` SHALL also carry `subject` in the same
  `{"raw":..,"resolved":..}` shape, via a `[semantic_assertion_property_map]` entry with
  `normalize = true`

#### Scenario: value_type and range_type appear on both the class and the instance
- **WHEN** a metric occurrence's class carries `value_type`/`range_type` in `properties`
- **THEN** the same occurrence's `qualifiers` SHALL also carry `value_type`/`range_type`

#### Scenario: object_name appears on both the class and the instance, as a plain value on both
- **WHEN** a metric occurrence's class carries a plain (non-normalized) `object_name` in `properties`
- **THEN** the same occurrence's `qualifiers` SHALL also carry `object_name` as a plain value, not
  wrapped in a resolved shape

### Requirement: Class-side and instance-side configuration share one entry format and one builder
The system SHALL use one shared configuration entry type and one shared property-building function
for both `[ontology_term_property_map]` and `[semantic_assertion_property_map]`, rather than
maintaining two separately-shaped config formats and multiple near-duplicate builder functions. The
builder function itself SHALL remain agnostic to how a resolved value was produced, accepting
already-computed resolved values rather than performing resolution itself.

#### Scenario: A single builder function serves both config sections
- **WHEN** either `kb.ontology_terms.properties` or `kb.semantic_assertions.qualifiers` is built from
  configuration
- **THEN** the same underlying property-building function SHALL be used for both, taking the
  configured entries, the occurrence field map, and a map of already-resolved values as input
