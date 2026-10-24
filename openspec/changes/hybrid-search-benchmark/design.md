## Context

Search over `kb.search_artifacts` fuses a lexical list (ParadeDB BM25 by default, optionally on LLM
query keywords) and a semantic list (pgvector cosine) with weighted RRF
(spec `KnowledgeStore/doc-repo/specs/202606/2026060201-spec-hybrid-search.md`, code
`server/api/kbhandler/search_registry.go`). The Question Generator stores known-item questions in
`testbed.generated_questions`; a `search_metrics` question has a `metric_id` that equals the
`artifact_id` of its row in `kb.search_artifacts_metric`. Question generation already runs as a
background goroutine with a run row in `testbed`, an `expires_at` deadline and page polling; this
change copies that pattern.

## Goals / Non-Goals

**Goals:**
- Measure the live metric search on the stored questions: lexical, semantic and hybrid, as the
  search API runs them today.
- Keep every run and every question's ranks in `testbed`, with the settings that produced them.

**Non-Goals:**
- Purposes other than `search_metrics`, other artifact types, or record-scoped search.
- Changing search behaviour or tuning weights from the page.
- Near-duplicate credit (the offline benchmark counted cosine ≥ 0.95 neighbours as hits); only the
  exact target metric counts.

## Decisions

**One query function for all three modes.** Each mode calls the backend's existing hybrid query
(`queryHybridSearchResultsParadeDB`, or `queryHybridSearchResults` when
`SEARCH_LEXICAL_BACKEND=postgres`) with weights (1, 0) for lexical, (0, 1) for semantic and the
configured `[artifact_search]` weights for hybrid. RRF with one weight at zero orders the other list
exactly as its own ranking, and keyword extraction, partition reads and the HNSW scan settings stay
identical to production. A result whose fused score is 0 came only from the zero-weighted list and
is treated as not found. Alternative: separate lexical-only and vector-only SQL — rejected, it
would be a second copy of the search to keep in sync.

**Rank window = candidate limit.** Each search asks for one page of `hybridCandidateLimit` (200)
rows. The target's 1-based position in it is its rank; absent means a miss (rank NULL). MRR uses
1/rank with misses as 0.

**Whole metric corpus.** Searches use artifact type `metric` with no record filter, like
`/kb/metrics/search`. The question's own document is not given to search.

**Question set.** A run takes every `testbed.generated_questions` row with the chosen purpose at
start time, ordered by id. Purpose is validated against the supported list (`search_metrics`).

**Storage.**
- `testbed.hybrid_search_benchmark_runs`: id, purpose, description, status
  (`running|done|failed`), error_msg, question_count, done_count, settings snapshot
  (lexical_backend, query_keywords, embedding_model, lexical_weight, semantic_weight,
  candidate_limit), created_by, created_at, finished_at, expires_at.
- `testbed.hybrid_search_benchmark_results`: run_id (cascade), question_id (SET NULL on delete),
  and a snapshot of the question (question, target_artifact_id, hop_type, lang, input_record_id),
  plus keywords (text[]), lexical_rank, semantic_rank, hybrid_rank, hybrid_top (jsonb, first 10
  artifact ids), error_msg. Unique (run_id, question_id).
- Scores are computed when a run is loaded, from its result rows, not stored.

**Worker.** One goroutine per run, questions processed one at a time (the shared embedding host
times out under concurrent load). Per question: embed once (`computeQueryEmbedding`), read
keywords (`searchQueryKeywords`, cached, so the searches reuse them), run the three searches,
insert the result row and bump `done_count` and `expires_at` (now + 10 min). A question whose
embedding or search fails gets a result row with `error_msg` and NULL ranks (scored as a miss); the
run continues. A run whose `expires_at` passes while `running` is marked failed on the next list
call, as question runs are.

**Search run IDs.** Search has no run table, so a sequence `kb.search_run_id_seq` supplies one
ID per search: per API request, per agent search call, per benchmark question. The ID and user go
in a `registrySearchConfig.call` field (the config already reaches every search function) and from
there into the context of `computeQueryEmbedding` / `searchQueryKeywords`, which pass them as
`RunID` / `UserID` to the LLM client. Alternative: a `kb.search_runs` log table — rejected for now,
only usage-log correlation was asked for.

**API** (admin only, same middleware check as the Question Generator):
- `POST /kb/hybrid-search-benchmark/runs` `{purpose, description}` → 202 with the run.
- `GET /kb/hybrid-search-benchmark/runs` → recent runs (50), with hybrid R@10 per run.
- `GET /kb/hybrid-search-benchmark/runs/:id` → run, scores (by mode × all / hop / lang), results.

**Page.** Start form (purpose, description), runs table, selected run's settings, score tables and
per-question table with three ranks. Polls the selected run every 2 s while it is running. All text
through Paraglide; root has `select-text`.

## Risks / Trade-offs

- [The question set changes as questions are generated] → each run stores its question rows, so
  runs stay comparable question by question; the page shows each run's question count.
- [LLM-written questions favour vector search, as the offline benchmark found] → shown per mode,
  not as one verdict.
- [A server restart leaves a run `running`] → `expires_at` heartbeat; marked failed on next list.
- [Embedding host slow] → sequential processing, per-question errors recorded, run continues.

## Migration Plan

One goose migration creating both tables (down drops them). No backfill.
