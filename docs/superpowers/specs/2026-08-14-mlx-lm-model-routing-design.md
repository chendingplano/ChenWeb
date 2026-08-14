# MLX LM Model Routing Design

## Goal

Make the locally hosted `mlx-community/Qwen3-30B-A3B-4bit` model available to
ChenWeb through `.models.toml`. Its `thinking_type` setting must control MLX
LM's per-request `enable_thinking` chat-template option. Thinking is disabled
by default.

## Scope

Add one MLX model definition and extend the shared OpenAI-compatible client
with a host-scoped mapping:

- `host = "mlx-lm"` identifies an MLX LM server.
- For that host, `thinking_type = "enabled"` sends
  `chat_template_kwargs: {"enable_thinking": true}`.
- `thinking_type = "disabled"` or an empty value sends
  `chat_template_kwargs: {"enable_thinking": false}`.
- Other hosts preserve their existing request payloads and behavior.

The model definition uses `http://127.0.0.1:8080`, which ChenWeb's existing
OpenAI-compatible endpoint builder resolves to `/v1/chat/completions`.

## Configuration

The logical model name will be `mlx-qwen3-30B-A3B`:

```toml
[mlx-qwen3-30B-A3B]
host = 'mlx-lm'
model_name = 'mlx-community/Qwen3-30B-A3B-4bit'
api_key = 'local'
base_url = 'http://127.0.0.1:8080'
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

Add the model host to the shared client's configuration and retained client
state. At request construction, add MLX-specific `chat_template_kwargs` only
when the host is `mlx-lm`; this avoids sending an unsupported extension to
other OpenAI-compatible services. The existing normalized values `enabled` and
`disabled` are used; an empty value maps to disabled for MLX.

## Verification

- Add shared-client tests covering enabled, disabled, and empty MLX thinking
  settings, plus a non-MLX regression case with no MLX extension.
- Run the focused shared Go tests.
- Parse ChenWeb's `.models.toml` through its existing model-import test path.
- Start the configured MLX server and make a local chat-completions request if
  it is available, confirming the model endpoint remains reachable.

## Documentation impact

Update the MLX LM user manual with the server command that leaves thinking
selection to ChenWeb request configuration. No prompt or database schema
changes are needed.
