# OpenMetadata Home3 Integration Design

## Summary

Integrate OpenMetadata into ChenWeb `home3` as a dedicated in-panel workspace with a hybrid shell:

- ChenWeb owns the surrounding layout, breadcrumb, controls, and optional context sync
- OpenMetadata owns the embedded metadata application UI
- authentication is true SSO with ChenWeb as the primary login authority
- the embedded experience is delivered through a ChenWeb-controlled same-origin reverse-proxy path

The intent is to make OpenMetadata feel like a first-class ChenWeb tool without rebuilding its GUI in ChenWeb.

## User Choices Captured

- Integration style: embedded app inside `ChenWeb::/home3`
- Interaction model: hybrid
- Authentication: true SSO
- Identity authority: ChenWeb auth is primary
- Layout: dedicated panel route inside `home3`

## Goals

- Make OpenMetadata accessible from `home3` without leaving the ChenWeb shell
- Preserve the OpenMetadata GUI rather than rebuilding it
- Keep authentication seamless for logged-in ChenWeb users
- Ensure the embed behaves like a native ChenWeb workspace
- Create a backend boundary that can evolve without rewriting the frontend integration

## Non-Goals

- Rebuilding OpenMetadata screens as native ChenWeb Svelte components
- Mirroring all OpenMetadata internal routes in SvelteKit
- Implementing deep ChenWeb-to-OpenMetadata entity sync in the first phase
- Modifying OpenMetadata core UI code unless absolutely necessary

## Recommended Approach

Use a same-origin reverse-proxied embed.

ChenWeb should expose an internal integration route such as:

- `/integrations/openmetadata/`

That route proxies the OpenMetadata UI and API from the local OpenMetadata service. `home3` then renders that URL inside a dedicated embedded workspace panel.

This is preferred over a raw cross-origin iframe because it gives:

- better cookie/session handling
- fewer browser restrictions
- a cleaner foundation for ChenWeb-primary SSO
- simpler long-term control over headers, CSP, and auth boundaries

## Alternatives Considered

### 1. Raw iframe to `http://localhost:8585`

Pros:

- fastest prototype
- minimal backend work

Cons:

- fragile for true SSO
- likely cookie and browser-policy issues
- weaker control over embed security and UX

### 2. Same-origin reverse-proxy embed

Pros:

- best fit for ChenWeb-primary SSO
- avoids cross-origin session problems
- lets ChenWeb control the integration boundary

Cons:

- more backend and proxy work
- requires careful header and asset forwarding

### 3. Native ChenWeb rebuild using OpenMetadata APIs

Pros:

- best UI consistency
- total ChenWeb control

Cons:

- far more expensive
- duplicates a mature GUI
- much higher maintenance burden

## Architecture

### Frontend

ChenWeb frontend changes live in the `home3` shell.

Primary integration points:

- `web/src/lib/components/home3/nav-rail.svelte`
- `web/src/lib/components/home3/content-panel.svelte`
- new component: `web/src/lib/components/home3/openmetadata-workspace.svelte`

Responsibilities:

- add a new `home3` nav child for OpenMetadata
- render the workspace in-panel rather than `window.open(...)`
- show ChenWeb-owned top bar controls
- surface loading, auth, and service failure states
- optionally coordinate dark mode and context shelf behavior

### Backend

ChenWeb backend should add an OpenMetadata integration module under the Echo server.

Responsibilities:

- validate the current ChenWeb session
- determine whether the current user may access OpenMetadata
- resolve or provision the corresponding OpenMetadata identity
- establish a usable OpenMetadata-authenticated session
- expose a same-origin reverse-proxy route for the embedded UI

Recommended backend surface:

- `GET /api/integrations/openmetadata/session`
- `GET /integrations/openmetadata/*`
- optional `POST /api/integrations/openmetadata/context`

### OpenMetadata

OpenMetadata should remain mostly unchanged.

The preferred model is configuration-driven integration:

- trust the same identity source or trust a ChenWeb-driven auth bootstrap
- avoid frontend patching inside OpenMetadata where possible

## SSO Contract

### Desired Login Flow

1. User logs into ChenWeb.
2. User opens `home3 -> OpenMetadata`.
3. ChenWeb frontend requests session bootstrap from `/api/integrations/openmetadata/session`.
4. ChenWeb backend validates the user session and prepares OpenMetadata access.
5. Frontend loads `/integrations/openmetadata/` inside the workspace panel.
6. The reverse proxy forwards to OpenMetadata with the necessary authenticated context.

### Preferred Auth Pattern

Preferred long-term pattern:

- ChenWeb and OpenMetadata trust the same external identity provider
- ChenWeb remains the user-facing login authority
- the integration layer performs server-side bootstrap rather than exposing auth complexity to the browser

Pragmatic fallback if direct shared-IdP integration is not ready:

- ChenWeb backend provisions or refreshes an OpenMetadata session on behalf of the logged-in user

### Access Rules

ChenWeb backend is the policy gate for panel access.

Examples:

- deny access to users without metadata privileges
- allow read-only users to launch browse/search views
- reserve elevated actions for mapped admin/editor roles

## Home3 UX Design

### Entry Point

Add an `OpenMetadata` child under `Tools` or another appropriate `home3` group.

Selecting it should switch `ContentPanel` into the embedded workspace view and stay inside the shell.

### Workspace Layout

The panel should contain:

- ChenWeb-owned top bar
- embedded OpenMetadata viewport
- optional retained right context shelf

### Top Bar Controls

Initial controls:

- breadcrumb: `Home > Tools > OpenMetadata`
- reload
- open in new tab
- back to ChenWeb
- auth/session status indicator
- optional context sync toggle

### Navigation Behavior

- ChenWeb owns entry and exit into the OpenMetadata workspace
- OpenMetadata owns its internal navigation inside the embedded app
- ChenWeb should not mirror all internal OpenMetadata routes in phase 1

### Context Sync

Phase 1 should not depend on deep context sync.

Phase 2 may add:

- selected ChenWeb domain mapped to OpenMetadata landing context
- selected knowledge object mapped to OpenMetadata search/entity deep links
- optional shelf-driven quick actions

## Reverse Proxy Behavior

The reverse proxy must correctly forward:

- HTML document requests
- JS/CSS/static assets
- API requests
- cookies and auth headers as needed
- websocket or streaming behavior if required by the embedded app

It must also normalize:

- origin/host headers
- redirect locations
- cookie scope
- CSP or frame-related headers

## Error Handling

### Session Bootstrap Failure

Show a ChenWeb-native error panel with:

- clear auth message
- retry button
- open in new tab fallback if allowed

### OpenMetadata Unavailable

Show service-unavailable state with:

- health summary
- retry
- optional link to operational diagnostics

### Session Expiry

If the embedded app loses auth:

- keep the ChenWeb shell visible
- show reconnect action in the workspace top bar
- avoid blank iframes without explanation

## Security Considerations

- ChenWeb backend must remain the source of truth for access gating
- avoid client-side-only auth handoffs
- prefer same-origin integration to reduce browser cookie friction
- log session bootstrap, access denial, and proxy failures
- treat proxy header forwarding conservatively to avoid identity spoofing

## Logging and Observability

Log at least:

- session bootstrap success/failure
- user-to-OpenMetadata identity mapping outcome
- proxy upstream failures
- unauthorized access attempts
- workspace launch latency

Suggested future metrics:

- launch success rate
- median workspace load time
- SSO bootstrap errors by type
- upstream OpenMetadata availability

## Testing Strategy

### Frontend

- component tests for workspace state transitions
- verify nav selection opens the panel view
- verify failure and loading states render correctly

### Backend

- unit tests for session bootstrap logic
- tests for authorization decisions
- proxy tests for route forwarding and header handling

### End-to-End

- logged-in ChenWeb user opens OpenMetadata in `home3`
- workspace loads without secondary login prompt
- reload and open-in-new-tab behave correctly
- unauthorized user receives clear denial state

## Implementation Phases

### Phase 1

- add nav item
- add `OpenMetadataWorkspace` panel
- add backend bootstrap endpoint
- add same-origin reverse proxy route
- establish working SSO handoff
- support reload and open-in-new-tab

### Phase 2

- add context sync
- refine shelf integration
- add richer status/telemetry
- support deeper entity launch links

## Risks

- SSO complexity may depend on what OpenMetadata supports in your auth stack
- reverse proxying may require iteration for headers, cookies, or redirects
- OpenMetadata UI behavior may assume its original origin in a few places
- if ChenWeb and OpenMetadata auth models differ too much, a temporary bootstrap strategy may be needed

## Recommended First Implementation Slice

Build the smallest end-to-end path that proves the architecture:

1. Add `home3 -> OpenMetadata` nav entry.
2. Render a new `OpenMetadataWorkspace` panel in `content-panel.svelte`.
3. Add ChenWeb backend endpoint for workspace bootstrap.
4. Add same-origin reverse proxy path.
5. Load the embedded UI successfully for an already logged-in ChenWeb user.

Once that works, layer in context sync and polish.
