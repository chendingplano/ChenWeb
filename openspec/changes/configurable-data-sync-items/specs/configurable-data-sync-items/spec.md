## ADDED Requirements

### Requirement: Runtime-created sync items
The system SHALL let a signed-in sysadmin create a new sync item definition from the Sync Data admin page, persisted so it is usable immediately without a code deploy, in addition to the compiled-in registry.

#### Scenario: Creating a table-kind item
- **WHEN** a sysadmin submits the New Data Syncher form with kind "table" and a valid table, cursor column, natural key, and column list
- **THEN** the item is persisted and immediately appears in the Sync Data list, previewable and syncable like a compiled item

#### Scenario: Duplicate id rejected
- **WHEN** a sysadmin submits a new item whose id matches an id already in the compiled registry or already persisted
- **THEN** the system rejects the submission with an error identifying the conflicting id and creates nothing

#### Scenario: Invalid identifier rejected
- **WHEN** a sysadmin submits a table name, column name, cursor column, or natural-key column that is not a valid SQL identifier (or, for the table name, schema-qualified identifier)
- **THEN** the system rejects the submission with a validation error and persists nothing

### Requirement: Editing a locally-created sync item
The system SHALL let a sysadmin edit a sync item this instance created (`origin = local`), replacing its shape with newly-submitted, validated values, and SHALL reset that item's stored sync state so the next sync starts fresh rather than risking a stale cursor against a changed shape.

#### Scenario: Editing a local item succeeds and resets state
- **WHEN** a sysadmin submits a valid edit for a `local` item that has previously been synced (has a stored cursor and last-synced time)
- **THEN** the item's stored shape is replaced with the submitted values, and its stored cursor, last-synced time, and last row count are cleared, so the next preview or apply for that item starts as if it had never been synced

#### Scenario: Editing a learned item is rejected
- **WHEN** a sysadmin attempts to edit an item with `origin = learned`
- **THEN** the system rejects the request, explaining the item's shape is owned by the source that advertised it, and changes nothing

#### Scenario: Editing a compiled item is rejected
- **WHEN** a sysadmin attempts to edit an item that exists only in the compiled registry
- **THEN** the system rejects the request, explaining it must be changed in code and redeployed, and changes nothing

### Requirement: Deleting a sync item
The system SHALL let a sysadmin delete any `local` or `learned` sync item, removing its stored shape and its stored sync state, but SHALL NOT allow deleting a compiled item.

#### Scenario: Deleting a local item removes its state too
- **WHEN** a sysadmin deletes a `local` item that has previously been synced
- **THEN** the item's stored shape and its stored sync state (cursor, last-synced time, row count, last error) are both removed, and the item no longer appears in the Sync Data list

#### Scenario: Deleting a learned item only clears the local cache
- **WHEN** a sysadmin deletes a `learned` item
- **THEN** this instance's cached copy of that item's shape and sync state are removed; the item may reappear on a later list refresh if its source still advertises it, and stays gone if the source no longer does

#### Scenario: Deleting a compiled item is rejected
- **WHEN** a sysadmin attempts to delete an item that exists only in the compiled registry
- **THEN** the system rejects the request and removes nothing

### Requirement: Table-with-files sync kind
The system SHALL support a `table_with_files` sync item kind, where one column of each row holds a server-local file path; applying such an item SHALL copy the referenced file to the target before upserting the row, and SHALL rewrite the row's file column to the file's new location on the target.

#### Scenario: Creating a table-with-files item
- **WHEN** a sysadmin submits the New Data Syncher form with kind "table_with_files", including a file column (present in the column list) and a target storage location
- **THEN** the item is persisted with that kind and file configuration

#### Scenario: File column required for table-with-files
- **WHEN** a sysadmin submits kind "table_with_files" without designating a file column, or designates one not present in the column list
- **THEN** the system rejects the submission and persists nothing

#### Scenario: Applying a table-with-files item copies the file before the row
- **WHEN** a sysadmin applies a sync for a `table_with_files` item and a fetched row's file column is non-empty
- **THEN** the target fetches that row's file from the source, saves it under the item's configured target directory with a new local name, and only then upserts the row with its file column rewritten to that new local path

#### Scenario: Row with an empty file reference
- **WHEN** a fetched row of a `table_with_files` item has an empty or null file column
- **THEN** the target upserts the row without attempting any file fetch

#### Scenario: Referenced file missing at the source
- **WHEN** the target requests a row's file from the source and the source's database row exists but the file itself is missing on the source's filesystem
- **THEN** the apply run fails for that item with an error, the row is not upserted, and the item's stored cursor is not advanced (consistent with the existing "failed apply does not advance the cursor" behavior)

### Requirement: Source-side file transfer endpoint
The source SHALL expose an internal, shared-secret-authenticated endpoint that streams the file referenced by one row of a `table_with_files` item, identified by the row's natural key, and SHALL NOT accept a client-supplied filesystem path.

#### Scenario: Fetching a row's file by natural key
- **WHEN** a target requests a `table_with_files` item's file for a given natural key, presenting a valid shared-secret token
- **THEN** the source looks up that row's file column value itself via the item's table/filter, and streams the file at that path

#### Scenario: Natural key does not match any row
- **WHEN** a target requests a file for a natural key that matches no row of the item (respecting the item's filter, if any)
- **THEN** the source returns a 404-class error and streams no file content

#### Scenario: Requested item is not a table-with-files kind
- **WHEN** a target requests the file endpoint for an item whose kind is `table` (no file column)
- **THEN** the source returns an error and streams no file content

### Requirement: Cross-instance item-definition discovery
A target SHALL be able to discover sync items that exist only in its configured source's runtime-created registry (not the compiled registry shared by every instance), by querying the source, and SHALL cache what it learns so previewing or applying that item does not depend on the source being reachable at that later moment.

#### Scenario: Source-only item appears on the target after a list refresh
- **WHEN** a sysadmin creates a new item on the source, and later a sysadmin on a target (configured with that source) refreshes the target's Sync Data list
- **THEN** the target queries the source for its advertised item definitions, persists any it does not already have (with `origin = learned`), and the item then appears in the target's list

#### Scenario: A learned item's shape is refreshed when the source edits it
- **WHEN** a sysadmin edits an already-learned item's shape on the source, and later a sysadmin on the target refreshes the target's Sync Data list
- **THEN** the target's cached copy of that item is overwritten to match the source's current shape

#### Scenario: Previously-learned item still works when the source is unreachable
- **WHEN** a target has already learned a source-only item's definition on a prior list refresh, and the source is unreachable at the moment a sysadmin clicks Preview or Sync for that item
- **THEN** the target still recognizes the item id (using its cached definition) and reports the same source-unreachable error as any other item, rather than an unknown-item error
