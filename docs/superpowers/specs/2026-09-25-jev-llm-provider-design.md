# Jev LLM Provider Design

## Goal

Allow ChenWeb callers that use the shared `api/llm` client to call Jev's hosted decision API through a configured model profile. Jev is not a chat-completions API; each call needs a state and one or more typed questions.

## Configuration

Add a `jev-latest` entry to `.models.toml` with `host = 'cloud'`, `model_type = 'llm'`, `model_name = 'jev-latest'`, `api_key = ''`, and `base_url = 'https://jev-ai.pro/api'`. The blank key is intentional: the adapter resolves it from the server environment variable `JEV_AI_API_KEY`; it must not be committed to the model file, logged, or returned to browser clients. TOML provider inference must recognize `jev-ai.pro` and import the account as provider `jev` with an empty key. Provider construction returns the existing missing-key error if the environment variable is unset or blank.

## Shared client API

Add `ProviderJev` to `shared/go/api/llm`. Extend `llm.Request` with a typed map of Jev question IDs to question definitions. A definition has `Type` (`noul`, `choice`, or `score`), `Instructions`, and `Criteria` with exactly one shape: a map of option IDs to descriptions for `choice`, or an ordered list of labels for `score`; `noul` has no criteria. Validate those combinations before sending. For example, a `choice` question serializes as `{"type":"choice","instructions":"Choose a team","criteria":{"billing":"Payments","technical":"Bugs"}}`.

Jev `Complete` uses the last `RoleUser` message's `Content` as the string `state`, the requested model as `model`, and the typed questions as `questions`; earlier messages are not sent. Reject a missing or whitespace-only user `Content`, and reject a last user message that uses multipart `Parts` rather than `Content`, with a useful local error before making an HTTP request. Missing questions also returns a useful local error.

The adapter posts JSON to `{BaseURL}/v1/systemone` with `Authorization: Bearer ...` and `Content-Type: application/json`. Its default base URL is `https://jev-ai.pro/api`, matching Jev's official SDK base URL; with the `/v1/systemone` path this produces the documented endpoint. For example, the response contains `{"model":"jev-1.13.0","answers":{"urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":379,"output_tokens":70}}`. `Complete` returns the `answers` JSON object compact-encoded as a string in `Response.Content` (not a JSON-quoted string), and maps `usage.input_tokens` and `usage.output_tokens` to `llm.Usage`; missing usage leaves `Response.Usage` nil. Provider failures use the existing provider-error type without exposing the API key. Provider error bodies must be truncated using the existing error convention and must not include request headers. `Stream` returns a clear unsupported-operation error because Jev does not stream.

## Integration behavior

The provider factory dispatches `ProviderJev` to the adapter and resolves the API key from `JEV_AI_API_KEY` when no key is passed in the provider config. The config does not silently translate generic chat requests into Jev questions; callers must provide the question definitions. Existing OpenAI, Anthropic, and compatible provider behavior remains unchanged.

## Verification

Use HTTP test servers to verify the request method, URL, authorization header, serialized state/model/questions, answer encoding, and token usage. Cover missing key, state, or questions; invalid question shapes; multipart state; non-success responses; malformed responses; and unsupported streaming. Parse a Jev `.models.toml` fixture and verify the provider is `jev`, the profile values are preserved, and the imported API key stays empty. Run the focused shared LLM and ChenWeb importer tests.

## Scope

This adds the shared provider adapter, typed request fields, environment-key resolution, TOML provider inference, and the Jev profile. It does not add a Jev-specific UI, automatic question generation, streaming, or a web-context integration.
