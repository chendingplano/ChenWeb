# DeepSeek Harness gateway

A private Bun service that lets ChenWeb act as a front end to **DeepSeek Harness** (`dsh`), the way its own Web UI is. dsh runs as itself and keeps everything it normally owns:
- its instructions (it reads `AGENTS.md` / `CLAUDE.md` in the workspace), agent loop, tools, skills and plugins;
- its profiles and settings under `~/.dsh`, its sandbox and approval policy;
- its session history.

The gateway passes the user's messages in, streams dsh's updates out, and relays approvals. ChenWeb (`server/api/harnessfrontend`) is its only client. It speaks the same harness front-end protocol v1 as `gateways/claude-code` and `gateways/codex`.

It connects to dsh through `dsh --profile acp`, dsh's [Agent Client Protocol](https://agentclientprotocol.com) server, over stdio. One dsh process serves every session; it is started on first use and again if it exits. dsh's `acp` profile shares `~/.dsh` (login, settings, sessions) with its other profiles, including the Web UI.

## Login

dsh signs in with its own stored login: `~/.dsh/.credentials.yaml` (keys saved in dsh's settings), then `~/.dsh/.env`. On this machine that is `~/.dsh/.env` with `DEEPSEEK_API_KEY` (mode 600). The gateway removes these variables before starting dsh:
- its own settings (`DEEPSEEK_HARNESS_*`);
- variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN` (which includes `DEEPSEEK_API_KEY`), so ChenWeb's own secrets (present when started through mise) do not reach the agent.

dsh also reads `<workspace>/.env` if one exists; keep the workspace free of one.

## Sessions

- A session is a dsh session. Each new message is one ACP prompt, as if typed into dsh.
- One turn runs at a time per session.
- After `DEEPSEEK_HARNESS_SESSION_IDLE_MINUTES` (default 30) without a turn, the session is closed in dsh (`session/close`). The next message resumes it (`session/resume`) from dsh's own log.
- Sessions are confined to `DEEPSEEK_HARNESS_WORKSPACE`. Only root sessions whose log records that folder can be read or resumed, which includes sessions started in dsh's Web UI in the same folder. Subagent sessions are left out.
- If the client disconnects, the turn is cancelled (`session/cancel`).

**History and the session list come from dsh's log.** dsh's ACP server does not replay history and lists only ids, so the gateway reads `~/.dsh/sessions/<project>/<id>/session.v4.jsonl.zstd` (or `$DSH_HOME/sessions`) itself:
- history: what the user typed (not dsh's injected instructions, runtime context or skill lists), the assistant's text and tool calls, and tool results; reasoning is never shown;
- list: ids from `session/list`, title, first prompt and time from the log; sessions with no message are left out. dsh does not list sessions open on its own connection, so the gateway adds those first.

The gateway reads log format version 4 only. A session in another version answers 502 naming the version; after a dsh upgrade that changes the format, `sessionlog.ts` needs updating.

## Models

`GET /v1/models` returns the session `model` option dsh offers (`<provider>:<model>`, for example `deepseek-official:deepseek-v4-pro`), dsh's default first. ACP offers the list only through a session, so the gateway opens one and closes it, once per dsh process; dsh keeps that session as an empty log, which the list hides. A turn that names a model sets it on the session (`session/set_config_option`) for this and later turns, as dsh's model picker does. dsh's reasoning effort stays at dsh's default (`high`).

## Permissions

dsh's own permission preset applies (`workspace-write` with approval policy `ask` in the shipped profiles): writes inside the workspace go ahead; anything else is refused by dsh's sandbox, and the model may ask to escalate. What dsh asks about becomes a `permission_request` event with the tool name and input of the matching tool call (and the model's justification as `reason`). Allow selects dsh's `allow_once` option and Deny its `reject_once`; the gateway never answers "always".

dsh's ACP server has no permission modes. `default` is the only one accepted; `acceptEdits` and `plan` end the turn with an error before anything reaches dsh, and `bypassPermissions` is refused by the protocol.

## Protocol v1

All routes require `Authorization: Bearer $DEEPSEEK_HARNESS_GATEWAY_SECRET`.

| Route | Purpose |
| --- | --- |
| `GET /health` | Liveness and workspace folder |
| `GET /v1/models` | Models dsh offers (read once per dsh process; no message is sent) |
| `GET /v1/sessions?limit&offset` | dsh's sessions in the workspace, most recent first |
| `GET /v1/sessions/:id/messages?limit&offset` | A session's history from dsh's log (reasoning omitted) |
| `POST /v1/turns` `{sessionId?, message, permissionMode?, model?}` | Runs a turn; NDJSON events: `session`, `text`, `tool_use`, `tool_result`, `permission_request`, `result`, `error` |
| `POST /v1/sessions/:id/interrupt` | Cancels the running turn |
| `POST /v1/sessions/:id/permissions/:requestId` `{behavior, updatedInput?, message?}` | Answers a permission request |

Tools appear under dsh's own names (`read`, `write`, `edit`, `bash`, …). dsh sends each assistant message whole once it is committed, so text arrives a message at a time rather than word by word. A `result` event's usage and `costUsd` are always 0, since ACP carries neither, and there are no `status` (retrying) events, since dsh keeps retries off ACP.

## Run

```bash
mise dev-deepseek-harness-gateway   # from ChenWeb; or, from this folder:
DEEPSEEK_HARNESS_GATEWAY_SECRET=... DEEPSEEK_HARNESS_WORKSPACE=/Users/cding/Workspace \
  DEEPSEEK_HARNESS_BIN=/Users/cding/Workspace/ThirdParty-2/deepseek-harness/apps/cli/lib/bin.js bun run gateway
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `DEEPSEEK_HARNESS_GATEWAY_SECRET` | required | Shared with ChenWeb |
| `DEEPSEEK_HARNESS_WORKSPACE` | required | Folder dsh works in |
| `DEEPSEEK_HARNESS_BIN` | `dsh` | The dsh executable. On this machine, the checkout's built CLI (`ThirdParty-2/deepseek-harness/apps/cli/lib/bin.js`; rebuild with `pnpm run build` there after updating it) |
| `DEEPSEEK_HARNESS_GATEWAY_HOST` / `_PORT` | `127.0.0.1` / `4320` | Listen address; keep it on loopback |
| `DEEPSEEK_HARNESS_SESSION_IDLE_MINUTES` | `30` | Idle time before a session is closed |
| `DSH_HOME` | `~/.dsh` | dsh's home, where the gateway finds session logs (passed on to dsh) |

In ChenWeb, set `DEEPSEEK_HARNESS_GATEWAY_URL=http://127.0.0.1:4320` and the same `DEEPSEEK_HARNESS_GATEWAY_SECRET`. The routes under `/api/v1/harness-frontend/deepseek-harness/...` are admin-only, because dsh has shell and file access as this machine's user.

## Good to know

- dsh's default composition uploads its session log to DeepSeek's official API with each request (`session-log-deepseek`), and its OTel feedback upload defaults to feedback only. These are dsh's own settings; change them in dsh's profile if needed.
- dsh is a developer preview (0.2.1-rc.2 when this gateway was written). Its ACP surface, profile layout or session format may change between versions; rerun the live check after upgrading.

## Test

- `bun test` and `bun run check` use a scripted stand-in for dsh's ACP server and fixture session logs.
- `DEEPSEEK_HARNESS_WORKSPACE=... DEEPSEEK_HARNESS_BIN=... bun run live-check.ts` runs three real turns: one reads a file, one recalls the first, and one writes a file in the home folder, which dsh asks about; the check approves only that write.
