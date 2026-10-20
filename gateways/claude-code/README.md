# Claude Code gateway

A private Bun service that lets ChenWeb act as a front end to **Claude Code**, the way VS Code is. Claude Code runs as itself and keeps everything it normally owns:
- its system prompt and agent loop;
- its built-in tools and MCP servers;
- skills, `CLAUDE.md`, user/project/local settings and memory;
- compaction, model fallback and session history.

The gateway passes the user's messages in, streams Claude Code's events out, and relays permission decisions. ChenWeb (`server/api/harnessfrontend`) is its only client.

It uses the Claude Agent SDK only as the connection to Claude Code. The SDK starts the same Claude Code program, configured as in a terminal or IDE: `systemPrompt` and `tools` use the `claude_code` preset, and all setting sources are loaded.

## Login

Claude Code signs in exactly as it does in a terminal or VS Code, using this machine's Claude Code login. The gateway removes these variables before starting Claude Code:
- `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`, which would otherwise take precedence over the login. Set `CLAUDE_CODE_GATEWAY_USE_API_KEY=1` to keep them.
- variables whose names contain `SECRET`, `PASSWORD` or `API_KEY`, so ChenWeb's own secrets (present when started through mise) do not reach the agent;
- the gateway secret.

## Sessions

- Each session is one long-lived Claude Code process. Each new message is pushed into it, as if typed into a terminal.
- One turn runs at a time per session.
- After `CLAUDE_CODE_SESSION_IDLE_MINUTES` (default 30) without a turn, the process is closed. The next message resumes the session from Claude Code's own transcript (`~/.claude/projects`).
- Sessions are confined to `CLAUDE_CODE_WORKSPACE`. Only sessions Claude Code lists for that folder can be read or resumed, which includes sessions started in VS Code or a terminal in the same folder.
- If the client disconnects, the turn is interrupted.

## Permissions

Claude Code's own permission rules apply first. Calls it would prompt for in a terminal become `permission_request` events. ChenWeb answers with allow or deny, and an allow can include `updatedInput`, which is how answers to `AskUserQuestion` are returned. Modes `default`, `acceptEdits` and `plan` are accepted; `bypassPermissions` is refused.

## Protocol v1

All routes require `Authorization: Bearer $CLAUDE_CODE_GATEWAY_SECRET`.

| Route | Purpose |
| --- | --- |
| `GET /health` | Liveness and workspace folder |
| `GET /v1/sessions?limit&offset` | Claude Code's sessions in the workspace |
| `GET /v1/sessions/:id/messages?limit&offset` | A session's history from Claude Code's transcript (thinking omitted) |
| `POST /v1/turns` `{sessionId?, message, permissionMode?}` | Runs a turn; NDJSON events: `session`, `text`, `tool_use`, `tool_result`, `permission_request`, `status`, `result`, `error` |
| `POST /v1/sessions/:id/interrupt` | Interrupts the running turn |
| `POST /v1/sessions/:id/permissions/:requestId` `{behavior, updatedInput?, message?}` | Answers a permission request |

## Run

```bash
mise dev-claude-code-gateway   # from ChenWeb; or, from this folder:
CLAUDE_CODE_GATEWAY_SECRET=... CLAUDE_CODE_WORKSPACE=/Users/cding/Workspace bun run gateway
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `CLAUDE_CODE_GATEWAY_SECRET` | required | Shared with ChenWeb |
| `CLAUDE_CODE_WORKSPACE` | required | Folder Claude Code works in |
| `CLAUDE_CODE_GATEWAY_HOST` / `_PORT` | `127.0.0.1` / `4318` | Listen address; keep it on loopback |
| `CLAUDE_CODE_SESSION_IDLE_MINUTES` | `30` | Idle time before a session's process is closed |
| `CLAUDE_CODE_GATEWAY_USE_API_KEY` | unset | `1` uses `ANTHROPIC_API_KEY` instead of the Claude Code login |

In ChenWeb, set `CLAUDE_CODE_GATEWAY_URL=http://127.0.0.1:4318` and the same `CLAUDE_CODE_GATEWAY_SECRET`. The routes under `/api/v1/harness-frontend/claude-code/...` are admin-only, because Claude Code has shell and file access as this machine's user.

## Test

- `bun test` and `bun run check` use a scripted stand-in for Claude Code.
- `CLAUDE_CODE_WORKSPACE=... bun run live-check.ts` runs two real turns.
- The Go test `go test -tags live -run TestLive ./api/harnessfrontend/` checks the whole chain through ChenWeb against a running gateway, including a real permission prompt.
