## ADDED Requirements

### Requirement: normalize is a named method, not a boolean
The system SHALL configure each property-map entry's normalization behavior as a named method string
(`normalize`) rather than a boolean, chosen from a fixed set: `""` (unset — no normalization),
`"system"` (a field-specific, hand-built resolution mechanism), `"simple"` (deterministic string
normalization only, no database interaction), `"moderate"` (table-backed matching without fuzzy
scoring), `"strong"` (the full keyword-concept identity resolution ladder, including fuzzy matching
and auto-creation on a miss).

#### Scenario: An entry's method name documents how it resolves
- **WHEN** an operator reads a `[[ontology_term_property_map.*]]` or
  `[[semantic_assertion_property_map.*]]` entry's `normalize` value
- **THEN** that value alone SHALL indicate which resolution mechanism applies to the field, without
  needing to read the implementation to know

### Requirement: Configuration fails loudly on an unrecognized or unimplemented method
The system SHALL reject configuration load, with an error identifying the offending field and value,
when any entry's `normalize` value is not one of the recognized method names, or is a recognized
method name with no implementation behind it.

#### Scenario: A typo'd method name fails config load
- **WHEN** an entry's `normalize` value is not `""`, `"system"`, `"simple"`, `"moderate"`, or
  `"strong"`
- **THEN** configuration load SHALL fail with an error naming the field and the invalid value

#### Scenario: moderate is recognized but rejected until implemented
- **WHEN** an entry's `normalize` value is `"moderate"`
- **THEN** configuration load SHALL fail with an error stating that `moderate` is a recognized method
  with no implementation yet, distinct from the error for a genuinely unrecognized value

### Requirement: The property-building function is agnostic to which method resolved a value
The system SHALL build `{"raw":..,"resolved":..}` shaped properties by checking only whether a field's
`normalize` value is non-empty, using an already-computed resolved value supplied by the caller — the
shared builder function SHALL NOT contain method-specific logic for `system`, `simple`, `moderate`, or
`strong`.

#### Scenario: Adding a future method requires no change to the shared builder
- **WHEN** a new field is configured with an existing method name and its resolved value is supplied
  via the caller's resolved-value map
- **THEN** the shared property-building function SHALL wrap it correctly without any change to that
  function itself
