## ADDED Requirements

### Requirement: Processor DAG edges are persisted per pipeline rule

`kb.pipeline_rules` SHALL carry a `depends_on_processors` array column naming sibling processors, within the same pipeline version, that must complete successfully before the rule's `target_processor` runs. This is independent of the rule's `predicate`/`effect` gate columns.

#### Scenario: A rule declares its upstream dependencies

- **WHEN** a pipeline version's rule for processor `C` is authored with `depends_on_processors =
  {A}`
- **THEN** the system SHALL persist that dependency edge on the rule row for `C` within that
  version.

### Requirement: Processor dependency metadata resolves from the union of code and registry

Processor `Requires`/`Produces` metadata SHALL be resolved by checking the existing hardcoded processor spec list first, falling back to the `kb.processor_registry` table only for processor names not found in the hardcoded list.

#### Scenario: An existing hardcoded processor resolves from code

- **WHEN** dependency metadata is requested for a processor already declared in the hardcoded spec
  list
- **THEN** the system SHALL return that processor's hardcoded `Requires`/`Produces`, without
  querying `kb.processor_registry`.

#### Scenario: A new processor not in the hardcoded list resolves from the registry

- **WHEN** dependency metadata is requested for a processor name not present in the hardcoded spec
  list but present as a `kb.processor_registry` row
- **THEN** the system SHALL return that row's `requires`/`produces` values.

### Requirement: Pipeline version creation validates processor closure

At creation time, for every processor `P` selected in a pipeline version and every artifact kind `A` in `P`'s `Requires`, the system SHALL require that some other processor `Q` in the same version's selected set has `A` in its `Produces`, or that `A` is guaranteed by a baseline processor that always runs (`static_analyzer`, `chunking`). If no such producer exists, the system SHALL reject the pipeline version creation and name the unsatisfied processor and artifact kind.

#### Scenario: Missing producer is rejected

- **WHEN** a pipeline version selects a processor that requires an artifact kind no selected or
  baseline processor produces
- **THEN** the system SHALL reject creation of the pipeline version, and the error SHALL name the
  processor and the missing artifact kind.

#### Scenario: Satisfied closure is accepted

- **WHEN** every `Requires` of every selected processor is covered by some selected or baseline
  processor's `Produces`
- **THEN** the system SHALL accept this check and proceed to the next validation.

### Requirement: Pipeline version creation validates DAG well-formedness

At creation time, the system SHALL require that a pipeline version's `depends_on_processors` edges form an acyclic graph and that every processor name referenced by any edge is itself one of the version's own selected processors. If either condition fails, the system SHALL reject the pipeline version creation and name the specific violation (the cycle, or the dangling reference).

#### Scenario: Cyclic dependency is rejected

- **WHEN** a pipeline version's `depends_on_processors` edges contain a cycle
- **THEN** the system SHALL reject creation of the pipeline version and name a processor in the
  cycle.

#### Scenario: Dependency on a processor outside the version is rejected

- **WHEN** a rule's `depends_on_processors` names a processor that is not part of the same pipeline
  version's selected processor set
- **THEN** the system SHALL reject creation of the pipeline version and name the dangling
  reference.

### Requirement: Pipeline version creation validates gate-fact availability

At creation time, for every `kb.pipeline_rules` gate with a non-trivial `predicate`, the system SHALL require that every document fact the predicate references is guaranteed to be produced by some processor that the DAG guarantees has already run before that gate is evaluated — an upstream node reachable via `depends_on_processors`, or a baseline processor that always runs first. If no such guarantee exists, the system SHALL reject the pipeline version creation and name the gate and the missing fact.

#### Scenario: A gate referencing an unavailable fact is rejected

- **WHEN** a gate's predicate references a document fact with no guaranteed upstream producer in
  the version's DAG
- **THEN** the system SHALL reject creation of the pipeline version, and the error SHALL name the
  gate and the missing fact.

#### Scenario: A gate referencing a guaranteed-available fact is accepted

- **WHEN** a gate's predicate references only facts produced by processors guaranteed to run
  upstream of the gate
- **THEN** the system SHALL accept this check.

### Requirement: Creation-time validation is never re-run at runtime

Once a pipeline version has passed all creation-time validation and been persisted, its correctness SHALL be treated as a permanent, proven property. No runtime code path SHALL re-run processor-closure, DAG well-formedness, or gate-fact-availability checks against an already-created version.

#### Scenario: Runtime execution does not re-validate an existing version

- **WHEN** a document is routed through an already-created pipeline version
- **THEN** the system SHALL NOT re-run any of the three creation-time validation checks against
  that version during the run.
