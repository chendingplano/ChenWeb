# Claude Code gateway

A private Bun service that runs SemOS agent services on the Claude Agent SDK (Claude Code as a library). ChenWeb reaches it through `ClaudeCodeHarnessAdapter` (`server/api/agentservicehandler/claude_code_adapter.go`). It accepts the same run protocol and routes as the Pi gateway (`POST /v1/runs`, `/v1/runs/:id/cancel`, `/v1/runs/:id/permissions/:requestId`, `/v1/summaries`, `GET /health`), all behind a bearer secret.

## Tool boundary

Each run is confined to SemOS's knowledge tools:

- `tools: []` turns off every Claude Code built-in tool: files, shell, web and subagents.
- SemOS's tools are the only tools offered. They come from an in-process MCP server named `semos`, and each one calls ChenWeb's `/api/internal/agent-tools/*` with the run's short-lived capability.
- `canUseTool` gates every call. It rejects anything that is not one of the run's tools on that in-process server, and it applies ask/auto approval. It also records the SDK tool-use ID, which ChenWeb uses to store the call and its sources. A tool call that skipped the gate fails.
- `settingSources: []`, `strictMcpConfig`, no skills, agents or plugins, and `persistSession: false`, so user settings, `CLAUDE.md`, other MCP servers and saved sessions are not loaded.
- The Claude Code process gets an empty per-run directory as its working directory and `HOME`, and only the model-credential variables. The gateway secret is never passed to it.

## Run

```bash
cd gateways/claude-code
bun install
CLAUDE_CODE_GATEWAY_SECRET=... ANTHROPIC_API_KEY=... bun run gateway   # or: mise dev-claude-code-gateway
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `CLAUDE_CODE_GATEWAY_SECRET` | required | Bearer secret between ChenWeb and this gateway (both directions). Use a different value from `PI_GATEWAY_SECRET`. |
| `ANTHROPIC_API_KEY` (or `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_BASE_URL`) | one required | Model credential passed to Claude Code. |
| `CHENWEB_INTERNAL_URL` | `http://127.0.0.1:1323` | Where the knowledge tools call ChenWeb. |
| `CLAUDE_CODE_GATEWAY_HOST` / `_PORT` | `127.0.0.1` / `4318` | Listen address. Keep it on loopback. |

In ChenWeb, set `CLAUDE_CODE_GATEWAY_URL=http://127.0.0.1:4318` and the same `CLAUDE_CODE_GATEWAY_SECRET`. The adapter registers only when both are set. Then choose **Claude Code (Agent SDK)** as the agent harness for a service on the admin page. Only provider `anthropic` is accepted.

## Differences from Pi

- **History.** The Agent SDK cannot be seeded with earlier assistant turns, so earlier turns, including stored tool results, are sent as a transcript inside `<earlier_turns>` ahead of the question.
- **History budget.** The gateway does not report model context windows, so ChenWeb uses its fallback history budget.
- **Thinking.** The profile's thinking level becomes the SDK `effort`, with adaptive thinking.
- **Output limit.** Output is capped per response (`CLAUDE_CODE_MAX_OUTPUT_TOKENS`) and in total across the turn.

## Test

`bun test` and `bun run check` need no model. `bun run smoke-live.ts` makes one real model call with `ANTHROPIC_API_KEY` against a fake ChenWeb. It prints the tools the session started with, so you can confirm only `mcp__semos__*` tools are present.
