## Roles

| Owned by the harness (Claude Code) | Owned by ChenWeb |
| --- | --- |
| System prompt, agent loop, built-in tools, MCP servers, skills, memory, CLAUDE.md, compaction, model fallback | Sign-in and admin authorization |
| Session transcript and resume | Which harness and session a request goes to |
| Permission rules from the user's settings | Showing permission prompts and relaying the decision |
| Model login (the machine's Claude Code login) | Relaying the event stream to the browser |

## Gateway

The gateway runs on loopback with a bearer secret. It is the only harness-specific part. A later harness (Codex, Pi as a full harness, ...) gets its own gateway speaking the same protocol, and ChenWeb needs only its URL and secret.

**Process environment.** Claude Code inherits the gateway's environment, minus:
- the gateway secret;
- `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`, so the Claude Code login is used, as in VS Code (`CLAUDE_CODE_GATEWAY_USE_API_KEY=1` keeps them);
- variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN`, so ChenWeb's own secrets (when started through mise) do not reach the agent.

**Sessions.**
- A session is one long-lived `query()` with streaming input: the gateway pushes each user message into it, as typing into a terminal would.
- One turn runs at a time per session.
- A session idle for `CLAUDE_CODE_SESSION_IDLE_MINUTES` (default 30) is closed. The next turn resumes it by ID, from Claude Code's own transcript.
- Resuming is allowed only for a session that Claude Code lists for the workspace folder.

**Events.** A small neutral vocabulary:
- `session`: id, model, cwd, permission mode
- `text`: the main agent's streamed text
- `tool_use`: id, name, input, parent tool-use id
- `tool_result`: tool-use id, error flag, text capped at 8 KiB
- `permission_request`: request id, tool name, input, title, description, reason
- `status`: retry attempt
- `result`: status, session id, duration, turns, cost, usage
- `error`

Hidden reasoning (thinking blocks) is not relayed. This first version also leaves out subagent text deltas; subagents' tool calls are relayed with their parent ID.

**Permissions.** Claude Code's own rules decide first; only the calls it would prompt for reach `canUseTool`. Allowed modes are `default`, `acceptEdits` and `plan`; `bypassPermissions` is refused. The decision may carry `updatedInput`, which is how answers to `AskUserQuestion` are returned. A turn whose client disconnects is interrupted, so no run continues unattended.

## ChenWeb

- `harnessfrontend.Host` is the protocol-v1 client. `Registry` maps harness names to hosts.
- Routes are admin/root only, because a full harness has shell and file access as the server's operating-system user.
- Each turn's NDJSON lines are checked to be JSON objects with a known `type`, then relayed as SSE events of that type. Session lists and messages are passed through.

## Not done here

- The browser page.
- Moving the Knowledge Desk services to this model.
- Multiple workspace folders.
- Running turns unattended after the browser disconnects.
