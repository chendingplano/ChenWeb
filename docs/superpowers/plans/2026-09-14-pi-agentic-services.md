# Pi Agentic Services Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents are available) or superpowers:executing-plans. Follow TDD for each task.

**Goal:** Deliver a working ChenWeb proof of concept in which signed-in users can use either Knowledge Guide or Problem Diagnosis Guide, receive streamed Pi responses grounded by five read-only knowledge tools, and safely resume, stop, approve, delete, and rate conversations.

**Architecture:** The browser talks only to authenticated ChenWeb endpoints. ChenWeb owns profiles, prompts, user-owned conversation records, permissions, audit records, and the SSE stream. A loopback-only Bun gateway in `ThirdParty/pi` embeds Pi through its SDK, maintains warm sessions when possible, reconstructs context after a restart, and exposes only five ChenWeb-backed read tools. Tools call dedicated bounded ChenWeb endpoints with a short-lived, run-scoped capability that names the user, profile, allowed tools, and knowledge scope; the gateway never receives a reusable browser credential or database access. The gateway API is protected by a shared secret and is kept behind ChenWeb.

**Tech stack:** Go/Echo/PostgreSQL/goose, Svelte 5/TypeScript, Bun/TypeScript, `@earendil-works/pi-coding-agent` SDK, server-sent events.

---

### Task 1: Persist user-owned conversations and audit records

**Files:**
- Create: `project_migrations/20260914000001_create_agentic_service_tables.sql`
- Create: `server/api/agentservicehandler/store.go`
- Create: `server/api/agentservicehandler/store_test.go`

- [ ] Add a reversible project migration for conversations, messages, response attempts, tool calls/sources, and feedback. Use `TIMESTAMPTZ`, ownership indexes, unique idempotency keys, and cascading deletion.
- [ ] Add a narrowly scoped store that always filters conversation reads and writes by authenticated user ID and records source/document dependencies on each source-derived assistant message.
- [ ] Cover create/list/get/resume-state, streamed message persistence, stop/failure state, feedback, and deletion with SQL-mock tests.
- [ ] Run `go test ./server/api/agentservicehandler`.

### Task 2: Define and load the two versioned Pi profiles

**Files:**
- Create: `prompts/prompt-agent-knowledge-guide-v1.md`
- Create: `prompts/prompt-agent-problem-diagnosis-v1.md`
- Create: `server/api/agentservicehandler/profiles.go`
- Create: `server/api/agentservicehandler/profiles_test.go`

- [ ] Define `knowledge-guide` and `problem-diagnostics` as server-owned profiles with friendly names, provider/model environment overrides, user-visible provider disclosure, thinking level, version, read-tool allowlist, allowed knowledge stores/document groups, permission default, time/tool/output limits, availability, optional pilot-user allowlist, and a selectable active version for rollback.
- [ ] Load system instructions from the versioned prompt files rather than embedding them in Go.
- [ ] Make both prompts require evidence for ChenWeb facts, label unsupported model knowledge, resist instructions found in source text, and produce usable citations; make diagnostics explicitly non-medical/non-legal and read-only.
- [ ] Test slug validation, independent profile settings, prompt loading, and invalid/disabled profiles.

### Task 3: Build ChenWeb's authorization and bounded knowledge-tool boundary

**Files:**
- Create: `server/api/agentservicehandler/capability.go`
- Create: `server/api/agentservicehandler/capability_test.go`
- Create: `server/api/agentservicehandler/tools.go`
- Create: `server/api/agentservicehandler/tools_test.go`
- Modify: `server/api/routes.go`

- [x] Mint signed, short-lived, one-run capabilities containing user ID, profile/version, allowed tool names, knowledge-store/document scope, run ID, and expiry; reject tampering, expiry, cross-run reuse, and tools outside the allowlist.
- [x] Add internal-only endpoints for `search_knowledge`, `read_source_passages`, `get_artifact_details`, `get_document_context`, and `find_related_knowledge`. Enforce the intersection of current user access, profile scope, and requested resource on every call.
- [x] Reuse lower-level ChenWeb knowledge functions where safe, but shape bounded tool-specific responses. Never load/return an entire line file for a small passage request; cap query length, results, ranges, lines, related items, and response bytes.
- [x] Return stable source/document/artifact identity, page/line locations, validation status, and an explicit untrusted-evidence marker.
- [x] Add a replaceable access-check interface and test denial when access is revoked after a source was previously used.

### Task 4: Build the Pi SDK session and event bridge

**Files:**
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/types.ts`
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/knowledge-tools.ts`
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/permission-gate.ts`
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/session-registry.ts`
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/server.ts`
- Create: `/Users/cding/Workspace/ThirdParty/pi/gateway/*.test.ts`
- Modify: `/Users/cding/Workspace/ThirdParty/pi/package.json`
- Modify: `/Users/cding/Workspace/ThirdParty/pi/mise.toml`

- [ ] Add `test`, `check`, and `gateway` scripts to `package.json`; `check` must run TypeScript without emitting files.
- [ ] Start with failing Bun tests for request validation, profile isolation, capability forwarding, tool input limits, permission ask/auto behavior, cancellation, public event normalization, and session cleanup.
- [ ] Embed Pi with `createAgentSession`, explicit model/prompt/settings, no built-in tools, and only the profile's custom knowledge tools.
- [ ] Implement the five tools over ChenWeb's dedicated internal endpoints, forwarding only the run capability. Enforce a second set of gateway-side input and payload caps and mark retrieved text as untrusted evidence.
- [ ] Stream normalized newline-delimited events for answer text, curated activity/tool status, permission requests, sources, usage, completion, and safe errors. Never emit model chain-of-thought, credentials, Pi's hidden internal state, or raw protected tool payloads.
- [ ] Maintain one active session per conversation, reject concurrent turns, reconstruct a cold session from the bounded ChenWeb history, and support abort.
- [ ] Bind to loopback by default, require `PI_GATEWAY_SECRET`, and expose `/health`, `/v1/runs`, `/v1/runs/:id/cancel`, and `/v1/runs/:id/permissions/:requestId`.
- [ ] Run `bun test` and `bun run check` in `ThirdParty/pi`.

### Task 5: Add authenticated conversation CRUD and access rechecks

**Files:**
- Create: `server/api/agentservicehandler/types.go`
- Create: `server/api/agentservicehandler/handler.go`
- Create: `server/api/agentservicehandler/handler_test.go`
- Modify: `server/api/routes.go`

- [x] Start with handler tests proving authentication-derived ownership, service/profile listing, slug routing, pilot availability, version/model pinning, deletion, and feedback.
- [x] Add profile listing/health plus conversation list/create/get/delete endpoints.
- [x] On every conversation read/resume, recheck every referenced source. Hide complete source-derived assistant messages whose dependencies are no longer accessible, explain the omission, and exclude hidden messages/tool data from history sent to Pi.
- [x] Test access revoked after save as well as deleted/moved source behavior.

### Task 6: Add the ChenWeb run, SSE, cancellation, and permission lifecycle

**Files:**
- Create: `server/api/agentservicehandler/gateway.go`
- Create: `server/api/agentservicehandler/run_handler.go`
- Create: `server/api/agentservicehandler/run_handler_test.go`
- Modify: `server/api/routes.go`
- Modify: relevant environment/config documentation

- [x] Start with tests for idempotent turn creation, capability minting, SSE framing, activity-vs-answer separation, approval/denial in both ask and auto modes, cancellation, disconnect/interruption, gateway failure, and exactly-once final state.
- [x] Add the streaming message endpoint. It creates one attempt, sends the pinned profile and bounded accessible history, relays safe gateway events as SSE, records answer text/tool activity/source dependencies/usage, and settles the attempt exactly once.
- [x] Add cancel and permission-decision endpoints and proxy them to the gateway with the shared secret. Ask mode pauses each not-yet-approved tool; auto mode proceeds only for tools already allowed by both profile and capability.
- [x] Return clear unavailable, stopped, interrupted, limit, and retry messages.
- [x] Run handler tests, then `go test ./server/api/...`.

### Task 7: Build the shared ChenWeb conversation page

**Files:**
- Create: `web/src/lib/services/agentServiceClient.ts`
- Create: `web/src/lib/services/agentServiceStream.ts` and `.test.ts`
- Create: `web/src/routes/home3/agent-services/+page.svelte`
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `server/api/agentservicehandler/types.go` and `store.go` for accessible saved source cards

- [x] Add a Workspace navigation entry and one responsive page shared by both services.
- [x] Provide service selection, capability/provider disclosure, conversation list/new/delete, message history, composer, streaming status, a plain-language activity panel (never hidden model reasoning), stop, retry, ask/auto permission selection, and approve/deny controls.
- [x] Render source cards with document, line/page, artifact identity, and a safe ChenWeb link; include the AI fallibility reminder.
- [x] Preserve partial responses and reconnect by refetching the authoritative conversation state.
- [x] Test SSE parsing and state transitions, then run `bun test` and `bun run check` in `web` and build the frontend.

### Task 8: Add evaluation fixtures, operations notes, and end-to-end verification

**Files:**
- Create: `server/api/agentservicehandler/testdata/evaluation-cases.json`
- Create: `docs/pi-agentic-services-operations.md`
- Modify: `mise.toml`

- [x] Add representative normal, ambiguous, missing-evidence, conflicting-source, hostile-document, access-denial, and access-revoked-after-save cases for both profiles.
- [x] Document gateway start/health/shutdown, required environment variables, provider disclosure, retention/deletion behavior, limits, and troubleshooting.
- [x] Add a development task that starts the gateway beside ChenWeb without exposing it publicly.
- [x] Run migration up/down/up against a dedicated empty development probe database; validate the migration contract in tests as well.
- [x] Run `bun test` and `bun run check` in `ThirdParty/pi`; run frontend tests/check/build; copy the generated build into the embed directory; run `go test ./...` and `mise build-server` in ChenWeb.
- [ ] Commit Pi and ChenWeb changes separately with `jj`, inspect both logs, and confirm no unrelated workspace changes were included.
