# Current Navigation User Profile Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the authenticated user's name and email in the Home3 navigation rail.

**Architecture:** Keep the nav rail's authenticated profile endpoint for its name and email. The User Info page reads the current authenticated session from `/auth/me`, which supplies profile fields, roles, account creation time, and current session authentication time; add phone to that response. Return only explicit session identity fields and do not serialize the full auth object.

**Tech Stack:** Go, Echo, Svelte 5, TypeScript.

---

## Files

- Modify `server/api/aiassistanthandler/handler.go`: derive name and email from `EchoFactory.NewFromEcho(...).IsAuthenticated()` and return only those fields.
- Modify `web/src/lib/components/home3/nav-rail.svelte`: replace the sample profile constant with typed reactive profile state and fetch the existing endpoint on mount.
- Modify `web/src/lib/components/home3/content-panel.svelte`: route the `__user_info__` selection to a dedicated profile view.
- Create `web/src/lib/components/home3/current-user-info-view.svelte`: load and display identity details from `/auth/me`, and edit first name, last name, and phone number.
- Modify `server/api/aiassistanthandler/handler.go` and `server/api/routes.go`: add an authenticated self-profile update endpoint.
- Modify `shared/go/api/auth/kratos.go`: include phone in the explicit `/auth/me` session identity traits.
- Modify `shared/svelte/src/lib/stores/auth.svelte.ts`: describe the optional phone trait in the frontend session type.

## Chunk 1: Authenticated profile response and rail display

### Task 1: Return the authenticated profile from the existing endpoint

**Files:**
- Modify: `server/api/aiassistanthandler/handler.go`

- [x] Create a request context with a handler-specific logger location and defer closing it.
- [x] Read the current identity with `rc.IsAuthenticated()`; return HTTP 401 when it is unavailable.
- [x] Resolve the name as trimmed `FirstName` plus `LastName`, then trimmed `UserName`, then email. Return exactly `{ "user": { "name": ..., "email": ... } }`. Prefer profile names because the auth mapping may set `UserName` to email. Do not serialize `ApiTypes.UserInfo`.

### Task 2: Load and display the current profile in the nav rail

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`

- [x] Define the minimal response type and initialize reactive profile state with blank name and email.
- [x] In `onMount`, fetch `/api/v1/ai-assistant/user-info` with same-origin credentials; require an OK status and validate that the JSON contains string `user.name` and `user.email` before updating state. Keep the profile blank on HTTP, parsing, or shape errors.
- [x] Render the initials, name, and email only from loaded current-user profile state; use email for initials when the returned name is blank. Do not restore sample identity on failure.

### Task 3: Render the selected User Info page

**Files:**
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Create: `web/src/lib/components/home3/current-user-info-view.svelte`

- [x] Route `sectionId === '__user_info__'` to the dedicated view so it does not fall through to the generic placeholder.
- [x] Fetch `/auth/me` and display first name, last name, email, phone number, roles, create time, and last login time, with loading, error, and retry states.
- [x] Format timestamps in local time and use an em dash for missing values.

### Task 4: Allow editing supported profile fields

**Files:**
- Modify: `server/api/aiassistanthandler/handler.go`
- Modify: `server/api/routes.go`
- Modify: `web/src/lib/components/home3/current-user-info-view.svelte`

- [x] Add authenticated `PUT /api/v1/ai-assistant/user-info` that updates only the current user's first name, last name, and phone in Kratos.
- [x] Add Edit, Save, and Cancel controls for those fields; validate phone format, preserve read-only fields, and show update errors.

### Task 5: Review the change

**Files:**
- Review: `server/api/aiassistanthandler/handler.go`
- Review: `server/api/routes.go`
- Review: `web/src/lib/components/home3/nav-rail.svelte`
- Review: `web/src/lib/components/home3/content-panel.svelte`
- Review: `web/src/lib/components/home3/current-user-info-view.svelte`
- Review: `shared/go/api/auth/kratos.go`
- Review: `shared/svelte/src/lib/stores/auth.svelte.ts`

- [x] Run the existing frontend check/build command from `web/package.json`.
- [x] Run `go work sync` and build the shared auth package plus ChenWeb server package. The shared auth change was synchronized and built in the earlier profile implementation; the ChenWeb server package was rebuilt for this endpoint.
- [x] Do not add or run tests unless the user asks for them.
- [x] Confirm the endpoint response contains no fields beyond `name` and `email`, and that both `/development` and `/home3` use the updated `NavRail`.
- [x] Commit the implementation using `jj` and confirm the `jj` log is linear.

## Reference

Design: `docs/superpowers/specs/2026-09-27-current-nav-user-profile-design.md`
