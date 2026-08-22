## MODIFIED Requirements

### Requirement: No safe match creates a provisional class
The system SHALL create or select a provisional stable class when no existing class safely matches a source-backed metric, rather than discard the metric or attach it classlessly. The provisional class SHALL be a real `kb.ontology_terms` row, visible through `kb.ontology_terms_current`, not merely an entry in `kb.ontology_term_headers` with no corresponding term row.

#### Scenario: New metric receives provisional class
- **WHEN** deterministic resolution finds no safe existing class
- **THEN** a provisional class and recorded resolution decision SHALL be available for the metric

#### Scenario: Provisional class is visible through the standard term catalog
- **WHEN** a provisional class is created because no existing class safely matched
- **THEN** `kb.ontology_terms_current` SHALL contain a row for that class's `term_id`, resolvable the same way any other governed term is
