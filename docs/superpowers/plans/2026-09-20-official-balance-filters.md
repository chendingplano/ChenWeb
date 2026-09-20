# Official Balance Filters and Granularity Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Spend Reports-style Time/API Key filters to official-balance charts and automatically select hourly or daily buckets.

**Architecture:** The balance endpoint gains optional inclusive date and API-key-name filters. The Svelte view reuses the existing report-filter state and presentation, while choosing the endpoint frequency solely from the selected Time preset.

**Tech Stack:** Go/Echo, PostgreSQL, Svelte 5, Vitest, ECharts.

---

## File map

- `server/api/llmreporthandler/handler.go` parses and validates the balance filters.
- `server/api/llmreporthandler/store.go` filters and aggregates official snapshots.
- `server/api/llmreporthandler/handler_test.go` covers handler forwarding and validation.
- `server/api/llmreporthandler/store_test.go` covers filter SQL arguments.
- `web/src/lib/components/home3/llm-activities-client.ts` serializes balance filters.
- `web/src/lib/components/home3/llm-activities-client.test.ts` verifies the request URL.
- `web/src/lib/components/home3/llm-activities-view.svelte` renders and applies the shared controls.

## Chunk 1: API contract

### Task 1: Test balance filters

- [x] Write failing handler/client tests for `from`, `to`, `api_key`, and hourly/daily frequency forwarding.
- [x] Run focused tests and confirm they fail because the endpoint/client lack filter support.
- [x] Add the minimum filter type, validation, safe API-key lookup, and SQL conditions.
- [x] Run focused tests until green.

## Chunk 2: Dashboard control

### Task 2: Test and render shared controls

- [x] Write a failing client URL test for filtered balance requests.
- [x] Update the client and run the test until green.
- [x] Replace Frequency with right-aligned Time and API Key controls matching Spend Reports.
- [x] Derive hourly only for Today/Yesterday; otherwise request daily buckets.
- [x] Keep custom-range validation and existing-chart retention during refresh.

## Chunk 3: Verification

### Task 3: Verify the completed change

- [x] Run focused `go test ./server/api/llmreporthandler` coverage for the changed handler behavior.
- [x] Run the frontend client tests, `npm run check`, ESLint for changed files, and production build.
- [x] Review the diff for scope and update this plan’s checkboxes.
