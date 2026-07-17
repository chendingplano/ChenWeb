## ADDED Requirements

### Requirement: Shared SiteHeader renders on the /home3 dashboard
The `/home3` dashboard page SHALL render the same `SiteHeader` component (top navigation: logo, primary nav links, language switcher, dark-mode toggle, login/logout) used by `/home3/knowledge` and `/semos` pages, positioned above the dashboard's own content, replacing the previous bespoke `HeroHeader` banner.

#### Scenario: Loading /home3 shows the shared header
- **WHEN** a user navigates to `/home3`
- **THEN** the top of the page shows the same logo, primary navigation (Home / Workspace / Knowledge Base / About), language switcher, and dark-mode toggle as `/home3/knowledge` and `/semos/workspace`
- **AND** the previous "MyAI Assistant v3.0" hero banner and its status strip ("agents active", "tasks running", "systems nominal") no longer appear

### Requirement: Sibling /home3 subroutes are unaffected
Other routes nested under `/home3` (`chunks`, `doc-structure`, `inputs`, `metrics`, `doc-review-report/[id]`) and `/home3/knowledge` SHALL NOT be affected by this change — the header swap SHALL be scoped to the `/home3` dashboard page only, not applied via a shared layout at the `/home3` route level.

#### Scenario: /home3/knowledge still renders exactly one SiteHeader
- **WHEN** a user navigates to `/home3/knowledge`
- **THEN** exactly one `SiteHeader` is rendered (from its existing route-scoped layout), not two

#### Scenario: Other /home3 subroutes are visually unchanged
- **WHEN** a user navigates to `/home3/chunks`, `/home3/doc-structure`, `/home3/inputs`, or `/home3/metrics`
- **THEN** these pages render exactly as they did before this change, with no new header added
