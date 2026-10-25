## ADDED Requirements

### Requirement: Queries of a benchmark question
For every benchmark result the system SHALL store each query text the question searched in
`testbed.hybrid_search_benchmark_queries`: the question as written (source `question`, seq 1)
and, for medium/high runs, every query the effort loop searched (source `effort`, with its attempt
number) in the order searched. Each query row SHALL store the BM25 keywords that query used.
Deleting a result or its run SHALL delete its queries.

#### Scenario: Low-effort question
- **WHEN** a low-effort run processes a question
- **THEN** the result has exactly one query row, source `question`, with the question text and its
  keywords

#### Scenario: Effort queries
- **WHEN** a high-effort run's loop searches two queries in attempt 1 and one in attempt 2
- **THEN** the result has four query rows: the question, then the three effort queries with
  attempts 1, 1 and 2

#### Scenario: Effort query without a trace
- **WHEN** an effort query's search falls back to lexical-only or fails
- **THEN** the query still has a row, with an error message and no candidate rows

### Requirement: Candidate lists per query
For each query row whose search ran the hybrid query, the system SHALL store in
`testbed.hybrid_search_benchmark_candidates` every candidate of each list the query built, each
row referencing the query row and carrying its stage, partition, artifact type, artifact ID, rank
within the stage and score:
- `lexical_partition`: each partition's BM25 candidates, score = BM25 score, rank within that
  partition (ParadeDB backend only);
- `lexical`: the merged lexical list, score = BM25 score (`ts_rank_cd` on the Postgres backend),
  with two ranks: the rank in the merged list and the rank within its partition (NULL on the
  Postgres backend);
- `semantic`: score = cosine similarity (1 − cosine distance), rank = semantic rank;
- `fused`: score = RRF score, rank = final order, plus the candidate's lexical and semantic ranks
  (NULL when absent from that list).

#### Scenario: Target traced through all lists
- **WHEN** a question's target is 4th in BM25 and 2nd in semantic search
- **THEN** its `lexical` row has rank 4 and its BM25 score, its `semantic` row has rank 2 and its
  similarity, and its `fused` row has lexical rank 4, semantic rank 2 and an RRF score equal to
  `w_l/(60+4) + w_s/(60+2)`

#### Scenario: Several partitions
- **WHEN** a traced search reads two partitions
- **THEN** `lexical_partition` rows rank each partition's candidates separately, and each `lexical`
  row names the partition it came from, its rank in that partition and its rank in the merged list

#### Scenario: Merged and partition ranks differ
- **WHEN** a candidate is 1st in `kb.search_artifacts_product` and 3rd after the merge
- **THEN** its `lexical` row has merged rank 3 and partition rank 1, and its `lexical_partition`
  row has rank 1

#### Scenario: Postgres backend
- **WHEN** the lexical backend is `postgres`
- **THEN** no `lexical_partition` rows are stored, `lexical` rows have no partition rank, and their
  scores are `ts_rank_cd` values

### Requirement: Trace matches the live search
The trace SHALL be produced from the same SQL as the live hybrid search, in the same database
snapshot as the search whose ranks are stored. The hybrid-mode search of each question and every
effort-loop search SHALL be traced; the lexical-only and semantic-only mode searches SHALL NOT. A
question's stored lexical, semantic and hybrid ranks SHALL equal its target's rank in the
question query's `lexical`, `semantic` and `fused` lists. When the fused list's order differs from
the search results, the query row SHALL record that the trace differs.

#### Scenario: Ranks agree with the trace
- **WHEN** a question's result has `hybrid_rank = 7`
- **THEN** the target's `fused` row for that question's `question` query has rank 7

#### Scenario: Live search unchanged
- **WHEN** the search API or the agent searches without a benchmark
- **THEN** no trace statement runs and the results are the same as before this change

### Requirement: Timings exclude the trace
Stored query timings SHALL exclude the time of the trace statements: `hybrid_ms` SHALL cover the
hybrid search only, and `effort_ms` the effort loop without its trace statements.

#### Scenario: Traced hybrid search
- **WHEN** a question's hybrid search takes 40 ms and its trace statement 35 ms
- **THEN** its `hybrid_ms` is about 40

### Requirement: Trace API
`GET /api/v1/kb/hybrid-search-benchmark/results/:id/trace` SHALL return, for a system
administrator, the result's target and its queries in order, each with its keywords, error and the
four lists: the first `limit` rows per list (default 20, 1–1000) plus the target's row when it
ranks lower, with each candidate's label when the artifact still exists.

#### Scenario: Trace returned
- **WHEN** an administrator requests the trace of a traced result
- **THEN** the response lists its queries with up to 20 rows per list and the target's rows

#### Scenario: Result from an earlier run
- **WHEN** the result was stored before tracing existed
- **THEN** the response has an empty query list

#### Scenario: Unknown result or bad limit
- **WHEN** the result does not exist, or `limit` is outside 1–1000
- **THEN** the request is rejected with HTTP 404 or 400
