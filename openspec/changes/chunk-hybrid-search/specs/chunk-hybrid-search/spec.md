## ADDED Requirements

### Requirement: Search original chunks
The system SHALL index one registry artifact per saved chunk using the existing positional chunk connection identifier, original compact source text, document metadata and page-aware source spans, retaining overlap spans separately.

#### Scenario: Chunk filter
- **WHEN** a client requests `/kb/search?artifact_types=chunk&q=...` or `/kb/chunks/search?q=...`
- **THEN** it receives chunk artifacts through the existing lexical/hybrid engine with source citations

### Requirement: Cover long chunks without duplicate results
The system SHALL embed every part of oversized chunk text in bounded overlapping windows and rank each chunk once by its best window distance before RRF.

#### Scenario: Relevant text after the first window
- **WHEN** a long protected table contains relevant text beyond its first 6,000 runes
- **THEN** an embedding window includes that text and its match can retrieve the parent chunk once

#### Scenario: Unavailable embedding
- **WHEN** embedding any window fails
- **THEN** the chunk remains searchable lexically and its missing embedding can be retried by embedding backfill

### Requirement: Maintain chunk search lifecycle
The system SHALL replace a record's chunk search rows and vectors atomically, remove obsolete rows on rechunking, and invalidate them when the supported pipeline invalidates saved chunks or input paths change.

#### Scenario: Replacement insert fails
- **WHEN** a replacement fails during database insertion
- **THEN** the prior index remains intact through transaction rollback and the operation reports failure

#### Scenario: Source rewrite
- **WHEN** static analysis invalidates saved chunks
- **THEN** their search rows and additional embeddings are removed

### Requirement: Backfill persisted chunk boundaries
The system SHALL provide an authenticated cursor-based backfill endpoint using the centralized database pool, finalized source lines and existing `.chunks` files, with per-record outcomes and no reconstruction fallback.

#### Scenario: Missing or stale chunks
- **WHEN** a record has no saved chunk file or references absent canonical lines
- **THEN** backfill reports that record as failed without inventing chunk boundaries

#### Scenario: Repeat backfill
- **WHEN** a record is backfilled again
- **THEN** its search rows and embedding windows are replaced idempotently
