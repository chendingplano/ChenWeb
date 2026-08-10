## ADDED Requirements

### Requirement: Binding and rule liveness is governed solely by `active`

`kb.pipeline_bindings` and `kb.pipeline_rules` rows SHALL determine whether they are currently in effect using only their own `active` column. No separate policy-level object SHALL gate liveness.

#### Scenario: A binding is live purely by its own active flag

- **WHEN** the system resolves which pipeline applies to an incoming request
- **THEN** it SHALL consider only `kb.pipeline_bindings` rows where `active = true`, with no join
  to any policy table.

#### Scenario: A gate is live purely by its own active flag

- **WHEN** the system resolves which gates apply to a processor
- **THEN** it SHALL consider only `kb.pipeline_rules` rows where `active = true`, with no join to
  any policy table.

### Requirement: `kb.pipeline_policies` no longer exists

The system SHALL NOT contain a `kb.pipeline_policies` table, and no code path SHALL read from or write to it.

#### Scenario: Policy table absent from schema

- **WHEN** the schema migrations for this change have been applied
- **THEN** `kb.pipeline_policies` SHALL NOT exist, and `kb.pipeline_bindings`/`kb.pipeline_rules`
  SHALL NOT have a `policy_id` column.

#### Scenario: Policy CRUD endpoints removed

- **WHEN** a client sends a request to any former pipeline-policy endpoint (list, create, activate)
- **THEN** the system SHALL respond that the operation does not exist.

### Requirement: At most one active default binding per context

The system SHALL enforce, via a database constraint, that at most one `kb.pipeline_bindings` row with `binding_kind = 'store_default'` and `active = true` exists per distinct context (the combination of `ks_store_id`, `user_id`, `tenant_id`, `input_record_id`, treating unset scope columns as a common sentinel value for comparison purposes).

#### Scenario: Second active default binding for the same context is rejected

- **WHEN** an active `store_default` binding already exists for a given context and a second
  active `store_default` binding is created for the same context
- **THEN** the database SHALL reject the insert or update with a uniqueness violation.

#### Scenario: Different contexts may each have their own active default binding

- **WHEN** two `store_default` bindings target different knowledge stores
- **THEN** both SHALL be permitted to be simultaneously active.

### Requirement: Routing provenance is recorded as pipeline name and version

Any record of "which pipeline definition produced this routing decision" (execution plans, alarm events, audit events, routing-clearance subjects, routing-enforcement requests) SHALL record the winning `kb.pipeline_bindings.id` together with the resolved `kb.pipelines.name` and `version`, not a policy id or policy version.

#### Scenario: Plan-fact provenance names the resolved pipeline

- **WHEN** a processor execution plan is built for a document
- **THEN** the plan's facts SHALL include the winning binding id and the resolved pipeline's name
  and version, and SHALL NOT include any policy id or policy version field.

#### Scenario: Alarm and audit events name the resolved pipeline

- **WHEN** a routing alarm or audit event is emitted for a document run
- **THEN** the event SHALL carry the resolved pipeline's name and version (and the winning binding
  id) instead of a policy id and policy version.

#### Scenario: D2 routing-clearance subjects and enforcement requests use pipeline provenance

- **WHEN** a routing-clearance subject is recorded or a routing-enforcement request is evaluated
- **THEN** it SHALL be keyed by the resolved pipeline's name and version (and binding id) rather
  than by policy id and policy version.

### Requirement: Ontology-module-release promotion authors a new pipeline version

When an approved ontology-module release is promoted into production routing, the system SHALL express that promotion as a new `kb.pipelines` version (going through the same atomic version-authoring path as any other pipeline change), not as a new `kb.pipeline_policies` row.

#### Scenario: Promotion produces a new pipeline version

- **WHEN** an ontology-module release is promoted
- **THEN** the system SHALL create a new pipeline version whose rules include the newly-approved
  module's bindings, and SHALL NOT create any `kb.pipeline_policies` row (since that table no
  longer exists).
