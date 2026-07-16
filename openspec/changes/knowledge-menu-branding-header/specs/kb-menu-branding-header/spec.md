## ADDED Requirements

### Requirement: Branding row at top of Knowledge menu
The Knowledge page (`/home3/knowledge`) left menu SHALL display a row above the existing "Knowledge" header row, containing the site Company Logo, a Language Control, and a Workspace button, whenever the menu is in its expanded state.

#### Scenario: Expanded menu shows all three controls
- **WHEN** a user loads `/home3/knowledge` with the menu expanded (default state)
- **THEN** the top of the left menu shows, in order, the Company Logo, a Language Control button, and a Workspace button, above the "Knowledge" label row

### Requirement: Company Logo links to semos home
The Company Logo in the Knowledge menu SHALL render the same branding (image or dot+wordmark) as the `/semos` site header, and SHALL link to `/semos`.

#### Scenario: Clicking the logo navigates to semos home
- **WHEN** a user clicks the Company Logo in the Knowledge menu
- **THEN** the browser navigates to `/semos`

#### Scenario: Branding image configured
- **WHEN** the site's `SiteConfig.branding.logo_image` is set
- **THEN** the Knowledge menu logo renders that image instead of the dot+text wordmark

### Requirement: Language Control switches locale
The Knowledge menu SHALL provide a Language Control button that cycles the active locale through the same set of locales used by `/semos`, using the same `setLocale`/`getLocale` mechanism.

#### Scenario: Clicking the language control switches locale
- **WHEN** a user clicks the Language Control button in the Knowledge menu
- **THEN** the active locale advances to the next locale in the configured `locales` list, and UI text using that locale updates accordingly

### Requirement: Workspace button navigates to semos Workspace
The Knowledge menu SHALL provide a Workspace button that navigates to `/semos/workspace`.

#### Scenario: Clicking Workspace navigates away
- **WHEN** a user clicks the Workspace button in the Knowledge menu
- **THEN** the browser navigates to `/semos/workspace`

### Requirement: Collapsed menu shows icon-only equivalents
When the Knowledge menu is collapsed (56px icon rail), the Logo, Language Control, and Workspace button SHALL each render as an icon-only button, stacked above the existing expand-toggle button, consistent with how other collapsed menu items render icon-only.

#### Scenario: Collapsed menu still exposes all three controls
- **WHEN** a user collapses the Knowledge menu
- **THEN** three icon-only buttons (Logo mark, Language toggle, Workspace) remain visible stacked at the top of the rail, each retaining its original click behavior and an accessible tooltip/aria-label

#### Scenario: Expanding the menu restores full controls
- **WHEN** a user expands a previously-collapsed Knowledge menu
- **THEN** the three icon-only buttons are replaced by the full branding row (logo mark + wordmark, language control, Workspace button)
