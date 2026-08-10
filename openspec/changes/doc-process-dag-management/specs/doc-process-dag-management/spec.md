## ADDED Requirements

### Requirement: Doc Process DAG search and view
The system SHALL provide a page at *Development → System Admin → Doc Process Pipeline → Doc
Process DAG* where a user can search Doc Process DAGs by name and view a DAG's details: its
pipeline (current version), its processors, its processor gates and DAG edges, whether it is the
system default, and its knowledge-store bindings.

#### Scenario: Search lists matching DAGs
- **WHEN** a user enters a search term on the Doc Process DAG page
- **THEN** the page lists every Doc Process DAG whose name or display name contains the term,
  with at most one entry per DAG name (its current version)

#### Scenario: Viewing a DAG shows full detail
- **WHEN** a user opens a Doc Process DAG from the list
- **THEN** the page shows the DAG's name, display name, description, version, processor set,
  per-processor gates and `depends_on_processors` edges, its system-default status, and any
  knowledge-store bindings that reference it

### Requirement: Doc Process DAG names are unique
A Doc Process DAG SHALL be identified by a unique name. Creating or renaming a DAG whose name
already exists SHALL be rejected, and no two DAGs SHALL share the same name.

#### Scenario: Creating a DAG with a duplicate name is rejected
- **WHEN** a user creates a Doc Process DAG whose name already belongs to another DAG
- **THEN** the system SHALL reject the creation with an error naming the duplicate name, and
  SHALL NOT persist any pipeline or rule row

#### Scenario: Modifying a DAG does not change its name
- **WHEN** a user modifies a DAG's processors, rules, or display metadata
- **THEN** the DAG keeps its existing name (rename is not part of modify; it is create-new plus
  delete-old)

### Requirement: A Doc Process DAG has at least one processor
A Doc Process DAG SHALL include at least one doc processor. Creating or modifying a DAG with an
empty processor set SHALL be rejected before anything is written.

#### Scenario: Empty processor set is rejected
- **WHEN** a user submits a DAG with no processors selected
- **THEN** the system SHALL reject the save with an error stating that at least one processor is
  required, and SHALL NOT persist the DAG

### Requirement: Doc Process DAG is validated before save
Creating or modifying a Doc Process DAG SHALL run the ADR 2026081001 DR8 creation-time checks
(processor closure, DAG well-formedness, gate-fact availability) before the DAG is persisted. A
DAG that fails validation SHALL NOT be saved, and the error SHALL name the specific violation.

#### Scenario: A cyclic DAG is rejected without persisting
- **WHEN** a user saves a DAG whose `depends_on_processors` edges contain a cycle
- **THEN** the system SHALL reject the save with an error naming the involved processors, and
  SHALL NOT persist any pipeline or rule row

#### Scenario: A DAG edge to a processor not in the DAG is rejected
- **WHEN** a user saves a DAG in which a processor depends on a processor that is not part of the
  DAG's processor set
- **THEN** the system SHALL reject the save with an error naming the dangling dependency

### Requirement: Doc Process DAG writes are transaction-protected
Creating, modifying, and deleting a Doc Process DAG SHALL each run in a single database
transaction spanning every affected table (`kb.pipelines`, `kb.pipeline_rules`, and for delete
`kb.pipeline_bindings`). Any failure SHALL roll back the entire operation, leaving the database
unchanged.

#### Scenario: A failed create leaves no partial rows
- **WHEN** a DAG creation fails partway through (e.g. validation or a uniqueness conflict)
- **THEN** no `kb.pipelines` row and no `kb.pipeline_rules` row for that DAG remain

#### Scenario: Deleting a DAG removes it atomically
- **WHEN** a user deletes a Doc Process DAG
- **THEN** the system SHALL remove, in one transaction, every pipeline version of that name, every
  rule row for those versions, and every binding that references them; on any error nothing is
  removed

### Requirement: Exactly one Doc Process DAG is the system default
The system SHALL have exactly one Doc Process DAG marked as the system default at all times. The
first DAG created SHALL become the default automatically when no default exists. Marking a DAG as
default SHALL unset the previous default. Unsetting the only default, and deleting the default
DAG, SHALL be rejected.

#### Scenario: First DAG becomes the default automatically
- **WHEN** a user creates a DAG and no DAG is currently marked as the system default
- **THEN** the newly created DAG SHALL be marked as the system default even if the user did not
  request it

#### Scenario: Marking a new default unsets the old one
- **WHEN** a user marks a different DAG as the system default
- **THEN** the previously-default DAG SHALL lose its default status in the same transaction

#### Scenario: Unsetting the only default is rejected
- **WHEN** a user tries to unmark the DAG that is the only system default
- **THEN** the system SHALL reject the change with an error stating that at least one system
  default must remain

#### Scenario: Deleting the default DAG is rejected
- **WHEN** a user tries to delete the DAG that is currently the system default
- **THEN** the system SHALL reject the delete and instruct the user to promote another DAG first

#### Scenario: A new version of the default DAG stays the default
- **WHEN** a user modifies the processors or rules of the default DAG (authoring a new version)
- **THEN** the new version SHALL be the system default and the superseded version SHALL not be

### Requirement: Modifying processors or rules authors a new version
Modifying a Doc Process DAG's processors or rules SHALL author a new immutable pipeline version
(version = prior + 1), mark the prior version `superseded`, and insert the new rules, all in one
transaction. Cosmetic-only changes (`display_name`, `description`) SHALL update the current version
in place without creating a new version.

#### Scenario: Processor change creates a new version
- **WHEN** a user adds or removes a processor from a DAG and saves
- **THEN** the system SHALL create a new pipeline version with the new processor set, supersede
  the prior version, and never mutate the prior version's processors

#### Scenario: Cosmetic change does not create a new version
- **WHEN** a user changes only a DAG's display name or description and saves
- **THEN** the system SHALL update the current version in place and SHALL NOT insert a new version
