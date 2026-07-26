## ADDED Requirements

### Requirement: Typst rendering of Phase 1 block types
The renderer SHALL emit Typst source for a canonical document, supporting the
block types `heading`, `paragraph`, `list`, `table`, `code`, `equation`,
`image`, and `quote`, and the inline types `text`, `strong`, `emphasis`, `code`,
`link`, `math`, `citation`, `cross_reference`. Output SHALL begin by importing
the theme file and emitting the document title as a top-level heading.

#### Scenario: Document preamble is emitted
- **WHEN** any document is rendered
- **THEN** the output begins with the theme import followed by the escaped
  document title as a level-1 heading

#### Scenario: Heading level maps to Typst depth
- **WHEN** a `heading` block with `level` 3 is rendered
- **THEN** the output line begins with three `=` characters followed by a space

#### Scenario: Ordered and unordered lists use distinct markers
- **WHEN** a `list` block is rendered with `ordered` true, and again with false
- **THEN** the ordered form uses `+` markers and the unordered form uses `-`
  markers, and multi-line item bodies are indented to stay within their item

#### Scenario: Code blocks are emitted as Typst raw
- **WHEN** a `code` block is rendered
- **THEN** the output is a `#raw` call carrying the block's language and verbatim
  text, and is not a Markdown fence

#### Scenario: Callout delegates to the theme
- **WHEN** a `callout` block with `role` `warning` is rendered
- **THEN** the output is a `#callout` call receiving the role, the title, and the
  rendered children, and contains no font, color, or size properties

### Requirement: Deterministic output
Rendering SHALL be deterministic: the same document rendered by the same
renderer version SHALL produce byte-identical output. The renderer SHALL NOT
traverse Go maps in iteration order for any output-affecting decision.

#### Scenario: Repeated renders are byte-identical
- **WHEN** the same document is rendered one hundred times in one process
- **THEN** every output is byte-identical

#### Scenario: Table cells follow declared column order
- **WHEN** a table whose row `cells` map is populated in arbitrary order is
  rendered repeatedly
- **THEN** cells are emitted in the order of the declared `columns` every time

#### Scenario: Golden output is stable
- **WHEN** the renderer runs against the golden fixture documents
- **THEN** output matches the committed golden files exactly

### Requirement: Typst escaping
All author-supplied text emitted into Typst content SHALL be escaped so that it
cannot be interpreted as Typst markup or directives. Escaping SHALL cover at
minimum the backslash, `#`, `$`, `*`, `_`, backtick, `@`, `<`, `>`, `[`, and `]`
characters.

#### Scenario: Markup characters in text are neutralized
- **WHEN** a paragraph contains the literal text `#set page(width: 1pt)`
- **THEN** the rendered output escapes the `#` so Typst treats it as literal text
  rather than a directive

#### Scenario: Backslashes are escaped before other characters
- **WHEN** text contains a backslash immediately followed by `#`
- **THEN** both characters appear escaped in the output and the result is not
  double-escaped

#### Scenario: Verbatim content is not escaped
- **WHEN** a `code` block's text contains Typst metacharacters
- **THEN** they are passed through inside the raw call rather than escaped as
  markup

### Requirement: Equation rendering with fallback
The renderer SHALL render an equation from its `normalized` AST when present,
and otherwise fall back to its `original` source — passing `typst` source
through and converting `latex`. Display equations SHALL be emitted in Typst
display form and inline equations in inline form.

#### Scenario: Phase 1 equation falls back to original source
- **WHEN** an equation with `parse_status: "skipped"`, no `normalized`, and
  `original` in `typst` format is rendered
- **THEN** the original source is emitted inside Typst math delimiters

#### Scenario: Display and inline forms differ
- **WHEN** an equation with `display` true is rendered, and again with false
- **THEN** the display form is emitted with spaced `$ ... $` delimiters and the
  inline form without spacing

#### Scenario: Unrenderable equation is an error
- **WHEN** an equation has no `normalized` AST and an `original` whose `format`
  is unsupported
- **THEN** rendering fails with an error naming the block `id` and the format

### Requirement: Unsupported constructs fail loudly
The renderer SHALL return an error identifying the offending block when it
encounters a block type it does not support, rather than skipping it or emitting
partial output.

#### Scenario: Unsupported block type errors
- **WHEN** a document containing a block type outside the Phase 1 set is rendered
- **THEN** rendering returns an error naming the block `id` and its type, and no
  output is returned

#### Scenario: Nested failure is attributed to the inner block
- **WHEN** an unsupported block appears inside a `callout`'s children
- **THEN** the error identifies the inner block, not the enclosing callout

### Requirement: No presentation properties in output decisions
The renderer SHALL derive all styling from the Typst theme and the block's
semantic `type` and `role`. It SHALL NOT read font, color, size, margin, or
positioning properties from the canonical document.

#### Scenario: Styling comes from the theme
- **WHEN** a `callout` block is rendered
- **THEN** the emitted call passes only semantic values, and the visual treatment
  is defined in the theme file
