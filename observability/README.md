# ChenWeb Observability

ChenWeb uses OpenTelemetry for traces and metrics, ClickStack for local storage/UI, and JSON log files tailed by a local OpenTelemetry Collector.

## Local Startup

Start ClickStack:

```bash
mise obs-up
```

Open HyperDX:

```text
http://localhost:8088
```

Create or copy the ClickStack ingestion API key from HyperDX, then run ChenWeb with:

```bash
export OBSERVABILITY_ENABLED=true
export OTEL_SERVICE_NAME=chenweb
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14318
export OTEL_EXPORTER_OTLP_HEADERS="authorization=${CLICKSTACK_API_KEY}"
export JIMO_LOG_FORMAT=json
export FILE_LOGGER=lumberjack
export LOG_FILE_DIR=/Users/cding/Apps/ChenWebLog
mise dev
```

If ClickStack is running without an ingestion key requirement, leave `CLICKSTACK_API_KEY` and `OTEL_EXPORTER_OTLP_HEADERS` empty.

## Ports

- HyperDX UI: `http://localhost:8088`
- OTLP HTTP: `http://localhost:14318`
- OTLP gRPC: `localhost:14317`
- ClickHouse HTTP, if exposed by the image: `http://localhost:18123`
- ClickHouse native, if exposed by the image: `localhost:19000`

## Signals

- Traces: emitted directly from ChenWeb through OTLP HTTP.
- Metrics: `http.server.duration` emitted directly from ChenWeb through OTLP HTTP.
- Logs: `JimoLogger` writes JSON logs to `LOG_FILE_DIR/app.log`; `chenweb-log-collector` tails that file and forwards logs to ClickStack.

## Agent Debugging Loop

1. Start observability with `mise obs-up`.
2. Start ChenWeb with the env vars above.
3. Reproduce the issue or run a UI/API journey.
4. Search by `service.name:chenweb`, `request.id`, route, status, or error text in HyperDX.
5. Use trace spans to identify slow request paths, then rerun the journey after code changes.

Keep document content, prompts, tokens, cookies, and secrets out of logs. Prefer IDs, counts, durations, statuses, and error categories.

## Agent Run Trace Normalization

ChenWeb has a reusable agent trace module in `server/api/agenttrace`. It treats
each agent runtime as a plugin: built-in adapters currently cover `codex` JSONL
and `claude_code` transcript-style JSONL, while future runtimes such as
QwenCode, OpenClaw, and OpenCode can register their own adapters. The module
normalizes tool calls, final output, token usage, and metadata into one `Trace`
schema, then runs deterministic scorers for CI or runtime checks.

The agent-platform worker stores normalized traces in `ap_agent_trace` and
serves them through:

```text
GET /api/v1/ap/w/:slug/traces?agent_kind=codex&limit=100
GET /api/v1/ap/w/:slug/runs/:id/trace
POST /api/v1/ap/w/:slug/runs/:id/trace/evaluate
```

The evaluation endpoint accepts deterministic checks such as expected answer
fragments, required tools, forbidden tools, and a max token limit. This mirrors
the AgentDog-style workflow: normalize a run, then score behavior separately
from capture.

It also emits an OpenTelemetry span named `agent_platform.agent_trace` for
HyperDX. Search HyperDX for:

```text
span.name:"agent_platform.agent_trace"
```

Default span fields include agent kind, run id, provider trace id, trace format,
event count, tool names/count, retrieved-context count, token usage, output
length, latency, and cost. Full prompt/output content is excluded unless
`AGENT_TRACE_OTEL_INCLUDE_CONTENT=true` is set before starting ChenWeb.

Use this module for agent behavior evaluation and API access. Use OpenTelemetry
for transport, storage, and dashboards.

## VS Code Proxy Capture

For Claude Code or Codex sessions launched directly from VS Code, ChenWeb can
also ingest `mitmproxy`-captured LLM HTTP traffic and emit it to HyperDX as
`agent_proxy.http_exchange` spans.

The local ingest endpoint is:

```text
POST /api/internal/mitmproxy/ingest
```

It is protected by the `MITM_TRACE_INGEST_TOKEN` environment variable and is
intended for local-only use with the helper addon in:

```text
tools/mitmproxy/
```

Search HyperDX for:

```text
span.name:"agent_proxy.http_exchange"
```
