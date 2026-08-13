# Metric Semantic Convergence Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make supported metrics converge automatically into accepted semantic assertions, including after their subject or governed terms become available.

**Architecture:** The normalizer owns canonicalizing extractor value vocabulary and preserving metric identity in a candidate payload. Association derives its assertion-kind eligibility from released ontology terms. The bounded backlog drain distinguishes payload-changing subject recovery from term-availability retries, while phase-D telemetry exposes deferred reasons.

**Tech Stack:** Go 1.25, PostgreSQL, `go-sqlmock`, existing ChenWeb assertion and document-processing packages.

---

## File map

- `server/api/ontology/assertions/metric_normalizer.go`: canonical value-range mapping and metric-definition candidate field.
- `server/api/ontology/assertions/metric_normalizer_test.go`: production vocabulary and payload regression tests.
- `server/api/ontology/assertions/associate_semantics.go`: released-term-driven metric acceptance and identity qualifier.
- `server/api/ontology/assertions/associate_semantics_test.go`: released exact-value acceptance tests.
- `server/api/ontology/assertions/backlog_drain.go`: dependency-aware deferred recovery.
- `server/api/ontology/assertions/backlog_drain_test.go`: recovery selection and changed-fingerprint tests.
- `server/api/ontology/assertions/telemetry.go`: deferred-reason report bucket.
- `server/api/ontology/assertions/telemetry_test.go`: report accounting test.
- `server/api/doc-processing/extract-metrics.go`: reusable metric-subject reconciliation entrypoint for recovery, if required after tracing the existing persistence seam.

## Chunk 1: Canonical value and released-term acceptance

### Task 1: Lock in extractor vocabulary normalization

**Files:**

- Modify: `server/api/ontology/assertions/metric_normalizer_test.go`
- Modify: `server/api/ontology/assertions/metric_normalizer.go`

- [x] **Step 1: Write failing table-driven tests** for `min`, `minimum`, and `min_threshold` (`>=` lower bound); `max` and `maximum` (`<=` upper bound); exact count/duration/ratio/specification (`=` exact value); and range aliases. Use raw production-style values including `≥30` and `≤30`.
- [x] **Step 2: Run** `go test ./server/api/ontology/assertions -run TestResolveMetricValue -count=1`; confirm unknown structured forms are reported as unparsed.
- [x] **Step 3: Implement** a single canonicalizer from accepted extractor terms to the existing structured forms before the existing switch.
- [x] **Step 4: Re-run** the targeted tests and then `go test ./server/api/ontology/assertions -count=1`.

### Task 2: Lock in exact-value acceptance from released terms

**Files:**

- Create: `server/api/ontology/assertions/associate_semantics_test.go`
- Modify: `server/api/ontology/assertions/associate_semantics.go`

- [x] **Step 1: Write a failing regression test** for exact-value governed-term eligibility.
- [x] **Step 2: Run** the focused test and confirm the missing eligibility seam.
- [x] **Step 3: Implement** rejection only for missing/unparsed assertion kinds; use the existing released-term lookup for every remaining `mea:<kind>`.
- [x] **Step 4: Re-run** the targeted and package tests.

## Chunk 2: Identity and deferred recovery

### Task 3: Carry the governed metric-definition identity

**Files:**

- Modify: `server/api/ontology/assertions/metric_normalizer.go`
- Modify: `server/api/ontology/assertions/associate_semantics.go`
- Modify: `server/api/ontology/assertions/metric_normalizer_test.go`

- [x] **Step 1: Write failing tests** proving `metric_definition_term_id` is selected into the candidate payload and emitted into accepted assertion qualifiers.
- [x] **Step 2: Run** the focused tests and verify the expected absent field.
- [x] **Step 3: Extend** the normalizer query/row/payload and association qualifier map without a schema migration.
- [x] **Step 4: Run** `go test ./server/api/ontology/assertions -count=1`.

### Task 4: Make deferred recovery dependency-aware

**Files:**

- Modify: `server/api/ontology/assertions/backlog_drain.go`
- Modify: `server/api/ontology/assertions/backlog_drain_test.go`
- Modify: `server/api/doc-processing/extract-metrics.go` only if existing subject persistence cannot be reused safely.

- [x] **Step 1: Write failing tests** for governed-term availability fingerprint stability/change detection.
- [x] **Step 2: Run** the focused test; confirm the helper is absent.
- [x] **Step 3: Implement** term dependency fingerprinting and retry using `DecisionCandidateStore.RetryDeferred`; keep re-normalization for referent changes. Fresh pipeline processing already invokes metric object reconciliation before Phase D; a later reconciled object link is picked up through re-normalization.
- [x] **Step 4: Run** assertions and document-processing package tests.

## Chunk 3: Observability and verification

### Task 5: Expose deferred reason counts

**Files:**

- Modify: `server/api/ontology/assertions/telemetry.go`
- Modify: `server/api/ontology/assertions/telemetry_test.go`
- Modify: `server/api/doc-processing/phase_d.go`

- [x] **Step 1: Extend the report regression coverage** with deferred-reason accounting.
- [x] **Step 2: Run** the focused telemetry suite.
- [x] **Step 3: Add** the bucket and include it in phase-D structured logs.
- [x] **Step 4: Re-run** the assertions package tests.

### Task 6: Full verification and documentation

**Files:**

- Modify: `docs/superpowers/plans/2026-08-13-metric-semantic-convergence.md` to mark executed steps.

- [x] **Step 1: Run** `gofmt -w` on changed Go files.
- [x] **Step 2: Run** `go test ./server/api/ontology/assertions ./server/api/doc-processing -count=1` from `ChenWeb`.
- [ ] **Step 3: Run** `go test ./...` from `ChenWeb` if the focused suite is clean. It was run and has unrelated pre-existing failures in `kbhandler`, `llmusage`, `ontology/keywords`, and `cmd/qudt-import`.
- [x] **Step 4: Inspect** `jj status` and `jj diff --summary`; update design/plan only where implementation meaningfully changed the documented approach.
- [ ] **Step 5: Commit** the implementation with `jj commit -m 'Fix metric semantic association convergence'`, then use `jj log` to verify a linear history.
