# OpenMetadata Home3 Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenMetadata to ChenWeb `home3` as a dedicated in-panel workspace with ChenWeb-owned shell controls, backend bootstrap/proxy support, and a ChenWeb-primary SSO integration boundary.

**Architecture:** ChenWeb will add a new `home3` workspace entry and render an `OpenMetadataWorkspace` component inside the existing content panel. The backend will add a small `openmetadatahandler` package that exposes a session/bootstrap JSON endpoint plus a same-origin reverse proxy path that forwards the OpenMetadata UI/API. Phase 1 keeps OpenMetadata internal navigation inside the embedded app and avoids deep entity sync.

**Tech Stack:** SvelteKit, Svelte 5, Echo, Go `net/http/httputil` reverse proxy, existing ChenWeb auth middleware, Go tests, frontend component tests where already established.

---

## File Structure

### Create

- `server/api/openmetadatahandler/handler.go`
  - Session/bootstrap endpoint and shared helper functions.
- `server/api/openmetadatahandler/proxy.go`
  - Reverse proxy construction, upstream rewriting, and header/cookie handling.
- `server/api/openmetadatahandler/types.go`
  - Small response/request structs for the integration surface.
- `server/api/openmetadatahandler/handler_test.go`
  - Unit tests for session/bootstrap and proxy helpers.
- `web/src/lib/components/home3/openmetadata-workspace.svelte`
  - ChenWeb-owned top bar plus embedded OpenMetadata viewport.
- `web/src/lib/components/home3/openmetadata-workspace.test.ts`
  - Component tests for loading/error/iframe-shell states if the current frontend test setup supports it.

### Modify

- `server/api/routes.go`
  - Register the OpenMetadata integration API endpoint and reverse proxy route.
- `web/src/lib/components/home3/nav-rail.svelte`
  - Add the OpenMetadata navigation entry and keep it in-panel.
- `web/src/lib/components/home3/content-panel.svelte`
  - Render `OpenMetadataWorkspace` when the new menu selection is active.
- `web/src/routes/home3/+page.svelte`
  - No behavior change expected beyond existing selection flow, but verify whether any active-menu defaults or layout state need adjustment.
- `.env` or local runtime config source used by ChenWeb server
  - Add OpenMetadata integration env vars if not already present.
- `USER_MANUAL.md` or project docs if ChenWeb has an integration manual section
  - Document required env vars and local run flow if this repo’s conventions require it.

### Reuse / Read Carefully

- `server/api/flowhandler/flowhandler_test.go`
  - Current lightweight Echo handler test style.
- `server/api/agentplatformhandler/handler.go`
  - Example of auth/user resolution helpers and backend package structure.
- `server/cmd/deepdoc/main.go`
  - Server startup, CORS, and runtime env context.
- `web/src/lib/components/home3/content-panel.svelte`
  - Existing panel-switching pattern used by `Canvas01`, `SkillMgmtView`, etc.
- `web/src/lib/components/home3/nav-rail.svelte`
  - Existing menu model and selection wiring.
- `docs/superpowers/specs/2026-05-14-openmetadata-home3-integration-design.md`
  - Approved design spec.

---

## Chunk 1: Backend Integration Surface

### Task 1: Scaffold the OpenMetadata handler package

**Files:**
- Create: `server/api/openmetadatahandler/types.go`
- Create: `server/api/openmetadatahandler/handler.go`
- Create: `server/api/openmetadatahandler/proxy.go`

- [ ] **Step 1: Create `types.go` with the minimal response shapes**

Define small structs for:

- `SessionResponse`
- `ErrorResponse`
- any small internal config struct used by the handler package

Include fields for:

- `status`
- `launch_url`
- `proxy_base_path`
- `display_name`
- `user_id`
- `capabilities`
- `message`

- [ ] **Step 2: Create `handler.go` with package skeleton and TODO-safe stubs**

Add:

- package declaration
- imports
- `GetSession(c echo.Context) error`
- `requireUser(...)` helper or local equivalent if reusing current auth style is not possible directly
- config/env helper for reading OpenMetadata upstream/base URL values

The first version may return a hardcoded placeholder JSON payload with `200 OK`, but it must compile and follow ChenWeb response conventions.

- [ ] **Step 3: Create `proxy.go` with a minimal proxy constructor**

Add:

- `func NewProxy() (http.Handler, error)`
- env-based upstream URL parsing
- `httputil.NewSingleHostReverseProxy(...)`

The first version may only forward requests without custom rewriting beyond a working baseline.

- [ ] **Step 4: Run compile check for the new package**

Run: `go test ./server/api/openmetadatahandler`

Expected: package compiles; tests may still be missing.

- [ ] **Step 5: Commit**

```bash
git add server/api/openmetadatahandler/types.go server/api/openmetadatahandler/handler.go server/api/openmetadatahandler/proxy.go
git commit -m "feat: scaffold openmetadata integration handler"
```

### Task 2: Add backend tests before finalizing behavior

**Files:**
- Create: `server/api/openmetadatahandler/handler_test.go`
- Modify: `server/api/openmetadatahandler/handler.go`
- Modify: `server/api/openmetadatahandler/proxy.go`

- [ ] **Step 1: Write the failing handler test for session bootstrap**

Test cases:

- authenticated request returns `200`
- response contains `launch_url`
- missing required env/config returns `500` or a documented config error

Follow the lightweight Echo test style used in `flowhandler_test.go`.

- [ ] **Step 2: Write the failing proxy helper test**

Test cases:

- valid upstream URL builds a proxy
- invalid upstream URL returns error

- [ ] **Step 3: Run tests and confirm failure**

Run: `go test ./server/api/openmetadatahandler -run 'Test(GetSession|NewProxy)' -v`

Expected: FAIL due to placeholder or missing behavior.

- [ ] **Step 4: Implement the minimal logic to make tests pass**

Implement:

- env/config parsing
- JSON response payload
- proxy constructor error handling

- [ ] **Step 5: Re-run tests**

Run: `go test ./server/api/openmetadatahandler -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/api/openmetadatahandler/handler_test.go server/api/openmetadatahandler/handler.go server/api/openmetadatahandler/proxy.go
git commit -m "test: cover openmetadata bootstrap and proxy helpers"
```

### Task 3: Register API and proxy routes in Echo

**Files:**
- Modify: `server/api/routes.go`
- Test: `server/api/openmetadatahandler/handler_test.go`

- [ ] **Step 1: Write the failing route-registration test or package-level integration test**

If route-registration tests are too heavy for current patterns, add focused tests around the exported handler functions and note route registration verification via manual smoke test.

- [ ] **Step 2: Register session bootstrap endpoint**

Add route under authenticated API group:

- `GET /api/v1/integrations/openmetadata/session`

- [ ] **Step 3: Register same-origin proxy route**

Add authenticated route outside or alongside the API group as appropriate for asset/UI forwarding:

- `GET /integrations/openmetadata`
- `GET /integrations/openmetadata/*`

Use the new proxy handler package.

- [ ] **Step 4: Make sure frontend-route auth middleware does not swallow the proxy path**

Update the frontend path exclusion logic in `RegisterRoutes` so `/integrations/openmetadata` is treated as backend/proxy traffic, not a Svelte route.

- [ ] **Step 5: Run backend tests**

Run: `go test ./server/api/...`

Expected: PASS for changed packages; if broader failures exist outside this work, record them explicitly.

- [ ] **Step 6: Commit**

```bash
git add server/api/routes.go server/api/openmetadatahandler/handler_test.go
git commit -m "feat: register openmetadata session and proxy routes"
```

### Task 4: Add auth/session bootstrap semantics

**Files:**
- Modify: `server/api/openmetadatahandler/handler.go`
- Modify: `server/api/openmetadatahandler/types.go`
- Test: `server/api/openmetadatahandler/handler_test.go`

- [ ] **Step 1: Write failing tests for auth-bound response behavior**

Cover:

- authenticated user info is reflected in response payload
- unauthorized request is denied consistently
- capability flags are present even if initially static

- [ ] **Step 2: Implement minimal ChenWeb-auth-primary behavior**

For phase 1:

- require authenticated ChenWeb user
- return a launch URL pointing at `/integrations/openmetadata/`
- return status/capability metadata needed by the frontend shell

Do not overbuild true IdP token exchange yet unless it already exists locally. Keep the backend boundary ready for that next step.

- [ ] **Step 3: Re-run focused tests**

Run: `go test ./server/api/openmetadatahandler -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/api/openmetadatahandler/handler.go server/api/openmetadatahandler/types.go server/api/openmetadatahandler/handler_test.go
git commit -m "feat: add bootstrap payload for openmetadata workspace"
```

---

## Chunk 2: Home3 Frontend Integration

### Task 5: Add navigation entry for OpenMetadata

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`

- [ ] **Step 1: Add the new child item under the chosen section**

Recommended location:

- `tools -> openmetadata`

Label:

- `OpenMetadata`

- [ ] **Step 2: Ensure selection stays in-panel**

Do not use `window.open(...)` for the nav action. Route selection should call `onSelect(...)` and let `content-panel.svelte` switch views.

- [ ] **Step 3: Run frontend checks for the modified file**

Run: `cd web && npm test -- --runInBand` or the repo’s actual frontend check command if available

Expected: the changed component compiles; if there is no stable component-test command, record that and use the project’s existing Svelte check/lint command instead.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/home3/nav-rail.svelte
git commit -m "feat: add openmetadata entry to home3 navigation"
```

### Task 6: Create the workspace shell component

**Files:**
- Create: `web/src/lib/components/home3/openmetadata-workspace.svelte`
- Create: `web/src/lib/components/home3/openmetadata-workspace.test.ts`

- [ ] **Step 1: Write the failing component test**

Cover at least:

- loading state while session bootstrap is in progress
- error state when bootstrap fails
- rendered iframe/embed shell when bootstrap succeeds
- top bar controls render: breadcrumb, reload, open-in-new-tab, back/close

- [ ] **Step 2: Run the test to confirm failure**

Run the smallest possible frontend test command for the new component.

Expected: FAIL because component does not exist yet.

- [ ] **Step 3: Implement the minimal workspace shell**

Include:

- ChenWeb-owned top bar
- bootstrap fetch to `/api/v1/integrations/openmetadata/session`
- embedded viewport pointing to returned `launch_url`
- reload button
- open-in-new-tab button
- simple auth/service error messaging

Keep phase 1 intentionally small:

- no deep context sync
- no postMessage bridge yet
- no route mirroring

- [ ] **Step 4: Re-run component tests**

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/home3/openmetadata-workspace.svelte web/src/lib/components/home3/openmetadata-workspace.test.ts
git commit -m "feat: add home3 openmetadata workspace shell"
```

### Task 7: Mount the workspace inside the content panel

**Files:**
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Test: `web/src/lib/components/home3/openmetadata-workspace.test.ts`

- [ ] **Step 1: Add the new conditional render branch**

Pattern should match existing `childId`-based routing in `content-panel.svelte`.

Recommended branch:

- `activeMenu?.childId === 'openmetadata'`

- [ ] **Step 2: Pass only the minimal props needed**

Likely:

- `darkMode`
- callbacks for leaving/reloading if needed

Avoid coupling the workspace to unrelated `home3` state until phase 2.

- [ ] **Step 3: Verify no existing branches regress**

Run the relevant frontend test/check command and manually inspect the branch order so existing `flow`, `prompt-optimizer`, and dashboard behavior remain unchanged.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/home3/content-panel.svelte
git commit -m "feat: mount openmetadata workspace in home3 content panel"
```

### Task 8: Add backend/frontend manual smoke checks

**Files:**
- Modify: `docs/superpowers/specs/2026-05-14-openmetadata-home3-integration-design.md` only if implementation learnings require updates
- Modify: project docs if needed

- [ ] **Step 1: Run backend tests**

Run: `go test ./server/api/...`

Expected: PASS for touched packages

- [ ] **Step 2: Run frontend checks**

Run the repo’s standard frontend validation command from `web/`

Expected: PASS

- [ ] **Step 3: Manual app smoke test**

Verify:

- logged-in ChenWeb user can open `home3`
- selecting `OpenMetadata` shows the workspace shell
- bootstrap call succeeds
- embedded app loads through `/integrations/openmetadata/`
- `Reload` works
- `Open in new tab` opens the same-origin integration route

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "feat: wire openmetadata into home3 workspace"
```

---

## Chunk 3: Follow-up Hardening

### Task 9: Prepare for true SSO completion work

**Files:**
- Modify: `server/api/openmetadatahandler/handler.go`
- Modify: local env/config docs

- [ ] **Step 1: Document the current phase-1 auth boundary**

Make clear in code comments or docs:

- ChenWeb is the access gate
- reverse proxy path is the launch surface
- shared-IdP exchange or stronger session bootstrap may still be needed for full production SSO

- [ ] **Step 2: Add explicit env names for future production wiring**

Examples:

- `OPENMETADATA_UPSTREAM_URL`
- `OPENMETADATA_PUBLIC_BASE_PATH`
- `OPENMETADATA_SSO_MODE`

- [ ] **Step 3: Add tests for missing/invalid env handling**

Run: `go test ./server/api/openmetadatahandler -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/api/openmetadatahandler/handler.go server/api/openmetadatahandler/handler_test.go
git commit -m "chore: prepare openmetadata integration for production sso wiring"
```

### Task 10: Optional phase-2 planning notes

**Files:**
- Modify: `docs/superpowers/specs/2026-05-14-openmetadata-home3-integration-design.md` only if needed

- [ ] **Step 1: Capture deferred items explicitly**

List as deferred:

- context sync from ChenWeb selection into OpenMetadata
- postMessage bridge if internal navigation sync becomes necessary
- shelf-aware metadata quick actions
- richer telemetry and health banners

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-05-14-openmetadata-home3-integration-design.md
git commit -m "docs: record deferred openmetadata integration enhancements"
```

---

## Verification Commands

### Backend

```bash
cd /Users/cding/Workspace/ChenWeb
go test ./server/api/openmetadatahandler -v
go test ./server/api/... 
```

### Frontend

```bash
cd /Users/cding/Workspace/ChenWeb/web
# Use the repo's existing frontend test/check command for changed files
```

### Manual Run

```bash
cd /Users/cding/Workspace/ChenWeb
# Start ChenWeb with the local OpenMetadata stack already running
# Open /home3 and select the OpenMetadata workspace entry
```

---

## Notes for the Implementer

- Stay phase-1 narrow. Do not build deep entity sync or a custom OpenMetadata navigation bridge yet.
- Keep the backend integration surface small and explicit.
- Prefer environment-driven upstream config over hardcoded URLs in business logic.
- Follow existing Echo handler/test patterns instead of introducing a new framework.
- Keep the `home3` integration consistent with current `content-panel.svelte` branch style.

Plan complete and saved to `docs/superpowers/plans/2026-05-14-openmetadata-home3-integration.md`. Ready to execute?
