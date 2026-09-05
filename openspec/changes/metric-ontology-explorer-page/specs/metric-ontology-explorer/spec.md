## ADDED Requirements

### Requirement: Menu placement under Ontology

The `home3/knowledge` sidebar SHALL expose a section **Metric Ontology Explorer** with id
`kb-metric-ontology-explorer` as a child of the existing `Ontology` menu group
(`kb-metric-ontology`), listed after the existing `Metric Ontology` child. Selecting it SHALL
render the `MetricOntologyExplorerView` component in the content area and SHALL leave the existing
`Metric Ontology` dashboard (`kb-metric-ontology`) unchanged. The hardcoded default label SHALL be
`Metric Ontology Explorer`, overridable through the existing `[knowledge-content]` menu-label
mechanism.

#### Scenario: Explorer appears in the Ontology group

- **WHEN** a user opens `home3/knowledge` and expands the `Ontology` menu group
- **THEN** the group shows `Metric Ontology` followed by `Metric Ontology Explorer`
- **AND** choosing `Metric Ontology Explorer` sets the active section to
  `kb-metric-ontology-explorer` and renders the explorer view

#### Scenario: Existing dashboard is untouched

- **WHEN** a user chooses `Metric Ontology`
- **THEN** the existing `MetricOntologyAnalysisView` renders exactly as before this change

#### Scenario: Menu label is localizable

- **WHEN** a `[knowledge-content]` labels table provides an override for
  `kb-metric-ontology-explorer` in the active locale
- **THEN** the sidebar shows the overridden label instead of `Metric Ontology Explorer`

### Requirement: Three-pane resizable workspace

The explorer view SHALL present three panes — a **canvas**, a **content viewer**, and a
**source-document viewer** — in one of two arrangements:

- **Left-Right**: a left column split top/bottom (canvas above, content viewer below) and a
  right column holding the source-document viewer.
- **Top-Bottom**: the canvas as a full-width top band, and a bottom row split left/right
  (content viewer left, source-document viewer right).

The boundaries between panes SHALL be drag-resizable, each pane SHALL have a minimum size that
drag cannot cross, and the chosen arrangement and pane sizes SHALL persist across reloads in the
same browser. A control SHALL switch between the two arrangements. A control SHALL reset pane
sizes to their defaults.

#### Scenario: Switch arrangement

- **WHEN** the workspace is in Left-Right and the user activates the arrangement control
- **THEN** the same three panes re-lay-out into Top-Bottom without losing the current canvas
  selection, open content tabs, or loaded document

#### Scenario: Resize a pane

- **WHEN** the user drags the splitter between two panes
- **THEN** the two panes resize together, neither shrinks below its minimum, and the canvas
  re-fits its content to the new pane size

#### Scenario: Sizes persist

- **WHEN** the user has resized panes and/or switched arrangement, then reloads the page
- **THEN** the workspace restores the last arrangement and pane sizes

#### Scenario: Reset

- **WHEN** the user activates the reset control
- **THEN** pane sizes return to their defaults while the arrangement is kept

### Requirement: Canvas orrery of the metric ontology

The canvas SHALL render a pan/zoom diagram with a central **Metric** node and nine satellite
nodes — **Document**, **Object**, **Keyword**, **Evidence**, **Metric Definition**, **Processor**,
**Product**, **Analysis**, **Misc** — each connected to Metric by an edge. Every node SHALL be a
rectangle containing an icon and its label. The Metric node SHALL be rendered larger than the
satellites. The canvas SHALL support wheel-zoom and drag-pan over an effectively unbounded area,
a control to re-fit the diagram, and SHALL re-fit when its pane is resized. The node set, the
edges, and each satellite's chain topology SHALL be defined statically in the component (no
network fetch shapes the diagram).

#### Scenario: Diagram renders

- **WHEN** the explorer view first renders
- **THEN** the canvas shows the Metric node at centre with the nine named satellites around it,
  each a rectangle with an icon and a label, Metric visibly larger than the satellites

#### Scenario: Pan and zoom

- **WHEN** the user scrolls the wheel over the canvas or drags its background
- **THEN** the diagram zooms toward the pointer / pans, and the re-fit control returns it to a
  framed view of all visible nodes

#### Scenario: Selecting a node drives the other panes

- **WHEN** the user clicks the Metric node or a satellite
- **THEN** that node becomes the focused node, the content viewer's entry tab shows its entry,
  and the source-document viewer updates per the source-document binding requirement

### Requirement: Unfoldable downstream chains

Seven satellites SHALL each unfold a fixed linear **chain** of downstream nodes when clicked, and
fold it when clicked again. At most one chain SHALL be open at a time — opening a chain SHALL fold
any other. While a chain is open, its satellite SHALL move to a reading position and the chain's
nodes SHALL be laid out clear of the other nodes; folding SHALL remove the chain's nodes and close
any record tabs opened from it. The four remaining satellites — Document, Product, Analysis, Misc —
SHALL have no chain; clicking them SHALL only focus them.

The chains SHALL be exactly:

- **Object** → `Object Mention` → `Object Node`
- **Keyword** → `Keyword Concept`
- **Metric Definition** → `Ontology Term` → `Class Contract Revision`
- **Processor** → `Extract Metrics` → `Normalize Assertion` → `Associate Semantics` →
  `Project Semantics`
- **Evidence** → `Assertion Evidence` → `Decision Candidate` → `Semantic Assertion`
- **Document** and the rest: no chain
  <!-- Product, Analysis, Misc: no chain -->

A breadcrumb SHALL show the currently open chain as `Metric · <satellite> ▸ <node> ▸ …`, and its
crumbs SHALL be clickable to open that chain node's record tab.

#### Scenario: Object unfolds and folds

- **WHEN** the user clicks the `Object` satellite
- **THEN** `Object Mention` and `Object Node` appear as chained nodes extending from `Object`
- **WHEN** the user clicks `Object` again
- **THEN** `Object Mention` and `Object Node` are removed and any tabs they opened are closed

#### Scenario: Only one chain open at a time

- **WHEN** the `Object` chain is open and the user clicks the `Keyword` satellite
- **THEN** the `Object` chain folds and `Keyword Concept` unfolds from `Keyword`

#### Scenario: Processor chain has four downstream nodes in order

- **WHEN** the user clicks the `Processor` satellite
- **THEN** the canvas shows `Extract Metrics`, `Normalize Assertion`, `Associate Semantics`, and
  `Project Semantics` chained in that order from `Processor`, laid out without overlapping other
  nodes

#### Scenario: Chainless satellites just focus

- **WHEN** the user clicks `Document`, `Product`, `Analysis`, or `Misc`
- **THEN** the node becomes focused and no chain unfolds

#### Scenario: Breadcrumb reflects the open chain

- **WHEN** the `Evidence` chain is open
- **THEN** a breadcrumb reads `Metric · Evidence ▸ Assertion Evidence ▸ Decision Candidate ▸
  Semantic Assertion`
- **AND** clicking the `Assertion Evidence` crumb opens that node's record tab

### Requirement: Content viewer with an entry tab and per-node record tabs

The content viewer SHALL always show an **entry tab** for the focused node — an editorial entry
with Definition, Connection, Processing, and Sources sections. Clicking a chain node SHALL open a
**record tab** for that node, activate it, and keep it open until the reader closes it or its
chain folds. Record tabs SHALL be individually closeable. Re-clicking a chain node whose tab is
already open SHALL re-activate that tab rather than duplicate it.

#### Scenario: Entry tab follows the focused node

- **WHEN** the user focuses the `Keyword` satellite
- **THEN** the entry tab shows the `Keyword` entry with its Definition/Connection/Processing/
  Sources sections

#### Scenario: Opening a record tab

- **WHEN** the user clicks the `Object Mention` chain node
- **THEN** a record tab titled `Object Mention` opens next to the entry tab and becomes active

#### Scenario: Record tabs are closeable and de-duplicated

- **WHEN** a record tab is already open and the reader clicks its chain node again
- **THEN** the existing tab is re-activated, not duplicated
- **WHEN** the reader closes a record tab
- **THEN** it is removed and the entry tab is shown if it was the active one

### Requirement: Record tabs read live data where an endpoint exists, otherwise show a schema panel

A record tab whose `kb.*` table has an existing read endpoint SHALL show live rows from it. The
tables that SHALL be live are `kb.metrics`, `kb.inputs`, `kb.artifact_objects`, `kb.object_nodes`,
`kb.keyword_concepts`, `kb.ontology_terms`, `kb.semantic_assertions`, and the ontology-analysis
summary. A record tab whose data has no read endpoint yet SHALL show a **schema panel** — the
table name, its column list, and a one-line description — and SHALL NOT fabricate rows. The tables
that SHALL show a schema panel in this change are `kb.class_contract_revisions`,
`kb.assertion_evidence`, `kb.semantic_decision_candidates`, a `kb.metric_definitions` listing, and
metric-scoped processor run logs. Live record tabs SHALL surface load and error states without
breaking the rest of the workspace.

#### Scenario: Live record tab

- **WHEN** the reader opens the `Keyword Concept` record tab
- **THEN** it shows rows fetched from the `kb.keyword_concepts` read endpoint, with visible
  loading and error states

#### Scenario: Schema-panel record tab

- **WHEN** the reader opens the `Class Contract Revision` record tab
- **THEN** it shows the table name `kb.class_contract_revisions`, its column list, and a short
  description, and shows no data rows

#### Scenario: A failing record fetch is contained

- **WHEN** a live record tab's fetch fails
- **THEN** that tab shows an error state and the canvas, entry tab, other tabs, and the
  source-document viewer keep working

### Requirement: Source-document viewer bound to the focused metric with evidence highlighting

The source-document pane SHALL embed the existing `shared-pdf-viewer`, loading the document of the
focused metric's source input via the existing input-file endpoint. When the focused node or an
open record tab identifies evidence spans, those spans SHALL be highlighted in the document and
the first SHALL be scrolled into view on request. Activating a highlight SHALL focus the
originating node. When the focused node has no associated document, the pane SHALL show an empty
state rather than an error.

#### Scenario: Document loads for the focused metric

- **WHEN** the Metric node (or a node that resolves to a metric with a source input) is focused
- **THEN** the source-document pane loads that input's document in `shared-pdf-viewer`

#### Scenario: Evidence highlights sync with the canvas

- **WHEN** the `Assertion Evidence` record tab is active and reports evidence spans
- **THEN** those spans are highlighted in the document, and a control scrolls the first into view

#### Scenario: No document

- **WHEN** the focused node has no associated source document
- **THEN** the pane shows an empty state and no error is raised

### Requirement: No new frontend dependency

The canvas SHALL be implemented as hand-rolled inline SVG driven by Svelte 5 runes. This change
SHALL NOT add any npm dependency (in particular no graph, charting, or force-layout library) and
SHALL NOT introduce a new global design token — it SHALL reuse the knowledge view's existing
theme tokens for light and dark mode.

#### Scenario: Dependency set unchanged

- **WHEN** this change is reviewed
- **THEN** `ChenWeb/web/package.json` has no added dependency and the canvas renders from inline
  SVG and component state only

#### Scenario: Theme parity

- **WHEN** the knowledge view is in dark mode
- **THEN** the explorer canvas, panes, and tabs render with the existing dark-mode tokens with no
  new token added
