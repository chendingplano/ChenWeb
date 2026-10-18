# Pi Harness Admin Page Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an administrator page at `ChenWeb/development, Coding Assistant => Harnesses => Pi` to check the Pi gateway and version the two existing Pi-backed service profiles.

**Architecture:** Add a database-backed immutable profile-version store and an admin API in the existing `agentservicehandler` package. Make conversation creation resolve the active profile and run/resume paths resolve the pinned version. Add an admin page component and nested development navigation entry, keeping secrets and tool permissions server-owned.

**Tech Stack:** Go, Echo, PostgreSQL/goose migrations, Svelte 5, TypeScript, Paraglide messages.

---

## File map

- `project_migrations/20261018000002_create_pi_agent_profile_versions.sql` — immutable profile versions and active-version pointers.
- `server/api/agentservicehandler/profile_store.go` — database persistence, initial seeding, version activation, and immutable snapshot loading.
- `server/api/agentservicehandler/profile_admin.go` — admin read/save/init/gateway handlers and validation.
- `server/api/agentservicehandler/profiles.go` — profile value validation, serialized profile fields, DB-backed resolution while retaining environment seed behavior.
- `server/api/agentservicehandler/handler.go`, `run_handler.go`, `tools.go` — use request context to resolve active/pinned profiles from persistent storage where needed.
- `server/api/routes.go` — construct the profile store and register the admin endpoints.
- `web/src/lib/services/piHarnessAdminService.ts` — typed admin API client.
- `web/src/lib/components/home3/pi-harness-admin-view.svelte` — gateway status, editable service profiles, validation and degraded initialization state.
- `web/src/lib/components/home3/nav-rail.svelte`, `content-panel.svelte` — add Coding Assistant → Harnesses → Pi in the development navigation and render its view.
- `web/messages/en.json`, `web/messages/zh-cn.json` — all visible strings in both locales.
- `server/api/agentservicehandler/profile_store_test.go`, `profile_admin_test.go`, `profiles_test.go` — persistence, validation, versioning and authorization coverage.
- `web/src/lib/services/piHarnessAdminService.test.ts` and/or component tests, following current web test conventions — client payload and error-state coverage.
- `docs/pi-agentic-services-operations.md`, `KnowledgeStore/doc-repo/devdocs/202610/2026101802-devdoc-harness-neutral-agentic-services.md` — describe the shipped admin configuration and version lifecycle.

## Chunk 1: Persist immutable profiles and resolve them for conversations

### Task 1: Add the profile version schema

**Files:**
- Create: `project_migrations/20261018000002_create_pi_agent_profile_versions.sql`
- Modify: `server/api/agentservicehandler/migration_test.go`

- [ ] Add a migration contract test that reads the exact migration path, following the existing tests in `migration_test.go`, and asserts the profile/version tables, uniqueness, active-version foreign key, immutable prompt snapshot columns, and rollback statements exist.
- [ ] Implement `kb.agentic_service_profile_versions` with one row per immutable `(service_slug, version)` and the complete serialized profile, prompt reference, hash, snapshot, and audit metadata.
- [ ] Implement `kb.agentic_service_profiles` as one active-version pointer per service, constrained to an existing version.
- [ ] Add appropriate primary/foreign keys, timestamp defaults, and goose up/down markers; do not add mutable fields to old versions.
- [ ] Add the focused migration contract test. Run it only if the user separately authorizes verification; otherwise report it unexecuted.

### Task 2: Implement seed and version persistence

**Files:**
- Create: `server/api/agentservicehandler/profile_store.go`
- Create: `server/api/agentservicehandler/profile_store_test.go`

- [ ] Define the narrow store interface needed for `EnsureSeeded`, `ListActive`, `LoadVersion`, and `ActivateVersion`.
- [ ] Write store tests for first-time seed, repeat seed without overwrite, active profile listing, exact-version loading, and a stale expected version returning a conflict.
- [ ] Implement seed of both current profiles from the existing environment-backed `ProfileRegistry`, including resolved prompt text and its SHA-256.
- [ ] Implement exact version loading so the persisted prompt snapshot is used and the source file is not reread for old versions.
- [ ] Implement `ActivateVersion` and `RestoreActiveVersion` as store primitives: activation locks the active row, compares expected version, assigns next `vN`, inserts immutable profile, updates active pointer, and commits; restore conditionally moves the pointer from the failed new version back to the previous version.
- [ ] Add store tests for these behaviors. Run `go test ./server/api/agentservicehandler -run 'TestProfileStore' -count=1` only if the user separately authorizes verification; otherwise report it unexecuted.

### Task 3: Validate profile edits and publish activated versions

**Files:**
- Modify: `server/api/agentservicehandler/profiles.go`
- Modify: `server/api/agentservicehandler/profile_store.go`
- Test: `server/api/agentservicehandler/profiles_test.go`, `profile_store_test.go`

- [ ] Add tests for supported service slugs; non-empty provider/model/store; known prompt selection; permission mode; enabled state; pilot IDs; zero/negative limits; and exact boundary behavior for every configurable limit.
- [ ] Validate prompt selections only within the approved `prompt-agent-*.md` set, reject missing/empty files, and snapshot the exact bytes and SHA-256 for every activated version.
- [ ] Enforce: max 64 tool calls, 600 elapsed seconds, 8,192 output tokens, 1,048,576 evidence bytes, 128,000 history tokens, and 32,768 carried tool-result bytes.
- [ ] Trim pilot IDs, drop empty items, deduplicate case-sensitively; reject empty stores. Keep tools fixed to the existing server-owned names regardless of input.
- [ ] Orchestrate activation in the registry: validate/prepare first, call the store activation, then atomically publish. If publish fails, call `RestoreActiveVersion`; keep the new immutable row inactive and return failure.
- [ ] Keep environment-backed behavior only until first successful seed. After seeding, DB errors fail profile resolutions safely; never use stale active settings.
- [ ] Add focused profile tests and confirm by inspection that invalid candidates cannot publish. Run tests only if the user separately authorizes verification.

### Task 4: Resolve active and pinned versions in request paths

**Files:**
- Modify: `server/api/agentservicehandler/handler.go`
- Modify: `server/api/agentservicehandler/run_handler.go`
- Modify: `server/api/agentservicehandler/tools.go`
- Modify: `server/api/routes.go`
- Test: existing handler/run/tool tests plus focused version-resolution cases

- [ ] Pass request context into profile resolution for service listing, conversation creation/detail, run start, and internal tool authorization.
- [ ] Wire one persistent profile store to user-facing handlers and internal tool authorization; don't introduce a second config cache with different access rules.
- [ ] New conversations resolve the active DB version and persist its slug/version/model. Existing conversations and tools resolve the exact pinned version, including provider, model, prompt snapshot, scope, permission, and limits.
- [ ] Before seed completion, keep current env-backed fallback behavior. After seed completion, profile lookup failures return a safe unavailable error.
- [ ] Add tests proving new conversations use active version, saved conversations retain all profile fields after activation, and missing pinned versions fail closed.
- [ ] Add focused request-path tests. Run `go test ./server/api/agentservicehandler -run 'Test(Agent|Conversation|Profile|Run|Tool)' -count=1` only if the user separately authorizes verification; otherwise report it unexecuted.

## Chunk 2: Add admin API and gateway check

### Task 5: Add administrator endpoints

**Files:**
- Create: `server/api/agentservicehandler/profile_admin.go`
- Modify: `server/api/routes.go`
- Test: `server/api/agentservicehandler/profile_admin_test.go`

- [ ] Add an admin guard that accepts authenticated admin/root users and rejects unauthenticated or ordinary users. Apply it on every admin endpoint, including reads.
- [ ] Register `GET /api/v1/system-admin/pi-harness` for active profiles, allowed prompt choices, gateway target (URL only), and initialization state.
- [ ] Register `PUT /api/v1/system-admin/pi-harness/profiles/:slug` for validated version creation and activation. Require `expected_active_version`; return `409` for a stale edit.
- [ ] Register idempotent `POST /api/v1/system-admin/pi-harness/initialize` to retry initial seeding only when no DB profile versions exist.
- [ ] Register `POST /api/v1/system-admin/pi-harness/gateway-check`; make the request server-side, authenticate with the configured gateway secret, use a short timeout, and return only healthy/unavailable status, elapsed time, and a server-generated check timestamp. Never return the secret or provider credentials.
- [ ] Keep safe errors generic to clients and log operational detail with the project's logger conventions.
- [ ] Add tests for auth/role outcomes on reads and writes, request size/unknown fields, input validation, stale saves, idempotent initialization, secret redaction, gateway timeout, and unavailable gateway. Also verify degraded reads expose read-only state, writes return `503` while storage/seeding is unavailable, and any failed persistence leaves the active pointer unchanged.
- [ ] Add focused handler tests. Run them only if the user separately authorizes verification; otherwise report them unexecuted.

## Chunk 3: Build the page, navigation, and documentation

### Task 6: Add page service and editable UI

**Files:**
- Create: `web/src/lib/services/piHarnessAdminService.ts`
- Create: `web/src/lib/components/home3/pi-harness-admin-view.svelte`
- Test: `web/src/lib/services/piHarnessAdminService.test.ts` and existing Svelte check conventions

- [ ] Define typed shapes for page state, profiles, update payloads, and safe gateway-check responses; use the existing authenticated request helper.
- [ ] Build the page with a compact gateway panel showing configured URL, latest check status/time, and an explicit **Check connection** action. Explain on the page that gateway URL and secret are deployment-managed and provider credentials never appear in the UI.
- [ ] Provide inputs for provider/model, thinking, prompt selection, stores, document groups, enabled, pilot users, permission mode, and limits. Render tools as read-only server-managed capability labels.
- [ ] Include expected active version in saves, show field/server validation and `409` conflict recovery, and announce the newly activated version. State that provider/model choices determine where user messages and retrieved evidence are sent.
- [ ] Show read-only initialization failure state and Retry initialization button; profile editing remains usable when only the gateway is down.
- [ ] Add Paraglide strings for gateway target/help, check action, result and timestamp, deployment-managed configuration, provider disclosure, validation, conflict, read-only seed state, and save outcomes in English and Chinese. Ensure selection works at the page root and match the existing development shell's theme conventions.
- [ ] Add frontend service tests for paths, serialization, and safe error handling.

### Task 7: Add the development navigation path

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Modify: `web/messages/en.json`, `web/messages/zh-cn.json`

- [ ] Add nested ids for Coding Assistant → Harnesses → Pi only to the development navigation tree.
- [ ] Add bilingual menu and page strings. Use role-based visibility for the Pi entry, while keeping the server admin check authoritative.
- [ ] Render `PiHarnessAdminView` for the Pi leaf selection and verify the active navigation highlighting works at the nested depth.
- [ ] Preserve page text selection and do not add `user-select: none` outside small controls.
- [ ] Run `cd web && bun run check` and the focused client tests only if the user separately authorizes verification; otherwise report them unexecuted.

### Task 8: Update operations and architecture docs

**Files:**
- Modify: `docs/pi-agentic-services-operations.md`
- Modify: `../KnowledgeStore/doc-repo/devdocs/202610/2026101802-devdoc-harness-neutral-agentic-services.md`

- [ ] Document that the Pi page manages service-profile versions while gateway URL/secret and provider credentials remain deployment-managed.
- [ ] Explain initialization from existing environment configuration, profile pinning, prompt snapshots, and gateway health result semantics.
- [ ] Update the devdoc's known limitations to describe the shipped Pi admin page without claiming support for other harnesses.
- [ ] Review all changed lines against the approved design and leave unrelated dirty metrics/harness work untouched.

## Final verification and integration

- [ ] Run the focused Go tests and frontend checks listed above only if the user separately authorizes verification; otherwise report them unexecuted.
- [ ] Inspect both ChenWeb and KnowledgeStore repo statuses; commit only this feature's files through `jj`, preserving the existing unrelated working-copy changes.
- [ ] Run `jj log` in each repo and confirm only expected linear feature commits were created.
- [ ] Report what changed, what docs changed/stale, and checks not run.
