## ADDED Requirements

### Requirement: Concept with no governed alignment auto-creates a metric_definition term
WHEN a resolved `kb.keyword_concepts` row has no accepted `aligns_to_term`
assertion to a governed `metric_definition` term (whether `auto-promoted`
or `included_in_release`), resolving a metric against that concept SHALL
create a new `kb.ontology_terms` row (`term_kind='metric_definition'`,
`status='auto-promoted'`) and an accepted `aligns_to_term` assertion linking
the concept to it, with no human action required.

#### Scenario: First metric for a brand-new concept
- **WHEN** a metric's name resolves (auto-creating if needed) to a `kb.keyword_concepts` row that has never been aligned to any governed term
- **THEN** a new `kb.ontology_terms` row is created with `status='auto-promoted'` and `term_kind='metric_definition'`, and `kb.metrics.metric_definition_term_id` on the triggering row is set to its `term_id`

#### Scenario: Second metric for the same concept reuses the existing auto-promoted term
- **WHEN** a later metric resolves to a concept that already has an accepted `aligns_to_term` assertion (whether to an `auto-promoted` or `included_in_release` term)
- **THEN** no new term is created; the existing term's id is reused

#### Scenario: Term and alignment are created atomically
- **WHEN** the term-insert step of auto-creation fails after the alignment step would otherwise have run
- **THEN** neither the term row nor the alignment assertion is left committed (no orphaned alignment pointing at a nonexistent term, no orphaned term with no alignment)

### Requirement: Auto-created term content is derived only from already-extracted structured fields
The payload of an auto-created `metric_definition` term SHALL be populated
from the triggering metric's own extracted fields and the concept's
label/aliases, never from a new LLM-authored definition.

#### Scenario: Metric with a populated formula_or_definition
- **WHEN** the triggering `kb.metrics` row has a non-empty `formula_or_definition`
- **THEN** the auto-created term's `definition` field is set to that value

#### Scenario: Metric with no formula_or_definition
- **WHEN** the triggering `kb.metrics` row's `formula_or_definition` is empty
- **THEN** the auto-created term's `definition` field is left empty rather than fabricated

#### Scenario: Metric unit resolves against the released QUDT catalog
- **WHEN** the triggering `kb.metrics` row's `metric_unit` matches an existing released `unit` term under the `quantity` module
- **THEN** the auto-created term's `permitted_units` references that released unit term

### Requirement: Auto-promoted terms are valid comparison-matrix row keys
`ComparisonStore.validateMetricKey` SHALL accept a `metric_key` referencing
a `kb.ontology_terms` row with `term_kind='metric_definition'` and `status`
in `('included_in_release', 'auto-promoted')`.

#### Scenario: Comparison scope references an auto-promoted term
- **WHEN** a comparison scope or cell is created with a `metric_key` equal to an `auto-promoted` term's `term_id`
- **THEN** the create call succeeds (previously only `included_in_release` was accepted)

#### Scenario: Comparison scope references a term that is neither released nor auto-promoted
- **WHEN** a comparison scope or cell is created with a `metric_key` equal to a `draft`/`in_review`/`approved`/`rejected`/`superseded` term's `term_id`
- **THEN** the create call is still rejected (unchanged behavior)

### Requirement: extract_metric_definitions is not selected by default
The default routed-processor selection SHALL no longer include
`extract_metric_definitions`; its code, tests, and prompt remain present in
the repository and it remains selectable via an explicit `operation` /
Dev Mode request.

#### Scenario: Default pipeline run
- **WHEN** a document is processed under the default (unspecified `operation`) pipeline
- **THEN** `extract_metric_definitions` does not run and no `kb.inputs.status` entry for it is written

#### Scenario: Explicit request still works
- **WHEN** a caller explicitly requests `operation: ["extract_metric_definitions"]` (or the Dev Mode equivalent)
- **THEN** the processor still runs and behaves exactly as before this change
