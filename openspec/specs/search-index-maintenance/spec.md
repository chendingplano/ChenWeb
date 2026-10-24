# search-index-maintenance Specification

## Purpose
Keep the ParadeDB BM25 indexes of the kb.search_artifacts partitions compact: a scheduled job
rebuilds, concurrently, any index left with many documents in pg_search mutable segments or many
deleted documents, which otherwise make every BM25 lookup slow. Created by archiving change
bm25-index-maintenance (2026-10-24).
## Requirements
### Requirement: Scheduled BM25 index maintenance
The system SHALL provide a scheduled job type `maintain_bm25_indexes` that inspects every BM25 index
on the `kb.search_artifacts_<type>` partitions and rebuilds with `REINDEX INDEX CONCURRENTLY` each
index whose documents in mutable segments exceed `max_mutable_docs` (default 500) or whose deleted
documents exceed `max_deleted_ratio` (default 0.5) of its live plus deleted documents.

#### Scenario: Index with many mutable documents
- **WHEN** the summary partition's BM25 index holds 789 documents in a mutable segment and the job runs with default parameters
- **THEN** the job rebuilds that index concurrently and records it as rebuilt with its document counts and duration

#### Scenario: Compact index
- **WHEN** an index has 300 mutable documents and 10% deleted documents
- **THEN** the job leaves it unchanged and records it as skipped

#### Scenario: New partition
- **WHEN** a new `kb.search_artifacts_<type>` partition with a BM25 index exists
- **THEN** the job inspects it without any code change

### Requirement: Safe, bounded runs
The job SHALL NOT block searches, SHALL rebuild at most `limit` indexes per run (default 11), SHALL
continue with the remaining indexes when one rebuild fails, and SHALL fail the run only when it
cannot list the indexes. With `dry_run` true it SHALL report the decisions without rebuilding.
Before inspecting, it SHALL drop invalid indexes on the partitions left by an interrupted
concurrent rebuild (names containing `_ccnew`).

#### Scenario: Dry run
- **WHEN** the job runs with `dry_run` true
- **THEN** no index is rebuilt or dropped and the result lists each index with the action it would take

#### Scenario: One rebuild fails
- **WHEN** rebuilding one index returns an error
- **THEN** the job records that index as failed with the error and continues with the others

#### Scenario: Leftover from an interrupted rebuild
- **WHEN** an invalid index `kb_search_artifacts_metric_bm25_ccnew` exists
- **THEN** the job drops it concurrently before inspecting and records the drop

### Requirement: Default hourly schedule
A migration SHALL create one enabled schedule for `maintain_bm25_indexes` running every 3,600
seconds with the default parameters, only when no schedule for that job type exists.

#### Scenario: Fresh database
- **WHEN** the migration runs on a database without a `maintain_bm25_indexes` schedule
- **THEN** one hourly, enabled schedule is created

#### Scenario: Existing schedule
- **WHEN** a `maintain_bm25_indexes` schedule already exists
- **THEN** the migration does not add another

