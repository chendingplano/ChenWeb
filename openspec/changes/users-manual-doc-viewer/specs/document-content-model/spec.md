## ADDED Requirements

### Requirement: Type-discriminated document model
The system SHALL define a `Document` type discriminated by a `type` field with at least the values `markdown`, `template-json`, `typst`, and `html`, so that any content-viewing UI can handle a document without knowing its concrete shape in advance.

#### Scenario: A markdown document carries raw markdown text
- **WHEN** a document of `type: 'markdown'` is constructed
- **THEN** it carries an `id` and a `markdown` string field usable by a markdown renderer

#### Scenario: Non-markdown document types are represented but not necessarily renderable
- **WHEN** a document of `type: 'template-json'`, `'typst'`, or `'html'` is constructed
- **THEN** the model accepts and type-checks its shape (e.g. `template-json` carries `templateId` and `data`) even though no renderer implementation is required for these types in this change

### Requirement: Document rendering is dispatched by type through one entry point
The system SHALL provide a single `renderDocument(doc)` function that dispatches on `doc.type` and returns the document's displayable HTML, so viewer components never need type-specific branching of their own.

#### Scenario: Rendering a markdown document
- **WHEN** `renderDocument` is called with a `markdown` document
- **THEN** it returns HTML produced by rendering that document's markdown content

#### Scenario: Rendering a not-yet-supported document type
- **WHEN** `renderDocument` is called with a `template-json`, `typst`, or `html` document
- **THEN** it raises a clear "not yet supported" error rather than silently returning empty or incorrect output

### Requirement: Document supply is abstracted behind a DocumentSource interface
The system SHALL define a `DocumentSource` interface exposing a tree listing (`listTree()`) and per-id document lookup (`getDocument(id)`), so that where documents come from (bundled files now, a database later) is decoupled from how they are rendered or browsed.

#### Scenario: A viewer consumes any DocumentSource the same way
- **WHEN** a viewer component is given a `DocumentSource`
- **THEN** it can render the source's tree and, for a selected leaf id, fetch and render that document without needing to know whether the source is file-backed or database-backed
