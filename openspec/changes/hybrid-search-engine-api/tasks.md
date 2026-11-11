## 1. Spike and baseline

- [ ] 1.1 Measure `pdb.more_like_this` (text form, literal document) latency on the entity,
      relation and metric partitions; record whether stored keywords or more-like-this should be
      the default lexical source for reference queries
- [ ] 1.2 Record a golden result set before any code moves: the hybrid-search benchmark
      questions plus provision and chunk queries, through `/kb/search` and
      `SearchAgentKnowledge` (artifact ids, order, scores)

## 2. Engine package

> Status 2026-11-11: the package exists and is the new code path (written beside, not moved out of,
> `kbhandler`). 2.2 and 2.4 (move `kbhandler`'s routes onto it, then delete the duplicate code)
> are open: `kbhandler/hybrid_search_benchmark*.go` was being edited in the same working copy, and
> the move needs the golden set of 1.2 first. Until then `kbhandler` and the engine carry two
> copies of the SQL. The provisions reviewer already uses the engine (`review-provisions.go`).

- [x] 2.1 Create `server/api/searchengine` with the request (`Query`, `Filters`, `Criteria`,
      `Options`) and response (`Result`, `Diagnostics`, removed-candidate counts) types of
      design §2–6; check with `go list` that it imports neither `kbhandler` nor `doc-processing`
- [ ] 2.2 Move the lexical (ParadeDB and Postgres), vector (registry rows and chunk windows) and
      fusion queries, the query embedding and the keyword clause from `kbhandler` into the
      package, unchanged
- [x] 2.3 Add `SetKeywordExtractor`; register today's extractor (model, prompt, cache, timeout)
      from `kbhandler` at startup; make extraction unavailable when no extractor is registered
- [ ] 2.4 Switch the HTTP routes, `SearchAgentKnowledge`, the effort loop and the benchmark
      worker and trace to the engine; the golden set from 1.2 must match exactly

## 3. New capabilities

- [x] 3.1 Reference queries: stored embedding (or embed the stored text), stored keywords (or
      more-like-this / most frequent terms), self-exclusion, error for an unknown reference,
      rejection of both or neither query form
- [x] 3.2 Exclusion filters (records, artifact ids) applied in every BM25 partition branch, the
      full-text query, the registry vector scan and the chunk-window scan
- [x] 3.3 Keyword weights: `pdb.boost` per keyword (ParadeDB), weighted sum of per-keyword
      `ts_rank_cd` (Postgres)
- [x] 3.4 Required, excluded and minimum-matched keywords, and minimum cosine, evaluated on the
      fused candidates; cosine for lexical-only candidates and best chunk window; counts of
      removed candidates per criterion
- [x] 3.5 Per-request keyword extraction switch; supplied keywords skip extraction
- [x] 3.6 Diagnostics on every result: lexical rank and score, vector rank, cosine, matched
      keywords

## 4. Tests

> Status 2026-11-11: `searchengine/engine_test.go` (unit) and `engine_integration_test.go`
> (read-only, needs `SEARCHENGINE_TEST_DATABASE_URL`; run against the dev database `miner`, both
> backends) pass. Not done: the `search-query-keywords` scenarios on the engine path, a disposable
> database, and the benchmark runs.

- [ ] 4.1 Unit tests for each scenario in `specs/hybrid-search-engine/spec.md` and the modified
      scenarios in `specs/search-query-keywords/spec.md`
- [ ] 4.2 Integration test on a disposable PostgreSQL with `pg_search` and `vector` (pattern of
      `chunk_search_integration_test.go`): exclusion before ranking, required keyword removing a
      vector-only result, cosine floor on a lexical-only result
- [ ] 4.3 Live benchmark page run before and after; scores within run-to-run variance

## 5. Documentation and catalogue

- [ ] 5.1 Hybrid-search capsule (`KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md`):
      engine entry point, query forms, filters, criteria, score contract and diagnostics
- [ ] 5.2 Update Artifact Search (`20261111-01`): row and `module-artifact-search.md` together
      (files, interface, use_when, version 1.1.0), then run the catalogue check
- [ ] 5.3 Devdoc for the search engine API (create-devdoc skill), linking this change
