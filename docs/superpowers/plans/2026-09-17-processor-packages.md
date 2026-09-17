# Processor Packages Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move processing config to the local override file and add package-driven processor selection with a Default automatic-pipeline test mode.

**Architecture:** Extend the existing `/api/v1/kb/config` response with `[doc-processing-packages]`. Keep the full processor checkbox catalog in the Svelte view, and use a pure state helper to map a selected package to checkbox state. Default launches omit `operation`; named packages retain explicit operation selection.

**Tech Stack:** Go, TOML, Echo API, Svelte 5, TypeScript, Bun tests.

---

## Chunk 1: Configuration and API

Files: `config.toml`, `config.local.toml`, `server/api/kbhandler/kb_config_handler.go`, `server/api/kbhandler/kb_config_handler_test.go`, `server/api/doc-processing/runtime.go`.

- [ ] Add failing tests proving package definitions load and the local `[doc-processing]` section supplies runtime defaults.
- [ ] Move the section and preserve package definitions in `config.local.toml`.
- [ ] Expose package definitions from the KB config endpoint and add runtime support for local-only processing settings.
- [ ] Run focused Go tests and vet.

## Chunk 2: Package selection state and launch payload

Files: `web/src/lib/services/kbService.ts`, `web/src/lib/components/home3/doc-processor-dashboard-state.ts`, corresponding tests, and `doc-processor-dashboard-view.svelte`.

- [ ] Add failing tests for package-to-checkbox mapping and Default-vs-explicit launch payload construction.
- [ ] Add typed package config, preserve all processor rows, and wire the pulldown to checkbox state.
- [ ] Make Default launch omit `operation`; keep explicit package/manual selections as operations.
- [ ] Run frontend tests and type checking.

## Chunk 3: Verification and handoff

- [ ] Run focused backend/frontend checks plus frontend build.
- [ ] Review diff, preserve unrelated worktree changes, and commit ChenWeb changes with `jj`.
