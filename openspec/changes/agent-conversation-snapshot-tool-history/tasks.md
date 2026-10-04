## 1. Pi gateway

- [x] 1.1 `knowledge-tools.ts`: send `x-chenweb-tool-call-id`; add `get_saved_tool_result` tool (input `tool_call_id`, shares the call counter, no sources)
- [x] 1.2 `types.ts`: tool list includes `get_saved_tool_result`; history assistant messages accept `toolCalls`; stores required only for knowledge tools
- [x] 1.3 `server.ts`: `seedHistory` replays tool calls as toolCall/toolResult messages, folding unavailable tools into text
- [x] 1.4 Tests; `bun test gateway`, `bun run check`; commit

## 2. ChenWeb storage and configuration

- [x] 2.1 Migration creating `kb.agentic_tool_results`
- [x] 2.2 `store.go`: `SaveToolResult`, `LoadToolResults` (by attempt, call order), `LoadSavedToolResult` (conversation-scoped); remove `ClearHistorySummary`
- [x] 2.3 `profiles.go`: `PI_HISTORY_SNAPSHOT_DUR` on the registry; `MaxHistoryToolResultBytes` per guide

## 3. ChenWeb behavior

- [x] 3.1 `types.go`: `CheckDocumentAccess`; snapshot `FilterResumeState` (removes old inaccessible sources/tool results, never hides messages, reports counts)
- [x] 3.2 `tools.go`: record results at the internal route; `get_saved_tool_result` route with snapshot check
- [x] 3.3 `history.go`: turns carry tool calls; shortened history text; remove `summaryCoversHidden`
- [x] 3.4 `run_handler.go` / `handler.go` / `gateway.go` / `routes.go`: use the new filter, send tool calls, offer the new tool
- [x] 3.5 Tests for filter, recording, retrieval, history tool calls; `go test`, `go vet`, `mise build-server`

## 4. Web

- [x] 4.1 Client type and page notices (snapshot, removal) in English and Chinese; `bun run check`

## 5. Docs and commit

- [x] 5.1 Devdoc 2026093001 and operations guide; commit each repo
