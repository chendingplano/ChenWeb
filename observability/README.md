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
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_HEADERS="authorization=${CLICKSTACK_API_KEY}"
export JIMO_LOG_FORMAT=json
export FILE_LOGGER=lumberjack
export LOG_FILE_DIR=/Users/cding/Apps/ChenWebLog
mise dev
```

If ClickStack is running without an ingestion key requirement, leave `CLICKSTACK_API_KEY` and `OTEL_EXPORTER_OTLP_HEADERS` empty.

## Ports

- HyperDX UI: `http://localhost:8088`
- OTLP HTTP: `http://localhost:4318`
- OTLP gRPC: `localhost:4317`
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
