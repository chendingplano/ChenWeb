## MODIFIED Requirements

### Requirement: History is built from whole turns
The system SHALL build the history sent to the agent engine only from complete turns. A complete turn is a `user` message and an `assistant` message that share an `attempt_id`, where the assistant message has `status = complete`. A user message whose answer is missing, failed, stopped, interrupted, or incomplete SHALL NOT be sent. A complete turn SHALL include, between the question and the answer, the tool calls of that attempt that have stored results and were not removed by the snapshot rule, in the order they were made.

#### Scenario: Failed turn is left out
- **WHEN** a conversation has turns Q1→A1 (complete), Q2→A2 (failed), and the user sends Q3
- **THEN** the history sent with Q3 contains Q1 and A1 only, and never two user messages in a row

#### Scenario: Turn carries its tool calls
- **WHEN** turn Q1→A1 made a `search_knowledge` call and a `read_source_passages` call
- **THEN** the history sent with the next question contains Q1, both tool calls with their results in that order, and A1

### Requirement: Earlier answers carry their cited sources
For each assistant message in the history that has saved sources remaining after snapshot filtering, the system SHALL append a footer that starts with `[Sources cited in this answer]` and lists up to 8 distinct sources. Each source shows its document title and, when known, its page and line range.

#### Scenario: Footer lists sources
- **WHEN** answer A1 cited "GB 1234-2020" page 5 lines 10–20
- **THEN** A1 in the next run's history ends with a footer that includes `GB 1234-2020 (p. 5, lines 10–20)`

## REMOVED Requirements

### Requirement: A summary covering a hidden answer is not used
**Reason**: Answers are no longer hidden. Under the snapshot rule (doc-2026091401 §11), answer and summary text is kept as saved even after access to a source is lost.
**Migration**: None. The summary is always sent when present.

## ADDED Requirements

### Requirement: Conversation history is a time-limited snapshot
The system SHALL treat each saved record as a snapshot of the knowledge base at the time it was saved. The snapshot period is `PI_HISTORY_SNAPSHOT_DUR` hours (positive integer, default 48). An invalid value SHALL stop startup. Age SHALL be measured per record: a source by its message's creation time, and a tool result by its own creation time.
- A record younger than the snapshot period SHALL be shown to the user and resent to the agent engine exactly as saved, regardless of later access or content changes.
- An older record SHALL be checked for access only, not for content changes:
  - a source whose document the user can no longer access SHALL be removed;
  - a tool result that references any document the user can no longer access SHALL be removed.
- Answer and summary text SHALL never be removed or hidden.
- The same filtered view SHALL be used for the conversation page, for the history sent to the agent engine, and for `get_saved_tool_result`.

#### Scenario: Revoked access inside the snapshot period
- **WHEN** the user loses access to a document cited one hour ago and reopens the conversation
- **THEN** the answer, its source reference, and the tool result are shown and resent as saved

#### Scenario: Revoked access after the snapshot period
- **WHEN** the user loses access to a document cited three days ago and the snapshot period is 48 hours
- **THEN** that source reference and every tool result referencing the document are removed, the answer text is still shown and resent, and the response reports the removal counts

#### Scenario: Changed content after the snapshot period
- **WHEN** a document cited three days ago was reprocessed but the user still has access
- **THEN** the source reference and tool results are kept

### Requirement: The page tells users history is a snapshot
The conversation response SHALL include `snapshot_hours`, `removed_sources`, and `removed_tool_results`. Whenever a saved conversation with messages is shown, the Knowledge Desk SHALL display a localized notice that the history is a snapshot of the knowledge base at the time of each turn and may differ from the current knowledge base. When any record was removed, it SHALL also display a localized notice that part of the conversation is unavailable because access changed.

#### Scenario: Snapshot notice
- **WHEN** a user opens a saved conversation
- **THEN** the page shows the snapshot notice in the user's language

#### Scenario: Removal notice
- **WHEN** the response reports `removed_tool_results = 2`
- **THEN** the page also shows the access-changed notice

### Requirement: Tool calls and full results are stored
The system SHALL store each tool call that reaches ChenWeb's internal tool route with a valid run capability and a tool-call ID. It SHALL store the tool name, the arguments, the exact response body returned, whether it was an error, and the IDs of every document the input or result references. The record SHALL belong to the run's attempt and SHALL be deleted with its conversation. A failure to store SHALL NOT change the response returned to the agent engine.

#### Scenario: Successful call stored
- **WHEN** `search_knowledge` returns three items from documents 4 and 9
- **THEN** a record exists with the arguments, the full response body, `is_error = false`, and document IDs 4 and 9

#### Scenario: Rejected call stored as error
- **WHEN** ChenWeb rejects a tool call with `403`
- **THEN** a record exists with the error body and `is_error = true`

### Requirement: Long tool results are shortened in the history
The system SHALL send a stored tool result in the history unchanged when its size is at most the guide's `MAX_HISTORY_TOOL_RESULT_BYTES` (default 4,096, at most 32,768). A larger result SHALL be replaced in the history by a valid JSON object that states it was truncated and gives the full size, the tool-call ID, the tool to retrieve it, and a prefix of the result. The stored record SHALL remain complete.

#### Scenario: Long result shortened
- **WHEN** a stored result is 20,000 bytes and the limit is 4,096
- **THEN** the history carries a truncated JSON object naming `get_saved_tool_result` and the tool-call ID, and the database still holds 20,000 bytes

### Requirement: The agent can retrieve a saved tool result
The system SHALL offer the tool `get_saved_tool_result`, taking `tool_call_id`, whenever a run offers knowledge tools. It SHALL return the stored response body of that tool call when the call belongs to the same conversation and user and was not removed by the snapshot rule. Otherwise it SHALL return a not-found error. It SHALL NOT produce citations.

#### Scenario: Retrieve full result
- **WHEN** the model calls `get_saved_tool_result` with the ID of an earlier call in the same conversation
- **THEN** it receives that call's full stored response

#### Scenario: Other conversation
- **WHEN** the ID belongs to another conversation
- **THEN** the tool returns not found

### Requirement: Unavailable tools are replayed as text
When a history tool call names a tool that the current run does not offer, the agent engine SHALL fold it into the turn's answer text as a short description instead of a tool-call message.

#### Scenario: No knowledge tools this run
- **WHEN** the user has lost every knowledge grant and the history contains `search_knowledge` calls
- **THEN** those calls appear as text in the earlier answers and the run starts without error
