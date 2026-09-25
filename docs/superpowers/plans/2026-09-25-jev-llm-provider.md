# Jev LLM Provider Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable ChenWeb to route any `model_type = 'decision-model'` profile to the Jev-compatible System One protocol, independent of its profile name.

**Architecture:** Add a vendor-neutral Jev-compatible provider to `shared/go/api/llm`, with typed question definitions on `llm.Request`, an official-host environment-key fallback, structured answer JSON in `Response.Content`, and no streaming support. Use `model_type` to select the protocol in the TOML importer. Preserve `model_type` through ChenWeb's model API and editor so users can create multiple decision-model profiles.

**Tech Stack:** Go 1.25, `net/http`, `encoding/json`, TOML model configuration.

---

## File map

- `shared/go/api/llm/types.go`: provider ID and Jev question types/request field.
- `shared/go/api/llm/client.go`: factory dispatch and `JEV_AI_API_KEY` fallback for Jev.
- `shared/go/api/llm/jev.go`: HTTP request, validation, response, usage mapping, and unsupported streaming.
- `shared/go/api/llm/jev_test.go`: focused HTTP behavior coverage.
- `shared/go/api/llm/client_test.go`: provider factory and key resolution behavior.
- `shared/go/api/ApiTypes/ApiTypes.go`: TOML model type field.
- `ChenWeb/server/api/llmimport/models_toml.go`: infer provider from `decision-model`.
- `ChenWeb/server/api/llmimport/models_toml_test.go`: importer mapping behavior.
- `ChenWeb/server/api/llmadminhandler/toml_handler.go` and tests: preserve model type in the TOML API.
- `ChenWeb/server/api/llmadminhandler/model_handler.go`: preserve model type in AddModel.
- `ChenWeb/web/src/lib/components/home3/llm-models-client.ts` and `llm-models-view.svelte`: allow model type edits in the TOML UI.
- `ChenWeb/web/src/lib/components/home3/llm-accounts-client.ts` and `llm-accounts-view.svelte`: carry model type through Add Model.
- `ChenWeb/.models.toml`: add the keyless `jev-latest` entry.

## Chunk 1: Shared Jev client

### Task 1: Add Jev request types and provider dispatch

**Files:**
- Modify: `shared/go/api/llm/types.go`
- Modify: `shared/go/api/llm/client.go`
- Modify: `shared/go/api/llm/client_test.go`

- [x] Add `ProviderJevCompatible`, retain `ProviderJev` as a compatibility alias, and add `ModelType` to `LLMModelDef`.
- [x] Keep typed Jev question validation, answer and usage mapping, and last plain-text user state handling.
- [x] Resolve `JEV_AI_API_KEY` only for the official Jev host when the configured key is blank; require configured keys for other compatible endpoints.
- [x] Dispatch the generic provider to the shared Jev-compatible adapter.

### Task 2: Implement Jev completion behavior

**Files:**
- Create: `shared/go/api/llm/jev.go`
- Create: `shared/go/api/llm/jev_test.go`

- [x] Validate nonblank `Content` on the last user message, reject multipart state, require at least one question, and validate question types/criteria before network I/O.
- [x] POST `{baseURL}/v1/systemone` with Bearer authorization, `model`, `state`, and `questions`; trim a trailing slash from configured base URLs.
- [x] Decode `answers` and optional token usage; return compact JSON for `answers` as `Response.Content`, and map usage fields to `llm.Usage`.
- [x] Wrap HTTP and decode failures in `ProviderError` without echoing request content or authorization headers; cap stored error bodies at 512 bytes. Cover truncation with an oversized provider error response.
- [x] Implement `Stream` with an explicit unsupported-operation error.
- [x] Cover serialization, response mapping, validation, key handling, HTTP failure, malformed response, and streaming behavior.

## Chunk 2: ChenWeb model registration

### Task 3: Recognize Jev in TOML import and register its profile

**Files:**
- Modify: `ChenWeb/server/api/llmimport/models_toml.go`
- Modify: `ChenWeb/server/api/llmimport/models_toml_test.go`
- Modify: `ChenWeb/.models.toml`

- [x] Infer `jev_compatible` from `model_type = 'decision-model'`; retain Jev hostname inference as a legacy fallback.
- [x] Add importer coverage for multiple arbitrary profile/model names and base URLs with decision-model type.
- [x] Set `[jev-latest]` to `model_type = 'decision-model'`.
- [x] Preserve and edit `model_type` through the TOML API, TOML editor, and Add Model flow.

## Documentation and verification

- The Jev devdoc in KnowledgeStore documents both the `.models.toml` configuration and the vendor-neutral provider call shape.
- Run `gofmt` on changed Go files.
- Run `cd shared/go && go test ./api/llm` and `cd ChenWeb && go test ./server/api/llmimport ./server/api/llmadminhandler`; all focused suites must pass.
- `shared/go` dependency files do not need updating; no dependency is added.

## Commit notes

- Commit `shared/go` and ChenWeb application changes separately through `jj`, only after confirming each repository has no unrelated changes. Never commit with raw `git`.
