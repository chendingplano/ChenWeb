# Pi Harness Admin Page Design

**Status:** Scope approved; this design document is pending user review  
**Date:** 2026-10-18  
**Location:** `ChenWeb/development, Coding Assistant => Harnesses => Pi`

## Purpose

Give ChenWeb administrators one place to configure the Pi-backed agent services and inspect the Pi gateway connection. The page configures ChenWeb's use of Pi; the gateway process and its provider credentials remain deployment-managed.

## Current behavior

- `PI_GATEWAY_URL` and `PI_GATEWAY_SECRET` are read at ChenWeb startup. The Pi gateway runs as a separate process.
- `knowledge-guide` and `problem-diagnostics` service profiles are assembled from code defaults, prompt files, and `PI_*` environment variables when ChenWeb starts.
- Conversations store a service slug, profile version, and model. `ProfileRegistry.ResolveVersion` serves the saved profile for later runs.
- The Pi gateway's authenticated `/health` endpoint can be checked from ChenWeb. The existing signed-in agent-service health endpoint checks ChenWeb routing only.
- The development navigation is assembled in `web/src/lib/components/home3/nav-rail.svelte`; content views are selected in `content-panel.svelte`.

## Proposed experience

Add a page at **Coding Assistant → Harnesses → Pi** in the development workspace. Restrict the page and its server endpoints to administrators (`admin` or `root`). The page has two sections:

1. **Gateway** — show the configured URL, whether ChenWeb can reach the authenticated Pi gateway health endpoint, and a time-stamped result from an explicit **Check connection** action. Do not return the shared secret or provider credentials to the browser. The URL and secret remain environment-managed; the page explains where these deployment settings come from.
2. **Service profiles** — show the existing `knowledge-guide` and `problem-diagnostics` profiles. Allow an administrator to edit provider, model, thinking level, prompt file selection, allowed knowledge stores, document groups, enabled state, pilot-user allowlist, permission default, and the existing resource limits. Keep allowed tools fixed to the server's supported read-only tool set. Show profile version and save as a new version; activating a new version affects new conversations while existing conversations keep resolving the version they pinned.

Show validation errors beside the affected fields. Saving is explicit. A successful save reports the activated version. The page should state that provider/model settings determine where messages and retrieved evidence are sent, consistent with the existing disclosure shown to users.

## Backend and data

- Add a versioned profile store in ChenWeb's project database. Seed it from the current built-in defaults, prompt files, and environment overrides when no database profile exists, preserving current deployments during rollout.
- Save the prompt file reference, SHA-256 hash, and exact prompt text snapshot with every immutable profile version. The browser only selects an approved prompt file and never edits or receives its contents. Validate prompt references against the approved agent prompt directory and reject empty or missing prompt files. The snapshot ensures a later file edit cannot change an older profile version.
- Validate profile fields server-side: supported services, non-empty provider/model/store list, known prompt file, valid permission mode, positive bounded resource limits, and normalized pilot-user entries. For the first version, allow at most 64 tool calls, 600 elapsed seconds, 8,192 output tokens, 1,048,576 evidence bytes, 128,000 history tokens, and 32,768 bytes per carried tool result. Trim pilot-user IDs, remove empty entries and duplicates, and compare IDs case-sensitively because user IDs are opaque identifiers. Reject an empty allowed-store list, matching current `LoadProfileRegistry` behavior. Ignore or reject tool-set changes; tools remain server-owned.
- A conversation resolves the complete immutable profile version it pinned, including provider, model, the stored prompt-text snapshot, allowed stores/groups, permission default, and limits. It must not reload prompt text from the file or combine the saved version with current active settings. Keep the file reference and hash for audit and to validate a newly activated version.
- Assign versions monotonically per service (`v1`, `v2`, …). A save includes the active version the administrator edited. In one database transaction, lock the service's active-version row, reject a stale expected version with `409 Conflict`, insert the immutable new version, and move the active pointer. Validate and prepare the registry snapshot before the transaction. Publish that snapshot atomically after commit. If publication unexpectedly fails, restore the previous active pointer in a compensating transaction and leave the new version inactive; do not report a successful activation.
- Treat the database as canonical across server processes. Each new conversation resolves the active profile version from the database; each existing conversation resolves its pinned immutable version from the database. The in-memory registry may cache immutable versions, but it must check the active pointer for new conversations. This avoids a separate notification or polling protocol. Before first-time seeding succeeds, ChenWeb may continue using its environment-backed profiles as its existing fallback mode. After seeding succeeds, any database outage makes profile reads/writes and new runs unavailable, including runs in existing conversations; already-running turns may finish. Never fall back to a stale active profile or environment settings after database profiles have been initialized.
- Seed the database from the current built-in defaults, prompt files, and environment overrides in one transaction when no profile versions exist. If the database is unavailable or seeding fails, ChenWeb continues using its environment-backed startup profiles and logs the seeding error. The admin read endpoint returns a read-only/degraded state and the page offers an explicit **Retry initialization** action backed by `POST /api/v1/system-admin/pi-harness/initialize`. Admin writes return a clear unavailable response until seeding succeeds. The retry endpoint is idempotent and seeds only when no profile versions exist. Do not overwrite database profiles from environment variables after the first successful seed.
- Add admin-only read, update, initialization-retry, and gateway-check endpoints under `/api/v1/system-admin/pi-harness`. The gateway-check endpoint runs on the ChenWeb server, uses the configured secret internally, has a short timeout, and returns only safe status and timing metadata.
- Do not add an endpoint for reading or changing gateway secrets. Do not make Pi credentials available to the browser.

## Navigation, localization, and access

- Add the nested development navigation item **Coding Assistant → Harnesses → Pi**, with Paraglide keys in both English and Chinese message files.
- Keep the page's root selectable for text copying and preserve existing `home3` shell behavior.
- Hide the menu item from non-admin users and enforce the same role check on every endpoint. The server check is authoritative.

## Failure behavior

- If the gateway is unreachable, show the last check result and a clear retry action; profile editing remains available because it does not require a live gateway.
- If a profile save fails validation or database persistence, leave the active profile unchanged and show the server's safe validation message.
- If profile storage is unavailable or first-time seeding fails, ChenWeb keeps its environment-backed startup behavior and the page is read-only. Later admin reads or an explicit retry attempt seeding again. After seeding succeeds, a database outage disables profile reads/writes and new runs until storage recovers; already-running turns may finish.
- If an older profile version is unavailable for a pinned conversation, preserve the existing safe unavailable/error behavior; never silently substitute the active version.

## Acceptance criteria

1. An administrator can open the page at the requested development navigation path; a non-admin cannot use the page or its endpoints.
2. The page can check gateway reachability without disclosing any secret and accurately distinguishes gateway health from ChenWeb API health.
3. An administrator can edit and activate either existing service profile using validated fields and the fixed supported tool set.
4. Saving creates an immutable profile version. New conversations pin the active version; existing conversations continue to resolve every setting from their saved version, including provider, model, prompt, access scope, and limits.
5. Environment-backed deployments continue to work before database profiles are initialized.
6. All visible strings support English and Chinese through Paraglide.

## Out of scope

- Editing Pi process launch settings, provider API keys, gateway shared secrets, or the capability-signing secret.
- Adding another harness or changing the adapter interface.
- Editing arbitrary system prompt text in the browser.
- Changing the fixed server-owned tool set or granting filesystem, shell, or unrestricted network tools.
- Rewriting existing conversations when a profile version changes.

## Documentation and verification

Update the Pi operations guide and harness-neutral agentic-services devdoc after implementation to describe the actual admin page, stored profile lifecycle, and gateway-check behavior. Add backend and frontend coverage for authorization, validation, version pinning, activation, safe gateway status, and English/Chinese strings. Do not run checks or tests unless requested; report any checks that are not run.

## Known workspace constraint

At design time, the ChenWeb working copy already contains unrelated metrics changes and ongoing harness-neutral agent-service changes. Implementation must preserve those edits and avoid committing them as part of this feature.
