## ADDED Requirements

### Requirement: Canonical document structure

The system SHALL represent a document as a `Document` value containing
`document_key`, `title`, `language`, `schema_version`, `content_version`, a
`metadata` object, and an ordered list of `blocks`, encoded as JSON per CDM
spec §1. The encoding SHALL round-trip losslessly.

#### Scenario: JSON round-trip preserves the document

- **WHEN** a valid canonical document is marshalled to JSON and unmarshalled back
- **THEN** the resulting `Document` is deeply equal to the original, including
  block order, nesting, and all optional fields that were set

#### Scenario: Schema version is recorded

- **WHEN** a document is created by the system
- **THEN** its `schema_version` is `"1.0"`

#### Scenario: Metadata uses the normative field names

- **WHEN** a document's metadata is encoded
- **THEN** the template-selection field is named `rendering_type`, `authors` is a
  JSON array of strings, and `create_time` / `modify_time` are RFC 3339 UTC
  timestamps

### Requirement: Identifier rules

The system SHALL distinguish three identifier kinds per CDM spec §1.1: the
database surrogate key, the globally stable `document_key`, and the per-document
block slug `Block.id`. Block IDs SHALL be text slugs, MUST be non-empty, and
MUST be unique within their document. The database surrogate key SHALL NOT
appear in canonical JSON.

#### Scenario: Duplicate block ID is rejected

- **WHEN** a document contains two blocks with the same `id`, at any nesting depth
- **THEN** validation fails with an error naming the duplicated `id`

#### Scenario: Empty block ID is rejected

- **WHEN** a document contains a block whose `id` is empty
- **THEN** validation fails with an error identifying the block's position

#### Scenario: Canonical JSON omits the surrogate key

- **WHEN** a document loaded from storage is encoded to canonical JSON
- **THEN** the output contains `document_key` and contains no database row id

### Requirement: Content model exclusivity

A block SHALL carry inline payload in `content`, OR nested blocks in `children`,
OR list items in `items` — never more than one of the three. `paragraph`,
`heading`, and `quote` use `content`; semantic container blocks use `children`;
`list` uses `items`.

#### Scenario: Block populating two payload fields is rejected

- **WHEN** a block sets both `content` and `children`
- **THEN** validation fails with an error naming the block `id` and the
  conflicting fields

#### Scenario: Paragraph with inline content is accepted

- **WHEN** a `paragraph` block sets only `content`
- **THEN** validation succeeds

#### Scenario: List items are validated recursively

- **WHEN** a `list` block contains an item whose nested block is invalid
- **THEN** validation fails and the error identifies the nested block

### Requirement: Type vocabulary

The system SHALL accept only the block types `paragraph`, `heading`, `list`,
`table`, `equation`, `code`, `quote`, `image`, `callout`, and only the inline
types `text`, `line_break`, `strong`, `emphasis`, `code`, `link`, `math`,
`citation`, `cross_reference`. A `callout` block MAY carry a `role` drawn from
`note`, `tip`, `important`, `warning`, `caution`. `warning` SHALL NOT be
accepted as a block type.

#### Scenario: Unknown block type is rejected

- **WHEN** a document contains a block with type `horizontal_stack`
- **THEN** validation fails with an error naming the unsupported type

#### Scenario: Warning as a block type is rejected

- **WHEN** a document contains a block with type `warning`
- **THEN** validation fails, directing the author to `callout` with
  `role: "warning"`

#### Scenario: Callout with a valid role is accepted

- **WHEN** a `callout` block sets `role` to `warning` and populates `children`
- **THEN** validation succeeds

#### Scenario: Inline line break is accepted

- **WHEN** a `paragraph` block contains a `line_break` inline node between two
  text nodes
- **THEN** validation succeeds

#### Scenario: Heading level is bounded

- **WHEN** a `heading` block has a `level` outside the range 1 to 6
- **THEN** validation fails with an error naming the block `id`

### Requirement: Table integrity

Every `table` block SHALL declare `columns` with unique, non-empty `key` values,
and every row's `cells` keys SHALL be a subset of those declared keys. A
declared key absent from a row SHALL be treated as an empty cell rather than an
error.

#### Scenario: Cell key not matching any column is rejected

- **WHEN** a table row has a cell keyed `total` and no column declares key `total`
- **THEN** validation fails with an error naming the unexpected key

#### Scenario: Missing cell key is allowed

- **WHEN** a table declares three columns and a row supplies only two cells
- **THEN** validation succeeds and the absent cell is treated as empty

#### Scenario: Duplicate column key is rejected

- **WHEN** a table declares two columns with the same `key`
- **THEN** validation fails with an error naming the duplicated key

### Requirement: Equation well-formedness

Every `equation` block SHALL carry a `math` value with a `parse_status` of
`success`, `failed`, or `skipped`, and at least one of `normalized` or
`original`. When `original` is present its `format` SHALL be one of `latex`,
`typst`, `asciimath`. Numeric literals in a normalized AST SHALL be exact
decimal strings.

#### Scenario: Equation with neither representation is rejected

- **WHEN** an `equation` block has `math` with neither `normalized` nor `original`
- **THEN** validation fails with an error naming the block `id`

#### Scenario: Unknown parse status is rejected

- **WHEN** an equation's `parse_status` is `partial`
- **THEN** validation fails with an error listing the permitted values

#### Scenario: Phase 1 skipped equation is accepted

- **WHEN** an equation carries `original` with format `latex` and
  `parse_status: "skipped"` and no `normalized`
- **THEN** validation succeeds

### Requirement: Validation reports all failures

The validator SHALL check a document against every invariant in CDM spec §1.2
and SHALL report all violations found, not only the first. Each violation SHALL
identify the offending block by `id` where one exists.

#### Scenario: Multiple violations are all reported

- **WHEN** a document has a duplicate block ID and an unknown block type
- **THEN** validation fails and the returned error describes both violations

#### Scenario: Valid document passes

- **WHEN** a document satisfies every invariant
- **THEN** validation returns no error
