# ParadeDB BM25 Search Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional ParadeDB BM25 lexical search for `kb.search_artifacts`, preserving pgvector hybrid search and PostgreSQL FTS fallback.

**Architecture:** Keep existing doc processor writes untouched: rows continue to upsert into `kb.search_artifacts`, and ParadeDB BM25 indexes update from table writes. Add `SEARCH_LEXICAL_BACKEND=paradedb` to switch lexical ranking/counts and hybrid lexical candidates to ParadeDB SQL; default remains PostgreSQL FTS.

**Tech Stack:** Go, PostgreSQL, goose SQL migrations, ParadeDB `pg_search` / `USING bm25`, pgvector.

---

## Chunk 1: Query Backend

**Files:**
- Modify: `server/api/kbhandler/search_registry.go`
- Test: `server/api/kbhandler/search_registry_test.go`

- [x] Write failing tests for ParadeDB backend selection and SQL shape.
- [x] Run focused kbhandler tests and verify failures.
- [x] Add backend selector and ParadeDB lexical count/query SQL.
- [x] Route hybrid lexical CTE through ParadeDB when enabled, with PostgreSQL fallback.
- [x] Run focused kbhandler tests and verify pass.

## Chunk 2: Migration

**Files:**
- Create: `project_migrations/20260603000002_add_paradedb_bm25_to_search_artifacts.sql`

- [x] Add `CREATE EXTENSION IF NOT EXISTS pg_search`.
- [x] Create one BM25 index per existing `kb.search_artifacts` partition using modern ParadeDB SQL.
- [x] Use `artifact_id` as `key_field`; it is unique inside each partition because the parent key is `(artifact_type, artifact_id)`.
- [x] Configure `jieba` tokenizer on searchable text fields for mixed English/CJK docs.
- [x] Add Down migration that drops the partition BM25 indexes and leaves the extension installed.

## Chunk 3: Verification

**Files:**
- Modify: `server/api/kbhandler/search_registry.go`
- Modify: `server/api/kbhandler/search_registry_test.go`
- Create: `project_migrations/20260603000002_add_paradedb_bm25_to_search_artifacts.sql`

- [x] Run `gofmt`.
- [x] Run focused `go test ./server/api/kbhandler -run 'Test.*Registry.*|TestSearch.*Returns|TestSearchAllArtifacts'`.
- [x] Run `go test ./server/cmd/config`.
- [x] Run `git diff --check`.
