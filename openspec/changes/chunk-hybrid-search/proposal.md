## Why

Hybrid search indexes extracted artifacts but not their original source chunks. Users need to retrieve passages even when topic or summary extraction has omitted relevant details.

## What Changes

- Index saved chunks as a twelfth artifact type, with original text, source spans and existing positional chunk identifiers.
- Support chunks in global hybrid search and a dedicated chunk search route.
- Add overlapping embedding windows for oversized chunks; return each chunk only once.
- Replace chunk indexes transactionally on rechunking, invalidate them with stale chunk artifacts, and backfill saved chunks through an authenticated API.
- Correct stale configuration and SQL details in the chunking and hybrid-search documents.

## Capabilities

### New Capabilities
- `chunk-hybrid-search`: searchable source chunks, complete long-passage semantic coverage, lifecycle and backfill.

### Modified Capabilities
None. Existing artifact types retain their retrieval behavior.

## Impact

ChenWeb doc-processing, kbsearch, kbhandler, routes and a project goose migration. No shared-library dependency or frontend changes. Search catalogue queries for `chunk search embedding` and `hybrid registry embedding` found Compact Document Lines (`20261110-01`), reused for compact rendering; its existing saved-chunk loader supplies the parsing implementation but search rejects missing artifacts. Existing registry insertion, embedding, RRF and backfill code is extended and registered as a reusable search module. No new allocated IDs: existing `<record>_chk_<sequence>` names are positional addresses, matching connection endpoints; embedding window numbers are positions within their parent chunk.
