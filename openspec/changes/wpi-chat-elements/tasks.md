## 1. Contract and validation

- [x] 1.1 Validator: chat-composer and chat-panel attributes, cross-attribute rules, one composer per page, pairing warnings, with Go tests.
- [x] 1.2 Class list reports `code_module_id`.

## 2. Frontend

- [x] 2.1 Shared chat store per page instance, harness-backed assistants only, filtered by the server's configured harnesses.
- [x] 2.2 `ChatComposerElement` and `ChatPanelElement`, registered as code modules; `wpi_chat_*` messages in English and Chinese.
- [x] 2.3 Properties panel groups and controls for both modules.

## 3. Registration and verification

- [x] 3.1 Migration `20261030000005_wpi_chat_elements.sql`, dry-run, applied and read back.
- [x] 3.2 Go tests, bun unit tests, `bun run check`, browser checks against the real components with a stubbed harness.
- [x] 3.3 Update the canonical documents, the reusable-module document and the WPI devdoc.
