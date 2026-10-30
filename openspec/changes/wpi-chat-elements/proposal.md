## Why

ChenWeb's Chat page (`chatter-01.svelte`) is hand-written Svelte. Its message box and its
side panel are to be reusable WPI elements, so that any WPI page can offer an AI assistant
without new code. The human-approved contracts are KnowledgeStore
`doc-repo/code_modules/interpreter-chat-composer.md` and `interpreter-chat-panel.md`, revision R2
(approved 2026-10-30).

## What Changes

- New element code modules and classes `chat-composer` (message box and tool bar) and
  `chat-panel` (Chat and Dialog tabs), which share the chat of their page instance.
- Only the harness-backed assistants (Claude Code, Codex, DeepSeek Harness, pi) are offered;
  a page chooses among them in the descriptor (`agents`, `default_agent`).
- Descriptor validation of their attributes, at most one composer per page, and app-load
  warnings for an unpaired composer or panel.
- Properties panel groups for the two modules; the class list reports `code_module_id`.
- Migration `20261030000005_wpi_chat_elements.sql` registers both modules and stores both
  classes as active.
- `chatter-01.svelte` and the Chat page are unchanged.

## Capabilities

### New Capabilities
- `wpi-chat-elements`: the chat-composer and chat-panel elements and their shared chat.

### Modified Capabilities
- `wpi-page-rendering`: the element types now include `chat-composer` and `chat-panel`, and
  each instance is drawn by its class copy's code module.

## Impact

- Frontend: `web/src/lib/wpi/chat-session.svelte.ts`, `chat-agents.ts`,
  `elements/ChatComposerElement.svelte`, `elements/ChatPanelElement.svelte`, the element
  registry, `types.ts`, `api.ts`, `PropertiesPanel.svelte`, `harnessFrontendService.ts`
  (`listHarnesses`, `HarnessRequestError`), messages `wpi_chat_*`.
- Backend: `server/api/wpi/chat_attributes.go`, `validate.go`, `bundle.go`, `store.go`.
- Uses the existing admin-only `/api/v1/harness-frontend` routes and the chatter prompt and
  slash-command routes; no new endpoints.
