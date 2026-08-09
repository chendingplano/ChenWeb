## ADDED Requirements

### Requirement: Staged catalog entries are auto-promoted into flagged provisional concepts
After an approved resource's staging import completes, the system SHALL, for every `kb.keyword_catalog_entries` row of that `(source, release)` whose promotion policy is enabled, create or converge on a `kb.keyword_concepts` row with `status='provisional'` and `gloss_source='auto:import:<source>'`, plus a matching `kb.keyword_surfaces` row with `provenance='auto:import:<source>'`, without requiring any human action.

#### Scenario: Approving a resource auto-promotes its entries
- **WHEN** an operator approves a resource whose promotion policy is enabled (the default)
- **THEN** every catalog entry for that resource's `(source, release)` ends up represented by a `kb.keyword_concepts` row tagged `gloss_source='auto:import:<source>'`, created without any human approving or authoring the promotion

#### Scenario: Repeated promotion of the same entry converges, not duplicates
- **WHEN** the same catalog entry is promoted more than once (e.g. a replayed Approve, or a retried background run)
- **THEN** the second and subsequent attempts resolve to the same `kb.keyword_concepts` row (by normalized-label content hash) rather than creating additional rows

### Requirement: Auto-promotion is enabled by default and admin-disable-able per resource
Every resource's staged entries SHALL auto-promote by default. An administrator SHALL be able to disable auto-promotion for a specific resource; this is the only gate on the behavior — there is no additional gating based on the source's data-trust classification, since access to this action is already restricted to System Admin.

#### Scenario: A newly-added resource auto-promotes with no configuration
- **WHEN** an operator approves a resource that has no promotion-policy row at all
- **THEN** its catalog entries are auto-promoted (absence of a row means enabled)

#### Scenario: An admin disables auto-promotion for a specific resource
- **WHEN** an administrator sets a resource's promotion policy to disabled
- **THEN** subsequent approvals of that resource do not auto-promote its catalog entries, and they remain staged-only pending optional human action

#### Scenario: An admin re-enables a previously-disabled resource
- **WHEN** an administrator sets a previously-disabled resource's promotion policy back to enabled
- **THEN** subsequent approvals of that resource auto-promote its catalog entries again

### Requirement: Promotion runs in the background and never blocks Approve
Auto-promotion SHALL be triggered after the Approve request's keyword-lexicon staging write completes, and SHALL NOT delay the HTTP response to the operator.

#### Scenario: Approve returns before promotion finishes
- **WHEN** an operator approves a resource with a large catalog and auto-promotion enabled
- **THEN** the Approve HTTP response completes without waiting for every catalog entry to be promoted

### Requirement: Auto-promoted concepts remain optionally human-reviewable
Nothing about auto-promotion SHALL prevent a human from subsequently reviewing, merging, or otherwise acting on an auto-promoted concept through existing mechanisms (e.g. the concept merge endpoint); such review SHALL remain optional, never required for the concept to be usable. This change SHALL NOT modify how or whether automatic reconciliation processes these concepts — that remains entirely the concern of the separately-maintained reconciliation subsystem.

#### Scenario: A human merges an auto-promoted concept
- **WHEN** an operator manually merges an auto-promoted (`gloss_source='auto:import:*'`) concept into another concept via the existing keyword-concept merge endpoint
- **THEN** the merge succeeds subject to the same guardrails any other concept merge is subject to, with no special-casing required
