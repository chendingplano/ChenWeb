## Context

A Knowledge Desk conversation is the durable session; a Pi session lasts one turn (devdoc 2026093001 §2.3.2). Today `RunHandler.Start` builds the history like this:

- Take every visible `user`/`assistant` message with `status = complete`.
- Skip any message longer than 16,000 characters.
- Keep the last 100.

ChenWeb then sends that history to the gateway. Problems:

- **No token budget.** Cost grows every turn until the context window overflows.
- **Byte limit.** The 128 KiB request limit, in both `shared/go/api/llm/gateway.go` and the gateway's `readJSON`, fails long conversations outright with "Pi run request too large".
- **Lost sources.** Tool results are not carried forward, so the model no longer knows what it cited.
- **Broken pairs.** A dropped answer (hidden, failed, stopped, interrupted) leaves its question in the history.
- **No titles.** Titles are never derived from content.

Constraints:

- **ChenWeb is the only store of record.** Pi stays stateless.
- **Model credentials live only in Pi.** ChenWeb never calls the guide's provider directly.
- **Hidden answers must not leak.** Content from answers hidden because of revoked or changed sources must not reach the model.
- **The shared library changes must be additive.** Other projects import `shared/go/api/llm`.
- **Three repositories, three separate commits:** shared, Pi (`ThirdParty/pi`), and ChenWeb.

## Goals / Non-Goals

**Goals:**
- History sent per turn stays within a budget derived from the pinned model's context window.
- Older turns are summarized rather than silently lost.
- Earlier answers carry a short list of the sources they cited.
- History contains only whole question-and-answer pairs.
- Conversations get a meaningful title from the first question.

**Non-Goals:**
- Replaying tool calls or retrieved passages from earlier turns. These are not stored, by design.
- Keeping Pi sessions alive between turns.
- Model-generated titles, which would add a provider call before the first answer.
- Letting users edit titles or view or edit the summary.
- Changing model pinning. A conversation still ends when its guide's model changes.

## Decisions

### D1. ChenWeb selects the history; the gateway reports the model's limits

ChenWeb already loads, filters and authorizes messages, so it keeps doing the selection. It needs the context window, which only Pi knows. A new loopback route, `GET /v1/models/:provider/:model`, returns `{provider, model, contextWindow, maxTokens}`, or `404` if Pi doesn't know the pair. ChenWeb caches the result per provider/model for 10 minutes.

- **Budget:** `historyBudget = max(2000, min(cap, floor(contextWindow × 0.25)))`. Here `cap` is `PI_<GUIDE>_MAX_HISTORY_TOKENS`, 32,000 when unset. The 25 % share leaves room for the system prompt, tool definitions, tool results within the turn, and output. The cap limits cost: Pi's catalog lists some models with a 1,000,000-token window (for example `claude-sonnet-4-5`), and a quarter of that would be 250k history tokens every turn.
- **If model info can't be fetched:** use `min(cap, 16000)` and log a warning. The run itself will fail with a clear error if the model is truly unknown.

*Alternatives:*
- Let the gateway trim, since it knows the window. But it can't store summaries and would have to tell ChenWeb what it dropped, which splits the logic across two places.
- Use a fixed per-guide budget only. Simpler, but it ignores the model and was explicitly asked to derive from it. The override still allows a fixed cap.

### D2. Token estimate without a tokenizer

`estimateTokens(s)` = ASCII bytes ÷ 4 + one token per non-ASCII rune. This is conservative for Chinese, where it overestimates slightly, and close for English. A per-provider tokenizer isn't worth the dependency because the budget already has a wide margin.

### D3. Whole pairs, newest first

History is built from **pairs**. A pair is a `user` message and an `assistant` message with the same `attempt_id`, where the assistant message has `status = complete` and is visible after `FilterResumeState`. Anything else is skipped as a unit.

- **Shortening:** over-long content is shortened so the gateway's 16,000-UTF-16-unit per-message limit is never hit. The cut is at 15,000 units minus the source footer, and ends with `…[truncated]`.
- **Selection:** pairs are taken newest-first until the next pair would exceed `historyBudget − summaryTokens`. They are then emitted in chronological order.
- **Hard limits:** at most 50 pairs (100 messages, the gateway's existing limit), and the request must also fit the byte limit (see D7).

### D4. Rolling summary stored on the conversation

New columns on `kb.agentic_conversations`:

- `history_summary TEXT NOT NULL DEFAULT ''`
- `history_summary_through_seq INTEGER NOT NULL DEFAULT 0`
- `history_summary_updated_at TIMESTAMPTZ`

The summary covers every pair whose user message has `sequence_no ≤ history_summary_through_seq`. Only pairs *after* that point are candidates for the raw history.

- **When it is used.** It is sent to the gateway as `historySummary` on the run. The gateway appends it to the system prompt under a fixed heading ("Summary of earlier turns in this conversation"), after the knowledge context. It is not added as a fake user/assistant turn, so the model doesn't mistake it for something the user said.
- **When it is refreshed.** After a turn completes (`outcome.Status == "completed"`), ChenWeb checks the unsummarized pairs. If their estimated tokens exceed `historyBudget`, it folds the oldest of them into the summary until the rest fit within `historyBudget / 2`. The fold works like this:
  1. Call `POST /v1/summaries` with the previous summary, the pairs to fold (including their source footers), the guide's provider/model, the summarizer system prompt (`prompts/prompt-agent-history-summary-v1.md`, sent by ChenWeb), and `maxTokens = min(1024, historyBudget / 4)`.
  2. Store the result with a compare-and-set on `history_summary_through_seq`, so concurrent folds can't overwrite each other.
- **It runs in the background, after the response has finished.** It uses its own 60-second timeout and doesn't delay the user. On failure it logs and does nothing. The next turn just trims oldest-first, and the next completed turn tries again.
- **Why after the turn rather than before.** Folding before the turn would put a model call in front of every overflowing turn. After the turn, the cost lands while the user is reading.

*Alternative:* turn on Pi's own compaction. That doesn't work, because the Pi session dies with the turn, so its compaction can't persist. It would also re-run every turn.

### D5. Hidden answers invalidate the summary

`FilterResumeState` can later hide an answer that was folded into the summary. At turn start, if any hidden message has `sequence_no ≤ history_summary_through_seq`, the summary is **not sent**. That turn trims oldest-first instead. The stored summary is then cleared (`history_summary = ''`, `through_seq = 0`), and the next completed turn rebuilds it from the visible pairs only. A rebuild works on the visible pairs, so revoked content never comes back.

### D6. Source footer on earlier answers

For each assistant message in the history, if `SourcesByMessage` has entries, append:

```
[Sources cited in this answer]
- <document_title> (p. <page>, lines <start>–<end>)
```

Each entry includes page and lines only when present. Entries are de-duplicated by document and line range, with at most 8. The footer is added after shortening and counts toward the budget. It gives the model enough to call `read_source_passages` or `search_knowledge` again without re-searching blind. Document IDs aren't included because titles plus locations are enough, and the prompts tell the model to cite through tools anyway.

### D7. Raise the request limit to 512 KiB

`shared/go/api/llm/gateway.go` (`len(raw) > 128*1024`) and the gateway `readJSON(request, 128 * 1024)` both move to 512 KiB. That's enough for the 64,000-character system prompt plus a budget-sized history at 3 bytes per CJK character. As a last check, ChenWeb marshals the request and, if it still exceeds 480 KiB, drops the oldest pair until it fits.

### D8. Title from the first question

When a turn starts on a conversation whose `title` is empty, ChenWeb sets the title from the message:

- whitespace collapsed to single spaces
- the first line only
- cut at 60 runes, with `…` added if it was cut

The write is guarded by `title = ''`, so an explicit title is never overwritten. The page now passes an empty title when creating a conversation, and keeps showing `m.home3_agent_services_untitled_conversation()` / `a_new_conversation()` until the first turn. A model-generated title was rejected because it adds latency and a provider call for little gain.

### D9. Where the code goes

- **ChenWeb:** a new `agentservicehandler/history.go` holds pure functions (pairing, footer, shortening, estimate, selection, fold planning), so most of the behavior is unit-testable without a database or gateway. `RunStore` gains `SetConversationTitleIfEmpty`, `SaveHistorySummary` (compare-and-set) and `ClearHistorySummary`. `GatewayBridge` gains `ModelInfo` and `Summarize`.
- **Shared** `llm.PiGatewayClient` gains:
  - `ModelInfo(ctx, provider, model)`
  - `Summarize(ctx, PiGatewaySummaryRequest)`, with usage capture under `PromptName: "pi_gateway_summary"`
  - `PiGatewayRun.HistorySummary`, sent as `historySummary` with `omitempty`

  All of these are additions, so existing callers are unaffected.
- **Gateway:** `validateRunRequest` accepts an optional `historySummary` (≤ 16,000 characters). `createRunResourceLoader` appends it. The new routes reuse the same bearer auth. `POST /v1/summaries` validates its body (≤ 512 KiB, ≤ 100 messages) and calls `modelRuntime.completeSimple(model, {systemPrompt, messages}, {maxTokens})`, with no tools and thinking off.

## Risks / Trade-offs

- [The summary loses detail the user later asks about] → Recent turns stay verbatim. The source footer lets the model re-read cited passages. Summaries carry forward and are not rewritten from scratch.
- [The token estimate is wrong for some content] → The 25 % budget share leaves a 4× margin. The byte check is a second guard.
- [Summarization fails repeatedly, for example because of the provider's rate limits] → Turns still work and simply trim oldest-first. Failures are logged under their own location code.
- [Summary content goes to the provider again] → It goes to the same provider and model the conversation already uses, which is covered by the existing provider disclosure.
- [Prompt injection persists through the summary] → The summarizer prompt treats the input as data. The summary is placed under a heading that says it is a summary of earlier turns, and it is subject to the same evidence-is-untrusted rules as before.
- [A race between a background fold and the next turn] → The compare-and-set write means a stale fold is discarded. The next turn uses whichever summary is stored when it starts.

## Migration Plan

1. Commit and test the shared library first, then run `go work sync`.
2. Commit the Pi gateway. The new fields are optional, so an old ChenWeb keeps working against it.
3. Commit ChenWeb with a goose migration. The new columns have defaults and no backfill is needed. All tables currently hold zero rows on `miner`.
4. Rollback: revert the ChenWeb commit. The extra columns are harmless. The gateway and shared changes are backward-compatible.

## Open Questions

- Whether 25 % of the context window is the right share. Revisit after the pilot using the recorded `input_tokens`.
