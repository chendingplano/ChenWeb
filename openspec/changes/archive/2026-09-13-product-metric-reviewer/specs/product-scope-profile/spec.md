## ADDED Requirements

### Requirement: Product scope profile model

The system SHALL persist a product scope profile as a versioned tree of scope nodes. A profile
SHALL carry a name, an optional product description, a `version` integer, a `status` of `draft` or
`ready`, and a tenant scope. Each scope node SHALL carry: `node_kind` ∈ {`product`, `module`,
`part`, `aspect`}, a `parent_node_id` (null only for the `product` root and for `aspect` nodes
attached to the root), `label`, `label_en`, `aliases`, `depth`, `origin` ∈ {`llm_proposed`,
`graph_expanded`, `user_added`}, `status` ∈ {`proposed`, `accepted`, `rejected`}, a `confidence`,
a `rationale`, and its grounding refs `object_id`, `concept_id`, `term_id` with a `grounding` state
∈ {`object_node`, `keyword_concept`, `ontology_term`, `ungrounded`}.

A profile SHALL have exactly one node of kind `product`. A profile SHALL NOT contain a cycle.

#### Scenario: Profile created from a product name

- **WHEN** a user submits `{"product_name": "Ventilator"}` to create a profile
- **THEN** the system creates a profile at `version` 1 with `status` `draft`
- **AND** creates exactly one node of kind `product` whose label is `Ventilator`, `depth` 0, and
  `status` `accepted`

#### Scenario: Cycle is rejected

- **WHEN** a node edit would make a node its own ancestor
- **THEN** the system rejects the edit with error code `CWB_KB_PMR_010` and leaves the profile
  unchanged

### Requirement: Decomposition proposal pass

The system SHALL propose a product decomposition with a single LLM call taking the product name,
the optional product description, and optional seed document ids. The prompt SHALL be loaded from
a file under `ChenWeb/prompts/` and SHALL NOT be hard-coded in Go. Every proposed node SHALL be
persisted with `origin` `llm_proposed`, `status` `proposed`, and the model's rationale and
confidence. The pass SHALL respect the configured maximum depth and maximum node count, discarding
proposed nodes beyond the budget and recording the truncation on the profile.

#### Scenario: Proposal produces a multi-level tree

- **WHEN** the proposal pass runs for `Ventilator`
- **THEN** nodes such as `Display`, `Breathing Circuit`, and `Battery` exist with `node_kind`
  `module` or `part`, `origin` `llm_proposed`, and `status` `proposed`
- **AND** a part of a part such as `Display Backlight` exists at `depth` 2 with its parent set to
  `Display`

#### Scenario: Node budget exceeded

- **WHEN** the model returns more nodes than the configured node budget
- **THEN** the system persists nodes up to the budget in breadth-first order
- **AND** records `truncated = true` and the discarded count on the profile

#### Scenario: Proposal call fails

- **WHEN** the LLM call errors or returns unparseable output
- **THEN** the profile remains at its prior version with `status` `draft`
- **AND** the failure is returned as `CWB_KB_PMR_011` and logged with the profile id

### Requirement: Grounding pass

The system SHALL attempt to ground every node against, in order, `kb.object_nodes` (exact or alias
match on `normalized_names`, then embedding similarity at or above the configured threshold),
`kb.keyword_concepts` with `scope = 'metric_subject'`, and `kb.ontology_terms` labels. Resolution
SHALL stop at the first source that produces a match at or above threshold, and the matching source
SHALL be recorded as the node's `grounding` state. A node that matches nothing SHALL be persisted
with `grounding` `ungrounded` and SHALL remain in the tree.

Where a node grounds to a `kb.object_nodes` row whose `reconcile_status` is `ambiguous` or
`pending_review`, the system SHALL record that state on the node and SHALL NOT choose a sense on
the user's behalf.

#### Scenario: Node grounds to a reconciled object

- **WHEN** the grounding pass processes a node labelled `Display` and an active `kb.object_nodes`
  row has `Display` among its `normalized_names`
- **THEN** the node's `object_id` is set to that row's `object_id` and its `grounding` is
  `object_node`

#### Scenario: Node grounds to a metric-subject concept

- **WHEN** no object node matches a node labelled `Airway Pressure` but a `kb.keyword_concepts` row
  in scope `metric_subject` has it as `pref_label`
- **THEN** the node's `concept_id` is set to that concept and its `grounding` is `keyword_concept`

#### Scenario: Node cannot be grounded

- **WHEN** no source matches a node's label at or above threshold
- **THEN** the node is persisted with `grounding` `ungrounded` and no grounding refs
- **AND** the node remains eligible for lexical matching during retrieval

#### Scenario: Grounded object is ambiguous

- **WHEN** a node's best object match has `reconcile_status` `ambiguous`
- **THEN** the node records that reconcile state
- **AND** the node is returned to the UI flagged for reviewer attention

#### Scenario: Semantic search is disabled

- **WHEN** `kbsearch.SemanticSearchEnabled` is false
- **THEN** grounding uses lexical and alias matching only and completes without error

### Requirement: Graph expansion pass

For every node carrying an `object_id`, the system SHALL expand the tree using accepted
`kb.semantic_assertions` with `predicate_term_id` in {`core:part_of`, `core:component_of`} and
`status = 'accepted'`, and using `kb.products` rows with `relation_type` in {`component_of`,
`contains_product`} whose product identity resolves to that object. Discovered parts SHALL be
inserted with `origin` `graph_expanded`, `status` `proposed`, and a recorded reference to the
source edge or product row. Expansion SHALL respect the configured depth and node budget and SHALL
NOT create duplicate nodes for an object already present in the profile.

#### Scenario: Expansion adds a part the model did not name

- **WHEN** an accepted `core:part_of` assertion links object `obj:humidifier` to the object grounded
  by the profile's root
- **AND** no node in the profile is grounded to `obj:humidifier`
- **THEN** a node for it is inserted with `origin` `graph_expanded` and the assertion id recorded

#### Scenario: Expansion does not duplicate an existing node

- **WHEN** an expansion edge points at an object already grounded by a node in the profile
- **THEN** no new node is created
- **AND** the existing node records the additional supporting edge

#### Scenario: No graph edges exist

- **WHEN** the expansion pass finds no accepted structural assertions and no `component_of` product
  rows for any grounded node
- **THEN** the pass completes successfully and adds no nodes

### Requirement: Aspect nodes from configured vocabulary

The system SHALL attach `aspect` nodes to the profile root from the aspect vocabulary configured in
`product-review.local.toml`. Each configured aspect SHALL declare an `aspect_key`, localized
`name_<locale>` and `desc_<locale>` fields, and either a non-empty list of `kb.products`
`relation_type` values it maps to or an explicit `match_mode = "lexical"`. Aspect nodes SHALL be
created with `status` `accepted` and `origin` `llm_proposed` only where the aspect list is derived
from a model call; aspects taken directly from configuration SHALL use `origin` `user_added`.

#### Scenario: Configured aspects are attached

- **WHEN** a profile is built and the configuration enables aspects `storage`, `transport`,
  `maintenance`, `usage_environment`, and `safety`
- **THEN** five `aspect` nodes are attached to the root with those `aspect_key` values

#### Scenario: Aspect carries its relation-type mapping

- **WHEN** the `storage` aspect is configured with `relation_types = ["storage_requirement"]`
- **THEN** the `storage` node records that mapping for use by retrieval

#### Scenario: Aspect labels are localized

- **WHEN** the aspect vocabulary is requested with `?lang=zh_cn`
- **THEN** each aspect's `name_zh_cn` and `desc_zh_cn` are returned, falling back to `name_en` and
  `desc_en` where a translation is absent

### Requirement: Profile curation and versioning

The system SHALL let a user accept, reject, edit, add, and delete scope nodes on a profile. Any
mutation to a profile's node set SHALL increment the profile's `version`. A profile SHALL be
marked `ready` only by explicit user action. Runs SHALL pin the `profile_version` they executed
against, and a completed run's node set SHALL NOT change when the profile is later edited.

#### Scenario: Rejecting a node excludes it from later runs

- **WHEN** a user sets a node's `status` to `rejected`
- **THEN** the profile's `version` increments
- **AND** runs started after that edit exclude the node from scope

#### Scenario: Editing a profile does not alter a completed run

- **WHEN** a profile is edited after a run has completed against `profile_version` 3
- **THEN** the completed run still reports `profile_version` 3 and its stored node set is unchanged

#### Scenario: User adds a part the system missed

- **WHEN** a user adds a node labelled `Oxygen Sensor` under the root
- **THEN** the node is persisted with `origin` `user_added` and `status` `accepted`
- **AND** the system attempts to ground it using the grounding pass rules
