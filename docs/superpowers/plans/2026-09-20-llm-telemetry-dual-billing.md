# LLM Telemetry Dual Billing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route ChenWeb AI-provider calls through shared LLM clients and expose reliable user-attributed usage, hourly official balances, and local CNY billing.

**Architecture:** `shared/go/api/llm` owns all provider/gateway transports and emits one capture record per invocation. ChenWeb owns the database sink, CNY pricing parser, hourly balance scheduling, report aggregation, and dashboard. Official balances are account/API-key and currency scoped; local billing is DeepSeek API-key/model scoped.

**Tech Stack:** Go, PostgreSQL/goose, shared LLM package, Echo, Svelte 5, ECharts, TOML.

---

## Chunk 1: Shared invocation and attribution boundary

### Task 1: Add user attribution to shared usage capture

**Files:**
- Modify: `shared/go/api/llm/types.go`
- Modify: `shared/go/api/llm/usage_capture.go`
- Modify: `shared/go/api/llm/openai.go`
- Modify: `shared/go/api/llm/openai_client.go`
- Modify: `shared/go/api/llm/anthropic.go`
- Test: `shared/go/api/llm/usage_capture_test.go`

- [ ] Write failing tests that a request user ID reaches the sink and an empty user ID logs an error without preventing the provider call.
- [ ] Run `cd shared/go && go test ./api/llm -run 'UserID|UsageCapture'` and verify the new expectations fail.
- [ ] Add `UserID` to request/capture types, propagate it for chat, structured output, embeddings, and streams, and log a structured error when absent.
- [ ] Write adapter-level failing tests that image, product-drawing, and Pi gateway requests pass their authenticated caller ID, while scheduled/system balance calls emit the required missing-ID error and persist NULL.
- [ ] Re-run the focused shared tests and then `cd shared/go && go test ./api/llm`.

### Task 2: Move AI HTTP transports into shared

**Files:**
- Create: `shared/go/api/llm/image.go`
- Create: `shared/go/api/llm/gateway.go`
- Create: `shared/go/api/llm/balance.go`
- Test: `shared/go/api/llm/image_test.go`
- Test: `shared/go/api/llm/gateway_test.go`
- Modify: `ChenWeb/server/api/imagehandler/generate.go`
- Delete after replacement: `ChenWeb/server/api/productdrawings/provider.go`
- Modify: `ChenWeb/server/api/productdrawings/handler.go`
- Modify: `ChenWeb/server/api/agentservicehandler/gateway.go`
- Modify: `ChenWeb/server/api/routes.go`

- [ ] Write failing shared-package tests for OpenAI-compatible and DashScope image requests, including capture on success/failure, Pi gateway delegation, and DeepSeek balance responses containing CNY and USD entries.
- [ ] Run the focused tests and verify they fail because the shared clients do not exist.
- [ ] Implement minimal shared image/gateway/balance clients with injected HTTP clients and usage capture; change ChenWeb adapters to call them and remove direct provider HTTP code.
- [ ] Verify no ChenWeb production source creates AI-provider HTTP requests outside `shared/go/api/llm` using `rg` plus focused Go tests.

## Chunk 2: Persistence and hourly official balances

### Task 3: Persist `user_id` and immutable multi-currency balance snapshots

**Files:**
- Create: `ChenWeb/project_migrations/20260920000001_llm_usage_user_and_dual_billing.sql`
- Modify: `ChenWeb/server/api/llmusage/sink.go`
- Modify: `ChenWeb/server/api/llmusage/sink_test.go`
- Modify: `ChenWeb/server/api/llmreconcile/service.go`
- Modify: `ChenWeb/server/api/llmreconcile/store.go`
- Modify: `ChenWeb/server/api/llmreconcile/service_test.go`
- Modify: `ChenWeb/server/api/llmreconcile/store_test.go`

- [ ] Write failing migration/store tests for `user_id` insertion; immutable CNY/USD snapshots; scheduled-slot idempotency; manual/retry snapshot preservation; raw archive uniqueness; non-secret key display IDs; and both new report tables.
- [ ] Run focused package tests and verify they fail.
- [ ] Add nullable `user_id`; an immutable `llm_balance_snapshot` with `NUMERIC` amount, currency, source, and timestamped raw payload reference; and persistence of every `balance_infos` entry. Retain account scope and never invent a model balance. Do not add a uniqueness constraint to snapshot rows.
- [ ] Add `llm_balance_capture_slot` with the sole scheduled idempotency constraint `(account_id, scheduled_hour)`; manual/retry responses remain separately stored immutable snapshots.
- [ ] Add a stable non-secret API-key fingerprint/display label for reports. Do not return, log, or group API responses by raw `api_key_ref`.
- [ ] Create `llm_official_balance_delta_report` keyed by account, period, and currency with `NUMERIC` opening/closing/delta fields and status, plus `llm_local_model_cost_report` keyed by account, model, period, and CNY with `NUMERIC` token-cost components and status. Add their indexes and migration rollback.
- [ ] Run migration SQL checks and focused Go tests.

### Task 4: Run DeepSeek reconciliation hourly with server lifecycle ownership

**Files:**
- Modify: `ChenWeb/server/api/llmreconcile/jobs.go`
- Modify: `ChenWeb/server/api/routes.go` or the established server startup location
- Modify: `ChenWeb/server/cmd/config/config.go`
- Test: `ChenWeb/server/api/llmreconcile/jobs_test.go`

- [ ] Write failing scheduler/bootstrap tests for the next-hour boundary, advisory-lock exclusion, exactly-once `RegisterRoutes` initialization, sink installation, Echo shutdown cancellation, and the documented NULL/system-user behavior for scheduled balance requests.
- [ ] Run it to verify failure.
- [ ] Replace the daily timer with an hour-boundary scheduler and database/advisory lock.
- [ ] Initialize sink installation and hourly reconciliation exactly once from `RegisterRoutes` after database availability; bind cancellation to Echo server shutdown.
- [ ] Run the focused scheduler/reconciliation suite.

## Chunk 3: Dual billing reports and APIs

### Task 5: Parse DeepSeek CNY pricing and calculate local spend

**Files:**
- Modify: `ChenWeb/server/api/llmimport/models_toml.go` or create `ChenWeb/server/api/llmbilling/pricing.go`
- Test: matching `*_test.go`
- Modify: `ChenWeb/server/api/llmreporthandler/store.go`
- Modify: `ChenWeb/server/api/llmreporthandler/store_test.go`

- [ ] Write failing tests for CNY pricing parsing, peak/off-peak determination, holidays/makeup workdays, model aliases, and per-model cache-hit/cache-miss/output math.
- [ ] Write failing no-float-loss and deterministic-rounding tests covering provider parsing, store scans, and JSON responses.
- [ ] Run those tests to verify failure.
- [ ] Implement a typed billing parser that excludes `[model-api-keys]` and billing sections from model import; use model-specific CNY-per-million-token rates, peak rules, China holidays, and deterministic decimal rounding. Unknown/malformed pricing and unpriced models fail closed.
- [ ] Add failing persistence tests for an incomplete official period, a balance-increase credit/top-up status, and independent official/local report rows; then implement those states.
- [ ] Carry decimal values as canonical strings (or a decimal type) from provider parsing through store scans and JSON responses.
- [ ] Add a dedicated per-account/per-model local-cost report using `NUMERIC`. Do not allocate official balance deltas to models.
- [ ] Add a separate official account/currency balance-delta report, marking missing boundaries as incomplete and balance increases as credit/top-up candidates rather than negative spend.
- [ ] Run `go test ./server/api/llmreporthandler ./server/api/llmimport`.

### Task 6: Expose balance history and dual report tracks

**Files:**
- Modify: `ChenWeb/server/api/llmreporthandler/handler.go`
- Modify: `ChenWeb/server/api/llmreporthandler/store.go`
- Modify: `ChenWeb/server/api/llmreporthandler/handler_test.go`
- Modify: `ChenWeb/server/api/llmreporthandler/store_test.go`

- [ ] Write failing handler/store tests for non-secret API-key-display/currency hourly balance series, separate official/local spend fields, and decimal JSON serialization.
- [ ] Write authorization tests proving only existing authorized admin endpoints return `user_id`; public/non-admin reports and events cannot retrieve another caller's identity.
- [ ] Verify failure, then implement the smallest response extension with account/API-key and currency labels.
- [ ] Confirm API-key filtering cannot merge currencies, expose secrets, or represent official values as per-model values.
- [ ] Run the package test suite.

## Chunk 4: Dashboard and end-to-end verification

### Task 7: Render official balances and local CNY model spending

**Files:**
- Modify: `ChenWeb/web/src/lib/components/home3/llm-activities-client.ts`
- Modify: `ChenWeb/web/src/lib/components/home3/llm-activities-client.test.ts`
- Modify: `ChenWeb/web/src/lib/components/home3/llm-activities-view.svelte`
- Test: `ChenWeb/web/src/lib/components/home3/llm-activities-view.test.ts` (create if the project’s Svelte test setup supports it)

- [ ] Write failing client tests for the extended report/balance payloads.
- [ ] Verify failure, implement typed response decoding, then make it pass.
- [ ] Add official account/API-key balance charts with independent CNY/USD series and retain per-model token/local-CNY-spend charts. Labels must identify the track and currency; do not render a derived official-per-model allocation.
- [ ] Run the relevant frontend test command from `ChenWeb/mise.toml`.

### Task 8: Verify coverage and documentation

**Files:**
- Modify: `ChenWeb/docs/superpowers/specs/2026-09-20-llm-telemetry-billing-design.md` if implementation decisions differ
- Modify: `KnowledgeStore` ADR/changelog only if the project’s documentation policy requires it after implementation

- [ ] Run `cd shared/go && go test ./...`.
- [ ] Run focused ChenWeb backend and frontend tests; run `go work sync` if shared dependencies changed.
- [ ] Use `rg` to confirm direct AI-provider/gateway HTTP is absent from ChenWeb production code.
- [ ] Review migration, API payloads, and UI labels for CNY/USD correctness; document intentionally unsupported provider pricing.
- [ ] Commit shared and ChenWeb changes separately with `jj`, then verify a linear `jj log`.
