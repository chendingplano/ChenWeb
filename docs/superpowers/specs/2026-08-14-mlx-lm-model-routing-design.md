# MLX LM Model Routing Design

## Goal

Make the locally hosted `mlx-community/Qwen3-30B-A3B-4bit` model available to
ChenWeb through `.models.toml`. Its `thinking_type` setting must control MLX
LM's per-request `enable_thinking` chat-template option. Thinking is disabled
by default.

## Scope

Add one MLX model definition and extend both shared OpenAI-compatible client
paths with a host-scoped mapping:

- `host = "mlx-lm"` identifies an MLX LM server.
- For that host, `thinking_type = "enabled"` sends
  `chat_template_kwargs: {"enable_thinking": true}`.
- `thinking_type = "disabled"` or an empty value sends
  `chat_template_kwargs: {"enable_thinking": false}`.
- MLX payloads contain `chat_template_kwargs` and never the existing provider
  extension `thinking`.
- Other hosts preserve their existing request payloads and behavior.

The model definition uses `http://127.0.0.1:8095`, which ChenWeb's existing
OpenAI-compatible endpoint builder resolves to `/v1/chat/completions`.

## Configuration

The logical model name will be `mlx-qwen3-30B-A3B`:

```toml
[mlx-qwen3-30B-A3B]
host = 'mlx-lm'
model_name = 'mlx-community/Qwen3-30B-A3B-4bit'
api_key = 'local'
base_url = 'http://127.0.0.1:8095'
timeout_sec = 300
thinking_type = 'disabled'
max_inflight = 1
max_requests_per_minute = 30
max_tokens_per_minute = 200000
token_reserve_per_call = 256
```

`max_inflight = 1` avoids concurrent requests exhausting the unified memory
used by this large local model.

## Implementation

Carry `Host` and `ThinkingType` from `ApiTypes.LLMModelDef` through
ChenWeb's direct model-resolution paths into both shared client types:

- JSON-only `OpenAIJSONClient` / `OpenAIJSONClientConfig`, used by document
  processors and one-shot reviewers.
- Tool-capable `openaiClient` / `ProviderConfig`, used by
  `BuildReviewerToolClient` for `Complete` and streaming/tool requests.

At request construction, both client types add MLX-specific
`chat_template_kwargs` only when `Host` is `mlx-lm`; this avoids sending an
unsupported extension to other OpenAI-compatible services. The normalized
values `enabled` and `disabled` are used; an empty value maps to disabled for
MLX. On the MLX path, do not send the existing `thinking` provider extension;
on non-MLX paths retain the current behavior of adding it only for enabled
thinking.

The admin/import database path is intentionally out of scope: this request
uses the `.models.toml` file directly through environment variables, and the
current imported account/profile schema does not persist a model host. The
importer must continue parsing the new entry without error, but importing it
does not promise MLX-specific thinking behavior.

## Verification

- Add shared-client tests covering enabled, disabled, and empty MLX thinking
  settings, asserting the exact `chat_template_kwargs` value and the absence
  of the legacy `thinking` field.
- Test non-MLX JSON extraction, generic `Complete`, and streaming/tool calls
  to confirm no MLX-specific field is added and existing enabled-thinking
  behavior remains unchanged.
- Test host propagation from `.models.toml` through the direct ChenWeb client
  construction paths. Parse the new entry through the existing importer as a
  compatibility check only.
- Run focused shared and ChenWeb Go tests.
- Start MLX LM with `--host 127.0.0.1 --port 8095` and make a local
  chat-completions request if the service is available, confirming the model
  endpoint remains reachable.

## Documentation impact

Update `ThirdParty/mlx-lm/USER_MANUAL.md` with this server command, which
leaves thinking selection to ChenWeb request configuration:

```sh
mise run server -- --host 127.0.0.1 --port 8095 --model mlx-community/Qwen3-30B-A3B-4bit
```

No prompt or database schema changes are needed.
