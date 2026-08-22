## ADDED Requirements

### Requirement: associate_semantics is the sole trigger for metric class creation
`kb.ontology_terms` `metric_definition` class terms SHALL be created or
selected only when `associate_semantics` processes a metric occurrence into a
represented or accepted `kb.semantic_assertions` row, never during
`extract_metrics`'s pre-normalize extraction stage.

#### Scenario: Extraction alone does not create a class
- **WHEN** `extract_metrics` runs against an input record and no metric occurrence has yet reached `associate_semantics`
- **THEN** no new `metric_definition` term is created for concepts first observed in that record

#### Scenario: An accepted metric occurrence creates or selects its class
- **WHEN** `associate_semantics` processes a metric candidate through to a represented or accepted assertion
- **THEN** a `metric_definition` class term is created (if none exists for the resolved identity) or selected (if one does) as part of that same processing step

### Requirement: Class synthesis always produces a catalog-visible term
Every class `associate_semantics` creates or selects SHALL be a real
`kb.ontology_terms` row, resolvable through `kb.ontology_terms_current` the
same way any other governed term is — including a provisional class for a
concept never seen before.

#### Scenario: A brand-new concept's class is visible in the term catalog
- **WHEN** `associate_semantics` synthesizes a class for a metric concept that has no prior `metric_definition_term_id`
- **THEN** `kb.ontology_terms_current` contains a row for the synthesized class's `term_id`

### Requirement: Class synthesis preserves the keyword-concept alignment link when available
Class synthesis SHALL reuse the existing keyword-to-term alignment mechanism
(`core:aligns_to_term`) to link the synthesized or selected term to a metric
occurrence's `keyword_concept_id` when one is present, exactly as
auto-promotion linked it before this change, and SHALL still create the term
with no alignment link when no `keyword_concept_id` is available.

#### Scenario: Metric occurrence with a resolved concept
- **WHEN** a metric occurrence's candidate payload carries a `keyword_concept_id`
- **THEN** the synthesized or selected class term is linked to that concept via `core:aligns_to_term`, and the link is idempotent across repeated occurrences of the same concept

#### Scenario: Metric occurrence with no resolved concept
- **WHEN** a metric occurrence's candidate payload carries no `keyword_concept_id`
- **THEN** class synthesis still creates or selects a term, with no alignment assertion written

### Requirement: Class synthesis runs inside the caller's transaction
The class synthesis call SHALL accept and use the caller's existing database
transaction rather than opening a new one, so a metric's assertion write and
its class creation commit or roll back together.

#### Scenario: A failure after class synthesis rolls back the class too
- **WHEN** class synthesis succeeds but a later step in the same metric-write transaction fails
- **THEN** the synthesized class is not persisted, exactly as the rest of the transaction is not persisted
