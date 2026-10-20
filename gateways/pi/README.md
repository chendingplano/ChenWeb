# Pi gateway

A private Bun service that lets ChenWeb act as a front end to **Pi** (`pi`, the coding agent from pi.dev), the way an IDE is a front end to a terminal agent. Pi runs as itself and keeps everything it normally owns:
- its instructions (`AGENTS.md` / `CLAUDE.md`, `SYSTEM.md`), agent loop, tools, extensions, skills and prompt templates;
- its settings, login and project trust under `~/.pi/agent`, including the packages listed there (on this machine, `ThirdParty/pi`);
- its session files.

The gateway passes the user's messages in, streams Pi's events out, and relays the user's answers to Pi's extension dialogs. ChenWeb (`server/api/harnessfrontend`) is its only client. It speaks the same harness front-end protocol v1 as `gateways/claude-code`, `gateways/codex` and `gateways/deepseek-harness`.

It connects to Pi through `pi --mode rpc`, Pi's headless JSON protocol over stdio (Pi's `docs/rpc.md`). Pi's RPC mode serves one session per process, so the gateway runs one Pi process per open session.

This is not the Knowledge Desk's Pi gateway (`ThirdParty/pi/gateway`, port 4317), where ChenWeb owns the conversation and Pi gets only ChenWeb's knowledge tools.

## Login

Pi signs in with its own stored login, `~/.pi/agent/auth.json` (on this machine: DeepSeek by API key, OpenAI Codex by OAuth). The gateway removes these variables before starting Pi:
- its own settings (`PI_HARNESS_*`);
- variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN`, so ChenWeb's own secrets (present when started through mise) do not reach the agent.

Pi also reads provider keys from the environment (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …, and keys that `~/.pi/agent/models.json` names). Those providers drop out of the model list unless their variable is named in `PI_HARNESS_PASS_ENV`. On this machine it names `DASHSCOPE_API_KEY`, which `models.json` uses for the `qwen` provider. Pi lists models whose key is only named, not present, so a `qwen` model is listed either way and fails when used if the key is missing or wrong.

## Sessions

- A session is a Pi session. Each new message is one RPC `prompt`, as if typed into Pi.
- One turn runs at a time per session. A turn ends when Pi reports `agent_settled` (after retries, compaction and queued follow-ups). A message that an extension command answers (such as `/name`) ends without an agent run.
- A new session starts `pi --mode rpc`; Pi writes its file, `~/.pi/agent/sessions/--<workspace>--/<timestamp>_<id>.jsonl`, once the first reply arrives. An earlier session starts `pi --mode rpc --session <file>`.
- After `PI_HARNESS_SESSION_IDLE_MINUTES` (default 30) without a turn, the session's Pi process is stopped. The next message resumes it from its file.
- Sessions are confined to `PI_HARNESS_WORKSPACE`. Only sessions in Pi's folder for the workspace whose header records that folder can be read or resumed, which includes sessions started in a terminal in the same folder.
- If the client disconnects, the turn is aborted (`abort`).
- A session open in a terminal Pi and in ChenWeb at the same time has two Pi processes writing one file. Pi does not guard against this; avoid it.

**History and the session list come from Pi's session files.** RPC mode reports only the session it runs, so the gateway reads the files itself (`sessionfile.ts`):
- history: the active branch (from the last entry back to the root, as Pi loads it), with what the user typed, the assistant's text and tool calls, tool results, and commands the user ran with `!`; thinking, extension-injected messages, compaction and branch summaries are left out;
- list: open sessions first, then files by modification time; the title is the session's name (`/name`) or its first message; sessions with no message are left out.

The gateway reads session format version 3 (the current one). A session in another version answers 502 naming the version, and is listed without a title; Pi itself migrates old files when it opens them.

## Models

`GET /v1/models` starts `pi --mode rpc --no-session`, asks for `get_available_models` and `get_state`, and stops it. Each model becomes `<provider>:<id>` (for example `deepseek:deepseek-v4-flash`), Pi's default first. Model ids containing characters outside protocol v1's model pattern (such as `/` in some routed providers) are left out. A turn that names another model sends `set_model`. **As with Pi's own `/model` picker, this also makes it Pi's default** in `~/.pi/agent/settings.json`, for new terminal sessions too. Pi's thinking level stays at Pi's setting (`defaultThinkingLevel`).

## Permissions

Pi has no sandbox and asks before nothing by itself: its tools read, write and run commands as this machine's user (Pi's `docs/security.md`). That is why the ChenWeb routes are admin-only. Approvals exist only where a Pi extension asks the user, for example Pi's `permission-gate.ts` example with `ctx.ui.confirm`. The gateway relays those dialogs:

| Pi extension dialog | Shown in ChenWeb | Answer sent to Pi |
| --- | --- | --- |
| `confirm` | approval card (`toolName: "confirm"`, the dialog's title and message) | Allow → `confirmed: true`; Deny → `confirmed: false` |
| `select` | question card (`AskUserQuestion`, one question with the options) | the chosen option; Deny → cancelled |
| `input`, `editor` | not shown | cancelled (free text cannot be entered from ChenWeb yet) |
| `notify`, `setStatus`, `setWidget`, `setTitle`, `set_editor_text` | not shown | none expected |

A dialog with a timeout is answered by Pi itself when it expires. Interrupting a turn cancels its open dialogs.

Pi has no permission modes. `default` is the only one accepted; `acceptEdits` and `plan` end the turn with an error before Pi is started, and `bypassPermissions` is refused by the protocol.

## Protocol v1

All routes require `Authorization: Bearer $PI_HARNESS_GATEWAY_SECRET`.

| Route | Purpose |
| --- | --- |
| `GET /health` | Liveness and workspace folder |
| `GET /v1/models` | Models Pi offers (cached for 10 minutes; no message is sent) |
| `GET /v1/sessions?limit&offset` | Pi's sessions in the workspace, most recent first |
| `GET /v1/sessions/:id/messages?limit&offset` | A session's history from its file (thinking omitted) |
| `POST /v1/turns` `{sessionId?, message, permissionMode?, model?}` | Runs a turn; NDJSON events: `session`, `text`, `tool_use`, `tool_result`, `permission_request`, `status`, `result`, `error` |
| `POST /v1/sessions/:id/interrupt` | Aborts the running turn |
| `POST /v1/sessions/:id/permissions/:requestId` `{behavior, updatedInput?, message?}` | Answers an extension dialog |

Text streams word by word (`text_delta`). Tools appear under Pi's own names (`read`, `write`, `edit`, `bash`, …), shown when they run. A `result` event's usage and `costUsd` are the sum over the turn's assistant messages, as Pi reports them; `numTurns` counts Pi's model turns. Automatic retries appear as `status` (`retrying`) events.

## Run

```bash
mise dev-pi-harness-gateway   # from ChenWeb; or, from this folder:
PI_HARNESS_GATEWAY_SECRET=... PI_HARNESS_WORKSPACE=/Users/cding/Workspace bun run gateway
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `PI_HARNESS_GATEWAY_SECRET` | required | Shared with ChenWeb |
| `PI_HARNESS_WORKSPACE` | required | Folder Pi works in |
| `PI_HARNESS_BIN` | `pi` | The Pi executable (here installed with `bun install -g @earendil-works/pi-coding-agent`) |
| `PI_HARNESS_GATEWAY_HOST` / `_PORT` | `127.0.0.1` / `4321` | Listen address; keep it on loopback |
| `PI_HARNESS_SESSION_IDLE_MINUTES` | `30` | Idle time before a session's Pi process is stopped |
| `PI_HARNESS_PASS_ENV` | empty | Comma-separated variable names passed to Pi despite the filter above |
| `PI_CODING_AGENT_DIR` | `~/.pi/agent` | Pi's configuration folder, where the gateway finds session files (passed on to Pi) |

In ChenWeb, set `PI_HARNESS_GATEWAY_URL=http://127.0.0.1:4321` and the same `PI_HARNESS_GATEWAY_SECRET`. The routes under `/api/v1/harness-frontend/pi/...` are admin-only, because Pi has shell and file access as this machine's user.

## Test

- `bun test` and `bun run check` use a scripted stand-in for `pi --mode rpc` and fixture session files.
- `PI_HARNESS_WORKSPACE=... bun run live-check.ts` runs four real turns: one reads a file, one recalls the first, one writes a file in the home folder, and one recalls again after the session is resumed in a new Pi process. Since Pi asks before nothing, the check loads a probe extension (`-e`, for the check only) that asks before that write; the check approves only it.
