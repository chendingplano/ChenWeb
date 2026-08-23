## MODIFIED Requirements

### Requirement: Identity-bearing properties carry a resolved value when normalized
For every artifact-type field configured with a non-empty `normalize` method, the system SHALL write
its signature entry onto `kb.ontology_terms.properties` as `{"raw": <value>, "resolved": <resolved
value>}`, and SHALL omit the key entirely for a field with no `normalize` method configured for the
occurrence's artifact type, or whose raw value is empty. The resolved value's source depends on the
configured method (see the `governed-property-normalization-methods` capability), but the shape
written to `properties` is uniform across every method, and `matchClassBySignature` reads it
identically regardless of which one produced it.

#### Scenario: A resolved field with an authoritative mapping carries a non-empty resolved value
- **WHEN** a field is configured with a non-empty `normalize` method, its raw value is non-empty, and
  its resolution mechanism has an authoritative answer
- **THEN** its `properties` entry SHALL have a non-empty `resolved` value

#### Scenario: A field awaiting curator triage carries an empty resolved value
- **WHEN** a field's configured method resolves via a curated bucket map (`system`), and its raw
  value's mapping row is not yet approved
- **THEN** its `properties` entry SHALL still be present with the `raw` value preserved, but
  `resolved` SHALL be empty

#### Scenario: An unconfigured or empty field is omitted, not written as null
- **WHEN** a field has no `normalize` method configured for the occurrence's artifact type, or its raw
  value is empty
- **THEN** the system SHALL omit that property key entirely rather than writing a null placeholder

#### Scenario: Fields with no normalize method keep their existing plain value shape
- **WHEN** a configured field has `normalize` unset (empty)
- **THEN** the system SHALL write it to `properties` as a plain value, unchanged from prior behavior

### Requirement: Signature composition is configured per artifact type via a typed field list
The system SHALL read which fields participate in the class-identity signature (`identity`) and which
fields carry a resolved value, and by which named method (`normalize`), from operator configuration
scoped per artifact type, rather than from a hardcoded field list. `identity` and `normalize` are
independent: a field may be configured with either, both, or neither.

#### Scenario: A field with no configured entry is never treated as identity-bearing
- **WHEN** an artifact type has no configuration entry for a given field
- **THEN** that field SHALL NOT participate in class-identity signature matching

#### Scenario: A field can be resolved without being identity-bearing
- **WHEN** a field is configured with `identity = false` (or omitted) and a non-empty `normalize`
  method
- **THEN** the system SHALL resolve its raw value in `{"raw":..,"resolved":..}` shape without
  including it in class-identity signature matching

#### Scenario: A field can be identity-bearing without being resolved
- **WHEN** a field is configured with `identity = true` and `normalize` unset
- **THEN** the system SHALL write it in plain-value shape, and it SHALL never contribute a resolved
  dimension to signature matching (only a non-empty `resolved` key can be compared for agreement)

#### Scenario: Misconfiguration is surfaced as a warning, not silently ignored
- **WHEN** an artifact type known to require signature-based resolution has zero `identity = true`
  configured entries
- **THEN** the system SHALL log a warning at configuration load time without refusing to start

## ADDED Requirements

### Requirement: metric_name and subject resolve via the strong method (keyword-concept identity)
The system SHALL resolve `metric_name` and `subject` using the `strong` method (`names.Resolver`'s
full tier ladder), each under its own dedicated scope so distinct fields never collide on the same raw
string.

#### Scenario: metric_name reuses its already-resolved concept id
- **WHEN** a metric occurrence's class-identity signature includes `metric_name`
- **THEN** the system SHALL use the concept id already resolved for that metric name
  (`kb.metrics.keyword_concept_id`) as its `resolved` value, without a new resolution call

#### Scenario: subject resolves via a dedicated keyword scope
- **WHEN** a metric occurrence's class-identity signature includes `subject`
- **THEN** the system SHALL resolve it via the `strong` method under a scope dedicated to the subject
  field, distinct from the scope used for `metric_name` or any other field

### Requirement: value_class resolves via the simple method
The system SHALL resolve `value_class` using the `simple` method (deterministic string normalization
only), based on a corpus check finding no synonym clusters that would justify a curated map.

#### Scenario: value_class never triggers a table lookup or keyword-concept resolution
- **WHEN** a metric occurrence's class-identity signature includes `value_class`
- **THEN** the system SHALL resolve it via deterministic string normalization only, with no database
  interaction and no keyword-concept identity involved

### Requirement: value_type and range_type resolve via the system method, each through its own curated bucket map
The system SHALL resolve `value_type` via a new curated bucket map (`kb.metric_value_bucket_map`,
dimension `value_type`) and `range_type` via its existing, unmodified curated bucket map
(`kb.metric_value_range_type_map`) — both `system`-method fields, neither using keyword-concept
resolution or term minting.

#### Scenario: range_type's resolved value comes from the existing mapper, not a new lookup
- **WHEN** a metric occurrence's class-identity signature includes `range_type`
- **THEN** the system SHALL use the `canonicalBucket` already produced by
  `ValueRangeTypeMapper.Lookup` for that occurrence as its `resolved` value

#### Scenario: value_type resolves via the new bucket map, never keyword-concept resolution
- **WHEN** a metric occurrence's class-identity signature includes `value_type`
- **THEN** the system SHALL resolve it via `kb.metric_value_bucket_map` only, and SHALL NOT create,
  observe, or otherwise touch any keyword-concept identity or governed ontology term for it

### Requirement: object_name is excluded from identity resolution
The system SHALL NOT configure `object_name` with any `normalize` method, and SHALL NOT apply any
resolution mechanism to it, because it is sourced from a separate, pre-existing object-identity system
(`kb.artifact_objects`/`kb.object_nodes`) that this change does not modify.

#### Scenario: object_name stays a plain, unresolved signature entry
- **WHEN** a metric occurrence's class-identity signature includes `object_name`
- **THEN** the system SHALL write it as a plain value (not `{"raw":..,"resolved":..}`), and it SHALL
  NOT contribute a resolved dimension to signature matching
