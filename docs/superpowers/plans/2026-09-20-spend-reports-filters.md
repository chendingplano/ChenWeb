# Spend Reports Filters and Chart Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add scoped time/API-key filtering and four-series full-width charts to the development dashboard's Spend Reports section.

**Architecture:** Extend the existing `/api/v1/llm/reports/models` endpoint with optional inclusive workspace-day and API-key filters. The server resolves configured API-key names from `.models.toml` and aggregates cache-hit, cache-miss, output, and spend fields. The Svelte view owns filter state and reloads only model reports; existing summary, balances, reconciliation, and usage-event flows remain unchanged.

**Tech Stack:** Go/Echo, PostgreSQL SQL aggregation, Svelte 5 runes, TypeScript, `svelte-echarts`/ECharts, Vitest, `go test`, SvelteKit checks.

---

## File map

- Modify `server/api/llmreporthandler/store.go`: report row fields, filter type, SQL aggregation, API-key name loading/matching.
- Modify `server/api/llmreporthandler/handler.go`: parse report query parameters and expose API-key options.
- Modify `server/api/llmreporthandler/handler_test.go`: HTTP query/response coverage.
- Modify `server/api/llmreporthandler/store_test.go`: SQL expectation and scan coverage for new fields/filters.
- Modify `web/src/lib/components/home3/llm-activities-client.ts`: typed filter request and API-key response types.
- Modify `web/src/lib/components/home3/llm-activities-client.test.ts`: request URL coverage.
- Modify `web/src/lib/components/home3/llm-activities-view.svelte`: filter state, custom-date validation, report-only reload, chart series, and vertical layout.
- Modify `docs/superpowers/specs/2026-09-20-spend-reports-filters-design.md`: only if implementation decisions materially differ from the approved design.

## Chunk 1: Backend report contract

### Task 1: Add failing handler/store tests

- [ ] Extend the handler stub interface and add a test request containing `from`, `to`, and `api_key`; assert the parsed filter reaches the store and the JSON response includes API-key options.
- [ ] Update the store SQL mock to include cache-hit, cache-miss, and API-key columns; add assertions for all new fields.
- [ ] Add a test for invalid date input returning HTTP 400.
- [ ] Run `go test ./server/api/llmreporthandler` and confirm the new tests fail against the old contract.

### Task 2: Implement backend filtering and aggregation

- [ ] Define a report filter type with optional inclusive `From`/`To` workspace dates and API-key name/reference.
- [ ] Extend `ModelActivityReport` with API-key name, cache-hit input tokens, cache-miss input tokens, output tokens, and spend fields while preserving JSON naming conventions.
- [ ] Extend the model-activity SQL query to aggregate `prompt_cache_hit_tokens` and `prompt_cache_miss_tokens`, join account API-key references, filter by workspace day, and group by provider/model/API-key/day.
- [ ] Parse `YYYY-MM-DD` query values strictly and reject malformed or inverted ranges with a clear 400 response.
- [ ] Load configured API-key names from the `[model-api-keys]` section of `.models.toml`, match names to their configured key reference server-side, and return only names/references safe for filtering. Never return secret values.
- [ ] Keep the existing report limit behavior and default recent-day behavior when no time filter is supplied.
- [ ] Run the focused Go tests until they pass.

## Chunk 2: Frontend API client and state

### Task 3: Add typed report filters and API-key options

- [ ] Add cache-token and API-key fields to `LLMModelActivityReport`.
- [ ] Add `LLMReportFilters` and the report response's API-key option type.
- [ ] Serialize only active `from`, `to`, and `api_key` values in `listLLMModelActivityReports` using `URLSearchParams`.
- [ ] Update client tests for no-filter and filtered URLs.
- [ ] Run the client test file and confirm it passes.

### Task 4: Implement UI filter state and loading

- [ ] Replace the model-report-only portion of `load()` with a dedicated `loadModelReports()` request so changing filters does not reload balances or usage events.
- [ ] Initialize the Time select to `Last 30 Days` and derive its date bounds in local/workspace calendar terms.
- [ ] Add API Key select options from the backend response, with an `All API Keys` option.
- [ ] Show native start/end date inputs only for Custom; validate both values and ordering before requesting.
- [ ] Display filter-loading state without clearing existing charts until the new response arrives.
- [ ] Keep existing error/notice handling and ensure report errors do not affect unrelated sections.

## Chunk 3: Chart and layout changes

### Task 5: Replace chart series and grouping

- [ ] Change chart grouping to include API-key name so a selected/all-key response cannot merge distinct keys into one series unintentionally.
- [ ] Remove Calls and combined Input Tokens series.
- [ ] Add exactly four bars named `Input (Cache Hit)`, `Input (Cache Miss)`, `Output`, and `Spend`.
- [ ] Keep token series on the token axis and Spend on the currency axis; keep tooltips and dark/light colors consistent with the existing dashboard.
- [ ] Rename the section heading and supporting copy to `Spend Reports`.

### Task 6: Make report cards full-width and vertical

- [ ] Change `.model-chart-grid` to a one-column layout.
- [ ] Set report cards and charts to fill the panel width and give charts a readable responsive height.
- [ ] Add responsive toolbar styles so filters wrap cleanly at narrow widths.
- [ ] Ensure labels, selects, date inputs, focus states, and validation messages remain keyboard accessible.

## Chunk 4: Verification

### Task 7: Run focused and project checks

- [ ] Run `go test ./server/api/llmreporthandler` from `ChenWeb`.
- [ ] Run the changed frontend client tests.
- [ ] Run `npm run check` or the repository's documented equivalent from `ChenWeb/web`.
- [ ] Run ESLint on the changed Svelte/TypeScript files if configured.
- [ ] Run the web production build if the focused checks pass.
- [ ] Review the diff for accidental changes to the unrelated dirty files and report any remaining limitations.
