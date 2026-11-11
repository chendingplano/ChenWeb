## 1. Storage and source indexing
- [x] 1.1 Add failing tests for chunk row content, strict artifact loading, window coverage and transactional replacement.
- [x] 1.2 Add goose partition/window schema, indexes and input-change invalidation; verify migration up/down/up.
- [x] 1.3 Implement chunk row building, window embedding and atomic persistence.

## 2. Retrieval and lifecycle
- [x] 2.1 Add failing tests for chunk type acceptance, semantic window grouping and backfill embedding coverage.
- [x] 2.2 Extend registry semantic candidates, existing embedding backfill and chunk routes.
- [x] 2.3 Index after chunking and invalidate with stale source artifacts; test success/failure paths.
- [x] 2.4 Implement and test bounded cursor-based saved-chunk backfill through the centralized pool.

## 3. Documentation and validation
- [x] 3.1 Update both requested documents and write the implementation devdoc.
- [x] 3.2 Register Artifact Search reuse with its module doc/row; run catalogue consistency check.
- [x] 3.3 Run affected tests, vet/build and OpenSpec validation; review changes and fix findings.
- [x] 3.4 Commit only task-owned paths with jj in ChenWeb and KnowledgeStore and confirm linear history.

## Validation record

- Real PostgreSQL/ParadeDB/pgvector integration: migration up/down/up, long-tail lexical and semantic retrieval, window deduplication, source filters, embedding failure/recovery, generation fencing, source-mutation serialization and two-connection-pool concurrency.
- Focused registry/search/chunk/embedding tests, affected-package vet and server build; OpenSpec strict validation and reusable-module catalogue consistency.
- Broader package suite has existing failures in the metrics-reader guard, input-delete path refusal tests and metric-score parity/runner tests. Reproduced all those failures in a temporary checkout of unchanged baseline `76c2fc1a544e`; they are outside this change.
- Review caught source-write races and lock-pool starvation; both were fixed and independently reviewed. Lock admission preserves half the bounded pool; one-connection configurations fail immediately.
- Existing corpus-wide backfill/model calls were not run. The migration is applied by normal local development-server reload; historical saved chunks can be indexed with the new backfill API.
