# Jev LLM Provider Design

## Goal

Allow ChenWeb callers that use the shared `api/llm` client to call Jev's hosted decision API through a configured model profile. Jev is not a chat-completions API; each call needs a state and one or more typed questions.

## Configuration

Add a `jev-latest` entry to `.models.toml` with `model_type = 'decision-model'`, `model_name = 'jev-latest'`, `api_key = ''`, and `base_url = 'https://jev-ai.pro/api'`. `decision-model` identifies the Jev-compatible System One protocol; profile names and model names do not select the adapter. TOML provider inference maps every `decision-model` profile to the provider `jev_compatible`, allowing multiple such profiles with different model names or base URLs. Preserve the Jev host as a legacy inference fallback for existing entries without `model_type = 'decision-model'`.

The blank key on the official Jev endpoint is intentional: the adapter resolves it from `JEV_AI_API_KEY` only when the configured base URL is `jev-ai.pro` or a subdomain. Other Jev-compatible endpoints use their configured API key. Keys must not be committed, logged, or returned to browser clients. Provider construction returns the existing missing-key error when no applicable key is configured.

## Shared client API

Add the vendor-neutral protocol provider ID `ProviderJevCompatible` to `shared/go/api/llm` and retain `ProviderJev` as a compatibility alias. Extend `LLMModelDef` with `ModelType`; when it is `decision-model`, ChenWeb's importer assigns provider `jev_compatible`. Extend `llm.Request` with a typed map of Jev question IDs to question definitions. A definition has `Type` (`noul`, `choice`, or `score`), `Instructions`, and `Criteria` with exactly one shape: a map of option IDs to descriptions for `choice`, or an ordered list of labels for `score`; `noul` has no criteria. Validate those combinations before sending. For example, a `choice` question serializes as `{"type":"choice","instructions":"Choose a team","criteria":{"billing":"Payments","technical":"Bugs"}}`.

Jev `Complete` uses the last `RoleUser` message's `Content` as the string `state`, the requested model as `model`, and the typed questions as `questions`; earlier messages are not sent. Reject a missing or whitespace-only user `Content`, and reject a last user message that uses multipart `Parts` rather than `Content`, with a useful local error before making an HTTP request. Missing questions also returns a useful local error.

The adapter posts JSON to `{BaseURL}/v1/systemone` with `Authorization: Bearer ...` and `Content-Type: application/json`. Its default base URL is `https://jev-ai.pro/api`, matching Jev's official SDK base URL; with the `/v1/systemone` path this produces the documented endpoint. For example, the response contains `{"model":"jev-1.13.0","answers":{"urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":379,"output_tokens":70}}`. `Complete` returns the `answers` JSON object compact-encoded as a string in `Response.Content` (not a JSON-quoted string), and maps `usage.input_tokens` and `usage.output_tokens` to `llm.Usage`; missing usage leaves `Response.Usage` nil. Provider failures use the existing provider-error type without exposing the API key. Provider error bodies must be truncated using the existing error convention and must not include request headers. `Stream` returns a clear unsupported-operation error because Jev does not stream.

## Integration behavior

The provider factory dispatches `ProviderJevCompatible` to the adapter. It resolves `JEV_AI_API_KEY` only for the official Jev host when no explicit API key is passed; other bases require an explicit key. The config does not silently translate generic chat requests into Jev questions; callers must provide the question definitions. Existing OpenAI, Anthropic, and OpenAI-compatible provider behavior remains unchanged. The ChenWeb TOML admin API and UI preserve and edit `model_type` so users can create multiple `decision-model` profiles.

## Verification

Use HTTP test servers to verify the request method, URL, authorization header, serialized state/model/questions, answer encoding, and token usage. Cover missing key, state, or questions; invalid question shapes; multipart state; non-success responses; malformed responses; and unsupported streaming. Parse multiple `decision-model` profiles with arbitrary names and base URLs and verify they import as `jev_compatible`; verify the official Jev key stays empty for import and uses the environment fallback only for the official host. Verify TOML admin reads and writes preserve `model_type`. Run focused shared LLM, importer, and TOML admin tests.

## Scope

This adds the shared provider adapter, typed request fields, official-host environment-key resolution, model-type-based TOML provider inference, TOML admin model-type editing, and the Jev profile. It does not add automatic question generation, streaming, or a web-context integration.
