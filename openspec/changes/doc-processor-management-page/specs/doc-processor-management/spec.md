## ADDED Requirements

### Requirement: Doc processors are stored in a searchable catalog table

The system SHALL persist doc processors in the `kb.doc_processors` table, keyed by the canonical
processor name (`name_as_id`). A row SHALL carry: `name_as_id` (unique, immutable),
`display_name`, `description`, `type` (one of `mandatory` | `configurable`), `require_llm`
(boolean), `status` (one of `active` | `disabled` | `suspended`), `notes`, `create_time`, and
`modify_time`. The initial roster SHALL be seeded from the capsule §7 "Doc Processing Pipeline"
table; processors the capsule types as `routed` / `routed (Phase C)` SHALL be stored with
`type = 'configurable'`.

#### Scenario: Initial roster is seeded
- **WHEN** the migration applies and the catalog is first listed
- **THEN** every processor in the capsule §7 table has a row with `status = 'active'`, the
  `mandatory`/`configurable` type from §7 (routed processors as `configurable`), and
  `require_llm` matching §7's "Require LLMs" column

#### Scenario: Duplicate name is rejected
- **WHEN** a create or update would store a `name_as_id` that already exists
- **THEN** the write is rejected and a clear error message is returned

#### Scenario: Invalid type is rejected
- **WHEN** a create or update submits a `type` other than `mandatory` or `configurable`
- **THEN** the write is rejected with a clean error message (no raw database constraint error)

#### Scenario: Invalid status is rejected
- **WHEN** a create or update submits a `status` other than `active`, `disabled`, or `suspended`
- **THEN** the write is rejected with a clean error message

### Requirement: Admin can search doc processors

The system SHALL list doc processors with an optional substring search over `name_as_id` and
`display_name`. The list SHALL include every field needed to render and edit a row.

#### Scenario: List all processors
- **WHEN** an admin opens the Doc Processors page without a search term
- **THEN** the page shows all rows ordered by `name_as_id`, each with its type, status, and
  require_llm indicator

#### Scenario: Filter by name
- **WHEN** an admin types a search term
- **THEN** the list is filtered to rows whose `name_as_id` or `display_name` contains the term
  (case-insensitive)

### Requirement: Admin can create a doc processor

The system SHALL let an admin create a doc processor by submitting `name_as_id`, `display_name`,
`type`, `require_llm`, `status`, `description`, and `notes`. `name_as_id` SHALL be required and
unique; `type` and `status` SHALL be validated; `status` SHALL default to `active` when omitted.

#### Scenario: Successful create
- **WHEN** an admin submits a complete, valid processor
- **THEN** a new `kb.doc_processors` row is created with `create_time`/`modify_time` set and the
  row appears in the list

#### Scenario: Create without name is rejected
- **WHEN** an admin submits a create with an empty `name_as_id`
- **THEN** the request is rejected with an error naming the missing field

#### Scenario: Create with existing name is rejected
- **WHEN** an admin submits a create whose `name_as_id` already exists
- **THEN** the request is rejected with a message indicating the name is already in use

### Requirement: Admin can update a doc processor

The system SHALL let an admin edit `display_name`, `description`, `type`, `require_llm`,
`status`, and `notes` of an existing row. `name_as_id` SHALL be immutable: renaming a processor
requires creating a new row and deleting the old one. Updating SHALL refresh `modify_time`.

#### Scenario: Successful update
- **WHEN** an admin edits editable fields of an existing processor
- **THEN** the row is updated, `modify_time` is refreshed, and the list reflects the new values

#### Scenario: Update of an unknown processor is rejected
- **WHEN** an admin updates a `name_as_id` that does not exist
- **THEN** the request returns a not-found error

### Requirement: Admin can delete a doc processor

The system SHALL let an admin delete a doc processor by its `name_as_id`. The delete SHALL remove
the catalog row; processor-name references in other tables (which store names as strings) are not
FK-linked and do not block the delete. The page SHALL require confirmation before deleting.

#### Scenario: Successful delete
- **WHEN** an admin confirms deletion of an existing processor
- **THEN** the row is removed and no longer appears in the list

#### Scenario: Delete of an unknown processor is rejected
- **WHEN** an admin deletes a `name_as_id` that does not exist
- **THEN** the request returns a not-found error
