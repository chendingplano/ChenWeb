# Configurable Image Generation Models Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Product Review and Generate 3D Product Drawings model selects use `[frontend].image_generation_models` from `config.local.toml`.

**Architecture:** Add the model list to ChenWeb's existing `FrontendConfigSection` and `/api/v1/kb/config` response. The affected Svelte components fetch that response on mount, select the first configured model, and render every configured value; missing/empty configuration falls back to `Qwen` and `OpenAI`.

**Tech Stack:** Go, Viper/TOML, Echo, Svelte 5, TypeScript, Vitest/Svelte check.

---

### Task 1: Add tested backend configuration

**Files:**
- Modify: `server/cmd/config/config.go`
- Modify: `server/api/kbhandler/kb_config_handler.go`
- Test: `server/cmd/config/image_generation_models_config_test.go`
- Test: `server/api/kbhandler/kb_config_handler_test.go`
- Modify: `config.toml`

- [ ] Write tests proving the TOML key unmarshals and the empty/missing value returns `Qwen` and `OpenAI`.
- [ ] Run the focused Go tests and observe the expected failure.
- [ ] Add the config field/getter, API response field, and base-config default.
- [ ] Run the focused Go tests and verify they pass.

### Task 2: Configure all affected frontend selects

**Files:**
- Modify: `web/src/lib/services/kbService.ts`
- Modify: `web/src/lib/components/home3/product-review-intake-view.svelte`
- Modify: `web/src/lib/components/home3/product-metric-review-view.svelte`
- Modify: `web/src/lib/components/home3/product-drawings-view.svelte`
- Modify: `web/src/lib/services/productDrawingService.ts`

- [ ] Add the typed frontend-config field and fetch it in the affected components.
- [ ] Replace hard-coded options with iteration over configured model names, preserving a first-model default and readable legacy labels.
- [ ] Widen drawing metadata model typing to support configured names.
- [ ] Run `bun run check` and the frontend build.

### Task 3: Final verification

- [ ] Run the focused Go package tests.
- [ ] Run the ChenWeb frontend checks/build and server build.
- [ ] Review the diff for unrelated changes and document any intentionally unchanged specs/docs.
