## ADDED Requirements

### Requirement: Resources nav entry
The shared `SiteHeader` nav SHALL include a "Resources" entry positioned between the "Development" and "About Us" entries, linking to `/resources`, and gated by authentication exactly like the "Development" and "Knowledge Base" entries.

#### Scenario: Resources entry visible in nav order
- **WHEN** any user views the site header nav
- **THEN** a "Resources" entry appears immediately after "Development" and before "About Us"

#### Scenario: Auth-gated navigation
- **WHEN** a logged-out user clicks "Resources"
- **THEN** navigation is blocked by the same `handleNavClick` auth gate used for "Development" (the user is prompted/redirected to log in rather than reaching `/resources`)

#### Scenario: Active-state highlight
- **WHEN** a logged-in user is on `/resources`
- **THEN** the "Resources" nav entry is rendered in its active/highlighted state

#### Scenario: Localized label
- **WHEN** the site language is English
- **THEN** the entry reads "Resources"
- **WHEN** the site language is Simplified Chinese
- **THEN** the entry reads its `zh-cn` label

### Requirement: Resources route renders shared dashboard
The `/resources` route SHALL be a real SvelteKit route (its own `+page.svelte` + `+layout.ts`) that renders the shared `Dashboard` component with `pageKey="resources"`, so the URL bar reads `/resources` and the page shows the same header + nav rail + content panel + context shelf layout as `/development`.

#### Scenario: Direct load shows dashboard
- **WHEN** a logged-in user loads `/resources` directly
- **THEN** the URL bar reads `/resources` and the shared `Dashboard` layout renders (no redirect to `/home3` or `/development`)

#### Scenario: Site config available
- **WHEN** `/resources` loads
- **THEN** its `+layout.ts` provides `siteConfig` to the `Dashboard` exactly as the `/development` route does

#### Scenario: Existing routes unaffected
- **WHEN** this change is deployed
- **THEN** `/home3` and `/development` continue to render and behave exactly as before
