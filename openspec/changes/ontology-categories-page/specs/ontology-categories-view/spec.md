## ADDED Requirements

### Requirement: Ontology menu item is named Ontology
The `home3/knowledge` Ontology section SHALL show its first child item as "Ontology" in English and "本体" in Chinese, and SHALL NOT show the label "Metric Ontology" for that item, including when a `kb.page_config` label override is present.

#### Scenario: Renamed item in English
- **WHEN** a user with the English locale opens `home3/knowledge` and expands Ontology
- **THEN** the first child item is labelled "Ontology"

#### Scenario: Renamed item overrides stay consistent
- **WHEN** the `kb.page_config` rows for `kb-metric-ontology` are read in both languages
- **THEN** their label is "Ontology" (en) and "本体" (zh-cn)

### Requirement: Categories page lists all category trees
The system SHALL provide a Categories child item under Ontology. The page SHALL list every category tree returned by `GET /api/kb/category-trees`, showing each tree's canonical name and its preferred label in the active language.

#### Scenario: Trees are listed
- **WHEN** the Categories page loads and the API returns trees
- **THEN** every tree is listed once

#### Scenario: No trees exist
- **WHEN** the API returns an empty tree list
- **THEN** the page shows an empty-state message and no chart

#### Scenario: Load failure is reported
- **WHEN** the tree list request fails
- **THEN** the page shows an error message with a retry action

### Requirement: Selected tree is drawn with a chosen layout
When a user selects a tree, the page SHALL load it from `GET /api/kb/category-trees/:name` and draw it as an ECharts tree. A pulldown SHALL let the user choose one of four layouts: left-right (`LR`), right-left (`RL`), top-bottom (`TB`) and bottom-top (`BT`). The default layout SHALL be left-right. Changing the pulldown SHALL redraw the same tree without refetching it.

#### Scenario: Default layout
- **WHEN** a user selects a tree and has not changed the pulldown
- **THEN** the tree is drawn left-right

#### Scenario: Layout change redraws without refetch
- **WHEN** a user selects "top-bottom" in the pulldown after a tree has loaded
- **THEN** the tree is redrawn top-bottom and no new tree request is sent

#### Scenario: Tree with validation problems
- **WHEN** the API returns `valid: false`
- **THEN** the tree is still drawn and the problems are listed above the chart

### Requirement: Edges are drawn in a chosen line style
A second pulldown, "Line style", SHALL let the user choose how edges are drawn: broken lines (right-angled elbows) or curves. The default SHALL be curves. Changing it SHALL redraw the same tree, in the current layout, without refetching it.

#### Scenario: Default line style
- **WHEN** a user selects a tree and has not changed the line-style pulldown
- **THEN** each parent is joined to its children by a smooth S-curve

#### Scenario: Broken lines
- **WHEN** a user selects "Broken lines" in the line-style pulldown
- **THEN** each parent is joined to its children by right-angled elbow lines, and no tree request is sent

### Requirement: Leaf click opens the artifact panel
When a user clicks a leaf node of the drawn tree, the page SHALL open a right-side panel listing every artifact bound to that node, from `GET /api/kb/category-trees/:name/nodes/:node_id/members?direct=true`. Each row SHALL show the artifact type, artifact ID, record ID, and whether the binding is stale. Clicking an inner node SHALL NOT open the panel.

#### Scenario: Leaf with artifacts
- **WHEN** a user clicks a leaf that has bindings
- **THEN** the panel lists each binding with its artifact type, artifact ID, record ID and stale flag

#### Scenario: Leaf without artifacts
- **WHEN** a user clicks a leaf that has no bindings
- **THEN** the panel shows an empty-state message for that leaf

#### Scenario: Inner node click
- **WHEN** a user clicks an inner node
- **THEN** no request is sent and the panel does not open

#### Scenario: Truncated member list
- **WHEN** a leaf returns the maximum number of members
- **THEN** the panel notes that only the first rows are shown

### Requirement: Page text is translated
All text on the Categories page and the renamed menu item SHALL come from Paraglide messages with both English and Chinese translations in `web/messages/en.json` and `web/messages/zh-cn.json`.

#### Scenario: Chinese locale
- **WHEN** a user with the zh-cn locale opens the Categories page
- **THEN** every visible label, pulldown option, empty state and error message is in Chinese
