# KB Input Store Metadata Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure uploaded `kb.inputs` records and ZIP child records contain the active knowledge store's `ks_store_id` and `ks_desc`.

**Architecture:** `UploadInputs` resolves the store description from `kb.knowledge_store` using the submitted store ID, making the database authoritative. `doc-service` loads both metadata fields from the parent ZIP record and passes them into child-record insertion.

**Tech Stack:** Go, PostgreSQL, Echo, sqlmock.

---

## Chunk 1: Upload metadata

### Task 1: Add a failing direct-upload regression test

**Files:**
- Modify: `server/api/kbhandler/upload_handler_test.go`

- [ ] Expect an active-store description lookup and assert the upload insert receives that description even when the multipart `ks_desc` value is absent or different.
- [ ] Run `go test ./server/api/kbhandler -run TestUploadInputs -count=1` and confirm the new expectation fails before implementation.

### Task 2: Resolve the active store description in the upload handler

**Files:**
- Modify: `server/api/kbhandler/upload_handler.go`

- [ ] Query `kb.knowledge_store.ks_desc` by `ks_store_id` after validating the upload store ID.
- [ ] Use the resolved description in every inserted upload row and return a clear server error if the store cannot be read.
- [ ] Keep the existing transaction and staging cleanup behavior unchanged.
- [ ] Run the focused handler tests and confirm they pass.

## Chunk 2: ZIP child inheritance

### Task 3: Add a failing ZIP inheritance regression test

**Files:**
- Modify: `server/cmd/doc-service/main_test.go`

- [ ] Extend the ZIP-child sqlmock scenario to load parent `ks_store_id` and `ks_desc` and require both values in the child insert.
- [ ] Run `go test ./server/cmd/doc-service -run TestProcessStagingOnceZipChild -count=1` and confirm it fails before implementation.

### Task 4: Propagate store metadata to ZIP children

**Files:**
- Modify: `server/cmd/doc-service/main.go`

- [ ] Load the parent ZIP record's `ks_store_id` and `ks_desc` alongside its tenant ID.
- [ ] Extend child ingestion arguments and `upsertStagedInputRecord` to write both fields for newly created child rows.
- [ ] Preserve existing-row behavior so metadata on already-uploaded records is not overwritten.
- [ ] Run the focused ZIP tests and the complete package tests.

## Chunk 3: Verification

- [ ] Run `gofmt` on modified Go files.
- [ ] Run `go test ./server/api/kbhandler ./server/cmd/doc-service`.
- [ ] Inspect the diff and confirm no unrelated dirty-worktree changes were modified.
