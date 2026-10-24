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
Benchmark**. The page SHALL start runs, list past runs and show a selected run's settings, scores
and per-question ranks, refreshing while the run is in progress. All text SHALL be available in
English and Chinese, and page text SHALL be selectable.

#### Scenario: Start and watch a run
- **WHEN** an administrator starts a run on the page
- **THEN** the run is selected and its progress and scores refresh until it is done or failed
