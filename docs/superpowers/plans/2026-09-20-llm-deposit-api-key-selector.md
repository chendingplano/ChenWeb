# LLM Deposit API Key Selector Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Populate the LLM deposit form with configured API-key names and resolve the selected name to the first matching account server-side.

**Architecture:** Add a safe API-key name endpoint and make the deposit handler resolve a submitted key name from `.models.toml` before it writes the existing account-keyed balance snapshot. The client loads those names alongside accounts, submits `api_key_name`, and styles both native selects as clear dark inputs.

**Tech Stack:** Go, Echo, TOML configuration, Svelte 5, TypeScript, Node test runner.

---

### Task 1: Server-side safe API-key selection and resolution

**Files:**
- Modify: `server/api/llmadminhandler/handler.go`
- Modify: `server/api/llmadminhandler/store.go`
- Modify: `server/api/routes.go`
- Test: `server/api/llmadminhandler/handler_test.go`

- [x] Write tests that GET options without returning a secret and POST a configured key name that resolves to the first `api_key_ref` match.
- [x] Run `go test ./server/api/llmadminhandler` and confirm the new tests fail.
- [x] Add minimal config parsing, option endpoint, account lookup, and missing-account validation.
- [x] Run `go test ./server/api/llmadminhandler` and confirm it passes.

### Task 2: Client wiring and visible dark selectors

**Files:**
- Modify: `web/src/lib/components/home3/llm-accounts-client.ts`
- Modify: `web/src/lib/components/home3/llm-accounts-client.test.ts`
- Modify: `web/src/lib/components/home3/llm-accounts-view.svelte`

- [x] Write client tests for loading API-key names and submitting `api_key_name`.
- [x] Run the focused web test and confirm it fails.
- [x] Add the API-key option client, load options on mount, replace the account selector with the API Key selector, and add explicit select dark-mode styling using the approved A treatment.
- [x] Run the focused test and frontend check/build.

### Task 3: Verify and document scope

**Files:**
- Modify: `docs/superpowers/specs/2026-09-20-llm-deposit-api-key-selector-design.md`

- [x] Run the focused Go and web tests plus the frontend build.
- [x] Confirm the UI returns only names, blocks unresolved keys, and preserves the existing snapshot schema.
- [ ] Commit the implementation and design documentation through `jj`.
