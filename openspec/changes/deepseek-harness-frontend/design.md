## Connection

The gateway starts `dsh --profile acp` (`DEEPSEEK_HARNESS_BIN`, by default `dsh`; on this machine the checkout's built `apps/cli/lib/bin.js`) with the workspace as its working directory, sends ACP `initialize` (protocol version 1), and keeps the process for all sessions: one ACP connection can run several sessions, each independent. If it exits, running turns end with an error, every session counts as closed, and the next request starts a new process.

Other dsh interfaces were rejected:
- `dsh --profile sdk` (dsh's own JSON-RPC SDK) has no permission requests, no cancel and no session list.
- `dsh headless --json` runs one task per process and cannot relay approvals.
- The Web UI's `/api` Remote methods are generated, internal and change between builds.

Verified on 2026-10-20 against dsh 0.2.1-rc.2: `session/new`, `session/list`, `session/resume` (in a new process, with the model answering from its earlier context), `session/prompt`, and `session/request_permission` (an escalated write outside the workspace; the denial reached the model).

## Mapping protocol v1 to ACP

| Protocol v1 | dsh ACP |
| --- | --- |
| session | ACP session: `session/new` `{cwd, mcpServers: []}`, `session/resume`, `session/list` `{cwd}`, `session/close` |
| turn | `session/prompt` with one text block; ends when the request answers; `session/cancel` |
| `text` | `agent_message_chunk`. dsh sends committed messages, not token deltas; messages with different `messageId`s are joined with a blank line |
| `tool_use` | `tool_call`: `toolCallId`, `title` as the tool name (dsh's own names: `read`, `write`, `edit`, `bash`, …), `rawInput` |
| `tool_result` | `tool_call_update` with status `completed` or `failed`; text content joined and capped |
| `permission_request` | `session/request_permission`. It carries only the tool-call id, so the tool name and input come from the matching `tool_call` |
| `result` | the `session/prompt` answer: `end_turn` → `completed`; `cancelled` → `interrupted`; `max_turn_requests` → `max_turns`; `max_tokens`, `refusal` → `error` with a message |
| models | the session's `model` config option |

Thoughts (`agent_thought_chunk`) are never relayed. `usage_update` (context-window use) is not relayed; usage and cost are reported as 0, since ACP carries neither. dsh keeps retries off the ACP wire, so no `status` event is emitted.

**Models.** ACP offers the model list only as a session's config options, with opaque values (a JSON pair such as `["deepseek-official","deepseek-v4-pro"]`) that do not fit protocol v1's model pattern. `GET /v1/models` creates a session, reads its `model` option, and closes it; dsh persists nothing for a session that never received a message. Each option becomes `{value: "<provider>:<model>", displayName: name}`, the current value first. A turn that names a model is matched to the session's option of the same provider and model and set with `session/set_config_option` before the prompt. dsh's reasoning-effort option is left at dsh's default.

**Approvals.** Allow selects the `allow_once` option and Deny selects `reject_once`; `allow_always` is never chosen for the user. If the matching option is missing, the gateway answers `cancelled`. Interrupting a turn cancels its open requests, as ACP requires. Permission request ids are the gateway's own (`dsh-<n>`).

**Permission modes.** dsh's ACP server has no modes. `default` leaves dsh's own permission preset in charge (`workspace-write` with approval policy `ask` in the shipped profiles). `acceptEdits` and `plan` are refused with an error rather than ignored, so a `plan` turn never runs with write access. `bypassPermissions` stays refused by protocol v1.

**History, titles and listing.** ACP's `session/list` returns only ids and folders, and dsh does not support `session/load`, so the gateway reads dsh's session log itself: `<DSH_HOME or ~/.dsh>/sessions/<project dir>/<session id>/session.v<N>.jsonl[.zstd]`, the highest version. Compressed logs are concatenated zstd frames, which `Bun.zstdDecompressSync` reads whole. The gateway reads format version 4 only (the current one); another version answers 502 with the version named, instead of guessing.

- History: `user/message` events whose `source.kind` is `user` (dsh's injected context messages have another kind); `assistant/message` text and `tool-call` blocks (reasoning dropped); `tool/result` messages as tool results.
- List: ids from `session/list` (newest first, paged by cursor); title from the last `session/title` event, first prompt from the first user message, time from the log file.

**Workspace confinement.** A session may be read or resumed only if its log's header names the workspace folder (resolved with `realpath` at start) as `cwd` and it is a root session (`delegationDepth` 0). `session/resume` checks the folder again on dsh's side.

**Environment.** As for the other gateways: the gateway's own variables and every variable whose name contains `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN` (which includes `DEEPSEEK_API_KEY`) are removed, so dsh uses its own login: `~/.dsh/.credentials.yaml`, then `~/.dsh/.env`. Variables named `DEEPSEEK_HARNESS_*` are removed too, so ChenWeb's settings do not appear among dsh's `DSH_*` environment facts.

## Not done here

- Reasoning effort: not selectable from ChenWeb.
- dsh's interactive extensions (plans, todos, elicitation, user questions) are not part of its ACP surface.
- Subagent output: only the root session's updates are shown.
- dsh's default session-log upload to DeepSeek (`session-log-deepseek`) is dsh's own behavior and stays as configured in dsh.
