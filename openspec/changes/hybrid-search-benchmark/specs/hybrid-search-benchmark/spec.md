## ADDED Requirements

### Requirement: Benchmark run over generated questions
The system SHALL let a system administrator start a hybrid search benchmark run for a question
purpose. A run SHALL use every `testbed.generated_questions` row with that purpose at start time.
Only the purpose `search_metrics` SHALL be accepted.

#### Scenario: Start a run
- **WHEN** an administrator posts purpose `search_metrics` and a description
- **THEN** a run row is created in `testbed.hybrid_search_benchmark_runs` with status `running`,
  the question count and the search settings in force, and the response is HTTP 202

#### Scenario: Unsupported purpose
- **WHEN** a run is requested with any other purpose
- **THEN** the request is rejected with HTTP 400 and no run is created

#### Scenario: No questions
- **WHEN** no question has the requested purpose
- **THEN** the request is rejected with HTTP 400 and no run is created

#### Scenario: Not an administrator
- **WHEN** a user who is not a system administrator calls the benchmark API
- **THEN** the request is rejected with HTTP 401 or 403

### Requirement: Three search modes on the live metric search
For each question the system SHALL search artifact type `metric` over the whole corpus three times
with the production hybrid query: lexical only (weights 1/0), semantic only (0/1) and hybrid (the
configured weights). It SHALL record the 1-based rank of the question's target metric among the
first 200 results of each search, treating results with a fused score of 0 and absent targets as
misses (NULL rank).

#### Scenario: Target found
- **WHEN** the target metric is the 3rd result of the hybrid search
- **THEN** the question's result row has `hybrid_rank = 3`

#### Scenario: Target missing
- **WHEN** the target metric is not among the scored first 200 results of a mode
- **THEN** that mode's rank is NULL

#### Scenario: Question fails
- **WHEN** the question cannot be embedded or a search errors
- **THEN** its result row stores the error with NULL ranks and the run continues with the next
  question

### Requirement: Search run IDs in LLM usage logs
Every search (search API request, agent search call, benchmark question) SHALL take a run ID from
`kb.search_run_id_seq`. Its query embedding and query keyword LLM calls SHALL record that run ID,
and the user when known, in `public.llm_usage_event`, with call reasons `search_query_embedding`
and `search_query_keywords`. A benchmark result SHALL store its question's search run ID.

#### Scenario: One search, one run ID
- **WHEN** a search embeds its query and extracts keywords
- **THEN** both usage events have the same `run_id`, and a different search has a different one

#### Scenario: Sequence unavailable
- **WHEN** the run ID cannot be allocated
- **THEN** the search still runs and its usage events have no run ID

### Requirement: Run search options
A run SHALL accept query keywords on/off, the lexical and semantic RRF weights (each 0–100, not
both 0) and candidates per list (1–1000); omitted options SHALL default to the live search
configuration, which the run list returns as `defaults`. The run SHALL use its options for every
question regardless of the server configuration, and SHALL store them with the keyword model and
prompt used.

#### Scenario: Keywords switched on for a run
- **WHEN** a run is started with query keywords on while `SEARCH_QUERY_KEYWORDS` is off
- **THEN** every question makes one keyword LLM call and the run shows the keyword model and prompt

#### Scenario: Invalid options
- **WHEN** both weights are 0 or candidates per list is outside 1–1000
- **THEN** the request is rejected with HTTP 400

### Requirement: Parallel questions
A run SHALL search up to its *parallel queries* option (1–32, default 8) questions at the same
time. A failed save or an expired run SHALL stop all workers.

#### Scenario: Parallel run
- **WHEN** a run with parallel queries 8 is started
- **THEN** up to 8 questions are searched at once and the run is marked done after all are stored

### Requirement: Keyword failure
When a question's keyword call fails or returns nothing, its searches SHALL run without keywords
(no further keyword calls) and its result SHALL record the failure.

#### Scenario: Keyword model unavailable
- **WHEN** the keyword model returns an error
- **THEN** the question makes one keyword call, its ranks use the raw query, and its error says so

### Requirement: Query timings
Each result SHALL store the query embedding, keyword extraction (a fresh LLM call, not the cache)
and each mode's search time in milliseconds. The page SHALL show a Query time column equal to
embedding + keywords + hybrid query, with the parts on hover.

#### Scenario: Timed question
- **WHEN** a question has been processed with keywords on
- **THEN** its result has all five timings and its Query time is their embedding + keywords + hybrid sum

### Requirement: Results stored in testbed
The system SHALL store one row per question in `testbed.hybrid_search_benchmark_results` with a
snapshot of the question (text, target artifact id, hop type, language, record id), the query
keywords used, the three ranks, the first 10 hybrid result ids and any error. When all questions
are processed the run SHALL be marked `done`.

#### Scenario: Run completes
- **WHEN** the worker has processed every question
- **THEN** the run's `done_count` equals `question_count`, status is `done` and `finished_at` is set

#### Scenario: Interrupted run
- **WHEN** a run is still `running` after its `expires_at` deadline
- **THEN** the next run listing marks it `failed`

### Requirement: Scores per mode
When a run is loaded the system SHALL report, for each mode, R@1, R@5, R@10 and MRR over all
questions and grouped by hop type and by language. Misses SHALL count as 0 in MRR.

#### Scenario: Scores reported
- **WHEN** a run with two questions has hybrid ranks 1 and NULL
- **THEN** hybrid R@1 is 0.5 and hybrid MRR is 0.5

### Requirement: Run Benchmark page
The development page SHALL show **System Admin → Benchmark → Hybrid Search Benchmark → Run
Benchmark**. The page SHALL start runs, list past runs with a View button, and show a selected
run's attributes in two groups (run; search settings including the keyword prompt and model name),
scores and per-question ranks, refreshing while the run is in progress. All text SHALL be available in
English and Chinese, and page text SHALL be selectable.

#### Scenario: Start and watch a run
- **WHEN** an administrator starts a run on the page
- **THEN** the run is selected and its progress and scores refresh until it is done or failed
