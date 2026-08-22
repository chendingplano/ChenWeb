## ADDED Requirements

### Requirement: kb.ontology_terms carries a general-purpose kind-specific properties bag
`kb.ontology_terms` (and its append-only mirror `kb.ontology_term_revisions`,
exposed through `kb.ontology_terms_current`) SHALL provide a `properties
JSONB` column for structured data specific to a term's `term_kind`,
replacing the `value_type`, `range_type`, and `permitted_unit_term_ids`
flat columns. Producers that don't populate kind-specific properties SHALL
leave the column NULL, unchanged from how the replaced columns behaved for
the 7 term kinds that never used them.

#### Scenario: A term kind with no structured properties
- **WHEN** a `class`, `property`, `individual`, `concept`, `quantity_kind`, `unit`, or `dimension` term is created by any existing producer (seed content, QUDT import, candidate promotion)
- **THEN** `properties` is NULL on the created row, exactly as `value_type`/`range_type`/`permitted_unit_term_ids` were NULL for these kinds before this change

#### Scenario: metric_definition auto-promotion populates properties
- **WHEN** the auto-promotion path (`EnsureAcceptedOrCreate`) creates a `metric_definition` term
- **THEN** `properties` is a JSON object with keys `value_type`, `range_type`, `permitted_unit_term_ids`, and `raw_unit` drawn from the triggering metric's synthesis input, in place of the removed flat columns

### Requirement: The append-only revision mirror stays synchronized on properties
`kb.sync_ontology_term_revision_after_insert()` and
`kb.ontology_terms_current` SHALL carry `properties` through exactly as
they carry every other `kb.ontology_terms` column, so a revision reader
never sees a base-table row whose `properties` isn't reflected in the
current-revision view.

#### Scenario: A new term version is inserted
- **WHEN** a new row is inserted into `kb.ontology_terms` with a non-null `properties` value
- **THEN** the trigger-synced row in `kb.ontology_term_revisions` and the corresponding `kb.ontology_terms_current` row expose the same `properties` value
