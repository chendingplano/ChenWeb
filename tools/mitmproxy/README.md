# mitmproxy LLM Trace Capture

This folder contains a local capture path for VS Code sessions that talk to
Claude Code or Codex outside ChenWeb's own agent runner.

`llm_trace_addon.py` listens to `mitmproxy` HTTP flows, filters for Anthropic
and OpenAI hosts, writes optional JSONL logs locally, and forwards each
captured request/response pair to ChenWeb:

```text
POST /api/internal/mitmproxy/ingest
```

ChenWeb then converts each exchange into an OpenTelemetry span named:

```text
agent_proxy.http_exchange
```

When observability is enabled, those spans show up in HyperDX.

## macOS Setup

Install mitmproxy:

```bash
brew install --cask mitmproxy
```

Generate mitmproxy certificates once:

```bash
mitmdump --version
```

Import `~/.mitmproxy/mitmproxy-ca-cert.pem` into Keychain Access and set the
certificate trust to `Always Trust`.

## Start ChenWeb

ChenWeb must be running locally with observability enabled and a local ingest
token configured:

```bash
cd /Users/cding/Workspace/ChenWeb
export OBSERVABILITY_ENABLED=true
export OTEL_SERVICE_NAME=chenweb
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14318
export OTEL_EXPORTER_OTLP_HEADERS="authorization=${CLICKSTACK_API_KEY}"
export MITM_TRACE_INGEST_TOKEN=replace-me
mise exec -- ./.cache/server.exe serve --dir ./pb_data --dev --http=:8080
```

## Start mitmproxy

```bash
cd /Users/cding/Workspace/ChenWeb
export MITM_TRACE_INGEST_TOKEN=replace-me
export MITMTRACE_JSONL_PATH="$HOME/.mitmproxy/chenweb-llm-trace.jsonl"
./tools/mitmproxy/start_llm_trace_capture.sh
```

Defaults:

- Proxy listener: `127.0.0.1:8081`
- ChenWeb ingest URL: `http://127.0.0.1:8080/api/internal/mitmproxy/ingest`
- Allowed hosts: `api.anthropic.com,api.openai.com`
- Body limit: `16384` bytes

Useful environment overrides:

- `MITMTRACE_ALLOWED_HOSTS`
- `MITMTRACE_MAX_BODY_BYTES`
- `MITMTRACE_INCLUDE_HEADERS=true`
- `CHENWEB_MITM_INGEST_URL`
- `MITMTRACE_AGENT_KIND`
- `MITMTRACE_AGENT_NAME`
- `MITMTRACE_AGENT_SESSION`

## Launch VS Code Through the Proxy

Close existing VS Code windows, then launch it from a shell with the proxy
environment:

```bash
export HTTP_PROXY=http://127.0.0.1:8081
export HTTPS_PROXY=http://127.0.0.1:8081
export NO_PROXY=127.0.0.1,localhost
export MITMTRACE_AGENT_NAME="VS Code"
open -na "Visual Studio Code"
```

If the extension runtime does not inherit the shell environment cleanly, use
macOS system proxy settings for `127.0.0.1:8081` while capturing.

## HyperDX Queries

Look for the proxy-captured exchanges with:

```text
span.name:"agent_proxy.http_exchange"
```

Helpful filters:

```text
llm.provider:"anthropic"
llm.provider:"openai"
agent.kind:"claude_code"
agent.kind:"codex"
```
