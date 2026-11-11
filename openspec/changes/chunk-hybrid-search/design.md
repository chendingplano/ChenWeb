## Context

The user approved extending the existing registry-based BM25/pgvector/RRF engine. Chunking currently persists ranges and run metadata only. Saved chunks and the finalized canonical line file remain authoritative.

## Goals / Non-Goals

Provide lexical and semantic source-passage retrieval, backfill and safe replacement. No new search engine, rechunking algorithm, generated summaries, frontend or graph traversal.

## Decisions

1. Add `kb.search_artifacts_chunk`, using the same FTS, record, Jieba BM25 and cosine HNSW index conventions as existing partitions. Store one row per chunk with existing connection ID, compact text, title/filename, page-aware regular spans and separate overlap spans in the semantic payload. Retain overlap text for context.
2. Split embedding input into at most 6,000-rune windows with 300-rune overlap. The first vector stays in the registry; additional vectors and exact input text go to `kb.chunk_search_windows`, cascading from their parent registry row. Batch through the existing embedder. If any window fails, leave the whole chunk lexical-only so retrying the standard backfill repairs it. This preserves every part of a long table without changing saved chunks. Alternatives: averaging vectors weakens precise matches; separate registry results per window duplicate chunks and alter public IDs.
3. Semantic retrieval merges bounded index-shaped nearest-neighbor scans over registry vectors and extra chunk windows, groups by parent chunk using minimum distance, then reranks and caps the semantic list before existing RRF. Apply identical structural filters to both scans. Existing trace CTEs see the deduplicated list. Each branch has the configured candidate budget; repeated windows can reduce distinct recall, an ordinary bounded ANN trade-off.
4. Serialize fixed-size chunking, static source analysis and saved-chunk backfill with a per-record session advisory lock, spanning file reads, writes and model calls. Capture `kb.chunk_search_state.generation` under the lock before loading records/files; recheck it in the publication transaction, and increment on replacement or invalidation. Buffered source lines must match the current canonical file. Nonblocking lock attempts release connections while waiting; bounded admission reserves at most half the configured pool for holders, and one-connection pools fail immediately instead of deadlocking. Input-change triggers still use the generation check to reject obsolete metadata snapshots. Chunking reports indexing persistence failures as operation failures. Canonical static-analysis rewrites invalidate rows and stale `.chunks` before writing. Out-of-band file edits require rechunking/backfill and are not detected on every search.
5. `POST /api/v1/kb/search/backfill-chunks` accepts optional `input_record_id`, `after_record_id` and bounded `limit`; it returns counts, per-record failures and the next cursor. Read saved chunk artifacts strictly, reject absent source line references, and never rebuild boundaries. Use centralized pools; no standalone tool or new connection pool. Existing embedding-only backfill becomes window-aware for chunks and does not create registry rows.

## Reused modules

- Compact Document Lines (`20261110-01`): `RenderLinesCompact` for searchable source text. No module API changes required.
- Existing uncatalogued registry/embedding/RRF implementation: extend in place, register as Artifact Search. Chunk-specific lifecycle helpers stay in doc-processing.

## Risks / Trade-offs

- Additional window storage and query branch cost: only oversized chunks need extra windows; ordinary chunk vectors keep their existing representation.
- File/database writes cannot be one transaction: invalidate stale rows before source changes; failures remain visible and rerunnable.
- Backfill may encounter stale/missing files: report errors and advance the record cursor, preserving failures for explicit retry.
- Existing lexical-only totals can undercount semantic matches; unchanged and documented.

## Migration Plan

Apply the project migration before running new binaries (normal startup runs goose). Test up/down/up in an isolated schema/database. Backfill saved chunk rows in cursor batches, then check lexical and semantic retrieval and window deduplication. Roll back application code before dropping new search objects. No automatic corpus-wide model calls during migration.
