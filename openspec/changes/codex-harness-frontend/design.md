## Connection

The gateway starts `codex app-server` over stdio, sends `initialize` (client `chenweb_harness_frontend`, `experimentalApi: true` so Codex's `request_user_input` questions are delivered) and `initialized`, and keeps the process for all sessions. If it exits, running turns end with an error, every thread counts as unloaded, and the next request starts a new process.

Other options were rejected:
- `codex exec --json` runs one non-interactive turn per process and cannot relay approvals.
- The Codex SDK wraps `codex exec` with the same limits.

## Mapping protocol v1 to Codex

| Protocol v1 | Codex app-server |
| --- | --- |
| session | thread (`thread/start`, `thread/resume`, `thread/read`, `thread/list`, `thread/unsubscribe`) |
| turn | `turn/start`; ends at `turn/completed`; `turn/interrupt` |
| `text` | `item/agentMessage/delta`; separate agent messages in one turn are joined with a blank line |
| `tool_use` / `tool_result` | `item/started` / `item/completed` for command, file-change, MCP, dynamic-tool, web-search, subagent and image-view items |
| `permission_request` | Codex's server requests: command, file-change and permission approvals, and `item/tool/requestUserInput` |
| `status` (retrying) | `error` notification with `willRetry` |
| `result` | `turn/completed` (`completed`, `interrupted`, `failed` → `error`), usage summed from `thread/tokenUsage/updated` |
| models | `model/list`, hidden models removed, Codex's default first |

Reasoning items are never relayed. Permission request IDs are the gateway's own (`codex-<n>`), so they fit ChenWeb's request-ID check whatever JSON-RPC ID Codex used.

**Questions.** `request_user_input` questions are emitted with `toolName: "AskUserQuestion"` and `{question, header, multiSelect: false, options}` per question. The page's allow decision carries `answers` keyed by question text; the gateway converts them to Codex's answers keyed by question ID.

**Sandbox per mode.** `default` sends no sandbox, so Codex's own configuration decides. `acceptEdits` → `workspace-write`, `plan` → `read-only`; on a live thread a mode change is sent as the turn's `sandboxPolicy`. `bypassPermissions` stays refused.

**Workspace confinement.** A thread may be read or resumed only if `thread/read` reports its `cwd` as the workspace folder (resolved with `realpath` at start). Listing filters by the same folder and by source kinds `cli`, `vscode` and `appServer`.

**Environment.** As for Claude Code: the gateway secret and every variable whose name contains `SECRET`, `PASSWORD` or `API_KEY` are removed, so Codex uses its own login.

## Not done here

- MCP elicitations: declined, since the page cannot show forms or links.
- Codex's free-text answers (`isOther`) and secret answers: the page offers only the listed options.
- Cost: Codex reports tokens only, so `costUsd` is 0.
