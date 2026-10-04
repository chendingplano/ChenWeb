# agent-conversation-history Specification

## Purpose
TBD - created by archiving change agent-conversation-history-budget. Update Purpose after archive.
## Requirements
### Requirement: History is built from whole turns
The system SHALL build the history sent to the agent engine only from complete turns. A complete turn is a `user` message and an `assistant` message that share an `attempt_id`, where the assistant message has `status = complete` and is visible to the user. A user message whose answer is missing, failed, stopped, interrupted, incomplete, or hidden SHALL NOT be sent.

#### Scenario: Failed turn is left out
- **WHEN** a conversation has turns Q1→A1 (complete), Q2→A2 (failed), and the user sends Q3
- **THEN** the history sent with Q3 contains Q1 and A1 only, and never two user messages in a row

#### Scenario: Hidden answer is left out with its question
- **WHEN** answer A1 is hidden because one of its cited sources was revoked
- **THEN** neither Q1 nor A1 is sent in the history

### Requirement: Over-long messages are shortened, not dropped
The system SHALL shorten any history message whose content, including its source footer, would exceed 15,000 UTF-16 code units, and SHALL end the shortened content with `…[truncated]`. It SHALL NOT drop the message.

#### Scenario: Long answer is shortened
- **WHEN** an earlier answer is 20,000 characters long
- **THEN** the history includes it, shortened to at most 15,000 UTF-16 code units and ending with `…[truncated]`

### Requirement: History fits a token budget derived from the model
The system SHALL limit the history to a token budget of `max(2000, min(cap, floor(contextWindow × 0.25)))`:
- `contextWindow` is the pinned model's context window as reported by the agent engine.
- `cap` is the guide's `MAX_HISTORY_TOKENS` setting, 32,000 when unset.

When the context window cannot be obtained, the system SHALL use `min(cap, 16000)`. The system SHALL select turns newest first until the next turn would exceed the budget minus the summary's estimated tokens, and SHALL send the selected turns in chronological order. At most 50 turns SHALL be sent.

#### Scenario: Old turns beyond the budget are not sent
- **WHEN** a conversation's unsummarized turns total more estimated tokens than the budget
- **THEN** only the newest turns that fit are sent, oldest first

#### Scenario: Per-guide cap is applied
- **WHEN** `PI_KNOWLEDGE_GUIDE_MAX_HISTORY_TOKENS=4000` and the model's context window is 200,000
- **THEN** the Knowledge Guide's history budget is 4,000 tokens

#### Scenario: Model info unavailable
- **WHEN** the agent engine cannot report the model's context window and no override is set
- **THEN** the history budget is 16,000 tokens and a warning is logged

#### Scenario: Default cap on a very large window
- **WHEN** no override is set and the model's context window is 1,000,000
- **THEN** the history budget is 32,000 tokens

### Requirement: The run request fits the gateway limit
The system SHALL accept run requests of up to 512 KiB in both the shared gateway client and the gateway. Before sending, ChenWeb SHALL drop the oldest history turns while the encoded request exceeds 480 KiB.

#### Scenario: Large history still starts a run
- **WHEN** the selected history would make the encoded request larger than 480 KiB
- **THEN** the oldest turns are dropped until it fits and the run starts

### Requirement: Older turns are folded into a rolling summary
After a turn completes successfully, if the estimated tokens of the unsummarized complete turns exceed the history budget, the system SHALL fold the oldest unsummarized turns, together with any existing summary, into a new summary until the remaining turns fit within half the budget. The summary SHALL be produced by the agent engine using the conversation's pinned provider and model, without tools. The summary and the sequence number it covers through SHALL be stored on the conversation with a compare-and-set on the previous covered-through value. Summarization SHALL run after the response has finished. A failure SHALL be logged and SHALL NOT affect the turn's outcome.

#### Scenario: Summary created after overflow
- **WHEN** a turn completes and the unsummarized turns exceed the budget
- **THEN** the oldest turns are summarized, the summary is stored, and later runs send only the turns after the covered sequence plus the summary

#### Scenario: Summarization fails
- **WHEN** the summary call fails or times out
- **THEN** the completed turn stays completed, the stored summary is unchanged, and the next run trims oldest turns instead

#### Scenario: Concurrent fold is discarded
- **WHEN** two folds start from the same covered-through value and one is stored first
- **THEN** the second one is not stored

### Requirement: The summary reaches the model as context, not as a turn
The system SHALL send the stored summary with each run as `historySummary`. The agent engine SHALL append it to the system prompt under the heading "Summary of earlier turns in this conversation". The summary SHALL NOT be inserted as a user or assistant message.

#### Scenario: Summary is in the system prompt
- **WHEN** a conversation has a stored summary and the user sends a new message
- **THEN** the model's system prompt ends with the summary heading and text, and the message history contains only real turns

### Requirement: A summary covering a hidden answer is not used
At the start of a turn, if any hidden message has a sequence number at or below the summary's covered-through value, the system SHALL NOT send the summary. It SHALL clear the stored summary so that the next fold rebuilds it from visible turns only.

#### Scenario: Revoked source invalidates summary
- **WHEN** an answer that was folded into the summary becomes hidden
- **THEN** the next run is sent without the summary, and the stored summary is cleared

### Requirement: Earlier answers carry their cited sources
For each assistant message in the history that has saved sources, the system SHALL append a footer that starts with `[Sources cited in this answer]` and lists up to 8 distinct sources. Each source shows its document title and, when known, its page and line range.

#### Scenario: Footer lists sources
- **WHEN** answer A1 cited "GB 1234-2020" page 5 lines 10–20
- **THEN** A1 in the next run's history ends with a footer that includes `GB 1234-2020 (p. 5, lines 10–20)`

### Requirement: Model limits are available from the gateway
The gateway SHALL expose `GET /v1/models/:provider/:model`, behind the same bearer authentication as other routes. It SHALL return `{provider, model, contextWindow, maxTokens}` for a known model and `404` for an unknown one.

#### Scenario: Known model
- **WHEN** ChenWeb requests `/v1/models/anthropic/claude-sonnet-4-5`
- **THEN** the gateway returns that model's context window and maximum output tokens

#### Scenario: Unknown model
- **WHEN** the provider/model pair is not in Pi's registry
- **THEN** the gateway returns `404`

### Requirement: Conversation title comes from the first question
When a turn starts on a conversation whose title is empty, the system SHALL set the title from the user's message:
- whitespace collapsed to single spaces
- only the first line kept
- cut at 60 characters, with `…` added if it was cut

It SHALL NOT overwrite a non-empty title. The Knowledge Desk page SHALL create conversations with an empty title and show its localized placeholder until a title is set.

#### Scenario: Title set on first turn
- **WHEN** a new conversation's first message is "What does GB 1234 require for tensile strength of the outer sheath at low temperatures?"
- **THEN** the conversation's title becomes the first 60 characters of that question followed by `…`

#### Scenario: Existing title kept
- **WHEN** a conversation already has a title and the user sends another message
- **THEN** the title is unchanged

