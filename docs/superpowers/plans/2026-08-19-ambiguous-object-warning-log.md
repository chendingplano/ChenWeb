# Ambiguous Object Warning Log Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist unresolved object-reconciliation ambiguity warnings to `kb.doc_proc_logs` with the complete candidate list.

**Architecture:** Reuse the existing `objectReconcileLogSink` and `LogReconcileObject` path. Move the candidate display-list construction into the outcome payload so the no-LLM ambiguity branch records the same candidates that it emits in the warning, without changing reconciliation results.

**Tech Stack:** Go, PostgreSQL, `sqlmock`, existing doc-processing logger.

---

### Task 1: Add the failing regression test

**Files:**
- Modify: `server/api/doc-processing/object_nodes_test.go`

- [x] Add a test for a tied candidate set with no LLM resolver and a SQL mock for one `kb.doc_proc_logs` insert.
- [x] Assert the row is `reconcile_object`, belongs to the input record, has `outcome=unresolved`, and contains every candidate display string.
- [x] Run `go test ./server/api/doc-processing -run TestReconcileArtifactObjectsLogsAmbiguousCandidateList -count=1` and confirm it fails because no insert occurs.

### Task 2: Implement persistence for the warning path

**Files:**
- Modify: `server/api/doc-processing/object_nodes.go`

- [x] Extend the reconciliation outcome payload with the warning-equivalent candidate display list.
- [x] Call `logReconcileOutcome` for unresolved ambiguities even when no LLM resolver is configured.
- [x] Keep the existing warning text and candidate arguments unchanged.
- [x] Run the focused regression test and confirm it passes.

### Task 3: Verify the package

**Files:**
- No additional files.

- [x] Run `gofmt` on changed Go files.
- [x] Run `go test ./server/api/doc-processing`.
- [x] Review the diff for unrelated changes and report any documentation impact.
