## ADDED Requirements

### Requirement: Knowledge page consumes resolution API
`/home3/knowledge` SHALL fetch its configurable content from `GET /api/v1/page-config/home3-knowledge?lang=<getLocale()>` instead of `GET /api/v1/kb/menu-config`, keeping the menu tree and ids owned by the component and rendering a page-owned item only when its `entry_key` appears in the response, using the returned label as an override.

#### Scenario: Seeded data renders unchanged
- **WHEN** the page loads with only seeded data (no operator changes)
- **THEN** the Wiki sidebar renders the same items and labels as before the rewiring, in the active locale

#### Scenario: Disabled or unauthorized item hidden
- **WHEN** a menu entry is disabled, suspended, or unauthorized for the user
- **THEN** it is absent from the response and the item does not render
- **AND** a parent whose children are all hidden collapses away

#### Scenario: Locale label override applied
- **WHEN** the active locale is `zh-cn` and an entry has a `zh-cn` label
- **THEN** the sidebar shows the `zh-cn` label; otherwise it falls back to the default label

### Requirement: Workspace page consumes resolution API
`/semos/workspace` SHALL fetch its masthead and app-tile configuration from `GET /api/v1/page-config/semos-workspace?lang=<getLocale()>` instead of `GET /api/v1/workspace/content-config`, keeping base content owned by `SiteConfig`/`WorkspaceApp.key` and rendering masthead fields and tiles only when their `entry_key` appears, using returned label/description overrides.

#### Scenario: Seeded data renders unchanged
- **WHEN** the page loads with only seeded data
- **THEN** the masthead and app tiles render the same content as before the rewiring, in the active locale

#### Scenario: Tile description override applied
- **WHEN** an app-tile entry has a description override for the active locale
- **THEN** the tile subtitle shows that override, else the `SiteConfig` default

### Requirement: Fail-open rendering on fetch failure
Both pages SHALL remain usable when the resolution request fails or is pending, falling back to their built-in default content rather than rendering empty.

#### Scenario: Fetch error falls back to defaults
- **WHEN** the resolution request errors or has not yet resolved
- **THEN** the page renders its built-in default structure and labels
