## ADDED Requirements

### Requirement: kb.semantic_assertions.qualifiers is populated from a configured map
`kb.semantic_assertions.qualifiers` SHALL be populated from an operator-configured
`<artifact_type>:<table_field_name>:<property_name>` map (`[semantic_assertion_property_map]`
in `config.toml`/`config.local.toml`, the same shape as `[ontology_term_property_map]`),
not from a hardcoded field list. Absent or empty-valued mapped fields SHALL be
omitted rather than written as null or empty string.

#### Scenario: A configured instance-level field is present on the occurrence
- **WHEN** a metric occurrence's field map has a non-empty value for a field named in `[semantic_assertion_property_map]`
- **THEN** the resulting `kb.semantic_assertions.qualifiers` JSON object contains that value under the configured property name

#### Scenario: A configured field is empty on the occurrence
- **WHEN** a metric occurrence's field map has an empty or absent value for a field named in `[semantic_assertion_property_map]`
- **THEN** that property is omitted from `qualifiers` rather than written as an empty value

#### Scenario: No fields are configured
- **WHEN** `[semantic_assertion_property_map]` has no entries for an artifact type
- **THEN** `qualifiers` is left NULL for that assertion, unchanged from a producer that never populated it

### Requirement: instance_of_term_id is not duplicated into qualifiers
`kb.semantic_assertions.qualifiers` SHALL NOT carry the assertion's resolved
class term ID, since that identity is already held by
`kb.semantic_assertions.instance_of_term_id`.

#### Scenario: A metric's resolved class term is not repeated in qualifiers
- **WHEN** a metric assertion is created with a resolved `instance_of_term_id`
- **THEN** `qualifiers` contains no key holding that same term ID
