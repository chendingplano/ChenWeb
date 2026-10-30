## Context

The element contracts are the KnowledgeStore documents `code_modules/interpreter-chat-composer.md`
and `interpreter-chat-panel.md` (R2). This file records only the implementation decisions.

## Decisions

- **D1 One chat per page instance.** `acquireChat(runtime, instanceId)` returns a reference-counted
  `ChatSession` keyed by the runtime and the page instance. Both elements acquire it when mounted
  and release it when destroyed; the last release aborts streaming replies. Hidden pages stay
  mounted, so hiding keeps the chat. The editor preview has its own runtime, so its chat is separate
  and makes no requests.
- **D2 Derived selection.** The offered assistants, the selected assistant and the selected model are
  getters over the configuration, the server's configured harnesses and the user's choice
  (`chat-agents.ts`), so the order in which the composer and the panel mount does not matter.
- **D3 Services, not actions.** The store calls `harnessFrontendService` directly; streaming,
  interrupting and permission decisions have no WPI action class. Authorization stays on the
  server's admin-only routes; a 401 or 403 becomes the "no access" notice.
- **D4 Validation in its own file.** `chat_attributes.go` holds the attribute checks, the
  one-composer page check and the pairing warnings; `validate.go` and `bundle.go` only call them.
- **D5 Theme.** The elements use theme tokens. The shared `harness-dialog.svelte` draws white text on
  its accent, so the panel passes a fixed indigo accent that reads in both themes.

## Risks / Trade-offs

- The panel cannot fill its pane's height until the layout supports stretching; its class gives
  it a 640px default height (accepted in review, to be addressed later).
- Prompts and slash commands still come from the stub `/api/v1/chatter` routes.
