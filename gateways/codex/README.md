# Codex gateway

A private Bun service that lets ChenWeb act as a front end to **Codex**, the way the Codex VS Code extension is. Codex runs as itself and keeps everything it normally owns:
- its instructions (`AGENTS.md`), agent loop, tools and MCP servers;
- `~/.codex/config.toml`, project trust, sandbox and approval rules;
- skills, memories and thread history.

The gateway passes the user's messages in, streams Codex's events out, and relays approvals. ChenWeb (`server/api/harnessfrontend`) is its only client. It speaks the same harness front-end protocol v1 as `gateways/claude-code`.

It connects to Codex through `codex app-server`, the JSON-RPC interface the Codex IDE extension uses, over stdio. One app-server process serves every session; it is started on first use and again if it exits.

## Login

Codex signs in exactly as it does in a terminal or VS Code, using this machine's Codex login (`~/.codex/auth.json`). The gateway removes these variables before starting Codex:
- the gateway secret;
- variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN` (which includes `OPENAI_API_KEY` and `CODEX_API_KEY`), so ChenWeb's own secrets (present when started through mise) do not reach the agent.

## Sessions

- A session is a Codex thread. Each new message starts a turn on it, as if typed into Codex.
- One turn runs at a time per session.
- After `CODEX_SESSION_IDLE_MINUTES` (default 30) without a turn, the thread is unloaded (`thread/unsubscribe`). The next message resumes it from Codex's own history (`~/.codex/sessions`).
- Sessions are confined to `CODEX_WORKSPACE`. Only threads Codex recorded for that folder can be read or resumed, which includes threads started in VS Code or a terminal in the same folder. The list shows threads from the CLI, VS Code and app-server clients; `codex exec` runs and subagent threads are left out.
- If the client disconnects, the turn is interrupted.

## Models

`GET /v1/models` returns Codex's own model list (`model/list`, hidden models left out), with Codex's default first. A turn that names a model starts the thread on it, or switches a running thread to it for this and later turns, as Codex's model picker does. A turn without a model uses `model` from `~/.codex/config.toml`; if that model is not available to the account, Codex fails the turn and the gateway relays its error.

## Permissions

Codex's own sandbox and approval rules apply first. What Codex would ask about in its own UI becomes a `permission_request` event:

| Codex request | `toolName` | Allow / Deny sends Codex |
| --- | --- | --- |
| `item/commandExecution/requestApproval` | `Shell` | `accept` / `decline` |
| `item/fileChange/requestApproval` | `Edit` | `accept` / `decline` |
| `item/permissions/requestApproval` | `Permissions` | the requested permissions for this turn / none |
| `item/tool/requestUserInput` | `AskUserQuestion` | the chosen option per question / no answers |

Codex's questions are shown in the same shape as Claude Code's `AskUserQuestion`, so ChenWeb's option buttons work for both; the allow decision's `updatedInput.answers` (keyed by question text) is turned into Codex's answers (keyed by question ID). MCP elicitations (forms and links an MCP server asks for) are declined, because ChenWeb cannot show them yet.

Permission modes map to Codex's sandbox:
- `default` leaves Codex's configuration in charge;
- `acceptEdits` uses `workspace-write`;
- `plan` uses `read-only`.

`bypassPermissions` is refused. On macOS, Codex's `workspace-write` sandbox can also write to temporary folders such as `/tmp` without asking.

## Protocol v1

All routes require `Authorization: Bearer $CODEX_GATEWAY_SECRET`.

| Route | Purpose |
| --- | --- |
| `GET /health` | Liveness and workspace folder |
| `GET /v1/models` | Models Codex offers this account (cached 10 minutes; no message is sent) |
| `GET /v1/sessions?limit&offset` | Codex's threads in the workspace, most recently updated first |
| `GET /v1/sessions/:id/messages?limit&offset` | A thread's history from Codex (reasoning omitted) |
| `POST /v1/turns` `{sessionId?, message, permissionMode?, model?}` | Runs a turn; NDJSON events: `session`, `text`, `tool_use`, `tool_result`, `permission_request`, `status`, `result`, `error` |
| `POST /v1/sessions/:id/interrupt` | Interrupts the running turn |
| `POST /v1/sessions/:id/permissions/:requestId` `{behavior, updatedInput?, message?}` | Answers a permission request |

Codex items appear as these tools: commands as `Shell`, file changes as `Edit`, MCP calls as `mcp__<server>__<tool>`, web searches as `WebSearch`, subagent calls as `Agent`, image views as `ViewImage`. A `result` event's `costUsd` is always 0, since Codex reports tokens, not cost.

## Run

```bash
mise dev-codex-gateway   # from ChenWeb; or, from this folder:
CODEX_GATEWAY_SECRET=... CODEX_WORKSPACE=/Users/cding/Workspace bun run gateway
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `CODEX_GATEWAY_SECRET` | required | Shared with ChenWeb |
| `CODEX_WORKSPACE` | required | Folder Codex works in |
| `CODEX_GATEWAY_HOST` / `_PORT` | `127.0.0.1` / `4319` | Listen address; keep it on loopback |
| `CODEX_SESSION_IDLE_MINUTES` | `30` | Idle time before a thread is unloaded |
| `CODEX_BIN` | `codex` | The Codex executable |

In ChenWeb, set `CODEX_GATEWAY_URL=http://127.0.0.1:4319` and the same `CODEX_GATEWAY_SECRET`. The routes under `/api/v1/harness-frontend/codex/...` are admin-only, because Codex has shell and file access as this machine's user.

## Test

- `bun test` and `bun run check` use a scripted stand-in for the app-server.
- `CODEX_WORKSPACE=... bun run live-check.ts` runs three real turns: one reads a file, one recalls the first, and one writes a file in the home folder, which Codex asks about and the check approves.
