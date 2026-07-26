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

### Requirement: Preview shows the rendered document on request
The editor SHALL show the server-rendered SVG pages on an explicit preview
action, and SHALL NOT trigger rendering on every keystroke.

#### Scenario: Preview is explicit
- **WHEN** the author types in a block
- **THEN** no render request is issued

#### Scenario: Preview shows rendered pages
- **WHEN** the author requests a preview of a saved document
- **THEN** the rendered pages are displayed, including the generated table of
  contents and figure/table/formula lists

### Requirement: Editor strings are localizable
User-visible strings in the editor SHALL go through the existing Paraglide i18n
mechanism rather than being hard-coded.

#### Scenario: No hard-coded user-facing strings
- **WHEN** the editor components are inspected
- **THEN** user-visible labels resolve through the i18n mechanism
