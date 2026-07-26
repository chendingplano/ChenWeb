## ADDED Requirements

### Requirement: CDM documents are reachable over HTTP
The system SHALL expose the CDM Phase 1 packages over HTTP under
`/api/v1/cdm`, registered in the existing `/api/v1` group and behind
`authmiddleware.AuthMiddleware`. Handlers SHALL delegate validation to
`cdm/model` and persistence to `cdm/store` rather than reimplementing either.

#### Scenario: Routes require authentication
- **WHEN** any `/api/v1/cdm/*` endpoint is called without valid authentication
- **THEN** the request is rejected by the same middleware that guards the
  existing `/api/v1/kb/*` routes, and no handler code runs

#### Scenario: Handlers do not duplicate validation
- **WHEN** an invalid document is submitted to save
- **THEN** the rejection originates from `model.Validate`, and the handler
  contains no independent copy of the invariant checks

### Requirement: Canonical CDM JSON is the wire format
The request and response body for document load and save SHALL be the canonical
document JSON (`model.Document`) itself. The system SHALL NOT define a separate
data-transfer shape for documents.

#### Scenario: Load returns canonical JSON
- **WHEN** a stored document is loaded through the API
- **THEN** the response body unmarshals directly into `model.Document` and
  carries the same `schema_version` the document was stored with

#### Scenario: Save round-trips without loss
- **WHEN** a document is loaded, re-submitted unchanged, and loaded again
- **THEN** the second load is byte-identical to the first apart from
  `content_version`

### Requirement: Creating a document writes and links both rows atomically
Creating a CDM document SHALL insert its `kb.inputs` row and its
`kb.cdm_documents` row in a single transaction and SHALL populate
`kb.cdm_documents.input_record_id` with the created input row's id. The
`kb.inputs` row SHALL be written in the draft form required by CDM §10.1, so
the document is invisible to both the parse and doc-processing worklists.

#### Scenario: The document-to-input link is populated
- **WHEN** a new document is created through the API
- **THEN** its `kb.cdm_documents.input_record_id` is non-null and references
  the `kb.inputs` row created in the same transaction

#### Scenario: A new draft sits off both worklists
- **WHEN** a new document is created
- **THEN** its input row derives `parse_state = 'parsed_success'` and
  `pipeline_state = 'success'`, and neither worklist query returns it

#### Scenario: Failure writes neither row
- **WHEN** creation fails after the input row insert
- **THEN** no `kb.inputs` row and no `kb.cdm_documents` row remain

### Requirement: `document_key` is server-allocated and human-readable
The system SHALL allocate `document_key` at creation as `doc:<slug>` derived
from the document title, appending a numeric suffix when needed to satisfy the
global uniqueness `kb.cdm_documents.document_key` requires. Clients SHALL NOT
supply it.

#### Scenario: Key is derived from the title
- **WHEN** a document titled "Jaro-Winkler Similarity" is created
- **THEN** its `document_key` is `doc:jaro-winkler-similarity`

#### Scenario: Colliding titles get distinct keys
- **WHEN** a second document with the same title is created
- **THEN** it receives a distinct key and creation succeeds

### Requirement: Saving enforces optimistic concurrency
`Store.Save` SHALL accept the `content_version` the caller expects and SHALL
apply the increment only when the stored version still matches, within the same
statement as the increment. A mismatch SHALL be reported as a typed stale
version error carrying the expected and actual versions, and SHALL write
nothing.

#### Scenario: A current save succeeds and increments
- **WHEN** a document at `content_version` 7 is saved with expected version 7
- **THEN** the save succeeds and the response reports `content_version` 8

#### Scenario: A stale save is rejected
- **WHEN** a document at `content_version` 8 is saved with expected version 7
- **THEN** the save is rejected, the stored document is unchanged, and the
  error reports both the expected and the actual version

#### Scenario: Two concurrent saves do not both succeed
- **WHEN** two clients both load `content_version` 7 and both save
- **THEN** exactly one succeeds and the other is rejected as stale

### Requirement: Published documents are read-only
`Store.Save` SHALL refuse to write a document whose linked `kb.inputs` row is in
the published state, returning a typed frozen error naming the document. The
check SHALL live in the store so that every writer inherits it.

#### Scenario: Saving a published document is refused
- **WHEN** a save targets a published document
- **THEN** it is rejected with a frozen error and the stored document is
  unchanged

#### Scenario: Saving a draft is allowed
- **WHEN** a save targets a document that has not been published
- **THEN** it succeeds

### Requirement: Validation failures are reported per block
When a submitted document fails validation, the API SHALL return the full
violation list from `model.ValidationError` as structured JSON, not a flattened
string, so a client can attribute each violation to the block that caused it.

#### Scenario: All violations are returned
- **WHEN** a document with three distinct invariant violations is saved
- **THEN** the response lists all three rather than stopping at the first

#### Scenario: A duplicate block slug names the offending block
- **WHEN** a document containing two blocks with the same id is saved
- **THEN** the error identifies the duplicated slug

### Requirement: Publishing hands the document to the pipeline
`POST /api/v1/cdm/documents/:key/publish` SHALL perform the CDM §10.1 publish
transition on the existing input row — clearing the `doc_processing` status
entry so `pipeline_state` derives back to `pending` — and SHALL NOT create a new
input row.

#### Scenario: Publish enqueues for doc-processing only
- **WHEN** a draft is published
- **THEN** the doc-processing worklist returns it and the parse worklist does
  not

#### Scenario: Publish is a transition, not an insert
- **WHEN** a draft is published
- **THEN** the number of `kb.inputs` rows for that document is unchanged

### Requirement: Preview renders the published artifact on demand
`GET /api/v1/cdm/documents/:key/render` SHALL return the paginated SVG pages
produced by the same Typst path publishing uses, keyed by `content_version`, and
SHALL serve a cached rendering when one exists for that version. It SHALL NOT
render on every edit.

#### Scenario: Preview matches what publishing produces
- **WHEN** a document is previewed and then published at the same
  `content_version`
- **THEN** the SVG pages are identical

#### Scenario: A repeat preview is served from cache
- **WHEN** the same `content_version` is previewed twice
- **THEN** the second request does not invoke the Typst binary

#### Scenario: Editing invalidates the preview
- **WHEN** a document is saved, incrementing `content_version`
- **THEN** the next preview renders fresh rather than returning the prior
  version's pages

### Requirement: Documents are listed and scoped by tenant
`GET /api/v1/cdm/documents` SHALL list CDM documents scoped to the caller's
tenant, resolved through the linked `kb.inputs` row.

#### Scenario: Listing is tenant-scoped
- **WHEN** documents exist under two tenants
- **THEN** a caller sees only their own tenant's documents
