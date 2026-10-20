## Deployment model

The gateway is a separate loopback process, like Pi's. It is not embedded in the Go server, because the Agent SDK is a TypeScript/Python library that spawns the Claude Code binary. Each run gets an empty temporary directory as its working directory and `HOME`, and only model-credential environment variables. The directory is removed when the run ends.

## Tool constraint

| Concern | Mechanism |
| --- | --- |
| Built-in tools (Bash, Read/Write/Edit, WebFetch/WebSearch, Task, ...) | `tools: []`. Verified live: the session's init message lists only `mcp__semos__*`. |
| Other MCP servers, user/project settings, CLAUDE.md, skills, plugins, subagents | `strictMcpConfig`, `settingSources: []`, `skills: []`, `plugins: []`, `agents: {}` |
| Calls to anything else | `canUseTool` denies any tool that is not this run's tool on the `semos` server with `source: "sdk"` |
| Knowledge scope, user access | Unchanged: ChenWeb verifies the run capability on every internal tool call |
| Approval | `canUseTool` applies the ask/auto gate; `permission_request` events carry the SDK `toolUseID` |

The MCP tool handler does not receive the SDK's tool-use ID. `canUseTool` records it against the tool name and input, and the handler takes the matching ID. A handler call with no recorded ID fails. This ties every executed call to a gated decision and to the ID ChenWeb stores sources and results under.

## History

The SDK accepts only user messages as input, so earlier turns are rendered as a transcript in `<earlier_turns>`, followed by `<current_question>`. A system-prompt note tells the model to treat the transcript as context. History is put in the user message, not the system prompt, so user-supplied text never gains system-prompt authority. This counts as `history_replay`: the model receives the bounded history, including stored tool results.

## Limits

- `maxTurns` is the profile's tool-call limit plus one, and `canUseTool` also counts calls.
- Output is capped by `CLAUDE_CODE_MAX_OUTPUT_TOKENS` per response and by a running total from streamed `message_delta` usage.
- Elapsed time is enforced by aborting the query.

## Not done

- No live model round-trip: the available API key had no credit. Only tool loading and error mapping were verified live.
- Step 5 (the same evaluation cases against Pi and Claude Code) remains.
