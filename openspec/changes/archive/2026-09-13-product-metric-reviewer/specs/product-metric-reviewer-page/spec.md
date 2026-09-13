## ADDED Requirements

### Requirement: Product Metric Reviewer page

The system SHALL provide a page at `/home3/product-metric-review` presenting, for a selected
profile and run: a scope-tree pane, a results pane, and an evidence drawer. The page SHALL ship
light and dark themes and SHALL localize its chrome through the existing i18n message layer, with
aspect labels taken from the server's locale-resolved aspect vocabulary.

#### Scenario: Page loads a completed run

- **WHEN** a user opens the page with a run id
- **THEN** the scope tree, the results, and the run's report are rendered

#### Scenario: Page renders in both themes

- **WHEN** the page is viewed under a dark theme
- **THEN** every surface renders with its dark palette and no unstyled region

### Requirement: Scope-tree editing

The scope-tree pane SHALL render the profile's nodes hierarchically, showing each node's kind,
origin, grounding state, and artifact count for the current run. It SHALL let a user accept,
reject, edit, add, and delete nodes, and SHALL indicate when an edit has moved the profile ahead of
the displayed run's pinned version.

#### Scenario: Origin and grounding are visible per node

- **WHEN** the scope tree is rendered
- **THEN** each node shows whether it was proposed by the model, expanded from the graph, or added
  by a user, and whether it is grounded

#### Scenario: Editing after a run warns of staleness

- **WHEN** a user edits the profile while viewing a completed run
- **THEN** the page indicates the displayed run used an earlier profile version and offers a re-run

#### Scenario: Ambiguous grounding is surfaced

- **WHEN** a node grounded to an object whose reconcile status is `ambiguous`
- **THEN** the node is visibly flagged in the tree

### Requirement: Results exploration

The results pane SHALL group results by scope node and SHALL let a user filter by node, tier, path,
document, and text, and sort by score. The `document_scope` tier SHALL be collapsed by default.
Selecting a scope node in the tree SHALL filter the results to that node.

#### Scenario: Selecting a node filters results

- **WHEN** a user selects the `Display` node in the scope tree
- **THEN** the results pane shows only results matched to `Display`

#### Scenario: Scoped-only results are collapsed by default

- **WHEN** a run's results are first rendered
- **THEN** `document_scope` results are collapsed behind an expandable group showing their count

#### Scenario: Empty result set is explained

- **WHEN** a run produced no results
- **THEN** the pane states that no artifacts matched and links to the run's gap list

### Requirement: Evidence drill-down

Selecting a result SHALL open an evidence drawer showing the artifact's fields, its document title
and number, its source line spans, its inclusion reason, and the paths that found it. The drawer
SHALL offer navigation to the source document at the cited location using the existing document and
PDF-locator surfaces.

#### Scenario: Evidence names why the result was included

- **WHEN** a user opens a `document_scope` result
- **THEN** the drawer states that the artifact matched no scope node and names the document and the
  reason the document was in scope

#### Scenario: Navigation to the cited location

- **WHEN** a user activates the source link on a result with line spans
- **THEN** the source document opens at the cited location

### Requirement: Report and gap view

The page SHALL present the run's report: the per-node coverage table, the gap list, the run's
counts including truncation, and the diff against the previous run when one exists.

#### Scenario: Gaps are presented as findings

- **WHEN** the report tab is opened for a run with gaps
- **THEN** the nodes with zero artifacts are listed with their grounding state

#### Scenario: Truncation is disclosed

- **WHEN** a run truncated results against a cap
- **THEN** the report view states how many results were truncated

#### Scenario: Diff is shown when a prior run exists

- **WHEN** the displayed run is not the first run of its request
- **THEN** the report view shows artifacts added, removed, and re-tiered since the previous run
