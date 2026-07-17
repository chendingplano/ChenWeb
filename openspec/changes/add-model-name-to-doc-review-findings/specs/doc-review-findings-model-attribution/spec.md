## ADDED Requirements

### Requirement: Findings record the generating model
Every finding persisted to `kb.doc_review_findings` SHALL carry the name of the LLM model that generated it, stored as `model_name` in the `metadata` JSONB column, whenever the reviewer resolved a model name for the call that produced the finding.

#### Scenario: Chunk-based text reviewer finding
- **WHEN** a chunk-based text reviewer (e.g. `legal_compliance`, `regulatory_compliance`) produces findings via its LLM call and `ReviewerConfig.ModelName` is resolved for that call
- **THEN** each persisted finding's `metadata.model_name` equals `ReviewerConfig.ModelName`

#### Scenario: Artifact reviewer finding
- **WHEN** an artifact reviewer (`metrics`, `provisions`, `inventory_items`, `entities`, or `metrics_completeness`) produces a finding via its LLM call and `ReviewerConfig.ModelName` is resolved for that call
- **THEN** the persisted finding's `metadata.model_name` equals `ReviewerConfig.ModelName`

#### Scenario: Model name surfaced on the API response
- **WHEN** a client fetches findings through the doc-review findings API
- **THEN** each `FindingItem` in the response includes `model_name` matching the value stored in `metadata.model_name`, without the client needing to parse `metadata`

#### Scenario: Historical findings without a model name
- **WHEN** a finding row was persisted before this change and has no `model_name` key in `metadata`
- **THEN** the API SHALL return that finding with an empty/absent `model_name` rather than erroring
