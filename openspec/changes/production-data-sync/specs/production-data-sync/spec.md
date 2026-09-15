## ADDED Requirements

### Requirement: Sync item registry
The system SHALL maintain a compiled-in registry of syncable data items, where each item identifies a source Postgres table, a change-cursor column, a natural key (one or more columns forming a unique constraint), the column list to transfer, and an optional row filter.

#### Scenario: Registry is shared by both sync roles
- **WHEN** the ChenWeb backend starts, whether running as the sync source (dev Mac) or the sync target (a deployed box)
- **THEN** it loads the same compiled-in registry of sync items, so both the source's pull handler and the target's apply logic operate on identical item definitions

### Requirement: kb_product_names sync item
The system SHALL register `kb.product_names` as a syncable item, scoped by filter to rows where `source = 'cn_nmpa_medical_device_classification_catalog'`, using `(source, seq_no, product_name)` as its natural key.

#### Scenario: Locally-created rows are excluded
- **WHEN** a deployed box's own doc-processing pipeline has created `kb.product_names` rows with `status = 'proposed'` (a different `source` value than the catalog import)
- **THEN** a sync of the `kb_product_names` item never fetches, upserts, or otherwise touches those rows

### Requirement: update_time maintained automatically
`kb.product_names.update_time` SHALL be set to the current time automatically on every row update, via a database trigger, so it can serve as a reliable "changed since" cursor.

#### Scenario: Row update bumps update_time
- **WHEN** any column of an existing `kb.product_names` row is updated
- **THEN** that row's `update_time` is set to the time of the update, regardless of whether the application code that performed the update explicitly set `update_time`

### Requirement: Source pull API
The system SHALL expose an internal HTTP endpoint that returns rows of a registered sync item whose cursor-column value is strictly greater than a caller-supplied cursor, ordered deterministically, along with the next cursor to use.

#### Scenario: Fetching changes since a cursor
- **WHEN** a target requests changes for a registered item with `since=<cursor>`
- **THEN** the source returns all matching rows (respecting the item's filter, if any) with `cursor_col` greater than `<cursor>`, ordered by `(cursor_col, natural key)`, together with `next_cursor` equal to the maximum `cursor_col` value among the returned rows and a `has_more` flag

#### Scenario: Fetching from an empty cursor
- **WHEN** a target requests changes for a registered item with no prior cursor (first-ever sync)
- **THEN** the source returns all matching rows for that item, as if `since` were the beginning of time

#### Scenario: Unknown item id
- **WHEN** a request names a sync item id that is not in the registry
- **THEN** the source returns a 404-class error and fetches no data

### Requirement: Source pull API authentication
The source pull endpoint SHALL reject any request that does not present a valid shared-secret bearer token, verified using a constant-time comparison, and SHALL fail closed if no secret is configured.

#### Scenario: Missing or wrong token
- **WHEN** a request to the source pull endpoint omits the `Authorization: Bearer <token>` header or presents a token that does not match the configured shared secret
- **THEN** the source rejects the request with an unauthorized error and returns no row data

#### Scenario: Secret not configured
- **WHEN** the source's shared-secret environment variable is unset or empty
- **THEN** the source pull endpoint rejects all requests with a configuration error rather than allowing unauthenticated access

### Requirement: Target incremental apply
When applying a sync for a registered item, the target SHALL upsert each fetched row by the item's natural key (inserting new rows, updating existing rows that match), and SHALL NOT delete any target row, including rows present at the target but absent from the source response.

#### Scenario: New row appears at the source
- **WHEN** a fetched row's natural key does not match any existing target row
- **THEN** the target inserts it as a new row with a locally-generated primary key

#### Scenario: Existing row changed at the source
- **WHEN** a fetched row's natural key matches an existing target row
- **THEN** the target updates that row's non-key columns to the fetched values, leaving its local primary key unchanged

#### Scenario: Row removed at the source
- **WHEN** a row was previously synced to the target but no longer exists at the source (e.g. deleted there)
- **THEN** the target's copy of that row is left in place; the target never deletes rows as part of applying a sync

### Requirement: Sync cursor persistence
The target SHALL persist, per sync item, the cursor value reported by the source as `next_cursor`, and SHALL only advance the stored cursor after all rows up to that cursor have been successfully applied.

#### Scenario: Successful apply advances the cursor
- **WHEN** an apply run fetches and upserts all changed rows for an item without error
- **THEN** the target stores the source-reported `next_cursor` as that item's new starting point for the next sync

#### Scenario: Failed apply does not advance the cursor
- **WHEN** an apply run fails partway (e.g. the source is unreachable, or a database error occurs while upserting)
- **THEN** the target's stored cursor for that item is left unchanged, so the next sync attempt re-fetches from the same starting point

### Requirement: Admin sync UI
The system SHALL provide a System Admin → Resources → Sync Data page listing every registered sync item with its last-synced time and row count, and SHALL let a signed-in sysadmin preview and apply a sync for any item.

#### Scenario: Previewing a sync
- **WHEN** a sysadmin clicks "Preview" for a sync item on the Sync Data page
- **THEN** the target queries the source for changes since the item's current stored cursor and displays the count of changed rows found, without upserting any data or advancing the cursor

#### Scenario: Applying a sync
- **WHEN** a sysadmin clicks "Sync" (apply) for a sync item on the Sync Data page
- **THEN** the target fetches and upserts all changed rows for that item, advances its stored cursor on success, and displays the number of rows synced

#### Scenario: Source unreachable
- **WHEN** a sysadmin triggers a preview or apply while the source (dev Mac) is unreachable
- **THEN** the page displays an error message and the item's stored cursor and last-synced state remain unchanged
