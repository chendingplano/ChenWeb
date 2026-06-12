# Agent Trace

`server/api/agenttrace` normalizes coding-agent telemetry into one ChenWeb trace
schema, then runs deterministic evaluations over that trace. It is intentionally
small, similar in spirit to AgentDog: capture one run, adapt it into a canonical
shape, score the behavior, and optionally persist/export the result.

## Core Model

- `Trace`: canonical agent run with input, output, tool calls, token usage,
  latency/cost fields, events, and metadata.
- `Plugin`: adapter interface for one agent runtime.
- `Registry`: maps ChenWeb `runtime_kind` values such as `codex` or
  `claude_code` to plugins.
- `Scorer`: deterministic checks such as `ContainsAnswer`, `UsedTools`,
  `AvoidedTools`, and `UnderTokenLimit`.

## Built-In Plugins

- `CodexPlugin`: parses `codex exec --json` JSONL streams, including
  `turn.completed.usage` token counts and completed tool items.
- `ClaudeCodePlugin`: parses Claude Code transcript-style JSONL with user,
  assistant, `tool_use`, `tool_result`, usage blocks, and final `result`
  events from `--output-format stream-json --verbose`.

## Usage

```go
reg := agenttrace.NewDefaultRegistry()

trace, err := reg.Parse(ctx, agenttrace.ParseInput{
    AgentKind: "codex",
    RunID:     "ap-task-run-id",
    Prompt:    issueMarkdown,
    RawLog:    codexJSONL,
})
if err != nil {
    return err
}

report := agenttrace.RunEvaluations([]agenttrace.EvalRun{{
    Case: agenttrace.TestCase{
        Name: "safe-agent-run",
        Scorers: []agenttrace.Scorer{
            agenttrace.AvoidedTools("send_email", "delete_db"),
            agenttrace.UnderTokenLimit(100000),
        },
    },
    Trace: trace,
}})
```

## Adding Another Agent

Implement `Plugin` and register it:

```go
type QwenCodePlugin struct{}

func (QwenCodePlugin) Kind() string { return "qwencode" }

func (QwenCodePlugin) Parse(ctx context.Context, in agenttrace.ParseInput) (agenttrace.Trace, error) {
    // Convert QwenCode logs, OTel events, or transcript JSONL into Trace.
}

reg := agenttrace.NewDefaultRegistry()
_ = reg.Register(QwenCodePlugin{})
```

Keep adapters narrow: they should convert native run data into `Trace`, not
perform policy decisions. Put behavior checks in scorers so they are reusable
across Codex, Claude Code, OpenClaw, OpenCode, QwenCode, and future agents.

## ChenWeb Integration

The agent platform worker buffers machine-readable stdout events after a run,
normalizes them through `NewDefaultRegistry`, stores the full trace in
`ap_agent_trace`, and emits an OpenTelemetry span named
`agent_platform.agent_trace`.

Fetch a stored trace through the workspace-scoped API:

```text
GET /api/v1/ap/w/:slug/traces?agent_kind=codex&limit=100
GET /api/v1/ap/w/:slug/runs/:id/trace
```

Run ad-hoc deterministic checks against a stored trace:

```text
POST /api/v1/ap/w/:slug/runs/:id/trace/evaluate
Content-Type: application/json

{
  "contains_answer": ["summary"],
  "used_tools": ["command_execution"],
  "avoided_tools": ["send_email"],
  "max_tokens": 100000
}
```

HyperDX receives summary attributes by default: agent kind, run id, provider
trace id, trace format, event count, tool names/count, retrieved-context count,
token usage, output length, latency, and cost. Full input/output/tool content is
intentionally excluded from OTel unless the server starts with:

```sh
export AGENT_TRACE_OTEL_INCLUDE_CONTENT=true
```

# References
[1] "AgentDog", https://dzone.com/articles/agentdog-ai-agent-observability
