## Why

ChenWeb's hybrid search (BM25 + vector similarity, fused by reciprocal rank) can only be called
through its HTTP handlers and one agent function locked to a knowledge store. Modules that need
retrieval inside the server cannot use it, so they carry a private copy:
`docprocessing.FindSimilarArtifactsOnTheFly` / `queryArtifactHybridCandidates`, used by the four
document reviewers (metrics, provisions, inventory items, entities) and by two index-time
clustering steps. The copy dates from June 2026 and missed every later improvement: BM25 with
the Jieba tokenizer, query keywords, partition-aware BM25. Its keyword half never matches,
because it requires every word of a 50–400-word query (0 of 205 provisions found a single
candidate on 2026-11-10). It also ignores knowledge-store and document-group scope.

A provision reviewer looking for similar provisions is an ordinary search request. The engine
should serve it, and it should offer what such callers need and the engine lacks today:
- a query that refers to an artifact the engine has already indexed;
- filters that exclude records;
- criteria on keywords and on similarity;
- a documented meaning for its scores.

## What Changes

- **Engine entry point.** The retrieval core moves out of `kbhandler` into a new package,
  `server/api/searchengine`, which imports neither `kbhandler` nor `doc-processing`, so every
  server module can call it. The HTTP routes and `SearchAgentKnowledge` call the same entry
  point and keep their current results.
- **Two query forms.** A *text* query, which the engine embeds (as today), and a *reference*
  query, `(artifact_type, artifact_id)`, for which the engine uses its own stored text,
  embedding and keywords. Callers never pass vectors; embedding stays inside the engine.
- **Filters before ranking.** Today's scope and structural filters, plus exclusions (records,
  artifact ids). Every filter applies to the keyword list and the vector list before fusion,
  so excluded artifacts do not take result slots.
- **Caller criteria.** Keywords with weights; required, excluded and "at least N of these"
  keywords; a minimum cosine similarity. Weights change ranking; the other criteria remove
  results from the final list, whichever list found them.
- **LLM steps per request.** Query-keyword extraction can be switched off per request and is
  skipped when the caller supplies keywords. Effort levels stay an HTTP-route option.
- **Score contract.** The engine documents its fused score and how to read it, and returns
  per-result diagnostics: keyword rank and score, vector rank, cosine similarity, matched
  keywords.
- **Out of scope (follow-up changes):** moving the reviewers and the clustering steps onto the
  engine, then removing `FindSimilarArtifactsOnTheFly` and `queryArtifactHybridCandidates`. No
  HTTP parameter changes.

## Capabilities

### New Capabilities
- `hybrid-search-engine`: the engine's callable contract: entry point, query forms, filters,
  keyword and similarity criteria, scores and diagnostics.

### Modified Capabilities
- `search-query-keywords`: extraction is skipped when a request supplies keywords or turns it
  off; keywords carry weights in the BM25 clause; the raw-query fallback applies to text
  queries, while reference queries fall back to terms selected from the referenced artifact.

## Impact

- Code: new `ChenWeb/server/api/searchengine`. Moved from `kbhandler`: `search_registry.go`
  (lexical, semantic and fused queries), `search_embedding_query.go`, the keyword clause of
  `search_query_keywords.go`. Updated callers in `kbhandler`: `summary_search_handler.go`,
  `agent_search.go`, `search_effort.go`, the benchmark worker and trace.
- No schema change: the BM25, HNSW and full-text indexes already exist.
- Docs: `KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md` (score contract,
  entry point) and module doc `module-artifact-search.md`.
- Reusable modules: the catalogue (capability `search-retrieval`, `search hybrid search`) lists
  Artifact Search (`20261111-01`); this change extends it and updates its row and doc. Compact
  Document Lines (`20261110-01`) is unrelated.
- IDs: each search keeps taking its run ID from the existing shared sequence `llm_run_id`
  (`llmrun`, through `shared/go/api/idmanager`); no new sequence.
