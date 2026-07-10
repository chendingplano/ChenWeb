# Document Review Depth and Output Controls

**Date:** 2026-07-10  
**Source ADR:** `KnowledgeStore/doc-repo/adrs/202607/2026071001-adr-doc-review-max-controls.md`

## Goal

Limit document-review token use and report size without discarding already completed work. Separate the number of related artifacts retrieved from the number sent to an LLM, add a request-level review depth, and use that depth to select per-reviewer soft limits for findings and analyses.

## Decisions

### Related-artifact input limit

The existing reviewer-specific environment variables continue to control retrieval:

- `METRIC_REVIEW_MAX_MATCHES`
- `PROVISION_REVIEW_MAX_MATCHES`
- `ENTITY_REVIEW_MAX_MATCHES`
- `INVENTORY_REVIEW_MAX_MATCHES`

A new `MAX_MATCHES_TO_LLM` environment variable controls how many of the retrieved, ranked matches are included in each LLM payload. It defaults to `3` and must be at least `1`. Metrics, provisions, entities, and inventory-item reviewers slice their matches only when constructing the payload; retrieval and match assembly retain their reviewer-specific limits.

The recently added hard cap of 10 on the reviewer-specific retrieval settings is removed. That cap conflates retrieval breadth with LLM input size and is unnecessary once payload size has a separate control.

### Review depth

`POST /api/v1/doc-review/requests` accepts `review_depth` with allowed values `1`, `2`, and `3`. Missing or zero values normalize to `1`; every other value returns HTTP 422. Review depth is stored on `kb.doc_review_requests`, returned in request APIs, and reused by restarts because it is part of the immutable request intent.

The submission UI displays a three-option Review Depth control in Step 2 beside the check-level controls, defaults it to 1, includes it in the Step 4 summary, and sends it in the request body. TypeScript service types expose the field on submissions and request status objects.

Migration `20260710000001_add_doc_review_request_depth.sql` adds:

```sql
review_depth INT NOT NULL DEFAULT 1 CHECK (review_depth BETWEEN 1 AND 3)
```

Its down migration removes the column. Application validation provides a useful API error; the database constraint protects non-API writers.

### Configured output limits

The root of `doc-review.local.toml` and each `[reviewers.<aspect>]` block may contain:

```toml
max_findings = [100, 200, 300]
max_analyses = [100, 200, 300]
```

Each array maps review depths 1, 2, and 3 to limits. Built-in defaults for both settings are `[100, 200, 300]`. Root values override the built-ins, and reviewer values override the root values. The checked-in local configuration intentionally uses `[10, 20, 30]` at the root for testing.

Configured arrays must contain exactly three positive integers. Invalid arrays fail configuration loading with an error that identifies the setting and scope. Omitted arrays inherit normally. Resolution produces scalar `MaxFindings` and `MaxAnalyses` values for a specific reviewer and request depth.

An output is an analysis when `strings.EqualFold(strings.TrimSpace(finding.FindingType), "analysis")` is true. Missing, blank, and unknown finding types count as findings, as do all known non-analysis types. This conservative rule makes classification deterministic without requiring every reviewer to share a closed finding-type vocabulary. Limits apply independently to each reviewer, not to the whole review run.

## Components and Data Flow

### Request lifecycle

1. The frontend submits `review_depth`.
2. `AcceptRequest` normalizes and validates it, then stores it on `kb.doc_review_requests`.
3. `RequestStatus` and its TypeScript counterpart expose the depth. The central `loadRequest` SELECT/Scan path returns it for direct request reads, request-with-findings reads, normal execution, restart, and recovery flows.
4. `RunReview` copies the request depth to `ReviewProcessor`.
5. `buildReviewers` resolves each reviewer's effective depth-specific limits and places them in its `ReviewerConfig`.
6. The reviewer's scheduler counts normalized results as tasks finish and decides whether another work unit may start.

The run table does not duplicate review depth. Runs belong to a request, and no endpoint mutates review depth after submission. `RestartRequest` first calls `loadRequest`, creates a new run containing the existing run-scoped fields, and leaves depth on the request. The subsequent `RunReview` call reloads the same request and copies its stored depth to `ReviewProcessor`. `RecoverStalledReviews` delegates to `RestartRequest`, so it follows the same path. No restart or recovery path reconstructs depth from `kb.doc_review_runs`. Tests cover direct execution, explicit restart, and stalled-run recovery retaining a non-default depth.

### Soft-limit scheduler

A small concurrency-local `reviewWorkGate` owns the effective finding and analysis limits, synchronized completed-result counts, the scheduler's ordered phase queues, and a claimed-index set. Its interface is limited to:

- record the findings returned by one completed work unit;
- atomically claim the next index from the active phase only when neither category has reached its limit;
- return a final snapshot for logging and tests.

Both scheduler paths use incremental work claiming:

- `runReviewerConcurrent`, used by standard per-window and per-block reviewers;
- `runArtifactUnitsWindowGrouped`, used by metrics, provisions, and inventory-item reviewers.

Checking the completed counts, removing an index from the active phase queue, and adding that original input index to the claimed set occur under the same mutex in one `claimNext` operation. A worker receives an index only after that atomic operation; therefore it cannot pass a budget check, pause, and claim work after another completion reaches a limit. Work claimed before the limiting completion is already in flight and is allowed to finish, which intentionally permits bounded overshoot. Once either completed count reaches its configured limit, `claimNext` returns no work and no unclaimed unit starts.

Units withheld by the budget are not errors. They are counted as completed for progress reporting. The scheduler derives their deterministic zero-based input indexes by subtracting the claimed-index set from all original input indexes; no goroutine is created for them. This works for both naturally ordered standard work and non-contiguous artifact seed indexes.

The artifact scheduler preserves its seed, stagger, and remainder behavior with two explicit phase queues containing original unit indexes. Before any seed goroutine starts, the coordinator atomically reserves the currently eligible seed indexes through the same work gate and adds them to the claimed set; it then launches those reserved seeds concurrently. Thus all initially reserved seeds are legitimately in flight even if one finishes during the stagger. After the stagger, bounded workers claim remainder indexes one at a time through the same gate. If a seed completion has reached either limit, no remainder index is claimable. Any seed or remainder index not reserved or claimed remains in the unclaimed set and is reported as skipped. Seed concurrency can contribute to soft-limit overshoot, as allowed by the ADR.

`entitiesReviewer` uses the standard concurrent scheduler and receives the same behavior. Reviewers with a single work unit also receive the limits, although one task may itself return more than a configured maximum because results are never truncated.

### Logging

When work is withheld, emit one warning per reviewer execution rather than one line per skipped unit. To satisfy the ADR requirement to log which work was canceled, the aggregate entry contains both the count and the deterministic zero-based indexes of the withheld units. The log contains:

- `reviewer`
- `review_depth`
- `max_findings`
- `max_analyses`
- `findings`
- `analyses`
- `skipped_units`
- `skipped_unit_indexes`
- `record_id` when available through the existing review context

Normal completion without withheld work produces no new warning. The new match limit is included in the existing per-reviewer completion logs as `matches_sent_to_llm` where a reviewer already logs match counts.

## Error and Compatibility Behavior

- Existing API clients that omit `review_depth` keep depth 1 behavior.
- Existing database rows receive depth 1 through the migration default.
- Missing TOML limit settings use built-in defaults.
- Invalid TOML arrays fail configuration loading; they do not silently disable limits.
- `MAX_MATCHES_TO_LLM` uses `envInt("MAX_MATCHES_TO_LLM", 3, 1)`: unset, blank, malformed, and integer-overflow values resolve to the default `3`; parsed zero and negative values clamp to `1`; positive parsed values are used unchanged.
- Cancellation caused by the caller or pipeline stop remains an error/sentinel and takes precedence over budget-based withholding.
- The scheduler never truncates a task's returned results and never deletes persisted findings to enforce a limit.

## Testing

Backend tests cover:

- built-in, root, and reviewer config precedence for both arrays;
- rejection of malformed arrays;
- depth defaulting and rejection of values outside 1 through 3;
- request insertion and loading of review depth;
- explicit restart and stalled-run recovery retaining a non-default request depth;
- propagation of depth-specific limits into reviewer execution;
- `MAX_MATCHES_TO_LLM` payload slicing for all four artifact reviewer types while retrieval limits remain unchanged;
- removal of the hard retrieval cap of 10;
- classification of `finding_type == "analysis"` separately from other findings;
- classification of blank, differently cased, whitespace-padded, and unknown finding types;
- atomic work claiming when a completion reaches either limit, allowing only already-claimed in-flight overshoot;
- progress completion and aggregate skipped-unit logging with deterministic withheld indexes;
- artifact seed/stagger behavior under a reached budget;
- interleaved, non-contiguous artifact seed indexes where a seed reaches a limit during the stagger, no remainder starts, and the exact unclaimed original indexes are logged;
- caller cancellation remaining distinguishable from budget withholding.

Frontend tests, where the existing component test setup permits, cover the default selection and request serialization. At minimum, TypeScript checking verifies the UI and service contract, and backend handler/controller tests verify the wire contract.

Verification runs include the focused doc-review package tests, the project migration tests or migration validation path, frontend checks, `go test ./server/api/doc-reviews/...`, and an affected-project build.

## Documentation Impact

After implementation, update the source ADR to `Accepted` or `Implemented` according to repository convention and fill in Code Changes, Operational Behaviors, Database Migrations, Data Formats, Environment Variables, Consequences, Tests, and Documentation Impact. Document that `[100, 200, 300]` are built-in defaults while the local TOML's `[10, 20, 30]` values are deliberate testing overrides.

No prompt changes are required. No shared-library API changes are required. The request schema, migration history, local doc-review configuration, frontend review form, reviewer scheduling behavior, and reviewer environment-variable documentation are affected.

## Out of Scope

- Hard truncation of completed findings or analyses.
- A run-wide aggregate budget across reviewers.
- Per-package limit overrides.
- Changing match ranking or retrieval algorithms.
- Making review depth mutable after request creation.
- Adding a fourth or custom depth.
