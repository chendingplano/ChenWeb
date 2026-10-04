## Context

Today the history replays only text. `FilterResumeState` hides an answer when any of its sources fails `CheckSourceWithGroups`, which checks both access and content (fingerprint and version). The rolling summary is cleared when it covers a hidden answer. Tool arguments and results are not stored. `kb.agentic_tool_calls` keeps only identifiers, and is written at settlement for completed turns only.

The revised requirements make history a snapshot:
- within `PI_HISTORY_SNAPSHOT_DUR` hours, everything is kept as saved;
- after that, only access is rechecked, and only the inaccessible resources themselves are removed;
- tool calls and full results are stored and replayed, shortened when long, with a tool to fetch the full result.

## Goals / Non-Goals

**Goals:**
- Tool calls and results reach later turns as real tool messages.
- Full results are stored and retrievable.
- Snapshot filtering follows §11 exactly.

**Non-Goals:**
- Removing answer or summary text derived from a resource whose access was lost. This is an open question (§18).
- Showing tool results on the page.
- Emitting citations from `get_saved_tool_result`. Citations come only from live tool calls, which are verified at settlement.

## Decisions

### D1. ChenWeb records results at its internal tool route

ChenWeb already produces every tool result. The gateway adds an `X-ChenWeb-Tool-Call-ID` header to each internal tool request. After authentication, `InternalToolHandler.execute` marshals the response once, stores it, and returns the same bytes.

- **What is stored:** `{attempt_id, gateway_tool_call_id, tool_name, arguments, result, is_error, document_ids}`. `document_ids` is the input's document plus every result item's document.
- **What is recorded:** tool and validation errors from ChenWeb (400, 403, 413) are recorded as error results. Calls the gateway rejects before reaching ChenWeb (bad input, permission denied, call limit) are not recorded. They are therefore not replayed, and the history never holds a tool call without its result.
- **Failures:** a storage failure is logged. The model still gets its result; the call just won't appear in later history.

*Alternative:* have the gateway stream results back to ChenWeb as events. Rejected: the same large payloads would travel twice, and ChenWeb would have to trust the gateway's copy of data ChenWeb itself produced.

New table `kb.agentic_tool_results`, separate from `kb.agentic_tool_calls`. Results arrive during the run, before settlement creates the audit rows, and storing them separately leaves settlement unchanged. Key: `UNIQUE (attempt_id, gateway_tool_call_id)`. It cascades on attempt delete, so deleting a conversation removes its results.

### D2. History turns carry tool calls; the gateway replays them as messages

`GatewayHistoryMessage` gains an optional `toolCalls: [{id, name, arguments, result, isError}]` on assistant messages. `completeTurns` attaches the attempt's stored results, in call order.

The gateway's `seedHistory` replays such a turn as:
1. an assistant message containing the `toolCall` blocks;
2. one `toolResult` message per call;
3. the final assistant text.

A call whose tool is not in the current run's tool list is folded into the final assistant text as a short `[Earlier tool call …]` line. Providers reject a tool call for a tool that isn't defined in the request, which happens when the user has lost every grant.

### D3. Shortening and the saved-result tool

ChenWeb builds each result's history text:
- Not shortened: `{"untrusted_evidence":true,"evidence":<stored JSON>}`.
- Over the guide's `MaxHistoryToolResultBytes` (default 4,096; at most 32,768): `{"untrusted_evidence":true,"truncated":true,"full_bytes":N,"retrieve_with":"get_saved_tool_result","tool_call_id":"…","evidence_prefix":"<first bytes>"}`. This stays valid JSON.

`get_saved_tool_result` takes `{tool_call_id}`. ChenWeb resolves the run's attempt to its conversation and owner, finds the stored result for that tool-call ID in the same conversation, applies the snapshot rule (D4), and returns the stored bytes verbatim. The original result already fit the run's evidence limit, and a conversation is pinned to its profile version. The tool is added to `allowedTools` whenever the run has knowledge tools, so the existing rule "tools need a store" still holds.

### D4. Snapshot filtering

`FilterResumeState(ctx, state, sources, toolResults, now, window, canAccess)`:
- Age is measured per record: a source's age is its message's `created_at`, and a tool result has its own `created_at`.
- Records younger than `window` are kept unchanged.
- An older source is removed if `canAccess(documentID)` fails.
- An older tool result is removed if any of its `document_ids` fails.
- `canAccess` is a new access-only check, `CurrentSourceAccessChecker.CheckDocumentAccess`. It runs the grant query without the fingerprint and version conditions. Results are cached per call.
- Messages are never hidden.
- The response carries `removed_sources`, `removed_tool_results` and `snapshot_hours`. The page shows localized notices.

`summaryCoversHidden`, `ClearHistorySummary` and the `HiddenMessageIDs` and `OmissionNotice` fields go away, because nothing is hidden any more.

Settlement still checks the **current** turn's sources against live content. That is a live citation check, not history.

### D5. Configuration

- `PI_HISTORY_SNAPSHOT_DUR`: hours, positive integer, default 48. It is read once in `LoadProfileRegistry` and stored on the registry, because both handlers already hold the registry. An invalid value stops startup, matching the other agent settings.
- `PI_<GUIDE>_MAX_HISTORY_TOOL_RESULT_BYTES`: per guide, at most 32,768.

## Risks / Trade-offs

- [History grows faster with tool results] → Results are shortened to 4 KB each, and turns still fit the token budget. The byte guard drops the oldest turns.
- [A user sees content after losing access, inside the window] → This is the intended §11 behavior. The page always says the history is a snapshot.
- [Stored results add sensitive data] → They follow the conversation's ownership and deletion rules (cascade). §12 was revised to allow this.
- [Prompt caching and stale evidence] → Old evidence is replayed as saved. The snapshot notice tells the user, and the model can call live tools to refresh it.

## Migration Plan

1. Pi gateway first. The header and the new history fields are additive, so an older ChenWeb still works.
2. ChenWeb with a goose migration that creates `kb.agentic_tool_results`. No backfill is needed, because no tool results were stored before.
3. Rollback: revert ChenWeb. The extra table is harmless.

## Open Questions

None.
