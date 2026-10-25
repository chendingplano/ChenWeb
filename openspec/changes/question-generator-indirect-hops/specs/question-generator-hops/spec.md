## ADDED Requirements

### Requirement: Questions and generation jobs tabs
The Question Generator results panel SHALL have Generated questions and Question generation jobs
tabs, available in English and Chinese with keyboard navigation. Generated questions SHALL be
selected initially and retain the existing filtering, selection, archiving, and List/Cards views.
Switching tabs SHALL preserve the question browser state. Question-only filters SHALL be hidden
while viewing jobs.

#### Scenario: Switch back to questions
- **WHEN** a user selects questions, opens jobs, and returns to Generated questions
- **THEN** the question selection, filters, and List/Cards view remain unchanged

### Requirement: List all generation jobs
The jobs tab SHALL list saved generation runs across all documents, newest first, using pagination
of 25 jobs per page. Each row SHALL show Run ID, document ID or random-batch label, status,
generated/target question counts, start time, completed duration, description, and View action.
View SHALL load the selected job into the existing generation status panel, including any failure
details. Running jobs SHALL use the flashing amber status badge, respecting reduced motion.
The list SHALL offer manual refresh and refresh every 2.5 seconds while its displayed page has
running jobs. Leaving the jobs tab SHALL cancel its pending list requests and timers.

#### Scenario: More than fifty jobs
- **WHEN** 61 jobs exist
- **THEN** all jobs are reachable in three pages, including the oldest 11 on page 3

#### Scenario: Running job completes
- **WHEN** a displayed job finishes
- **THEN** the next list refresh shows its terminal status, final progress and duration

### Requirement: Paginated runs API
GET `/kb/question-generator/runs` SHALL accept optional `page` (1–1,000,000, default 1) and
`page_size` (1–100, default 50). Requests specifying either SHALL return
`{runs, total, page, page_size}`, preserving the optional document filter. Unpaginated calls SHALL
retain the legacy `{runs}` response with the latest 50 runs. Invalid pagination SHALL return 400.

#### Scenario: Invalid page
- **WHEN** page is zero, negative, non-integer, or above 1,000,000
- **THEN** the server rejects the request with HTTP 400 before querying jobs

### Requirement: Requested questions select distinct metrics
For a specified document, the page SHALL accept a requested question count from 1 to 3,000
as `num_questions_to_generate`, rather than a metric count. The server SHALL retrieve all primary
document metrics, retain distinct metric IDs linked to usable canonical source lines, and randomly
sample `ceil(requested / 3)` metrics without replacement. Each selected metric SHALL produce one
question of each hop type. The page SHALL show the rounded question total before submission.
The question count SHALL take precedence over legacy metric/hop counts. Batch and legacy requests
without this field SHALL retain their existing behavior.

#### Scenario: Round up to complete triplets
- **WHEN** 10 questions are requested and at least 4 usable metrics exist
- **THEN** 4 distinct metrics are sampled and 12 questions are generated

### Requirement: Confirm an insufficient metric pool
When fewer than the required metrics are available, the server SHALL return HTTP 409 with
`available_metrics`, `available_questions`, `requested_questions`, and `confirmation_required`,
before creating a run or making a model call. The page SHALL show the reduced question count and
ask for confirmation in English or Chinese. On acceptance it SHALL resubmit with
`confirmed_available_questions` equal to that count. The server SHALL recheck capacity and require
another confirmation if the count changed. Once accepted, generation SHALL use the eligible metric
pool and canonical lines captured by that successful check.

#### Scenario: Confirm fewer questions
- **WHEN** 10 questions are requested but only 2 usable metrics exist and the user confirms
- **THEN** the run targets 6 questions using each of the 2 metrics once

#### Scenario: Cancel fewer questions
- **WHEN** the user cancels the shortage prompt
- **THEN** no run starts and the generation settings remain available for editing

#### Scenario: No usable metrics
- **WHEN** no usable metrics exist
- **THEN** `available_questions` is 0 and `confirmation_required` is false, and the page explains that no run started

#### Scenario: Source unavailable
- **WHEN** the document's canonical lines cannot be loaded
- **THEN** generation fails before creating a run rather than reporting a metric shortage

### Requirement: Hop types measure indirectness
Generated `search_metrics` questions SHALL use hop types as follows: a one-hop question names the
target metric (or a close paraphrase) and asks for its value, limit, unit or condition; a two-hop
question does not name the metric and describes a situation, purpose or condition whose answer is
the metric, needing one inference to identify it; a multi-hop question does not name the metric
and needs a chain of at least two inferences to identify it. Every question SHALL have exactly one
target metric and SHALL NOT ask for several values.

#### Scenario: Indirect two-hop question
- **WHEN** a two-hop question is generated for the metric "Test duration"
- **THEN** it asks about the situation (e.g. how long recording must continue after steady state) without the words "test duration"

### Requirement: Name-leak validation
The server SHALL reject a two-hop or multi-hop question whose text contains its target metric's
`metric_name` or `metric_name_en` (case-insensitive; names of two or more characters), with an
error that the correction round passes back to the model.

#### Scenario: Leaked name
- **WHEN** a multi-hop question for "试验持续时间" contains "试验持续时间"
- **THEN** validation fails with an error naming the leaked name, and the model gets one correction attempt

#### Scenario: One-hop question
- **WHEN** a one-hop question contains its metric's name
- **THEN** validation accepts it

### Requirement: Preserve independently valid questions
Generation SHALL validate questions independently and preserve all accepted questions even when
other rows are malformed, omitted, duplicated, ungrounded, or fail the name-leak rule. An initial
response's accepted rows SHALL be committed before requesting its one correction attempt. The
correction SHALL use the same sampled metrics, language targets and source snapshot, and SHALL
only fill unresolved `(set_no, hop_type)` slots. Earlier accepted rows SHALL NOT be overwritten
or discarded if a correction regresses, fails, times out, or is interrupted. Duplicate checks
SHALL consider retained questions, ignoring replacements for already-accepted slots.

Each write SHALL atomically insert accepted rows and update actual saved per-hop counts. A
document visit SHALL increment documents_processed once, even when a correction appends rows.
Unresolved questions SHALL leave the job Failed, with its saved questions and rejection
diagnostics accessible. The status panel SHALL report the saved count in English and Chinese.
Batch generation SHALL continue its planned metric-set budget across other documents and retain
all successful rows, without silently resampling metrics to replace rejected slots.

#### Scenario: One question still fails
- **WHEN** two questions pass and a third still leaks its target metric name after correction
- **THEN** the two successful questions remain saved, counters are 1/1/0 by hop, and the job is Failed

#### Scenario: Correction regresses a successful question
- **WHEN** an initial valid question becomes invalid in the correction while the missing question is repaired
- **THEN** the initial valid version and the newly repaired question are retained and the job completes

#### Scenario: Correction fails after partial commit
- **WHEN** valid questions have been saved and the correction model fails or the worker is cancelled
- **THEN** the saved questions and counters remain intact

#### Scenario: Original response position
- **WHEN** question 57 fails after earlier invalid rows have been removed from accepted output
- **THEN** the diagnostic still identifies its original response position as question 57

### Requirement: English name in the model input
The model input for each sampled metric SHALL include `metric_name_en` when `kb.metrics` has one.
If the English names cannot be loaded, generation SHALL continue and the check SHALL use the
stored name only.

#### Scenario: English names unavailable
- **WHEN** the English-name query fails
- **THEN** a warning is logged and the questions are generated and checked against `metric_name`
