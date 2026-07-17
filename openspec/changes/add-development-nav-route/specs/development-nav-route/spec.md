## ADDED Requirements

### Requirement: Development nav item in the shared SiteHeader
The shared `SiteHeader` SHALL show a "Development" nav entry between "Knowledge Base" and "About Us", linking to `/development`. It SHALL be gated and highlighted using the same mechanism as the existing "Workspace" and "Knowledge Base" entries (auth-required, active-state highlighted when the current route is `/development`).

#### Scenario: Development entry appears in nav order
- **WHEN** a user views the `SiteHeader` on any page (desktop or mobile nav)
- **THEN** the nav shows, in order: Home, Workspace, Knowledge Base, Development, About Us

#### Scenario: Unauthenticated click is gated
- **WHEN** a logged-out user clicks "Development"
- **THEN** navigation is prevented, the user is routed to `/semos`, and the login prompt is shown — matching the existing behavior for "Workspace" and "Knowledge Base"

#### Scenario: Active state highlights on /development
- **WHEN** an authenticated user is on `/development`
- **THEN** the "Development" nav entry is styled as active, and no other nav entry is

### Requirement: /development renders the same dashboard as /home3
Navigating to `/development` SHALL render the same dashboard content and behavior as `/home3` (shared `SiteHeader`, nav rail, content panel, context shelf), under its own URL — not a redirect back to `/home3`.

#### Scenario: /development shows the dashboard at its own URL
- **WHEN** an authenticated user navigates to `/development`
- **THEN** the address bar shows `/development` (no redirect to `/home3`)
- **AND** the page renders the same dashboard content as `/home3`: `SiteHeader`, nav rail, content panel, and context shelf

#### Scenario: /home3 is unaffected
- **WHEN** an authenticated user navigates to `/home3`
- **THEN** it renders exactly as it did before this change, with no behavior change from the existence of `/development`
