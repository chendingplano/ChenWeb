## ADDED Requirements

### Requirement: Shared chat of a page instance

The `chat-composer` and `chat-panel` elements of one page instance SHALL share one chat: the
offered and selected assistant and model, the open sessions, the active session and its
conversation. The chat SHALL NOT be part of the input doc, SHALL NOT be saved and SHALL NOT make
the page dirty. It SHALL be kept while the page is hidden and discarded, stopping any streaming
reply, when the page instance closes. A page SHALL have at most one `chat-composer`.

#### Scenario: Composer and panel on one page
- **WHEN** a page has a chat-composer in one pane and a chat-panel in another, and the user sends a message
- **THEN** the reply streams into the panel's Chat tab and the session select shows the session titled from the message

#### Scenario: Two composers
- **WHEN** a page is written with two chat-composer instances
- **THEN** the write fails with an error at the second one

#### Scenario: Unpaired element
- **WHEN** an app loads a page with a chat-composer and no chat-panel
- **THEN** the bundle carries the warning that replies are not shown

### Requirement: Real assistants chosen in the descriptor

The composer SHALL offer only the harness-backed assistants `Claude Code`, `Codex`,
`DeepSeek Harness` and `pi`: by default all four, else those its `agents` lists, in that order,
keeping only those whose gateway the server has configured. `default_agent` SHALL be one of the
resolved `agents`. A refused request (401 or 403) SHALL show "You do not have access to this
assistant." in the conversation, and no reply SHALL be made up.

#### Scenario: Unconfigured gateway
- **WHEN** the server has gateways for claude-code and pi only
- **THEN** the agent menu lists Claude Code and pi

#### Scenario: Assistant not allowed
- **WHEN** an instance sets `"agents": ["OpenClaw"]`
- **THEN** validation fails at `/agents/0`

### Requirement: Configurable composer

The composer SHALL support `placeholder`, `rows` (1–20, default 3), `slash_commands` (default
true) and `toolbar`, an ordered list of distinct controls from `agent`, `model`, `prompt`,
`text_editor` and `new_session` (default all five), with Send, or Stop while a reply streams,
always last. Ctrl+Enter or ⌘+Enter SHALL send; Enter SHALL insert a new line.

#### Scenario: One-assistant composer
- **WHEN** an instance sets `"agents": ["pi"], "toolbar": ["model"]`
- **THEN** only the model select and Send are shown and messages go to the pi harness

### Requirement: Configurable panel tabs

The panel SHALL support `tabs`, an ordered list of distinct tabs from `chat` and `dialog`
(default both), and `initial_tab`, one of them. The Chat tab SHALL show the session select,
History and close controls and the conversation; the Dialog tab SHALL show the compact history.
With one tab the tab bar SHALL NOT be drawn. The tab bar SHALL follow the WAI-ARIA tabs pattern.

#### Scenario: Keyboard
- **WHEN** the Chat tab has focus and the user presses ArrowRight
- **THEN** the Dialog tab is selected and focused
