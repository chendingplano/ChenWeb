## REMOVED Requirements

### Requirement: Branding row at top of Knowledge menu
**Reason**: Superseded by the shared `SiteHeader` now rendered above the entire page (see the `knowledge-page-header` capability), which already provides site branding, language switching, and navigation back to the Workspace. Keeping a second, menu-local copy of the same affordances would be redundant.
**Migration**: No user-facing migration — the equivalent actions (go to site home, switch language, go to Workspace) are now available via the `SiteHeader` at the top of the page instead of via the Knowledge menu's own top row.

#### Scenario: Expanded menu shows all three controls
- **WHEN** a user loads `/home3/knowledge` with the menu expanded (default state)
- **THEN** the top of the left menu shows, in order, the Company Logo, a Language Control button, and a Workspace button, above the "Knowledge" label row

### Requirement: Company Logo links to semos home
**Reason**: Superseded by `SiteHeader`'s own logo, which already links to `/semos`.
**Migration**: Use the logo in the `SiteHeader` at the top of the page.

#### Scenario: Clicking the logo navigates to semos home
- **WHEN** a user clicks the Company Logo in the Knowledge menu
- **THEN** the browser navigates to `/semos`

#### Scenario: Branding image configured
- **WHEN** the site's `SiteConfig.branding.logo_image` is set
- **THEN** the Knowledge menu logo renders that image instead of the dot+text wordmark

### Requirement: Language Control switches locale
**Reason**: Superseded by `SiteHeader`'s own language switcher.
**Migration**: Use the language switcher in the `SiteHeader` at the top of the page.

#### Scenario: Clicking the language control switches locale
- **WHEN** a user clicks the Language Control button in the Knowledge menu
- **THEN** the active locale advances to the next locale in the configured `locales` list, and UI text using that locale updates accordingly

### Requirement: Workspace button navigates to semos Workspace
**Reason**: Superseded by `SiteHeader`'s own "Workspace" navigation link.
**Migration**: Use the "Workspace" link in the `SiteHeader`'s primary navigation.

#### Scenario: Clicking Workspace navigates away
- **WHEN** a user clicks the Workspace button in the Knowledge menu
- **THEN** the browser navigates to `/semos/workspace`

### Requirement: Collapsed menu shows icon-only equivalents
**Reason**: The entire branding row (expanded and collapsed states) is removed; there is no longer a collapsed-state icon variant to maintain.
**Migration**: None — the `SiteHeader` does not change appearance based on the Knowledge menu's collapsed/expanded state.

#### Scenario: Collapsed menu still exposes all three controls
- **WHEN** a user collapses the Knowledge menu
- **THEN** three icon-only buttons (Logo mark, Language toggle, Workspace) remain visible stacked at the top of the rail, each retaining its original click behavior and an accessible tooltip/aria-label

#### Scenario: Expanding the menu restores full controls
- **WHEN** a user expands a previously-collapsed Knowledge menu
- **THEN** the three icon-only buttons are replaced by the full branding row (logo mark + wordmark, language control, Workspace button)
