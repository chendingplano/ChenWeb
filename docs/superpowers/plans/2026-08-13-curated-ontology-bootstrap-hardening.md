# Curated Ontology Bootstrap Hardening Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden curated ontology bootstrap against the six review findings.

**Architecture:** Keep one importable seed package. Its service-facing entry
point reports a nonfatal measurement prerequisite warning, while its explicit
CLI-facing entry point remains strict. Curated releases use content-derived
versions and never replace an active release at startup.

**Tech Stack:** Go 1.25, database/sql, PostgreSQL, existing ontology stores,
go-sqlmock.

---

## Chunk 1: Seed package safety

### Task 1: Add behavioral seed tests

**Files:**
- Create: `server/api/ontology/seed/seed_test.go`
- Modify: `server/api/ontology/seed/seed.go`

- [ ] Write failing tests with `go-sqlmock` for: inactive `quantity` producing
  a nonfatal measurement warning; content-derived versions; no activation when
  a module already has an active release; changed preferred labels being
  superseded before replacement; and the literal PostgreSQL-user fallback.
- [ ] Run `go test ./server/api/ontology/seed` and confirm each test fails for
  the reviewed behavior.
- [ ] Implement the smallest package-level changes to pass these tests.
- [ ] Re-run `go test ./server/api/ontology/seed`.

### Task 2: Wire safe startup and CLI configuration

**Files:**
- Modify: `server/cmd/deepdoc/main.go`
- Modify: `server/cmd/doc-processor/main.go`
- Modify: `server/cmd/ontology-seed/main.go`
- Modify: `server/cmd/ontology-seed/main_test.go`

- [ ] Add failing tests for a dedicated Deepdoc bootstrap timeout and the
  literal CLI user fallback.
- [ ] Pass a 3-minute context to Deepdoc bootstrap, log nonfatal deferred
  module warnings in both services, and correct the CLI fallback.
- [ ] Run targeted command tests and builds.

## Chunk 2: Operational alignment

### Task 3: Align the existing mise task

**Files:**
- Modify: `mise.toml`

- [ ] Update stale task comments and make the verifier use the same `/tmp`
  PostgreSQL host default as the CLI.
- [ ] Run `mise tasks` or the repository's TOML validation command.

### Task 4: Verify and commit

- [ ] Run `go test ./server/api/ontology/seed ./server/cmd/ontology-seed ./server/cmd/deepdoc ./server/cmd/doc-processor`.
- [ ] Run `go build` for the three command packages and `git diff --check`.
- [ ] Commit only the scoped ChenWeb changes with `jj commit`, preserving any
  unrelated worktree changes.
