## ADDED Requirements

### Requirement: Pipeline versions are immutable

Once a `kb.pipelines` row is created, its `processors[]` and every associated `kb.pipeline_rules` row for that version SHALL NOT be modified by any `UPDATE` statement. `display_name` and `description` are the only columns that MAY be updated in place on the current row for a given `name`.

#### Scenario: Attempting to change processors on an existing version is rejected

- **WHEN** a caller requests a processor-set change against an existing `kb.pipelines` row
- **THEN** the system SHALL create a new row (a new version) rather than updating the existing row,
  and no existing row's `processors[]` column SHALL ever change value after creation.

#### Scenario: Cosmetic edits do not create a new version

- **WHEN** a caller updates only `display_name` or `description` for a pipeline
- **THEN** the system SHALL update the current row in place and SHALL NOT insert a new version.

### Requirement: A new pipeline version is authored atomically

Creating a new pipeline version SHALL insert the `kb.pipelines` row, every associated `kb.pipeline_rules` row (gates and DAG edges), and run all creation-time validation in a single database transaction that either fully commits or fully rolls back. There SHALL be no API path that adds, removes, or edits a `kb.pipeline_rules` row against an already-created pipeline version.

#### Scenario: Validation failure during authoring rolls back the whole version

- **WHEN** a new pipeline version's rule set fails any creation-time validation check
- **THEN** the system SHALL reject the entire pipeline version creation and SHALL NOT persist the
  `kb.pipelines` row or any of its `kb.pipeline_rules` rows.

#### Scenario: No incremental edit path exists after creation

- **WHEN** a caller wants to change anything about an existing pipeline version's processor set or
  rules
- **THEN** the only supported action SHALL be authoring a new version; no endpoint SHALL accept a
  rule addition/removal against a version that has already committed.

### Requirement: Creating a new version supersedes the prior current version

When version N+1 of a named pipeline is created, version N SHALL transition to `status = 'superseded'` in the same transaction, and version N+1 SHALL be the current version for that name.

#### Scenario: Prior version is marked superseded on new version creation

- **WHEN** version N+1 of pipeline `p` is successfully created
- **THEN** version N of pipeline `p` SHALL have `status = 'superseded'` and version N+1 SHALL have
  `status = 'active'`.

### Requirement: Pipeline versions cannot be deleted

The system SHALL NOT provide any operation that physically deletes a `kb.pipelines` row. Removing a pipeline from active use SHALL only be achievable by superseding it with a new version or by no binding referencing it.

#### Scenario: Delete-pipeline endpoint is removed

- **WHEN** a client sends a request to the pipeline delete endpoint
- **THEN** the system SHALL respond that the operation does not exist (the endpoint SHALL be
  removed, not merely disabled), and no `kb.pipelines` row SHALL ever be removed by any code path.

### Requirement: Pipeline name and version together are unique

`kb.pipelines` SHALL enforce `UNIQUE (name, version)`, allowing multiple versions of the same named pipeline to coexist as distinct rows.

#### Scenario: Duplicate name+version rejected

- **WHEN** a creation attempt would produce a `(name, version)` pair that already exists
- **THEN** the database SHALL reject the insert with a uniqueness violation.
