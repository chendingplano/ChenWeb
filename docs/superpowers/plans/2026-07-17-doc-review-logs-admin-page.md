# Doc Review Logs Admin Page Implementation Plan

> **For agentic workers:** Execute directly in the current session as requested; keep changes focused and verify each layer.

**Goal:** Add a paginated Doc Review Logs page at System Admin → Logs, including JSON and artifact-detail dialogs.

**Architecture:** Add a read-only `kbhandler` list endpoint and SQL store for `kb.doc_review_logs`. Add one home3 Svelte view, wire it into the existing content panel and menu, and reuse the existing artifact wiki endpoint with `include_article=0` for recognized Unit Keys.

**Tech Stack:** Go, Echo, PostgreSQL, Svelte 5, TypeScript.

---

## To-do

- [ ] Add store, handler, route, and Go tests for paginated/filterable review logs.
- [ ] Add the Svelte log table with filters, pagination, JSON detail dialog, and artifact dialog.
- [ ] Wire `SYSTEM ADMIN → Logs → Doc Review Logs`.
- [ ] Run focused Go and web verification; commit only task changes with jj.
