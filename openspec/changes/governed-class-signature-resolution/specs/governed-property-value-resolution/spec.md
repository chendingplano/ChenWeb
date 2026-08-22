## ADDED Requirements

### Requirement: Every dimension resolves through one generic table, never discarding the raw value
The system SHALL resolve a raw occurrence field value against a governed catalog per named
dimension via `kb.governed_property_value_map`, keyed by `(dimension, normalized_raw_value)`, and
SHALL preserve the original raw value regardless of whether resolution succeeds.

#### Scenario: Unrecognized raw value is proposed, not dropped
- **WHEN** a dimension's normalized raw value has no existing `kb.governed_property_value_map` row
- **THEN** the system SHALL insert a `status='proposed'` row for that `(dimension, raw_value)` and
  the raw value SHALL still be written to the occurrence's signature entry

#### Scenario: Approved mapping resolves to its governed term
- **WHEN** a dimension's normalized raw value has an existing `status='approved'` row with a
  non-null `term_id`
- **THEN** lookup SHALL return that `term_id` as authoritative for the occurrence

#### Scenario: Non-approved status never returns a usable term_id
- **WHEN** a dimension's normalized raw value resolves to a `status` of `proposed`, `ambiguous`, or
  `rejected`
- **THEN** lookup SHALL return no `term_id`, even if a best-effort `term_id` value exists on the row

### Requirement: Repeated observation of the same raw value is idempotent per dimension
The system SHALL increment `occurrence_count` and update `last_seen_record_id` for a
`(dimension, raw_value)` pair on each distinct resolution call, without creating duplicate rows.

#### Scenario: Same raw value seen twice increments once each time
- **WHEN** the same `(dimension, raw_value)` pair is resolved for two different source records
- **THEN** the system SHALL update the single existing row's `occurrence_count` and
  `last_seen_record_id` rather than inserting a second row

### Requirement: The resolution mechanism assigns no term_kind and mints no governed terms
The system SHALL NOT create, select, or otherwise decide which `term_kind` an approved dimension
value's `term_id` belongs to; approving a `proposed` row into a governed `term_id` SHALL remain an
external (curator or administrative) action against the table.

#### Scenario: Resolver code performs no term-kind selection
- **WHEN** a dimension value is resolved, proposed, or approved
- **THEN** no code path in the resolution mechanism SHALL construct, infer, or persist a `term_kind`
  on behalf of the approval
