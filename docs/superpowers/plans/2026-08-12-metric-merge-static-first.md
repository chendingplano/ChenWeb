# Metric Merge Static-First Implementation Plan

> **For agentic workers:** Use the TDD workflow while implementing this plan.

**Goal:** Avoid metric-merge LLM calls whenever source spans and structured
fields identify a deterministic outcome.

**Architecture:** Keep exact deduplication and no-overlap additions, add a
conservative static classifier for overlap cases, and pass only unresolved
connected components to the existing merge-resolution LLM. The persistence and
ontology-resolution paths remain unchanged.

**Tech Stack:** Go, existing `doc-processing` merge logic, table-driven unit
tests.

---

## Chunk 1: Static classification

### Task 1: Add failing merge-classification tests

**Files:**
- Modify: `server/api/doc-processing/metrics_merge_test.go`

- [x] Add tests proving that a candidate with no source-span overlap is added
  without a pending group.
- [x] Add a test proving one overlapping candidate with one statically unique
  existing match is resolved without a pending group.
- [x] Add a test proving distinct structured identities sharing a source span
  are resolved statically.
- [x] Add a test proving two equally plausible overlapping existing metrics
  remain in one pending group.
- [x] Run the focused tests and confirm the new static-match expectations fail
  against the current implementation.

### Task 2: Implement the static classifier

**Files:**
- Modify: `server/api/doc-processing/metrics_merge.go`

- [x] Add normalized field comparison helpers using existing metric map and
  source-span normalization utilities.
- [x] Classify each remaining candidate as added, statically resolved, or
  unresolved.
- [x] Preserve existing IDs for static merges and retain candidate fields for
  the existing dirty/upsert path.
- [x] Build connected components only from unresolved candidates and the
  existing rows they still overlap.
- [x] Run the focused merge tests until they pass.

### Task 3: Regression verification

**Files:**
- No additional files expected.

- [x] Run `go test ./server/api/doc-processing -count=1`.
- [x] Run related ontology package tests; one unrelated resolver-mode test suite
  is currently blocked by pre-existing `mode.go` edits.
- [x] Run `go vet` for the affected packages.
- [x] Review the diff for accidental changes to resolver or extraction logic.

### Task 4: Commit

- [ ] Commit only the intended static-first merge files through `jj commit`.
- [ ] Verify the resulting `jj log` is linear and contains no unexpected
  divergent commits.
