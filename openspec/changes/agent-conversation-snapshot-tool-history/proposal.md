## Why

The requirements (doc-2026091401, revised 2026-10-04, §9.1.6, §11, §12) now treat conversation history as a time-limited snapshot of the knowledge base, and require tool calls and their full results to be stored and carried in the history. The current implementation does the opposite:

- It hides a saved answer as soon as any cited source changes or becomes inaccessible.
- It does not store tool arguments or results.
- It replays only the question and answer text, so Pi repeats searches and forgets which searches came up empty.

## What Changes

- **Store tool calls in full.** The gateway passes each tool call's ID to ChenWeb's internal tool route. ChenWeb stores the arguments, the exact result it returned (or the error), and the documents the result references, in a new `kb.agentic_tool_results` table.
- **Replay tool calls in the history.** Each earlier turn is sent as the question, then the assistant's tool calls with their results, then the final answer. The gateway's `seedHistory` turns these into real tool-call and tool-result messages. A tool call whose tool is not available in the current run is rendered as text instead.
- **Shorten long results in the history.** A result over `PI_<GUIDE>_MAX_HISTORY_TOOL_RESULT_BYTES` (default 4,096) is shortened in the history and marked with its full size and tool-call ID. The stored copy stays complete.
- **New tool `get_saved_tool_result`.** It returns the full stored result of an earlier tool call in the same conversation. It is offered whenever the run has knowledge tools.
- **Snapshot rule replaces hiding.** `PI_HISTORY_SNAPSHOT_DUR` (hours, default 48) sets the snapshot period.
  - Records saved within it are shown and resent exactly as saved.
  - For older records, only sources and tool results that reference a document the user can no longer access are removed. There is an access check but no content check.
  - Answers and summaries are never hidden.
  - **BREAKING** (API): `hidden_message_ids` and `omission_notice` are replaced by `removed_sources`, `removed_tool_results` and `snapshot_hours` in the conversation response.
- **Page notices.** The Knowledge Desk always shows a localized snapshot notice on a saved conversation, plus a localized removal notice when anything was removed.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `agent-conversation-history`:
  - whole-turn pairing no longer depends on hidden answers;
  - turns now carry tool calls;
  - the summary-invalidation requirement is removed;
  - new requirements for snapshot filtering, tool-result storage and shortening, and the saved-result tool.

## Impact

- **ChenWeb** `server/api/agentservicehandler/`: `tools.go` (records results, new route), `types.go` (snapshot filter, access-only check), `history.go`, `run_handler.go`, `handler.go`, `store.go`, `profiles.go`, `gateway.go`. Also `server/api/routes.go` wiring and a new migration.
- **ChenWeb web:** `agentServiceClient.ts`, `routes/home3/agent-services/+page.svelte`, and the message files.
- **Pi gateway** `ThirdParty/pi/gateway/`:
  - `knowledge-tools.ts`: sends the tool-call ID header; adds the saved-result tool.
  - `types.ts`: history tool calls.
  - `server.ts`: `seedHistory` replays tool calls.
- **Shared library:** no change.
- **Docs:** devdoc 2026093001 and the operations guide.
