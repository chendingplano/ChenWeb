## ADDED Requirements

### Requirement: Metrics API returns governed identifier fields
`GET /api/v1/kb/metrics` (list) and the single-metric fetch used by the metric detail view SHALL include `metric_id`, `keyword_concept_id`, `metric_definition_term_id`, and `value_range_type_error` for every `kb.metrics` row, reading directly from the like-named columns on `kb.metrics`, omitted from the JSON response when the underlying column value is `NULL`.

#### Scenario: Metric with all four identifiers set
- **WHEN** a `kb.metrics` row has non-null `metric_id`, `keyword_concept_id`,
  `metric_definition_term_id`, and `value_range_type_error`
- **THEN** the API response for that metric includes all four fields with their
  column values

#### Scenario: Metric with no governed identifiers set
- **WHEN** a `kb.metrics` row has `NULL` `keyword_concept_id`,
  `metric_definition_term_id`, and `value_range_type_error`
- **THEN** the API response for that metric omits those three fields (consistent with
  `omitempty` handling of every other nullable field on this response)

### Requirement: Metadata block displays governed identifiers
The Metrics page's per-metric "Metadata" attribute group SHALL display `metric_id`, `keyword_concept_id`, `metric_definition_term_id`, and `value_range_type_error` as text attributes, each shown only when the corresponding API field is present and non-empty, alongside the group's existing attributes (ID, Document ID, Name, Confidence, Desc, Formula, Explicit).

#### Scenario: Selecting a metric with governed identifiers
- **WHEN** a user selects a metric whose API record has `metric_id`,
  `keyword_concept_id`, `metric_definition_term_id`, and `value_range_type_error` all
  present
- **THEN** the Metadata group renders four additional attribute entries showing those
  values, alongside the existing ID/Document ID/Name/Confidence/Desc/Formula/Explicit
  entries

#### Scenario: Selecting a metric without governed identifiers
- **WHEN** a user selects a metric whose API record has `keyword_concept_id`,
  `metric_definition_term_id`, and `value_range_type_error` absent
- **THEN** the Metadata group renders no entry for those three fields (matching how
  every other absent/empty attribute in the group is already hidden), while still
  showing `metric_id` if present
