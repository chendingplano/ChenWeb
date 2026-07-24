## ADDED Requirements

### Requirement: Two-panel layout with resizable divider
The User's Manual page (nav id `docs-users-manual`) SHALL render a left tree panel and a right content panel separated by a vertical divider that the user can drag to change the relative widths of the two panels.

#### Scenario: Dragging the divider resizes the panels
- **WHEN** the user presses down on the divider and drags it horizontally
- **THEN** the left panel's width follows the drag within a clamped min/max range and the right panel fills the remaining space

#### Scenario: Releasing the mouse stops resizing
- **WHEN** the user releases the mouse button after dragging the divider
- **THEN** further mouse movement no longer changes the panel widths

### Requirement: Left panel folder nodes expand and collapse
The left panel SHALL render a menu tree where nodes with children (folders) toggle their children's visibility when clicked, without navigating or changing the right panel's content.

#### Scenario: Clicking a collapsed folder expands it
- **WHEN** the user clicks a folder node that is currently collapsed
- **THEN** its child nodes become visible and the folder's expand indicator reflects the open state

#### Scenario: Clicking an expanded folder collapses it
- **WHEN** the user clicks a folder node that is currently expanded
- **THEN** its child nodes become hidden and the folder's expand indicator reflects the closed state

### Requirement: Left panel leaf nodes load content into the right panel
The left panel SHALL render leaf nodes (nodes without children) that, when clicked, display that node's associated document content in the right panel.

#### Scenario: Clicking a leaf node shows its content
- **WHEN** the user clicks a leaf node in the tree
- **THEN** the right panel renders that leaf's Markdown content as formatted HTML and visually marks that leaf as the selected node

#### Scenario: No leaf selected yet
- **WHEN** the User's Manual page is first opened and no leaf has been clicked
- **THEN** the right panel shows a placeholder prompting the user to select a page from the tree

### Requirement: Manual content is served through a static DocumentSource
The User's Manual page SHALL obtain its tree structure and documents through a `DocumentSource` implementation (see `document-content-model`) whose current backing is Markdown files bundled with the frontend at build time, with no backend API or database dependency for this change.

#### Scenario: Manual renders with no network calls to a manual-specific API
- **WHEN** the User's Manual page loads and a leaf is selected
- **THEN** its content is available and rendered without any HTTP request to a manual-content backend endpoint

#### Scenario: Viewer renders via renderDocument, not a Markdown-specific call
- **WHEN** the viewer renders a selected leaf's document
- **THEN** it does so by calling the shared `renderDocument(doc)` entry point rather than calling a markdown parser directly, so swapping in other document types later requires no change to the viewer
