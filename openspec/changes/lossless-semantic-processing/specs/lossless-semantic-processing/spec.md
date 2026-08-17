## ADDED Requirements

### Requirement: Every identifiable artifact produces a semantic result

For every source-backed artifact that extraction can identify, the system SHALL produce exactly one
of: a normalized semantic instance with source evidence; a raw-preserved semantic instance with an
explicit unresolved, malformed, missing, ambiguous, or nonconforming state and source evidence; or,
when the artifact family cannot yet instantiate, a durable unresolved semantic occurrence and outcome
record that remains eligible for later materialization. The system SHALL NOT silently skip, delete, or
make an artifact unreachable because semantic normalization is incomplete.

#### Scenario: Normalization succeeds
- **WHEN** a processor normalizes an identifiable artifact and all required capabilities produce usable output
- **THEN** a normalized semantic instance exists with source evidence and a `semantic:normalized` outcome disposition

#### Scenario: Normalization is incomplete for an instantiable family
- **WHEN** a processor cannot fully normalize an artifact belonging to a family modeled as ontology object instances
- **THEN** a raw-preserved semantic instance is created with the explicit non-success state and source evidence
- **AND** the system does not fall back to an unresolved semantic occurrence

#### Scenario: Family cannot yet instantiate
- **WHEN** an identifiable artifact belongs to a family with no compliant semantic-instance adapter
- **THEN** exactly one active unresolved semantic occurrence and one outcome envelope are persisted
- **AND** the artifact is discoverable through the generic semantic-discovery API

#### Scenario: Artifact is never dropped
- **WHEN** any content-level semantic problem occurs during processing
- **THEN** the artifact remains reachable from later semantic processing and from downstream consumers

### Requirement: Raw data is preserved independently of normalization

Raw source fields SHALL be treated as immutable processing inputs and normalized fields as derived
interpretations. Normalization SHALL NOT overwrite the raw representation. A failed parse SHALL
preserve the exact offending value and its declared or inferred datatype. A mapping decision SHALL
record both the raw input and the selected canonical value.

#### Scenario: Parse failure preserves the literal
- **WHEN** a value literal cannot be parsed
- **THEN** the exact raw literal and observed datatype are preserved and remain queryable

#### Scenario: Normalization does not mutate raw
- **WHEN** a processor writes a normalized value for an artifact
- **THEN** the artifact-family row's raw values, units, datatypes, labels, conditions, source wording, and line spans are unchanged

#### Scenario: Correction does not rewrite the source
- **WHEN** a human or autonomous correction changes an interpretation
- **THEN** a new decision is recorded and either a non-identity assertion revision or a different claim identity is created
- **AND** what the source originally expressed is unchanged

### Requirement: Ownership of raw content is unambiguous

The artifact-family row SHALL be authoritative for the current raw occurrence. An assertion's raw
payload SHALL be an immutable normalization-time snapshot recording the source occurrence
revision/fingerprint it was copied from. Evidence quotes and spans SHALL be provenance excerpts, not
authoritative copies. An outcome's `raw_fragment` SHALL be populated only when no identified artifact
row can preserve that content. Derived search and projection copies SHALL be rebuildable caches.

#### Scenario: Snapshot fingerprint matches the occurrence
- **WHEN** an assertion is created from a raw occurrence
- **THEN** the assertion's raw snapshot fingerprint equals the referenced source occurrence revision

#### Scenario: Reprocessing does not mutate history
- **WHEN** an artifact is reprocessed and its extraction changes
- **THEN** a new occurrence or superseding current link is created according to the family lifecycle
- **AND** no historical assertion snapshot is mutated to resemble the new extraction

#### Scenario: raw_fragment is only a last resort
- **WHEN** an identified artifact row can preserve the content
- **THEN** the outcome's `raw_fragment` is left empty

### Requirement: Canonical identity selects exactly one identity-value branch

The canonical serializer SHALL select exactly one of `normalized_value`, `raw_fingerprint`, `missing`,
or `not_applicable` as the identity-value branch. The branch SHALL be determined by whether an
authoritative parsed semantic value exists, expressed through the existing canonical payload shape,
and SHALL NOT be determined by the outcome disposition label.

#### Scenario: Parsed but unmapped value converges by normalized value
- **WHEN** a metric literal parses successfully but its range-type mapping is unresolved or proposed
- **THEN** the outcome disposition is `semantic:raw_preserved` because class-derived bucket fields are unavailable
- **AND** the claim identity still uses the `normalized_value` branch

#### Scenario: Unparsed value uses the raw fingerprint branch
- **WHEN** no authoritative parsed semantic value exists
- **THEN** canonical identity uses the family-governed raw-value fingerprint, observed datatype, explicit value state, and required semantic context

#### Scenario: Distinct unparsed values do not converge
- **WHEN** two artifacts carry different unparsed raw values
- **THEN** their canonical identities differ and they do not converge merely because they share an error state

#### Scenario: Normalized snapshot does not block convergence
- **WHEN** two occurrences with different source wording, spans, models, and prompts normalize to the same semantic value
- **THEN** their canonical identities are equal and they converge on one claim
