## 1. Shared library (`shared/go/api/llm/gateway.go`)

- [x] 1.1 Add `HistorySummary string` to `PiGatewayRun` and send it as `historySummary` with `omitempty` (wire and capture structs)
- [x] 1.2 Raise the run request limit from 128 KiB to 512 KiB
- [x] 1.3 Add `ModelInfo(ctx, provider, model) (PiGatewayModelInfo, error)` calling `GET /v1/models/:provider/:model`, with `ErrPiGatewayModelUnknown` returned on 404
- [x] 1.4 Add `Summarize(ctx, PiGatewaySummaryRequest) (string, error)` calling `POST /v1/summaries`, with usage capture (`PromptName: "pi_gateway_summary"`)
- [x] 1.5 Unit tests with `httptest` for the three additions; run `go test ./...` in `shared/go`, `go work sync`, commit separately

## 2. Pi gateway (`ThirdParty/pi/gateway/`)

- [x] 2.1 `types.ts`: optional `historySummary` (≤ 16,000 chars) on `RunRequest`; `validateSummaryRequest` for the summaries body
- [x] 2.2 `server.ts`: raise `readJSON` limit for runs to 512 KiB; append the summary under "Summary of earlier turns in this conversation" in `createRunResourceLoader`
- [x] 2.3 `server.ts`: add `GET /v1/models/:provider/:model` (200 with `contextWindow`/`maxTokens`, 404 unknown)
- [x] 2.4 `server.ts`: add `POST /v1/summaries` using `modelRuntime.completeSimple` with no tools, returning `{summary}`; injectable for tests
- [x] 2.5 Tests in `gateway.test.ts` for validation, summary append, model route, summaries route; run `bun test gateway` and `bun run check`; commit

## 3. ChenWeb data and configuration

- [x] 3.1 Goose migration adding `history_summary`, `history_summary_through_seq`, `history_summary_updated_at` to `kb.agentic_conversations`
- [x] 3.2 `store.go`: load the new columns into `Conversation` (not exposed in JSON); add `SetConversationTitleIfEmpty`, `SaveHistorySummary` (compare-and-set on through-seq), `ClearHistorySummary`
- [x] 3.3 `profiles.go`: `MaxHistoryTokens` limit and `PI_<GUIDE>_MAX_HISTORY_TOKENS` override (positive integer, default 32,000)
- [x] 3.4 Prompt file `prompts/prompt-agent-history-summary-v1.md` and loading it at startup

## 4. ChenWeb history building (`history.go`)

- [x] 4.1 `estimateTokens`, `shortenForHistory` (UTF-16 limit + `…[truncated]`), `sourceFooter`
- [x] 4.2 `completeTurns` pairing by attempt id over visible messages, honoring summary through-seq
- [x] 4.3 `historyBudget(contextWindow, override)` and `selectHistory` (newest-first, ≤ 50 turns, chronological output)
- [x] 4.4 `planFold` (which turns to fold so the rest fit in half the budget) and `titleFromMessage`
- [x] 4.5 Unit tests for every function in 4.1–4.4

## 5. ChenWeb run integration

- [x] 5.1 `gateway.go`: `HistorySummary` on `GatewayRunRequest`; `ModelInfo` (10-minute cache) and `Summarize` on the bridge
- [x] 5.2 `run_handler.go`: set the title on first turn; replace the history loop with `selectHistory`; drop or clear an invalidated summary; byte-size guard at 480 KiB
- [x] 5.3 `run_handler.go`: after a completed turn, run the fold in the background with a 60 s timeout and its own logger location
- [x] 5.4 Update `run_handler_test.go` fakes and add tests: failed-turn exclusion, summary sent, summary invalidated, fold scheduled, title set once
- [x] 5.5 Web: create conversations with an empty title in `+page.svelte`; `bun run check`
- [x] 5.6 `go test` / `go vet ./server/api/agentservicehandler/` and `mise build-server`; commit

## 6. Documentation

- [x] 6.1 Update devdoc `2026093001-devdoc-pi-agentic-services.md` (§2.3.2, §2.4 gateway routes, §2.8 new suffix, finding 7, §3) and `docs/pi-agentic-services-operations.md` if it lists env vars; commit in KnowledgeStore
