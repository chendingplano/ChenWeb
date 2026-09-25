# Jev LLM Provider Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable ChenWeb's shared Go LLM client to call Jev's structured decision API through a Jev model profile.

**Architecture:** Add a dedicated Jev provider adapter to `shared/go/api/llm`, with typed question definitions on `llm.Request`, environment-based key resolution, structured answer JSON in `Response.Content`, and no streaming support. Register the Jev provider in ChenWeb's `.models.toml` importer and add the `jev-latest` profile without storing a secret.

**Tech Stack:** Go 1.25, `net/http`, `encoding/json`, TOML model configuration.

---

## File map

- `shared/go/api/llm/types.go`: provider ID and Jev question types/request field.
- `shared/go/api/llm/client.go`: factory dispatch and `JEV_AI_API_KEY` fallback for Jev.
- `shared/go/api/llm/jev.go`: HTTP request, validation, response, usage mapping, and unsupported streaming.
- `shared/go/api/llm/jev_test.go`: focused HTTP behavior coverage.
- `shared/go/api/llm/client_test.go`: provider factory and key resolution behavior.
- `ChenWeb/server/api/llmimport/models_toml.go`: infer provider `jev` from `jev-ai.pro`.
- `ChenWeb/server/api/llmimport/models_toml_test.go`: importer mapping behavior.
- `ChenWeb/.models.toml`: add the keyless `jev-latest` entry.

## Chunk 1: Shared Jev client

### Task 1: Add Jev request types and provider dispatch

**Files:**
- Modify: `shared/go/api/llm/types.go`
- Modify: `shared/go/api/llm/client.go`
- Modify: `shared/go/api/llm/client_test.go`

- [x] Add `ProviderJev` and a typed `JevQuestion` whose custom wire encoding emits the Jev `criteria` key: `noul` emits no criteria; `choice` emits its nonempty option-ID-to-description map; `score` emits its nonempty ordered label list. Reject unknown types, criteria on `noul`, missing criteria on `choice`/`score`, and the wrong criteria field for each type. Add `JevQuestions map[string]JevQuestion` to `Request`.
- [x] Make `NewClient` resolve an empty Jev `APIKey` from trimmed `JEV_AI_API_KEY`; unset, empty, or whitespace-only values return `ErrMissingAPIKey`. Preserve explicit config key precedence and existing missing-key behavior for all other providers.
- [x] Dispatch Jev clients to `jevClient`; default Jev base URL to `https://jev-ai.pro/api`.
- [x] Add a factory case for Jev and a focused missing-key/env-fallback check.

### Task 2: Implement Jev completion behavior

**Files:**
- Create: `shared/go/api/llm/jev.go`
- Create: `shared/go/api/llm/jev_test.go`

- [x] Validate nonblank `Content` on the last user message, reject multipart state, require at least one question, and validate question types/criteria before network I/O.
- [x] POST `{baseURL}/v1/systemone` with Bearer authorization, `model`, `state`, and `questions`; trim a trailing slash from configured base URLs.
- [x] Decode `answers` and optional token usage; return compact JSON for `answers` as `Response.Content`, and map usage fields to `llm.Usage`.
- [x] Wrap HTTP and decode failures in `ProviderError` without echoing request content or authorization headers; cap stored error bodies at 512 bytes. Cover truncation with an oversized provider error response.
- [x] Implement `Stream` with an explicit unsupported-operation error.
- [x] Cover serialization, response mapping, validation, HTTP failure, malformed response, and streaming behavior.

## Chunk 2: ChenWeb model registration

### Task 3: Recognize Jev in TOML import and register its profile

**Files:**
- Modify: `ChenWeb/server/api/llmimport/models_toml.go`
- Modify: `ChenWeb/server/api/llmimport/models_toml_test.go`
- Modify: `ChenWeb/.models.toml`

- [x] Infer provider `jev` for the `jev-ai.pro` host before generic OpenAI-compatible inference.
- [x] Add an importer fixture asserting Jev provider, base URL, model profile values, and blank account key.
- [x] Add `[jev-latest]` with `host = 'cloud'`, `model_type = 'llm'`, `model_name = 'jev-latest'`, `api_key = ''`, and `base_url = 'https://jev-ai.pro/api'`, matching the file's existing rate and timeout fields.

## Documentation and verification

- No separate developer guide is needed for this initial adapter; document the call shape on the exported request types and in package comments.
- Run `gofmt` on changed Go files.
- Run `cd shared/go && go test ./api/llm` and `cd ChenWeb && go test ./server/api/llmimport`; both focused package suites must pass.
- `shared/go` dependency files do not need updating; no dependency is added.

## Commit notes

- Commit `shared/go` and ChenWeb application changes separately through `jj`, only after confirming each repository has no unrelated changes. Never commit with raw `git`.
