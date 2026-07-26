## ADDED Requirements

### Requirement: CDM schema
The system SHALL create four tables in the `kb` schema via goose migrations:
`kb.cdm_documents`, `kb.cdm_blocks`, `kb.cdm_renderings`, `kb.cdm_projections`.
They SHALL follow existing `kb` conventions — `BIGSERIAL` primary keys,
`create_time` / `update_time` timestamps, `ON DELETE CASCADE` from child tables
to `kb.cdm_documents` — and SHALL NOT use `uuid` keys or `created_at` naming.

#### Scenario: Migration creates the tables
- **WHEN** the goose migrations are applied to a database without them
- **THEN** all four `kb.cdm_*` tables exist with the specified key and timestamp
  conventions

#### Scenario: Migration is reversible
- **WHEN** the migration's down step is applied
- **THEN** all four tables are dropped and no other `kb` object is affected

#### Scenario: Block references are text, not uuid
- **WHEN** the schema is inspected
- **THEN** `kb.cdm_blocks.block_id` and `kb.cdm_blocks.parent_block_id` are text
  columns and `kb.cdm_projections.block_ids` is a text array

#### Scenario: Existing tables are untouched
- **WHEN** the migration is applied
- **THEN** `kb.chunks`, `kb.chunk_ranges`, and `kb.semantic_projections` are
  unchanged in schema and content

### Requirement: Canonical JSON is authoritative
`kb.cdm_documents.semantic_document` SHALL hold the complete canonical JSON and
SHALL be the single source of truth. `kb.cdm_blocks` SHALL be a derived
flattening of that JSON, rewritten transactionally on every document write. The
promoted columns `doc_type`, `rendering_type`, and `authors` SHALL be written
from the same JSON in the same transaction.

#### Scenario: Blocks are rebuilt on save
- **WHEN** a document is saved with a block removed since the previous save
- **THEN** the corresponding `kb.cdm_blocks` row no longer exists, and the
  remaining rows match the new `semantic_document`

#### Scenario: Promoted columns match the JSON
- **WHEN** a document is saved
- **THEN** `doc_type`, `rendering_type`, and `authors` equal the values inside
  `semantic_document.metadata`

#### Scenario: Write is atomic
- **WHEN** rebuilding `kb.cdm_blocks` fails partway through a save
- **THEN** the whole transaction rolls back and the previously stored document
  and its blocks remain intact

### Requirement: Documents are validated before storage
The store SHALL reject any document that fails validation, before any write
occurs.

#### Scenario: Invalid document is not persisted
- **WHEN** a document that violates a content-model invariant is submitted for
  saving
- **THEN** the store returns a validation error and no row is written to any
  `kb.cdm_*` table

### Requirement: Content versioning
`kb.cdm_documents.content_version` SHALL increment on every write that changes
the document's content, and SHALL be the version recorded on derived artifacts
in `kb.cdm_renderings` and `kb.cdm_projections`.

#### Scenario: Saving changed content increments the version
- **WHEN** a document is saved with modified block content
- **THEN** its `content_version` is greater than the previous value

#### Scenario: Renderings are keyed by the version they were produced from
- **WHEN** a rendering is stored for a document
- **THEN** its `content_version` equals the document's `content_version` at
  render time, and re-rendering the same version with the same renderer version
  does not create a duplicate row

### Requirement: Block slug uniqueness is enforced by the database
The `(document_id, block_id)` pair SHALL be unique. On violation the store SHALL
return a typed conflict error identifying the colliding slug. The system SHALL
NOT auto-rename, auto-suffix, or otherwise silently alter a block slug.

#### Scenario: Colliding slug returns a conflict error
- **WHEN** a document is saved containing two blocks that resolve to the same
  slug
- **THEN** the store returns a typed conflict error naming the slug, and no
  write is committed

#### Scenario: Slug is never rewritten silently
- **WHEN** a save is rejected for a slug conflict
- **THEN** no stored block slug has been modified

### Requirement: CDM documents are registered in `kb.inputs`
Every CDM document SHALL have a corresponding `kb.inputs` row with `type = 'cdm'`
so it is visible to existing knowledge search and artifact tooling. The row's
`status` SHALL be written so that `parse_state` derives to `parsed_success` and
`pipeline_state` derives to `success`. `tenant_id` and `ks_store_id` SHALL be
carried on that row.

#### Scenario: Creating a CDM document creates its input row
- **WHEN** a CDM document is created
- **THEN** a `kb.inputs` row exists with `type = 'cdm'` and the document
  references it by `input_record_id`

#### Scenario: CDM documents never appear on the parse worklist
- **WHEN** the parse worklist query selecting `parse_state = 'pending'` runs
- **THEN** no `type = 'cdm'` row is returned

#### Scenario: CDM documents never appear on the doc-processing worklist
- **WHEN** the worklist query selecting
  `parse_state = 'parsed_success' AND pipeline_state = 'pending'` runs
- **THEN** no `type = 'cdm'` row is returned

#### Scenario: Derived states are asserted directly
- **WHEN** a CDM input row is inserted and its derived columns are read back
- **THEN** `parse_state` is `parsed_success` and `pipeline_state` is `success`

### Requirement: Deleting a document removes its derived rows
Deleting a `kb.cdm_documents` row SHALL cascade to its blocks, renderings, and
projections.

#### Scenario: Cascade removes derived rows
- **WHEN** a CDM document row is deleted
- **THEN** its `kb.cdm_blocks`, `kb.cdm_renderings`, and `kb.cdm_projections`
  rows are removed
