# LLM Manual Spending Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record and audit manual provider total spending and expose currency-aware total-spending chart bars.

**Architecture:** The admin handler resolves configured API-key names to existing account IDs and appends `set-total-spending` snapshots. It exposes manual records with key names mapped server-side. The reporting query excludes those entries from balance snapshots, selects the latest manual total per account/currency/chart bucket, and returns CNY/USD total-spending fields for the chart.

**Tech Stack:** Go, Echo, PostgreSQL, Svelte 5, TypeScript, ECharts, Bun test runner.

---

### Task 1: Manual total-spending persistence and audit API

**Files:**
- Modify: `server/api/llmadminhandler/handler.go`
- Modify: `server/api/llmadminhandler/store.go`
- Modify: `server/api/llmadminhandler/handler_test.go`
- Modify: `server/api/routes.go`

- [ ] Write failing tests for persisting `set-total-spending` and returning safe manual records.
- [ ] Run `go test ./server/api/llmadminhandler` to verify failure.
- [ ] Add store methods, handlers, and routes; resolve names without serializing API-key references.
- [ ] Run focused Go tests to verify success.

### Task 2: Total-spending chart report fields

**Files:**
- Modify: `server/api/llmreporthandler/store.go`
- Modify: `server/api/llmreporthandler/store_test.go`
- Modify: `server/api/llmreporthandler/handler_test.go`

- [ ] Write failing query/response tests for latest manual values per account/currency/bucket and total-spending fields.
- [ ] Run focused Go tests to verify failure.
- [ ] Exclude manual total entries from balance pivots/current balances; add CNY and USD total-spending values.
- [ ] Run focused Go tests to verify success.

### Task 3: Accounts-page form and manual-record list

**Files:**
- Modify: `web/src/lib/components/home3/llm-accounts-client.ts`
- Modify: `web/src/lib/components/home3/llm-accounts-client.test.ts`
- Modify: `web/src/lib/components/home3/llm-accounts-view.svelte`
- Modify: `web/src/lib/components/home3/llm-accounts-view.test.ts`

- [ ] Write failing client/view tests for total-spend requests and visible record columns.
- [ ] Run focused web tests to verify failure.
- [ ] Implement shared-form action switching and a newest-first records list.
- [ ] Run focused web tests and Svelte check.

### Task 4: Chart bars

**Files:**
- Modify: `web/src/lib/components/home3/llm-activities-client.ts`
- Modify: `web/src/lib/components/home3/llm-activities-view.svelte`

- [ ] Add client fields and the two Total Spending bar series, with CNY and USD axes.
- [ ] Run frontend checks and production build.

### Task 5: Final verification and documentation

**Files:**
- Modify: `docs/superpowers/specs/2026-09-20-llm-manual-spending-design.md`

- [ ] Run focused Go and web tests, Svelte check, and production build.
- [ ] Commit implementation and plan via `jj`.
