## Context

The hybrid search lives in `kbhandler` (`search_registry.go`, `search_embedding_query.go`,
`search_query_keywords.go`). Its unexported entry `queryRegistrySearchResults` embeds the query
text, optionally asks an LLM for keywords, runs a lexical list (ParadeDB BM25 with Jieba per
artifact partition, or Postgres full text) and a vector list (pgvector HNSW, plus chunk windows
since `chunk-hybrid-search`), and fuses them:
`score = wl/(60 + lexical_rank) + ws/(60 + vector_rank)`. Its callers are the HTTP routes, the
effort-level loop, the benchmark worker and trace, and `SearchAgentKnowledge` (the only exported
function, which requires a knowledge-store scope and at most 20 results).

`kbhandler` imports `doc-processing`, so `doc-processing` cannot call the search. That is why the
index-time linking step copied it in June 2026 (`queryArtifactHybridCandidates`). The copy later
became `FindSimilarArtifactsOnTheFly`, which the four document reviewers use, and two clustering
steps still use the original. The copy has no scope filters, and its keyword half never matches.

Facts this design relies on (checked 2026-11-11 on the dev database):

| Fact | Value |
|---|---|
| ParadeDB / pgvector | `pg_search` 0.21.8, `vector` 0.8.1 |
| BM25 indexes | one per artifact partition, `key_field = artifact_id`, Jieba on the six text fields |
| `pdb.more_like_this(key_value)` | fails on text keys ("operator does not exist: text = bigint") |
| `pdb.more_like_this('{"search_document": "<text>"}')` with the text passed as a literal | works: 416_prv_24 returns itself, then 416_prv_29, 48, 52 …; ~200 ms on the provision partition (205 rows). Inside a join it fails ("Unsupported query shape") |
| `pdb.boolean(must, should, must_not)`, `pdb.boost(factor, query)` | available; no minimum-should-match |
| Stored keywords (`kb.search_artifacts.keywords`) | provisions 205/205 (avg 4), topics, summaries, scene blocks, semantic projections mostly filled; metrics, products, inventory items none |
| Stored embeddings | none for entities and relations; metrics 2,201/2,400 |

## Goals / Non-Goals

**Goals:**
- One retrieval engine that every server module can call, with the same ranking for all.
- Callers express what they want (text or a reference, filters, criteria), never how to embed.
- Exclusions and scope apply before ranking.
- A documented score that all modules read the same way, plus diagnostics.
- Existing routes and `SearchAgentKnowledge` return the same results for the same inputs.

**Non-Goals:**
- Moving the reviewers and the clustering steps onto the engine (follow-up changes), and
  removing the copy.
- New HTTP parameters or response fields.
- Effort levels for internal callers: the effort loop stays a `kbhandler` layer over the engine.
- Changing the fusion formula, the candidate limit (200 per list) or the index layout.

## Decisions

### 1. A `searchengine` package below both `kbhandler` and `doc-processing`

```
   kbhandler ──────┐        doc-reviews ──┐        doc-processing (later)
   (HTTP, agent,   │                      │        (clustering)
    effort loop)   ▼                      ▼               │
                 server/api/searchengine  ◀───────────────┘
                   │   query building, filters, criteria, fusion,
                   │   query embedding, keyword clause, diagnostics
                   ▼
                 kbsearch (registry, vectors) · appconfig · llmrun · shared llm client
```

`searchengine` imports neither `kbhandler` nor `doc-processing`. The keyword *extraction* step
needs prompt loading (`docprocessing.LoadPromptByRef`), so it is supplied as a hook:
`searchengine.SetKeywordExtractor(fn)`, registered by `kbhandler` at startup with today's
extractor (model, prompt, cache, timeout unchanged). Without a hook (for example in the
doc-processor), extraction is unavailable and the fallback rules apply.

*Alternatives:* export a function from `kbhandler` (doc-processing still could not call it, and
handler code would keep the engine); grow `kbsearch` (it would take LLM and config
dependencies it does not have today).

### 2. Query forms: text or reference, never vectors

```go
type Query struct {
    Text string        // text query: the engine embeds it
    Ref  *ArtifactRef  // reference query: {Type, ID} of an indexed artifact
}
```

Exactly one is set. For a **reference** query the engine reads the referenced row of
`kb.search_artifacts`:

- *vector:* its stored embedding; when missing, the engine embeds the stored `search_document`
  with the configured model (the same path as a text query);
- *keyword list:* the caller's keywords if given, otherwise the stored `keywords`, otherwise
  `pdb.more_like_this` on the stored `search_document` (ParadeDB, text passed as a literal), or
  the most frequent search-document terms on the Postgres backend;
- the referenced artifact itself is always excluded from the results.

Vectors are not part of the request: the engine owns the model, dimension and length cap, and a
vector from another model would silently return wrong results.

### 3. Filters apply to both lists before fusion

```go
type Filters struct {
    ArtifactTypes                     []string
    RecordIDs, ExcludeRecordIDs       []int64
    ExcludeArtifactIDs                []string
    KnowledgeStoreID, DocumentGroup   string
    Structural                        StructuralFilters // today's category/type/facet filters
}
```

Every filter is added to the lexical list (each BM25 partition branch, or the full-text query)
and to the vector list (registry rows and chunk windows) before ranking, extending today's
structural-filter rule to exclusions. This removes the crowding seen in the reviewers, where a
document's own provisions took the top slots and were dropped afterwards.

### 4. Criteria: weights rank, everything else filters

```go
type Keyword struct { Term string; Weight float64; Required, Excluded bool }
type Criteria struct {
    Keywords   []Keyword
    MinMatched int     // at least N of the non-excluded keywords
    MinCosine  float64 // 0 = no floor
}
```

| Criterion | Kind | Applies to | Implementation |
|---|---|---|---|
| `Weight` (default 1.0) | scoring | lexical list | ParadeDB: `pdb.boost(weight, …)` per keyword; Postgres: weighted sum of per-keyword `ts_rank_cd` |
| `Required` | filter | every result | lexical list: `pdb.boolean(must => …)`; vector-only results: a match check on the fused candidates |
| `Excluded` | filter | every result | `must_not`, and the same check |
| `MinMatched` | filter | every result | per-candidate count of matched keywords over the fused candidates (no native minimum-should-match in 0.21.8) |
| `MinCosine` | filter | every result | cosine computed for every fused candidate, including lexical-only ones (vectors are stored); for a chunk, its best window |

Filter criteria are evaluated on the fused candidate set (at most 2 × 200 rows per partition
branch) with `artifact_id = ANY($ids)`, so their cost is bounded by the candidate count, not
the corpus. Keyword matching uses the same rule as the BM25 clause: every token of the keyword
occurs in one indexed text field.

### 5. LLM steps per request

`Options.KeywordExtraction` is `default` (today's `SEARCH_QUERY_KEYWORDS` switch), `off` or `on`
(still requires the switch and a registered extractor). Caller-supplied keywords always skip
extraction. Each keyword call keeps the search's run ID (`llm_run_id`) and call location
`MID-20261024-01`; query embedding keeps `MID-20261024-02`.

### 6. Score contract and diagnostics

```go
type Result struct {
    ArtifactType, ArtifactID string; RecordID int64
    Rank  int      // 1-based position in this response
    Score float64  // fused score, see below
    Diag  Diagnostics // LexicalRank, LexicalScore, VectorRank, Cosine, MatchedKeywords
    // display fields as today: labels, snippet, source title/filename, spans, payload, keywords
}
```

The contract, written into the `hybrid-search-engine` spec and the hybrid-search capsule:

- `Score = wl/(60 + lexical_rank) + ws/(60 + vector_rank)`; a missing rank contributes 0; `wl`,
  `ws` come from `[artifact_search]`.
- It orders the results of **one** response. It is not comparable across queries, and it is not
  a relevance probability: the top result of a query with nothing relevant still scores
  `wl/61 + ws/61` (about 0.033 at 1.0/1.0).
- Absolute questions ("is anything similar enough?") use `MinCosine` or `Diag.Cosine`.
- `LexicalScore` is BM25 (ParadeDB) or `ts_rank_cd` (Postgres): comparable within one backend and
  one partition only.

### 7. Same results for existing callers

The routes, `SearchAgentKnowledge`, the effort loop and the benchmark map their inputs onto the
engine without criteria, so the SQL they produce is unchanged. A golden test records today's
result lists for a fixed question set (benchmark questions plus provision and chunk queries) and
compares them after the move.

## Reused modules

- **Artifact Search (`20261111-01`), extended.** Its retrieval files (`search_registry.go`,
  `search_embedding_query.go`, the keyword clause) move into `searchengine`; its registry,
  embedding and chunk-window code in `kbsearch` and `doc-processing` is reused unchanged. Its
  row and doc are updated (files, interface, version 1.1.0).
- No other catalogued module applies. Compact Document Lines (`20261110-01`) serializes LLM
  input and is not involved.
- `FindSimilarArtifactsOnTheFly` is not extended: it is the copy this change makes unnecessary.

## Risks / Trade-offs

- [`pdb.more_like_this` by key is broken in 0.21.8 and the text form fails inside joins] → the
  engine reads the referenced text first and passes it as a literal; measure latency on the
  largest partitions (entity, relation: 24k–43k rows) before relying on it, with stored
  keywords as the first choice.
- [Filter criteria checked on fused candidates can return fewer than `top_k` results] →
  documented; callers widen `top_k` or loosen criteria; the response says how many candidates
  were removed by each criterion.
- [The extractor hook is process-global state] → set once at startup; tests set and reset it.
- [Moving code changes behaviour for existing callers] → golden results test (decision 7) and
  the live benchmark page before and after.
- [Entities and relations have no stored vectors] → reference queries on them embed the stored
  text at query time (one embedding call each), the same cost the copy has today.

## Migration Plan

No schema change. Ship the package and the moved code together; run the golden test and a
benchmark run. Rollback: revert the commit; no data changes.

## Open Questions

- Should `/kb/search` expose `Diag` (for example behind `explain=1`) for the benchmark and
  trace pages? Not in this change.
- Default `top_k` for internal callers: proposed 20, matching today's agent search cap.
