# 3D Product Drawing Review Page Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let authenticated Development users generate a ventilator drawing, review it in the System Admin → Resources page, and explicitly keep or ignore it.

**Architecture:** Extend the existing product-drawing provider into a two-phase pending-artifact service. Generated PNGs are stored in a temporary directory until Keep moves them into the shared KnowledgeStore product-drawings directory; Ignore removes them. Add a typed Svelte service and a Svelte 5 workbench-style view wired into the existing nav rail/content panel.

**Tech Stack:** Go, Echo v4, standard-library filesystem/HTTP APIs, Svelte 5 runes, TypeScript, existing ChenWeb dashboard components, Vitest.

---

## Chunk 1: Pending-artifact backend

### Task 1: Add failing lifecycle tests

**Files:**
- Modify: `server/api/productdrawings/handler_test.go`

- [ ] **Step 1: Write tests for pending generation**

  Add tests asserting generation returns JSON with an opaque token, preview URL, prompt, model, and expiry; writes only to pending storage; and does not create a permanent product-drawing file.

- [ ] **Step 2: Write tests for preview, keep, and ignore**

  Add tests for authenticated-handler behavior using Echo contexts: preview returns the pending PNG, Keep moves it to the permanent output directory and returns filename metadata, Ignore removes it, repeated Ignore is safe, and invalid/expired tokens return `404`.

- [ ] **Step 3: Run tests to verify failure**

  Run: `cd server && go test ./api/productdrawings -run 'Pending|Keep|Ignore' -v`

  Expected: FAIL because pending endpoints and lifecycle types do not exist.

### Task 2: Implement pending storage and lifecycle handlers

**Files:**
- Modify: `server/api/productdrawings/models.go`
- Modify: `server/api/productdrawings/handler.go`
- Modify: `server/api/productdrawings/prompt.go`

- [ ] **Step 1: Add pending metadata and configuration**

  Define pending response metadata, a pending-directory resolver (with `PRODUCT_DRAWINGS_PENDING_DIR` override and a safe temporary default), token metadata, and a bounded expiry duration.

- [ ] **Step 2: Implement pending generation**

  Add `GeneratePending`, reuse the canonical prompt/provider, generate PNG bytes, create an opaque random token, write the pending PNG exclusively, and return JSON without exposing absolute paths.

- [ ] **Step 3: Implement secure preview**

  Add `ServePendingContent`, validate token format and metadata, reject expired/missing tokens as `404`, and stream only the server-resolved PNG.

- [ ] **Step 4: Implement Keep and Ignore**

  Add `KeepPending` to move pending content exclusively into `PRODUCT_DRAWINGS_DIR` and return the final filename; add `IgnorePending` to remove pending content safely. Ensure failures do not leave partial permanent files.

- [ ] **Step 5: Preserve existing endpoint behavior**

  Keep `POST /api/v1/product-drawings` working for existing callers, using the same provider/prompt internals while retaining its existing PNG response contract.

- [ ] **Step 6: Run backend tests**

  Run: `cd server && gofmt -w api/productdrawings/*.go && go test ./api/productdrawings -v`

  Expected: PASS.

## Chunk 2: Route registration and backend documentation

### Task 3: Register pending routes

**Files:**
- Modify: `server/api/routes.go`

- [ ] **Step 1: Add route registration tests or route assertions**

  Extend the existing route test pattern to verify the new routes are registered under the authenticated `/api/v1` group.

- [ ] **Step 2: Register endpoints**

  Add `POST /product-drawings/generate`, `GET /product-drawings/pending/:token/content`, `POST /product-drawings/pending/:token/keep`, and `DELETE /product-drawings/pending/:token`.

- [ ] **Step 3: Run route tests**

  Run: `cd server && go test ./api -run 'Route|ProductDrawing' -v`

  Expected: PASS.

### Task 4: Update backend feature documentation

**Files:**
- Modify: `docs/features/product-drawing-generation-api.md`

- [ ] **Step 1: Document the review flow**

  Add request/response examples for generate, preview, keep, and ignore; explain pending expiry, environment variables, authentication, and the permanent shared output path.

## Chunk 3: Frontend API client and navigation wiring

### Task 5: Add typed frontend service tests first

**Files:**
- Create: `web/src/lib/services/productDrawingService.ts`
- Create: `web/src/lib/services/productDrawingService.test.ts`

- [ ] **Step 1: Write failing service tests**

  Test request methods, JSON parsing, image URL construction, error extraction, and return types for generate, keep, and ignore.

- [ ] **Step 2: Run tests to verify failure**

  Run: `cd web && npm test -- src/lib/services/productDrawingService.test.ts`

  Expected: FAIL because the service does not exist.

- [ ] **Step 3: Implement the service**

  Add typed functions using same-origin credentials and the exact backend routes. Keep the image URL as an API URL and never expose filesystem paths.

- [ ] **Step 4: Run service tests**

  Run: `cd web && npm test -- src/lib/services/productDrawingService.test.ts`

  Expected: PASS.

### Task 6: Wire the Development navigation

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] **Step 1: Add the navigation item**

  Add `sysadmin-resources-product-drawings` labeled `Generate 3D Product Drawings` under the existing Resources children.

- [ ] **Step 2: Add the content-panel branch**

  Import the new view and render it for that child ID, passing `darkMode` like the neighboring resource views.

- [ ] **Step 3: Run type checking**

  Run: `cd web && npm run check`

  Expected: PASS after the view exists; if run before Task 7, the expected missing-component failure is acceptable.

## Chunk 4: Product drawing review UI

### Task 7: Add view state tests

**Files:**
- Create: `web/src/lib/components/home3/product-drawings-view.test.ts`

- [ ] **Step 1: Write failing state tests**

  Test idle → generating → pending, Keep success, Ignore success, generation errors, disabled actions while busy, prompt visibility, and cleanup invoking Ignore for an uncommitted pending token.

- [ ] **Step 2: Run tests to verify failure**

  Run: `cd web && npm test -- src/lib/components/home3/product-drawings-view.test.ts`

  Expected: FAIL because the view/state module does not exist.

### Task 8: Implement the view

**Files:**
- Create: `web/src/lib/components/home3/product-drawings-view.svelte`
- Create: `web/src/lib/components/home3/product-drawings-view-state.ts`

- [ ] **Step 1: Implement the state model**

  Add a small testable state/controller layer for generation, pending metadata, keep, ignore, error state, and best-effort cleanup.

- [ ] **Step 2: Implement the Svelte view**

  Build the approved technical-workbench layout with header, generation CTA, loading/error states, large accessible image preview, collapsible exact prompt, model/expiry metadata, explicit Keep and Ignore buttons, success feedback, and generate-again action. Follow Svelte 5 event syntax and the existing darkMode design tokens.

- [ ] **Step 3: Run frontend tests and checks**

  Run: `cd web && npm test -- src/lib/services/productDrawingService.test.ts src/lib/components/home3/product-drawings-view.test.ts && npm run check`

  Expected: PASS.

## Chunk 5: Final verification

### Task 9: Verify the complete feature

**Files:**
- No new files; verify all changed files.

- [ ] **Step 1: Run focused backend verification**

  Run: `cd server && go test ./api/productdrawings ./api -run 'ProductDrawing|Route' -v && go build ./... && go vet ./api/productdrawings ./api`

- [ ] **Step 2: Run frontend verification**

  Run: `cd web && npm test -- src/lib/services/productDrawingService.test.ts src/lib/components/home3/product-drawings-view.test.ts && npm run check`

- [ ] **Step 3: Review diff boundaries**

  Run: `git -C /Users/cding/Workspace/ChenWeb diff --check && git -C /Users/cding/Workspace/ChenWeb status --short`

  Expected: only the product drawing backend, prompt/docs, frontend service/view/navigation, and associated tests are changed by this task; existing unrelated work remains untouched.
