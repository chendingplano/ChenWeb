## Why

Knowledge Desk conversations (`kb.agentic_conversations`) are the real sessions for Pi-backed guides: ChenWeb stores every turn and replays the earlier text to a fresh, one-turn Pi session each time. That replay has no budget. Long conversations get more expensive every turn. Eventually they either exceed the 128 KiB gateway request limit or overflow the model's context window, and the turn fails. Follow-up questions lose track of what was cited, dropped turns leave two user messages in a row, and every conversation is titled "New conversation". The devdoc (`2026093001-devdoc-pi-agentic-services.md` §2.3.2, finding 7) records these gaps. They need fixing before the pilot puts real users on long conversations.

## What Changes

- **History budget.** ChenWeb picks which history to send by an estimated token budget instead of the fixed "last 100 messages":
  - The budget comes from the pinned model's context window, which the Pi gateway reports through a new `GET /v1/models/:provider/:model` endpoint. It is capped per guide by a new `PI_<GUIDE>_MAX_HISTORY_TOKENS` setting (default 32,000).
  - The request must also fit the gateway body limit.
- **Rolling summary.** Older turns that no longer fit are folded into a summary stored on the conversation:
  - It is produced after a completed turn by a new gateway endpoint, `POST /v1/summaries`, which uses the guide's own model.
  - It is sent to Pi with each run and appended to the system prompt.
  - A summary that covers an answer which is now hidden (its sources were revoked or changed) is not used.
- **Cited sources carried forward.** Each earlier answer in the history ends with a short list of the sources it cited (title, page, lines).
- **Whole turns only.** History is built from complete question-and-answer pairs. A question whose answer was hidden, failed, stopped, or never finished is left out. Over-long messages are shortened instead of dropped.
- **Titles.** A conversation's title is set from its first question, trimmed to a short single line, when the first turn starts. The page now creates conversations with an empty title and shows its localized placeholder until then.
- **Gateway request limit** is raised from 128 KiB to 512 KiB on both sides (shared Go client and gateway), so the token budget, not the byte limit, normally decides what is sent.

## Capabilities

### New Capabilities

- `agent-conversation-history`: how a Knowledge Desk conversation's earlier turns are selected, summarized, annotated with sources, and sent to the agent engine each turn. This also covers how the conversation title is set.

### Modified Capabilities

(none — no existing spec covers the agent services)

## Impact

- **ChenWeb** `server/api/agentservicehandler/`:
  - `run_handler.go` builds the history and triggers summarization.
  - `store.go` gains summary and title persistence.
  - `profiles.go` gains the `MAX_HISTORY_TOKENS` override.
  - `gateway.go` gains model info, summarize, and the summary field.
  - New `history.go` and a new prompt file `prompts/prompt-agent-history-summary-v1.md`.
- **ChenWeb migration:** adds `history_summary`, `history_summary_through_seq`, and `history_summary_updated_at` to `kb.agentic_conversations`.
- **ChenWeb web:** `routes/home3/agent-services/+page.svelte` creates conversations with an empty title.
- **Shared library** `shared/go/api/llm/gateway.go`. These are additive and backward-compatible:
  - `PiGatewayRun.HistorySummary`
  - `PiGatewayClient.ModelInfo` and `PiGatewayClient.Summarize`
  - a 512 KiB request limit
- **Pi gateway** `ThirdParty/pi/gateway/`:
  - `GET /v1/models/:provider/:model` and `POST /v1/summaries`
  - `historySummary` in `RunRequest`
  - a 512 KiB body limit
- **Model-provider usage:** one extra summarization call per conversation each time history overflows the budget, sent to the same provider the guide already uses (no new data destination).
- **Docs:** devdoc §2.3.2, §2.4, §2.8, finding 7 and §3 need updating after implementation.
