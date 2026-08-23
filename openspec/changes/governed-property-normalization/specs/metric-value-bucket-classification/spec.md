## ADDED Requirements

### Requirement: A generic resolve-or-propose bucket map serves closed-vocabulary classification dimensions
The system SHALL resolve a raw classification value against a curated map per named dimension via
`kb.metric_value_bucket_map`, keyed by `(dimension, raw_value)`, structured identically to the
existing `kb.metric_value_range_type_map` pattern (a plain canonical string, not a governed term
reference), and SHALL preserve the original raw value regardless of whether resolution succeeds. Only
`value_type` uses this table today; the schema is dimension-keyed so a future field with a similar
synonym-cluster problem can be added without a schema change.

#### Scenario: Unrecognized raw value is proposed, not dropped
- **WHEN** a dimension's normalized raw value has no existing `kb.metric_value_bucket_map` row
- **THEN** the system SHALL insert a `status='proposed'` row for that `(dimension, raw_value)` and the
  raw value SHALL still be available to the caller

#### Scenario: Approved mapping resolves to its canonical bucket
- **WHEN** a dimension's normalized raw value has an existing `status='approved'` row with a non-null
  `canonical_bucket`
- **THEN** lookup SHALL return that `canonical_bucket` as authoritative

#### Scenario: Non-approved status never returns a usable canonical bucket
- **WHEN** a dimension's normalized raw value resolves to a `status` of `proposed` or `ambiguous`
- **THEN** lookup SHALL return no canonical bucket, matching `kb.metric_value_range_type_map`'s
  existing "only approved is authoritative" contract exactly

### Requirement: This mechanism never assigns a governed term or touches the keyword-concept system
The system SHALL NOT create, select, or persist a governed ontology term, nor create or observe any
keyword-concept identity, as part of resolving a `kb.metric_value_bucket_map` dimension.

#### Scenario: Resolution stays a plain string, never a term or concept
- **WHEN** a `kb.metric_value_bucket_map` dimension value is resolved, proposed, or approved
- **THEN** no code path in this mechanism SHALL construct, infer, or persist a `term_id` or
  `concept_id` on behalf of it

### Requirement: kb.metric_value_range_type_map remains a separate, unmodified table
The system SHALL NOT migrate, merge, or otherwise alter `kb.metric_value_range_type_map` or its
existing consumers as part of introducing `kb.metric_value_bucket_map`.

#### Scenario: range_type continues resolving through its own existing table
- **WHEN** `range_type` is resolved for a metric occurrence
- **THEN** the system SHALL use `kb.metric_value_range_type_map` / `ValueRangeTypeMapper` exactly as
  it exists today, not `kb.metric_value_bucket_map`

### Requirement: value_class does not use this table
The system SHALL NOT create a `kb.metric_value_bucket_map` row or dimension for `value_class`, per the
corpus finding that its raw-value vocabulary is already clean and does not need curation.

#### Scenario: value_class resolves without any bucket-map involvement
- **WHEN** `value_class` is resolved for a metric occurrence
- **THEN** the system SHALL use the `simple` method (deterministic string normalization) only, never
  `kb.metric_value_bucket_map`
