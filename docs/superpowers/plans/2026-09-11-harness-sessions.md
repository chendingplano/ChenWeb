# Harness Sessions Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add database-backed Pi session browsing beside Chad Sessions using shared harness-neutral persistence tables.

**Architecture:** ChenWeb exposes one harness-filtered session API and reuses the existing session UI for Chad and Pi. Chad and Pi independently mirror new local sessions into `kb.harness_sessions` and `kb.harness_messages` through `HARNESS_DATABASE_URL`.

**Tech Stack:** Go/Echo, Svelte/TypeScript, PostgreSQL/goose, Python Chad, TypeScript Pi extension.

---

### Task 1: Harness-neutral database schema

**Files:** `ChenWeb/project_migrations/20260911000009_rename_chad_sessions_to_harness_sessions.sql`

- [ ] Add a migration creating the harness tables with `harness_name`, composite identity, and existing message metadata.
- [ ] Explicitly drop the old Chad tables as requested.
- [ ] Apply and verify the migration against the configured database.

### Task 2: Chad persistence

**Files:** `ThirdParty-2/chad/src/chad/db.py`, Chad tests/docs

- [ ] Change the writer to `HARNESS_DATABASE_URL`, harness tables, and `harness_name = 'chad'`.
- [ ] Add focused tests for the new table/field contract.
- [ ] Run Chad tests and commit only Chad feature changes.

### Task 3: Pi persistence extension

**Files:** `ThirdParty/pi/extensions/...`, `ThirdParty/pi/package.json`

- [ ] Implement a Pi extension that mirrors newly-created/updated JSONL sessions.
- [ ] Normalize Pi messages, tool calls, metadata, and timestamps into the harness tables.
- [ ] Use `HARNESS_DATABASE_URL` and avoid ChenWeb/shared dependencies.
- [ ] Add focused tests or a deterministic normalization test and run Pi checks.

### Task 4: ChenWeb API and navigation

**Files:** existing Chad sessions handler/client/view, navigation seed migration, routes/tests

- [ ] Generalize handler queries to accept a fixed harness name and use harness tables.
- [ ] Preserve the Chad page and add a Pi page with the same behavior.
- [ ] Add `Pi Sessions` to System Admin → LLM.
- [ ] Run focused Go and frontend tests.

### Task 5: Verification and commits

- [ ] Verify no stale `kb.chad_*` references remain in active code.
- [ ] Confirm database columns/types and harness filtering.
- [ ] Commit ChenWeb changes separately from Chad and Pi changes.
