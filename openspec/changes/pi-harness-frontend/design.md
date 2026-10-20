## Connection

The gateway starts `pi --mode rpc` (`PI_HARNESS_BIN`, by default `pi`) with the workspace as its working directory, one process per open session:
- a new session: `pi --mode rpc`; `get_state` gives the session id and model;
- an earlier session: `pi --mode rpc --session <file>`, with the file found in Pi's folder for the workspace;
- the model list: `pi --mode rpc --no-session`, stopped after `get_available_models` and `get_state`.

A session's process is stopped after `PI_HARNESS_SESSION_IDLE_MINUTES` without a turn. If it exits during a turn, the turn ends with an error and the next message resumes the session from its file.

Records are split on LF only, as Pi's RPC docs require (U+2028 and U+2029 are valid inside JSON strings). Commands carry the gateway's own ids (`gw-<n>`) and are matched to their responses.

Other Pi interfaces were rejected:
- the SDK (`AgentSession` in-process) would tie the gateway to the version of `@earendil-works/pi-coding-agent` it was installed with, rather than the `pi` the user runs, and would load Pi's resources through the gateway's own code paths;
- `pi -p` / `--mode json` runs one prompt per process and cannot relay extension dialogs or abort cleanly.

Verified on 2026-10-20 against pi 0.84.2: `get_available_models`, `get_state`, `prompt` (streamed `text_delta`s, `tool_execution_*`, per-message usage and cost), `--session <file>` in a new process (the model answered from its earlier context), and an extension `confirm` dialog relayed and approved.

## Mapping protocol v1 to Pi RPC

| Protocol v1 | Pi RPC |
| --- | --- |
| session | one `pi --mode rpc` process; `--session <file>` to resume |
| turn | `prompt`; ends at `agent_settled`. If Pi acknowledges the prompt without starting a run (an extension command or input handler), `get_state` right after shows `isStreaming: false` and the turn ends then |
| `text` | `message_update` with `text_delta`; a blank line between separate assistant messages |
| `tool_use` / `tool_result` | `tool_execution_start` (`toolCallId`, `toolName`, `args`) / `tool_execution_end` (`result.content` text, capped; `isError`) |
| `permission_request` | an extension's `confirm` or `select` dialog (`extension_ui_request`) |
| `status` (`retrying`) | `auto_retry_start` |
| `result` | `completed`; `interrupted` after an interrupt; `error` when the last assistant message stopped with `error` (Pi's `errorMessage`) or `length`. Usage and cost are summed over the turn's assistant messages (`message_end`); `numTurns` counts `turn_end` |
| interrupt | `abort` |
| models | `get_available_models`, as `<provider>:<id>`; Pi's current model (from `get_state`) first |

Thinking (`thinking_*` deltas and blocks) is never relayed. Compaction and queue events are not shown.

**Models.** Pi's values are `provider/id`, and `/` is outside protocol v1's model pattern, so the gateway uses `<provider>:<id>` (Pi's provider names have no colon) and splits on the first colon. Models whose id has characters outside the pattern are left out. A turn naming another model sends `set_model`, which in Pi also saves the model as Pi's default, exactly as Pi's own `/model` picker does. The gateway keeps that behavior rather than working around it.

**Dialogs.** Pi has no approval prompts of its own; extensions ask through `ctx.ui.*`, which RPC mode turns into `extension_ui_request`s. `confirm` becomes a `permission_request` (`toolName: "confirm"`, the dialog's title and message), answered `confirmed: true/false`. `select` becomes an `AskUserQuestion` request with one question, answered with the chosen option's `value` when it is one of the options, otherwise cancelled. `input` and `editor` are cancelled at once, as free text cannot be entered from ChenWeb yet. Fire-and-forget methods are ignored. Interrupting cancels open dialogs; a dialog's own timeout is left to Pi. Request ids are the gateway's own (`pi-<n>`).

**Permission modes.** Pi has none. `default` runs; `acceptEdits` and `plan` are refused with an error before Pi starts, so a `plan` turn never runs with Pi's full tools.

**History, titles and listing.** `sessionfile.ts` reads `<PI_CODING_AGENT_DIR or ~/.pi/agent>/sessions/--<cwd with / replaced by ->--/<timestamp>_<id>.jsonl` (the folder computed as Pi's `getDefaultSessionDir` does). Format version 3 only; another version answers 502 naming it.
- History: the active branch, from the last entry (Pi's leaf on load) back along `parentId`. `message` entries: user text and images, assistant text and `toolCall`s (thinking dropped), `toolResult`s, and `bashExecution` (the user's `!` commands) as a `bash` tool use with its result. `custom_message`, `compaction`, `branch_summary` and other entries are left out.
- List: open sessions first (by last turn), then files newest first; title from the last `session_info` name, otherwise the first user message; sessions with no user message are left out. An open session whose file Pi has not written yet is listed with the message ChenWeb sent.

**Workspace confinement.** A session may be read or resumed only if its file is in Pi's folder for the workspace (resolved with `realpath` at start) and its header records the workspace as `cwd` and the requested id.

**Environment.** The gateway's own variables (`PI_HARNESS_*`) and variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL`, or end in `_KEY` or `_DSN`, are removed, so Pi uses its stored login (`~/.pi/agent/auth.json`) and does not see ChenWeb's secrets. The other gateways now use the same filter; their earlier one (`SECRET|PASSWORD|API_KEY`) let `OPENMETADATA_ADMIN_TOKEN`, `PROMPT_OPTIMIZER_ENCRYPTION_KEY` and `TEST_DATABASE_URL` from ChenWeb's mise environment through. Because Pi's own configuration can name environment keys (`models.json` names `DASHSCOPE_API_KEY` for `qwen`), `PI_HARNESS_PASS_ENV` lists variables let through anyway.

## Not done here

- Free-text extension dialogs (`input`, `editor`).
- Thinking level: not selectable from ChenWeb.
- Steering and follow-up messages while a turn runs (Pi's `steer`, `follow_up`): one turn at a time, as for the other harnesses.
- Branch navigation (`/tree`, `fork`, `clone`): only the active branch is shown.
- Guarding against the same session being open in a terminal Pi and in ChenWeb at once.
