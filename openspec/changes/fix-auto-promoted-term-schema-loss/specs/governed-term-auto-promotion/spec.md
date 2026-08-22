## MODIFIED Requirements

### Requirement: Auto-created term content is derived only from already-extracted structured fields
The payload of an auto-created `metric_definition` term SHALL be populated
from the triggering metric's own extracted fields and the concept's
label/aliases, never from a new LLM-authored definition.

#### Scenario: Metric with a populated metric_desc
- **WHEN** the triggering `kb.metrics` row has a non-empty `metric_desc`
- **THEN** the auto-created term's `definition` field is set to that value

#### Scenario: Metric with an empty metric_desc but a populated formula_or_definition
- **WHEN** the triggering `kb.metrics` row's `metric_desc` is empty and `formula_or_definition` is non-empty
- **THEN** the auto-created term's `definition` field is set to `formula_or_definition`

#### Scenario: Metric with neither field populated
- **WHEN** both `metric_desc` and `formula_or_definition` are empty on the triggering `kb.metrics` row
- **THEN** the auto-created term's `definition` field is left empty rather than fabricated

#### Scenario: Metric unit resolves against the released QUDT catalog
- **WHEN** the triggering `kb.metrics` row's `metric_unit` matches an existing released `unit` term under the `quantity` module
- **THEN** the auto-created term's `properties.permitted_unit_term_ids` references that released unit term, and `properties.raw_unit` holds the source `metric_unit` string

#### Scenario: Metric unit does not resolve against the released QUDT catalog
- **WHEN** the triggering `kb.metrics` row's `metric_unit` is non-empty but does not resolve to any released `unit` term
- **THEN** `properties.permitted_unit_term_ids` is left empty, but `properties.raw_unit` still holds the source `metric_unit` string rather than discarding it
