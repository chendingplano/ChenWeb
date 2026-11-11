## ADDED Requirements

### Requirement: Engine entry point for server modules

The system SHALL provide hybrid search through a Go package, `server/api/searchengine`, that imports neither `kbhandler` nor `doc-processing`, and the HTTP search routes, the agent knowledge search, the effort-level loop and the search benchmark SHALL retrieve through that package.

#### Scenario: A server module searches
- **WHEN** a module in `doc-reviews` needs artifacts relevant to a query
- **THEN** it calls the `searchengine` entry point and receives ranked results, without an HTTP request

#### Scenario: Existing routes keep their results
- **WHEN** `GET /api/v1/kb/search` runs a query with the same parameters before and after this change
- **THEN** it returns the same artifacts in the same order with the same scores

### Requirement: Text and reference queries

A search request SHALL carry exactly one of a query text or a reference `(artifact_type, artifact_id)` to an artifact in `kb.search_artifacts`, and SHALL NOT carry a query vector; the engine SHALL compute or look up every embedding itself.

#### Scenario: Text query
- **WHEN** a request has the query text "thermal insulation of the cab"
- **THEN** the engine embeds the text with the configured embedding model and uses it for the vector list

#### Scenario: Reference query uses stored data
- **WHEN** a request references `(provision, 416_prv_24)` and that row has an embedding and keywords
- **THEN** the vector list uses the stored embedding and the lexical list uses the stored keywords, with no embedding or LLM call

#### Scenario: Reference without stored embedding
- **WHEN** a request references an entity whose row has no embedding
- **THEN** the engine embeds the row's stored search document and uses that vector

#### Scenario: Reference without stored keywords
- **WHEN** a request references a metric whose row has no keywords and the caller supplies none
- **THEN** the lexical list uses terms selected from the row's stored search document (ParadeDB more-like-this, or the most frequent terms on the Postgres backend)

#### Scenario: Referenced artifact excluded
- **WHEN** a request references `(provision, 416_prv_24)`
- **THEN** `416_prv_24` does not appear in the results

#### Scenario: Unknown reference
- **WHEN** the referenced artifact has no row in `kb.search_artifacts`
- **THEN** the engine returns an error naming the reference instead of an empty result

#### Scenario: Both or neither query form
- **WHEN** a request sets both a query text and a reference, or neither
- **THEN** the engine rejects the request

### Requirement: Filters apply before ranking

The engine SHALL apply every filter (artifact types, included and excluded record ids, excluded artifact ids, knowledge store, document group and today's structural filters) to both the lexical list and the vector list before fusion.

#### Scenario: Excluding the document under review
- **WHEN** a reference query for a provision of record 416 excludes record 416 and asks for 10 results
- **THEN** no result belongs to record 416 and up to 10 results from other records are returned, even if record 416 holds the most similar provisions

#### Scenario: Scope
- **WHEN** a request is limited to knowledge store `ks-1`
- **THEN** no result comes from a document outside `ks-1`, in either list

### Requirement: Keyword criteria

A request SHALL be able to supply keywords, each with a weight (default 1.0) and optional required or excluded flags, and a minimum number of matched keywords; weights SHALL change only the lexical ranking, while required, excluded and minimum-matched SHALL remove results from the final list whichever list found them.

#### Scenario: Weighted keyword
- **WHEN** keywords are "GB 16889" (weight 3) and "填埋" (weight 1)
- **THEN** among lexical matches, artifacts matching "GB 16889" rank above artifacts matching only "填埋", all else equal

#### Scenario: Required keyword removes a vector-only result
- **WHEN** "GB 16889" is required and an artifact is found only by the vector list and does not contain "GB 16889"
- **THEN** that artifact is not in the results

#### Scenario: Excluded keyword
- **WHEN** "医疗垃圾" is excluded
- **THEN** no result contains "医疗垃圾" in its indexed text fields

#### Scenario: At least N keywords
- **WHEN** four keywords are supplied with a minimum of 2 matched
- **THEN** every result matches at least two of them, where a keyword matches when all its tokens occur in one indexed text field

#### Scenario: Caller keywords skip extraction
- **WHEN** a request supplies keywords
- **THEN** no query-keyword LLM call is made for it

### Requirement: Minimum cosine similarity

A request SHALL be able to set a minimum cosine similarity, and the engine SHALL remove every result whose cosine similarity to the query vector is below it, including results found only by the lexical list; for a chunk the similarity SHALL be that of its best embedding window.

#### Scenario: Lexical-only result below the floor
- **WHEN** the minimum is 0.75 and an artifact found only by BM25 has cosine 0.42 to the query vector
- **THEN** that artifact is not in the results

#### Scenario: No floor
- **WHEN** the minimum is not set
- **THEN** no result is removed for its cosine similarity

### Requirement: Score contract and diagnostics

The engine SHALL return for each result its 1-based rank, its fused score `wl/(60 + lexical_rank) + ws/(60 + vector_rank)` (a missing rank contributing 0, `wl` and `ws` from `[artifact_search]`), and diagnostics (lexical rank, lexical score, vector rank, cosine similarity, matched keywords), and the spec and the hybrid-search capsule SHALL document that the fused score orders one response only and is not comparable across queries.

#### Scenario: Result found by both lists
- **WHEN** an artifact is 1st in the lexical list and 3rd in the vector list with weights 1.0 and 1.0
- **THEN** its score is 1/61 + 1/63 and its diagnostics show lexical rank 1 and vector rank 3

#### Scenario: Result found by one list
- **WHEN** an artifact appears only in the vector list
- **THEN** its lexical rank and lexical score are absent and its score comes from the vector rank alone

#### Scenario: Removed candidates reported
- **WHEN** filter criteria remove candidates after fusion
- **THEN** the response states how many candidates each criterion removed
