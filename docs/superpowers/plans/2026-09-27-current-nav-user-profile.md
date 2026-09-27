# Current Navigation User Profile Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the authenticated user's name and email in the Home3 navigation rail.

**Architecture:** Reuse the authenticated `GET /api/v1/ai-assistant/user-info` endpoint, changing its sample response to a minimal projection from the authenticated `ApiTypes.UserInfo`. The Home3 nav rail fetches the endpoint on mount and renders the returned profile, with deterministic name fallbacks and no sample identity on loading or failure.

**Tech Stack:** Go, Echo, Svelte 5, TypeScript.

---

## Files

- Modify `server/api/aiassistanthandler/handler.go`: derive name and email from `EchoFactory.NewFromEcho(...).IsAuthenticated()` and return only those fields.
- Modify `web/src/lib/components/home3/nav-rail.svelte`: replace the sample profile constant with typed reactive profile state and fetch the existing endpoint on mount.
- Modify `web/src/lib/components/home3/content-panel.svelte`: route the `__user_info__` selection to a dedicated profile view.
- Create `web/src/lib/components/home3/current-user-info-view.svelte`: load and display current name and email with loading and error states.

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
- [x] Fetch the existing current-user endpoint and display name and email, with loading, error, and retry states.

### Task 4: Review the change

**Files:**
- Review: `server/api/aiassistanthandler/handler.go`
- Review: `web/src/lib/components/home3/nav-rail.svelte`
- Review: `web/src/lib/components/home3/content-panel.svelte`
- Review: `web/src/lib/components/home3/current-user-info-view.svelte`

- [x] Run the existing frontend check/build command from `web/package.json`.
- [x] Do not add or run tests unless the user asks for them.
- [x] Confirm the endpoint response contains no fields beyond `name` and `email`, and that both `/development` and `/home3` use the updated `NavRail`.
- [ ] Commit the implementation using `jj` and confirm the `jj` log is linear.

## Reference

Design: `docs/superpowers/specs/2026-09-27-current-nav-user-profile-design.md`
