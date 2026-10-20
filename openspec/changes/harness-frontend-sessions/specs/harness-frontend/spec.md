## ADDED Requirements

### Requirement: The harness owns the agent
A front-end turn SHALL run on the harness's own system prompt, tools, settings, memory and session history. ChenWeb SHALL NOT add a system prompt, replace or remove the harness's tools, or store or resend conversation history.

#### Scenario: Continuing a session
- **WHEN** a turn names an existing session
- **THEN** the gateway sends only the new message to that session, and the harness supplies the earlier context

### Requirement: Claude Code uses the machine's Claude Code login
The Claude Code gateway SHALL start Claude Code without `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` unless `CLAUDE_CODE_GATEWAY_USE_API_KEY=1`. It SHALL NOT pass the gateway secret, or variables whose names contain `SECRET`, `PASSWORD`, `API_KEY`, `TOKEN`, `ENCRYPTION`, `CREDENTIAL` or `DATABASE_URL` or end in `_KEY` or `_DSN`, to Claude Code.

#### Scenario: API key in the gateway's environment
- **WHEN** the gateway runs with `ANTHROPIC_API_KEY` set by the ChenWeb environment
- **THEN** Claude Code reports `apiKeySource: none` and uses the signed-in account

### Requirement: Sessions are confined to the workspace folder
The gateway SHALL run sessions in `CLAUDE_CODE_WORKSPACE`. It SHALL list only sessions in that folder, and SHALL refuse to resume or read a session that is not among them.

#### Scenario: Foreign session
- **WHEN** a turn names a session ID that Claude Code does not list for the workspace folder
- **THEN** the gateway returns 404

### Requirement: One turn at a time per session
A session SHALL run one turn at a time; a second turn while one is running SHALL get 409. A turn whose client disconnects SHALL be interrupted.

#### Scenario: Second turn while busy
- **WHEN** a turn is sent to a session whose previous turn has not produced its `result`
- **THEN** the gateway returns 409 and the running turn continues

### Requirement: Permission prompts reach the user
When the harness asks to use a tool, the turn SHALL emit `permission_request` and wait. Answering it with allow (optionally with `updatedInput`) or deny SHALL resume the harness. Permission mode `bypassPermissions` SHALL be refused.

#### Scenario: Answering a question
- **WHEN** Claude Code calls `AskUserQuestion`
- **THEN** a `permission_request` with the questions is emitted, and an allow decision whose `updatedInput` carries the answers is passed to Claude Code

### Requirement: The harness lists its own models
The gateway SHALL list the models the harness offers the signed-in account, as the harness's own model picker does (for Claude Code, `supportedModels()`: aliases such as `default`, `opus`, `sonnet`, plus pinned IDs), without sending a message. A turn MAY name a model from that list. A new session SHALL start on it, and a live session on another model SHALL switch to it as `/model` does. The chat page's model menu SHALL show this list when the selected agent is harness-backed, and keep the configured list for other agents.

#### Scenario: Claude Code selected
- **WHEN** the user selects Claude Code in the chat page's agent menu
- **THEN** the model menu lists the Anthropic models Claude Code offers, labelled with its display names

#### Scenario: Switching model mid-session
- **WHEN** a turn names `sonnet` on a live session that was on `opus`
- **THEN** the session continues on `sonnet` with its history intact

### Requirement: The chat page is a front end to the harness
When a harness-backed agent is selected, the chat page SHALL send each message to the harness and stream its reply. It SHALL show tool calls (collapsed, with input and result), approval requests with allow and deny (and option buttons for the harness's questions), and stop the running turn on request. Its Sessions menu SHALL list the harness's stored sessions and open one with the history the harness returns. Answer text SHALL be rendered as Markdown with raw HTML escaped and only http(s) and mailto links kept.

#### Scenario: Approving a tool from the page
- **WHEN** Claude Code asks to use `Write` during a turn
- **THEN** the page shows an approval card, and clicking Allow lets Claude Code write the file and finish the turn

### Requirement: Admin-only front-end routes
The ChenWeb routes under `/api/v1/harness-frontend` SHALL require an admin or root user, and SHALL relay turn events as SSE without storing them.

#### Scenario: Non-admin user
- **WHEN** a signed-in user without an admin or root role calls any harness front-end route
- **THEN** the response is 403
