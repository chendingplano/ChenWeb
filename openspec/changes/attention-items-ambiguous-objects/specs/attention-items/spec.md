## ADDED Requirements

### Requirement: Attention items table
The system SHALL store data problems that need a human decision in `kb.attention_items`, one row
per problem, with an `item_type` that classifies the problem, a `status` of `open`, `resolved` or
`dismissed`, a `dedupe_key`, a `title`, type-specific evidence in `details`, the most recent
`input_record_id`, `occurrence_count`, `reopen_count`, `first_seen`, `last_seen`, and `resolved_by`,
`resolved_at` and `resolution` for closed items. The table SHALL be created by a goose
migration.

#### Scenario: Table exists after startup
- **WHEN** the application starts and runs project migrations
- **THEN** `kb.attention_items` exists with the columns above and rejects any `status` other than
  `open`, `resolved` or `dismissed`

#### Scenario: Items survive their source record
- **WHEN** the record named by an item's `input_record_id` is deleted
- **THEN** the attention item remains

### Requirement: Item types are validated in code
The system SHALL accept only item types declared in the attention package, and SHALL declare
`ambiguous-object-name` as the first type. Adding a type SHALL NOT require a schema change.

#### Scenario: Unknown type rejected
- **WHEN** code records an item whose type is not declared
- **THEN** recording fails with an error and no row is written

### Requirement: Repeats of one problem update a single item
The system SHALL keep one row per problem, identified by (`item_type`, `dedupe_key`) across all
statuses. Recording a problem that has an `open` item SHALL increment `occurrence_count`, set
`last_seen` and `input_record_id` to the latest values, and merge the new `details`, without
creating a row. Recording a problem whose item is `resolved` or `dismissed` SHALL reopen that
item: set `status` to `open`, increment `reopen_count` and `occurrence_count`, append the earlier
closing (status, `resolved_by`, `resolved_at`, `resolution`) to `details.resolution_history`,
clear `resolved_by`, `resolved_at` and `resolution`, and merge the new `details`.

#### Scenario: Repeat of an open problem
- **WHEN** a problem with an open item is recorded again
- **THEN** the item's `occurrence_count` increases by one, `last_seen` moves to now, and no new
  row is created

#### Scenario: Recurrence after resolution
- **WHEN** a problem whose item was resolved is recorded again
- **THEN** the same item becomes `open` with `reopen_count` 1, its earlier resolution is in
  `details.resolution_history`, its closing columns are empty, and the new evidence is in
  `details`

#### Scenario: Recurrence after dismissal
- **WHEN** a problem whose item was dismissed is recorded again
- **THEN** the same item becomes `open` with `reopen_count` increased and the dismissal in
  `details.resolution_history`

#### Scenario: Concurrent recording
- **WHEN** two pipelines record the same new problem at the same time
- **THEN** exactly one open item exists, with `occurrence_count` 2

### Requirement: Attention item API
The system SHALL provide `GET /api/v1/kb/attention-items` (filters `type` and `status`, paged by
`limit` and `offset`, newest `last_seen` first), `GET /api/v1/kb/attention-items/:id`,
`POST /api/v1/kb/attention-items/:id/resolve` and `POST /api/v1/kb/attention-items/:id/dismiss`.
Resolve and dismiss SHALL accept an optional `resolution` note, SHALL record the requesting user
in `resolved_by` and the time in `resolved_at`, and SHALL apply only to `open` items.

#### Scenario: List open items of one type
- **WHEN** a client requests `GET /api/v1/kb/attention-items?type=ambiguous-object-name&status=open`
- **THEN** the response lists only open items of that type, most recently seen first

#### Scenario: Resolve an open item
- **WHEN** a signed-in user posts to `/resolve` for an open item with a note
- **THEN** the item's status becomes `resolved`, `resolved_by` is that user, `resolved_at` is now,
  and `resolution` is the note

#### Scenario: Close an item that is not open
- **WHEN** a client posts to `/resolve` or `/dismiss` for an item that is already resolved or
  dismissed
- **THEN** the request fails with a client error and the item is unchanged

#### Scenario: Unknown item
- **WHEN** a client requests an item ID that does not exist
- **THEN** the response is 404
