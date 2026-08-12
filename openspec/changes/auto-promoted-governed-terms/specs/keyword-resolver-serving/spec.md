## ADDED Requirements

### Requirement: Resolver mode serves resolved identifiers to metric persistence
When `KEYWORD_RESOLVER_MODE` is `observe` or `on`, resolving a metric name
through `names.Resolver` SHALL return a usable `ConceptID` (and `TermID`
when an alignment exists) to the calling `ResolvingMetricsStore`, and that
value SHALL be persisted on the corresponding `kb.metrics` row.

#### Scenario: Resolver mode active, metric name matches an existing concept
- **WHEN** `KEYWORD_RESOLVER_MODE=observe` (or `on`) and `extract_metrics` persists a metric whose name resolves to an existing `kb.keyword_concepts` row
- **THEN** the saved `kb.metrics` row's `keyword_concept_id` is non-null and equals that concept's id

#### Scenario: Resolver mode off (default), no resolution occurs
- **WHEN** `KEYWORD_RESOLVER_MODE` is unset or `off`
- **THEN** `kb.metrics.keyword_concept_id` and `.metric_definition_term_id` remain null, unchanged from current behavior

### Requirement: Consumer-facing resolution behavior is verified against a live database before being relied upon
The `KEYWORD_RESOLVER_MODE` value used in any environment that the
`governed-term-auto-promotion` capability depends on SHALL be verified
against a real (non-mock) database run, not assumed from documentation —
spec `2026080403`'s own characterization of `"on"` mode ("K7", unusable)
could not be reproduced by reading `names.Resolver`'s current code and must
not be taken as still-accurate without a live check.

#### Scenario: Pre-rollout verification
- **WHEN** this change is deployed to any environment where `governed-term-auto-promotion` is expected to run
- **THEN** a real run against that environment's database has confirmed `kb.metrics.keyword_concept_id` populates for at least one previously-unresolved metric before the environment is relied upon for auto-promotion
