## 1. Spike and baseline

- [x] 1.1 Measure `pdb.more_like_this` (text form, literal document) latency on the entity,
      relation and metric partitions; record whether stored keywords or more-like-this should be
      the default lexical source for reference queries
      > 2026-11-11, dev database, the engine's own reference search (ParadeDB): entity (42,888 rows)
      > 96 ms more-like-this / 98 ms stored keywords; relation (23,933) 98 / 101 ms; metric (2,400)
      > 105 ms more-like-this (no stored keywords); provision (205) 98 / 102 ms. No measurable
      > difference, so stored keywords stay first (they are what the index-time extractor chose) and
      > more-like-this is the fallback, as implemented.
- [x] 1.2 Record a golden result set before any code moves: the hybrid-search benchmark
      questions plus provision and chunk queries, through `/kb/search` and
      `SearchAgentKnowledge` (artifact ids, order, scores)
      > Recorded 2026-11-11 by `kbhandler/search_golden_live_test.go` (`SEARCH_GOLDEN_MODE=record`):
      > 65 cases (4 questions per generated-question purpose, free-text queries typed and all-types,
      > page 2, type and record filters, a provision facet), each with total, order, score and
      > snippet, in four configurations (hybrid and lexical-only, ParadeDB and Postgres). Chunks
      > have no rows in the dev database, so chunk behaviour is covered by the disposable-database
      > integration test instead. `SearchAgentKnowledge` goes through the same
      > `queryRegistrySearchResults`, so it is covered by the same lists. The golden files are
      > local (not committed): they depend on the dev data.

## 2. Engine package

> Status 2026-11-11 (second session): done. `kbhandler` no longer carries a copy of the search SQL;
> `registrySearch*` there is an adapter over `searchengine`. The engine gained what the HTTP search
> needs: `Options.Offset`, per-request weights and candidate limit (`Config`), the keyword-only
> fallback floor (`Config.MinLexicalScore`), the Chinese substring fallback and highlighted snippets
> on the Postgres backend, `Count`, and `Response.Trace`.

- [x] 2.1 Create `server/api/searchengine` with the request (`Query`, `Filters`, `Criteria`,
      `Options`) and response (`Result`, `Diagnostics`, removed-candidate counts) types of
      design §2–6; check with `go list` that it imports neither `kbhandler` nor `doc-processing`
- [x] 2.2 Move the lexical (ParadeDB and Postgres), vector (registry rows and chunk windows) and
      fusion queries, the query embedding and the keyword clause from `kbhandler` into the
      package, unchanged
      > Not "unchanged" literally: the engine already had its own queries, so the `kbhandler` copies
      > were deleted and the engine extended to give the same results (see 2.4). The query embedding
      > and keyword extraction stay in `kbhandler` and reach the engine through the hooks (2.3);
      > `withPrecomputedEmbedding` hands the benchmark's own embedding to the searches that follow.
- [x] 2.3 Add `SetKeywordExtractor`; register today's extractor (model, prompt, cache, timeout)
      from `kbhandler` at startup; make extraction unavailable when no extractor is registered
- [x] 2.4 Switch the HTTP routes, `SearchAgentKnowledge`, the effort loop and the benchmark
      worker and trace to the engine; the golden set from 1.2 must match exactly
      > Result of the comparison (`SEARCH_GOLDEN_MODE=compare`): hybrid on ParadeDB (the default),
      > 65 of 65 cases identical in order, score, snippet and total; hybrid on Postgres 65 of 65;
      > lexical-only on Postgres 65 of 65 (order). Lexical-only on ParadeDB: 58 of 65 identical in
      > order; in the other 7 (long Chinese questions) the old code returned nothing and the engine
      > returns 20 results (the old ParadeDB statement failed and fell back to Postgres full text,
      > which finds nothing for them). Intended differences in the keyword-only fallback: `score` is
      > the fused score instead of the raw BM25 / `ts_rank_cd`; the snippet is the stored text, not
      > a ParadeDB highlight; at most 200 results can be paged (it was unlimited). Traces are built
      > from the engine's lists instead of a second statement in a `REPEATABLE READ` transaction.

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
> backends) pass, including paging, counts, traces and the Chinese substring fallback. The chunk
> integration test (`kbhandler/chunk_search_integration_test.go`, disposable database) passes
> through the engine on both backends. Not done: the `search-query-keywords` scenarios on the
> engine path as separate tests, and a benchmark-page run (the golden comparison stands in).

- [ ] 4.1 Unit tests for each scenario in `specs/hybrid-search-engine/spec.md` and the modified
      scenarios in `specs/search-query-keywords/spec.md`
- [ ] 4.2 Integration test on a disposable PostgreSQL with `pg_search` and `vector` (pattern of
      `chunk_search_integration_test.go`): exclusion before ranking, required keyword removing a
      vector-only result, cosine floor on a lexical-only result
- [ ] 4.3 Live benchmark page run before and after; scores within run-to-run variance

## 5. Documentation and catalogue

- [x] 5.1 Hybrid-search capsule (`KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md`):
      engine entry point, query forms, filters, criteria, score contract and diagnostics
- [x] 5.2 Update Artifact Search (`20261111-01`): row and `module-artifact-search.md` together
      (files, interface, use_when, version 1.1.0), then run the catalogue check
- [x] 5.3 Devdoc for the search engine API (create-devdoc skill), linking this change

> Docs 2026-11-11: capsule rewritten (§2, §8, §9, §11, §13.2); Artifact Search row and
> `module-artifact-search.md` at 1.1.0 (`modules.py check`: in sync); devdoc
> `KnowledgeStore/doc-repo/devdocs/202611/2026111104-devdoc-search-engine-api.md`.
