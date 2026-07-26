## ADDED Requirements

### Requirement: Document lifecycle
A CDM document SHALL progress through the states `editing`, `published`,
`rendered`, `line_file_generated`, and then into the standard doc-process
pipeline. A document's `kb.inputs` row SHALL be created with the document, in
`editing`, so that author-triggered extraction has an input row to attach
artifacts to. While `editing`, the row's status SHALL derive both
`parse_state='parsed_success'` and `pipeline_state='success'`, keeping it off
every worklist. Publish SHALL clear the `doc_processing` status entry so that
`pipeline_state` derives back to `'pending'`.

#### Scenario: Draft is invisible to the pipeline
- **WHEN** a document is created and left in `editing`
- **THEN** a `kb.inputs` row with `type='cdm'` exists for it, and no worklist
  selects it

#### Scenario: A draft's input row can host an on-demand processing run
- **WHEN** the doc-process pipeline is invoked directly against a draft's
  `kb.inputs` row, outside of either worklist
- **THEN** it runs to completion and attaches artifacts keyed to the draft's
  current `content_version`, and the draft's `parse_state` and `pipeline_state`
  remain unchanged by the run

*(The editor action that triggers such a run, and reconciliation of its output
against author-marked artifacts, are CDM Editor scope — ADR 2026072602 DR5b,
DR5c — not built in this change.)*

#### Scenario: Publish enqueues the document for doc-processing
- **WHEN** a document transitions from `editing` to `published`
- **THEN** the document's `kb.inputs` row has its `doc_processing` status entry
  cleared, `pipeline_state` derives to `'pending'`, and the document advances
  through rendering and line-file generation

#### Scenario: Published document enters doc-processing
- **WHEN** publish completes for a CDM document
- **THEN** the doc-processing worklist selecting
  `parse_state='parsed_success' AND pipeline_state='pending'` returns it, on the
  same terms as an uploaded document

#### Scenario: Republish supersedes prior derived artifacts
- **WHEN** an already-published document is edited and republished
- **THEN** `content_version` increments and the anchor map, SVG pages, and line
  file for the new version do not overwrite those of the prior version

### Requirement: Line file generation
Publishing SHALL generate a line file from the canonical AST, in the dialect the
existing extractors consume. Each line SHALL correspond to exactly one
anchorable unit — a paragraph, list item, table row, equation, heading, or code
block.

#### Scenario: Every unit becomes one line
- **WHEN** a document with three paragraphs and a four-row table is published
- **THEN** the line file contains one line per paragraph and one line per table
  row

#### Scenario: Line file drives existing extraction unchanged
- **WHEN** the doc-process pipeline runs over a CDM document's line file
- **THEN** extracted artifacts carry `source_line_spans` referring to those
  lines, with no CDM-specific handling in the extractors

### Requirement: Anchor map
Rendering SHALL produce an anchor map giving, for every line-file unit, its
location as `{page, x, y, w, h}` in points. The anchor map and the line file
SHALL be generated from the same AST in the same operation. Every line in the
line file SHALL have exactly one anchor.

#### Scenario: Anchor map covers every line
- **WHEN** a document is published
- **THEN** the anchor map contains exactly one entry per line-file line, and no
  entry lacks a corresponding line

#### Scenario: Coordinates are page-relative
- **WHEN** a document's content flows onto a second page
- **THEN** units on page two report `page: 2` with `y` measured from that page's
  origin, not cumulatively

#### Scenario: Anchor map is versioned with its render
- **WHEN** an anchor map is stored
- **THEN** it records the `content_version` and `renderer_version` it was
  produced from, so a stale map is detectable

### Requirement: Units spanning a page break
A unit that breaks across pages SHALL be recorded with paired start and end
marks, and the system SHALL derive one highlight fragment per page it occupies.
A single position plus a measured height SHALL NOT be used, because the measured
height of a unit in isolation does not reflect its laid-out extent.

#### Scenario: Page-spanning unit yields two fragments
- **WHEN** a unit starts on page 1 and ends on page 2
- **THEN** the anchor map yields a fragment on page 1 running from the start to
  the page content edge, and a fragment on page 2 from the content origin to the
  end mark

#### Scenario: Single-page unit yields one fragment
- **WHEN** a unit starts and ends on the same page
- **THEN** exactly one fragment is produced

### Requirement: Paginated page rendering
Rendering SHALL produce paginated SVG pages whose coordinate space matches the
anchor map, so that a highlight positioned from anchor coordinates aligns with
the rendered content. Typst HTML export SHALL NOT be used as the viewer target.

#### Scenario: Page geometry matches the anchor map
- **WHEN** an SVG page is rendered and a rectangle is positioned at an anchor's
  coordinates
- **THEN** the rectangle aligns with the anchored content

#### Scenario: One SVG per page
- **WHEN** a three-page document is rendered
- **THEN** three SVG pages are produced, each carrying its page box

#### Scenario: HTML export is not the viewer target
- **WHEN** the render target is selected for viewing
- **THEN** SVG is used, because Typst HTML export discards pagination and is
  documented as not production-ready

### Requirement: Navigate and highlight parity
Given an artifact's `source_line_spans`, the system SHALL resolve them to
highlight fragments for a CDM document using the same contract the PDF viewer
consumes for uploaded documents: a page number and a bounding box per fragment.

#### Scenario: Artifact resolves to a highlight location
- **WHEN** an artifact extracted from a CDM document is selected
- **THEN** its `source_line_spans` resolve, via the anchor map, to one or more
  `{page, x, y, w, h}` fragments

#### Scenario: Multi-line span produces contiguous fragments
- **WHEN** an artifact's span covers three consecutive lines on one page
- **THEN** the resolved fragments cover all three units

#### Scenario: Contract matches the uploaded-document path
- **WHEN** a highlight request is resolved for a CDM document and for an
  uploaded document
- **THEN** both return the same `{page, bbox}` shape, so a single viewer can
  render either without origin-specific logic
