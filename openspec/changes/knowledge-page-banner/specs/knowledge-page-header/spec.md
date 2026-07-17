## ADDED Requirements

### Requirement: Shared SiteHeader renders on the Knowledge page
The Knowledge page (`/home3/knowledge`) SHALL render the same `SiteHeader` component (top navigation: logo, primary nav links, language switcher, dark-mode toggle, login/logout) used by `/semos` pages, positioned above the page's own content.

#### Scenario: Loading the Knowledge page shows the shared header
- **WHEN** a user navigates to `/home3/knowledge`
- **THEN** the top of the page shows the same logo, primary navigation (Home / Workspace / Knowledge Base / About), language switcher, and dark-mode toggle as `/semos/workspace`

#### Scenario: Knowledge Base nav link is marked active
- **WHEN** a user is on `/home3/knowledge`
- **THEN** the "Knowledge Base" entry in the shared header's navigation is styled as the active link

### Requirement: Knowledge page content fits below the header without overflow
The Knowledge page's own fixed-viewport layout (left menu + content panel) SHALL be sized to account for the `SiteHeader`'s height so no content is clipped or forces an unintended page-level scrollbar.

#### Scenario: No vertical overflow at any viewport height
- **WHEN** a user loads `/home3/knowledge` at a given viewport height
- **THEN** the combined height of the `SiteHeader` and the page's own layout region equals the viewport height, with no additional scrollable overflow at the document level
