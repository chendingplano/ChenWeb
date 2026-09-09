# Product Drawing Generation API Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authenticated ChenWeb endpoint that generates a fresh labeled 3D ventilator drawing with the configured OpenAI image model and saves/returns the PNG from the shared KnowledgeStore product-drawings directory.

**Architecture:** Add a focused `productdrawings` handler package. Reuse the existing OpenAI-compatible image transport in `server/api/imagehandler` where possible, but keep the product-drawing prompt selection, shared-path persistence, and HTTP contract isolated. Inject a small provider function/interface in tests so no live model call is required.

**Tech Stack:** Go, Echo v4, existing ChenWeb route/auth/logger conventions, standard-library HTTP/JSON/filesystem APIs, existing image-generation configuration (`IMAGE_GEN_BASE_URL`, `IMAGE_GEN_API_KEY`, `IMAGE_GEN_MODEL`).

---

## Chunk 1: Prompt and provider boundary

### Task 1: Add the canonical ventilator prompt

**Files:**
- Create: `prompts/prompt-ventilator-exploded-view-v1.md`

- [ ] **Step 1: Write the prompt file**

  Add the approved prompt describing a clean 3D exploded-view ICU ventilator, its visible parts, numbered callouts, and exact labels. Keep it model-oriented prose without API code or credentials.

- [ ] **Step 2: Verify prompt contents**

  Run: `rg -n "ventilator|Outer housing|Touchscreen display|Main control board" prompts/prompt-ventilator-exploded-view-v1.md`

  Expected: the subject and required labels are present.

### Task 2: Define product-drawing types and provider interface

**Files:**
- Create: `server/api/productdrawings/models.go`
- Create: `server/api/productdrawings/prompt.go`
- Create: `server/api/productdrawings/provider.go`
- Test: `server/api/productdrawings/provider_test.go`

- [ ] **Step 1: Write failing prompt/provider tests**

  Test that the default request resolves to subject `ventilator`, loads the versioned prompt, selects the configured/default model, and that a fake provider receives the prompt and model.

- [ ] **Step 2: Run tests to verify failure**

  Run: `cd server && go test ./api/productdrawings -run 'TestPrompt|TestProvider' -v`

  Expected: FAIL because the package and implementation do not exist.

- [ ] **Step 3: Implement minimal types and provider boundary**

  Add request and provider result types, canonical prompt loading relative to the ChenWeb project root/configured prompt directory, and an interface/function type accepting context, model, and prompt and returning PNG bytes. Use the existing image-generation environment variables and default model behavior; do not hard-code secrets.

- [ ] **Step 4: Run focused tests**

  Run: `cd server && go test ./api/productdrawings -run 'TestPrompt|TestProvider' -v`

  Expected: PASS.

## Chunk 2: HTTP handler and filesystem persistence

### Task 3: Add handler tests first

**Files:**
- Create: `server/api/productdrawings/handler_test.go`

- [ ] **Step 1: Write failing handler tests**

  Cover successful `POST` behavior (`201`, `image/png`, `Content-Disposition`, `X-Drawing-Filename`, body bytes), default ventilator subject/model, unique saved PNG in a test directory, malformed JSON, unsupported subject/model, missing provider configuration, provider failure (`502`), and write failure (`500`). Use a fake provider and a temporary directory injected through handler dependencies.

- [ ] **Step 2: Run tests to verify failure**

  Run: `cd server && go test ./api/productdrawings -run 'TestGenerate' -v`

  Expected: FAIL because the handler is not implemented.

### Task 4: Implement the handler and persistence

**Files:**
- Modify: `server/api/productdrawings/models.go`
- Create: `server/api/productdrawings/handler.go`

- [ ] **Step 1: Implement request validation and defaults**

  Decode an optional JSON body, default the subject to `ventilator`, reject unsupported values, resolve the configured OpenAI Image 2.5 model, and return structured JSON errors.

- [ ] **Step 2: Implement generation and PNG persistence**

  Call the provider with the loaded prompt, reject empty/non-PNG provider output as a provider failure, create the shared directory when necessary, and write a timestamp/unique-ID filename with exclusive creation so existing files cannot be overwritten.

- [ ] **Step 3: Implement the PNG response**

  Return `201 Created`, `image/png`, attachment disposition, the exact saved filename header, and the same bytes written to disk. Log request lifecycle and failures using the existing `EchoFactory` logger pattern.

- [ ] **Step 4: Run handler tests**

  Run: `cd server && go test ./api/productdrawings -run 'TestGenerate' -v`

  Expected: PASS.

## Chunk 3: Route registration and live-provider adapter

### Task 5: Connect the OpenAI provider and route

**Files:**
- Modify: `server/api/productdrawings/provider.go`
- Modify: `server/api/routes.go`
- Test: `server/api/productdrawings/provider_test.go`

- [ ] **Step 1: Add provider HTTP tests**

  Use an `httptest.Server` to assert the OpenAI-compatible request uses the configured model/prompt, bearer authorization, one image, and decodes `b64_json` or image URL responses. Assert provider errors are returned without leaking the API key in the error text.

- [ ] **Step 2: Run provider tests to verify failure**

  Run: `cd server && go test ./api/productdrawings -run 'TestOpenAI' -v`

  Expected: FAIL until the live adapter is connected.

- [ ] **Step 3: Implement the adapter**

  Reuse or extract the existing OpenAI-compatible transport from `server/api/imagehandler/generate.go` so the product-drawing API supports the same configured endpoint/protocol and response formats. Preserve timeout and response-size safeguards.

- [ ] **Step 4: Register the endpoint**

  Import `productdrawings` and add `apiGroup.POST("/product-drawings", productdrawings.Generate)` beside the existing image-generation routes. Keep it inside the existing authenticated `/api/v1` group.

- [ ] **Step 5: Run provider and route tests**

  Run: `cd server && go test ./api/productdrawings ./api -run 'ProductDrawing|OpenAI' -v`

  Expected: PASS.

## Chunk 4: Documentation and verification

### Task 6: Document usage and verify the integration

**Files:**
- Create or modify: `docs/features/product-drawing-generation-api.md`

- [ ] **Step 1: Document configuration and usage**

  Document required environment variables, the default model, request/response examples, shared output directory, authentication requirement, and error statuses. Do not include real keys.

- [ ] **Step 2: Run formatting and focused verification**

  Run: `gofmt -w server/api/productdrawings/*.go server/api/routes.go && cd server && go test ./api/productdrawings`

  Expected: PASS.

- [ ] **Step 3: Run ChenWeb build and vet**

  Run: `cd server && go build ./... && go vet ./api/productdrawings ./api`

  Expected: successful build and no new vet findings.

- [ ] **Step 4: Review the final diff**

  Run: `git -C /Users/cding/Workspace/ChenWeb diff --check && git -C /Users/cding/Workspace/ChenWeb status --short`

  Expected: only the prompt, API implementation/tests/routes, and feature documentation are changed; existing user changes remain untouched.
