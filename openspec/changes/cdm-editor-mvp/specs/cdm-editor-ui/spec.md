## ADDED Requirements

### Requirement: The editor's document state is the CDM AST

The editor SHALL hold the document as an array of CDM blocks whose field names
and shapes match `model.Document`'s JSON encoding, so that saving is a
serialization rather than a conversion. The editor SHALL NOT maintain a separate
view model that must be kept in sync with the AST.

#### Scenario: Loading and saving is lossless

- **WHEN** a document is loaded into the editor and saved with no edits
- **THEN** the submitted JSON is equivalent to what was loaded, apart from
  `content_version`

#### Scenario: Editor types match the Go definitions

- **WHEN** the shared fixture documents are parsed by the editor's TypeScript
  types and re-serialized
- **THEN** the result round-trips against the same fixtures the Go tests use,
  so drift between the two type definitions is detected

### Requirement: Block structure is owned by the editor, not by a rich-text engine

The editor SHALL own block insertion, deletion, reordering, type changes, and
selection. A rich-text engine SHALL be used only for the inline content of
blocks that carry inline content.

#### Scenario: Block operations do not involve the inline editor

- **WHEN** a block is inserted, deleted, or reordered
- **THEN** the change is made to the block array directly and no rich-text
  engine instance mediates it

#### Scenario: Structured blocks use purpose-built editors

- **WHEN** a `table`, `list`, `code`, `equation`, `image`, or `callout` block is
  edited
- **THEN** it is edited through fields corresponding to its typed CDM
  properties, not through free rich-text input

### Requirement: The inline editor cannot express presentation

The inline editor's schema SHALL contain exactly CDM's inline vocabulary —
`text`, `strong`, `emphasis`, `code`, `link`, `math`, `citation`,
`cross_reference` — and SHALL NOT define any mark or node carrying font, size,
colour, alignment, or spacing.

#### Scenario: Presentation marks are unavailable

- **WHEN** the inline editor's schema is inspected
- **THEN** it defines no mark for font, size, colour, or alignment, so such a
  mark cannot be produced by any editor action or paste

#### Scenario: Pasted formatting is discarded

- **WHEN** rich text carrying font and colour styling is pasted into a block
- **THEN** the text is retained and the presentation styling is dropped

#### Scenario: Every inline type round-trips

- **WHEN** content using each of the eight CDM inline types is edited and
  serialized
- **THEN** each type maps to its CDM representation and back without loss

### Requirement: Block IDs are allocated at creation and never change

The editor SHALL assign each block a human-readable slug when the block is
created, derived from its heading text where available and otherwise from its
type, with a disambiguating suffix on collision. Editing a block's content
SHALL NOT change its id.

#### Scenario: Editing preserves block identity

- **WHEN** a block's content is edited and the document is saved
- **THEN** the block's id is unchanged from before the edit

#### Scenario: A new block receives a readable slug

- **WHEN** a heading block with the text "Score Range" is created
- **THEN** its id is a readable slug derived from that text, not a UUID

#### Scenario: A rejected duplicate slug is attributed

- **WHEN** a save is rejected because a block slug collides
- **THEN** the editor identifies the offending block to the author

### Requirement: The editor performs the full authoring loop

The editor SHALL support creating a document, editing every CDM Phase 1 block
type, saving, publishing, and previewing the rendered result.

#### Scenario: All Phase 1 block types are editable

- **WHEN** a document containing `heading`, `paragraph`, `list`, `table`,
  `code`, `equation`, `image`, `quote`, and `callout` is opened
- **THEN** each block is rendered and editable

#### Scenario: The loop completes

- **WHEN** an author creates a document, adds content, saves, and publishes
- **THEN** the document is stored, published, and its rendered pages are
  viewable

### Requirement: The editor exposes semantic actions, never formatting controls

The editing toolbar SHALL offer semantic actions — heading, emphasis, strong,
list, table, code, quote, link, equation, image, callout — and SHALL NOT offer
font, size, colour, or alignment controls.

#### Scenario: No formatting controls are present

- **WHEN** the editing toolbar is inspected
- **THEN** it contains no font, size, colour, or alignment control

### Requirement: A stale or refused save is explained to the author

The editor SHALL send the `content_version` it loaded with every save, and SHALL
tell the author what happened when a save is refused rather than failing
silently or discarding their work.

#### Scenario: A stale save is reported

- **WHEN** a save is rejected because the document changed elsewhere
- **THEN** the author is told the document changed and their local content is
  preserved

#### Scenario: A frozen document is explained

- **WHEN** the author edits a published document and saves
- **THEN** the editor explains that published documents are read-only and that
  continuing requires a new version

### Requirement: Preview updates live without flashing

The editor SHALL open preview automatically for an existing document and
debounce live rendering while the author edits. Live preview SHALL render the
in-memory draft without saving it. While a newer render is in flight, the
currently rendered SVG pages SHALL remain mounted; only the newest completed
response SHALL update the preview.

#### Scenario: Preview follows editing

- **WHEN** the author types in a block
- **THEN** a debounced draft-preview request is issued and the completed SVG
  is displayed without clearing the previous preview

#### Scenario: Preview shows rendered pages

- **WHEN** the author opens a saved document
- **THEN** the rendered pages are displayed, including the generated table of
  contents and figure/table/formula lists

#### Scenario: Typing continues during rendering

- **WHEN** the author edits again before a preview request completes
- **THEN** the obsolete request is canceled and cannot replace the newer
  preview

#### Scenario: Live preview does not save

- **WHEN** live preview renders unsaved edits
- **THEN** the document remains dirty and the Save button remains enabled

### Requirement: Editor strings are localizable

**Deferred (task 8.2, 2026/07/27), not implemented in this change.** Before
implementing this requirement, a grep across the whole frontend found that no
`/home3/*` feature uses Paraglide today — it is used only on the public
`/semos` marketing pages (42 message keys, all `semos_*`-prefixed); every
`home3` view component (`doc-review-report-view.svelte`,
`inputs-mgmt-view.svelte`, `document-review-view.svelte`, ...) hard-codes
English strings directly. Implementing this requirement as originally written
would have made the CDM editor the only `home3` feature with i18n, contrary
to this project's own "match existing style" guidance. Presented to the user
as an explicit choice rather than decided silently; the user chose to match
the `home3` precedent instead. CDM editor strings are hard-coded English,
same as every sibling `home3` feature. i18n for all of `home3` (CDM included)
remains a valid, separate future change if wanted — this requirement is not
rejected, only not undertaken piecemeal for one feature.

~~User-visible strings in the editor SHALL go through the existing Paraglide
i18n mechanism rather than being hard-coded.~~

~~#### Scenario: No hard-coded user-facing strings~~
~~- **WHEN** the editor components are inspected~~
~~- **THEN** user-visible labels resolve through the i18n mechanism~~
