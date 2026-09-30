## ADDED Requirements

### Requirement: Admin Page Placement
The system SHALL provide a "Review Metrics" page reachable from System Admin → LLM
in the home3 nav rail, alongside the existing LLM entries. The page SHALL be
rendered by client-side nav selection without its own URL route.

#### Scenario: Page reachable from admin nav
- **WHEN** an authenticated user expands System Admin → LLM
- **THEN** a "Review Metrics" entry is shown, and selecting it renders the page

### Requirement: Record Search and Selection
The page SHALL let the user search `kb.inputs` records. A purely numeric query SHALL
match the record ID exactly; any other query SHALL match the record title. Results
SHALL show at least the record ID, title and doc number. Clicking a result SHALL
select that record.

#### Scenario: Search by record ID
- **WHEN** the user enters `416` and searches
- **THEN** the result list contains record 416 only

#### Scenario: Search by title text
- **WHEN** the user enters part of a title and searches
- **THEN** records whose title contains that text are listed

#### Scenario: Select a record
- **WHEN** the user clicks a record in the result list
- **THEN** it becomes the selected record and its latest stored review, if any, is shown

### Requirement: Stored Review Retrieval
The page SHALL provide draggable dividers between the document menu and review,
and between the review and source PDF. Selecting a finding SHALL navigate the PDF
to its source page and highlight the source lines. New reports SHALL store
`source_line_spans` for missed findings and reviewed metric snapshots using the
extractor's `"n"` and `"n:m"` span format. Older reports SHALL remain navigable
through their existing `lines` fields.

#### Scenario: Navigate to a reviewed finding
- **WHEN** the user selects a missed metric, non-metric row, or attribute issue
- **THEN** the selected finding is visibly marked, and the PDF moves to the first cited page and highlights its cited lines

Selecting a record SHALL show that record's most recent stored review without
calling the LLM. When no review exists, the page SHALL say so and offer the Review
action.

#### Scenario: Review exists
- **WHEN** a record with a completed stored review is selected
- **THEN** the stored report is displayed with its review time and model name, and no LLM call is made

#### Scenario: No review exists
- **WHEN** a record with no stored review is selected
- **THEN** the page shows that the record has not been reviewed and shows the Review button

### Requirement: Review Execution and Force Flag
The page SHALL provide a Review button and a "Force to Review" checkbox. Pressing
Review without Force SHALL return the existing completed review when one exists,
and SHALL run a new LLM review only when none exists. Pressing Review with Force
SHALL run a new LLM review even when a completed review exists. A new review SHALL
be stored as a new record; earlier reviews SHALL be retained.

#### Scenario: Review without force, review exists
- **WHEN** the user presses Review with Force unchecked and a completed review exists
- **THEN** the existing review is shown and no LLM call is made

#### Scenario: Review without force, none exists
- **WHEN** the user presses Review with Force unchecked and no review exists
- **THEN** a new LLM review is started and stored

#### Scenario: Forced review
- **WHEN** the user presses Review with Force checked and a completed review exists
- **THEN** a new LLM review is started, stored as a new review, and shown when finished, and the earlier review remains in the database

#### Scenario: Review already running
- **WHEN** Review is pressed while a review of the same record is still running
- **THEN** no second run is started and the page keeps showing the running review's progress

### Requirement: Background Execution and Progress
A review run SHALL execute in the background so that it is not bound to the HTTP
request duration. While running, the page SHALL show that a review is in progress
and SHALL refresh until it completes or fails. A failed run SHALL show its error
message. A run left unfinished longer than the run timeout (e.g. by a server
restart) SHALL be reported as failed and SHALL NOT block a new review.

#### Scenario: Long review completes
- **WHEN** a review takes longer than the proxy timeout
- **THEN** the page continues to show progress and displays the report once the run is done

#### Scenario: Run fails
- **WHEN** the LLM call fails or its output cannot be parsed
- **THEN** the review is stored as failed with an error message, and the page shows that message

#### Scenario: Interrupted run
- **WHEN** a review has been running longer than the run timeout
- **THEN** it is reported as failed ("interrupted") and a new review can be started

### Requirement: Review Content
Each review SHALL be produced by an LLM from the document's line text and the
record's `kb.metrics` rows, and SHALL contain: a summary; missed metrics (with
source lines, value, unit, reason, severity); rows that should not be metrics (with
metric IDs, a category of `not_metric`, `duplicate` or `formula_input`, the original
for duplicates, and a reason); attribute issues for the kept rows (metric IDs, field,
stored value, suggested value, reason, severity); and recommendations. The server
SHALL compute the tally (stored, kept, per non-metric category, missed) itself and
SHALL discard metric IDs that are not among the reviewed rows. The review SHALL
store a snapshot of the reviewed metrics and the model and prompt names used.

#### Scenario: Tally computed server-side
- **WHEN** a review completes for a record with 73 metrics and the LLM lists 25 distinct IDs as non-metrics
- **THEN** the stored tally shows stored = 73 and kept = 48 regardless of any counts the LLM reported

#### Scenario: Unknown metric ID
- **WHEN** the LLM output cites a metric ID not among the record's reviewed metrics
- **THEN** that ID is removed from the stored report

#### Scenario: Record has no metrics
- **WHEN** Review is pressed for a record with no `kb.metrics` rows
- **THEN** no LLM call is made and the page reports that the record has no extracted metrics

### Requirement: Prompt and Model Configuration
The review prompt SHALL be read from the file under `prompts/` named by the
`REVIEW_METRICS_PROMPT` environment variable. The model SHALL be the one picked
in the page's Model menu, which lists only the `.models.toml` entries with
`model_type = 'llm'` (served by `GET /api/v1/kb/metric-reviews/models`, keys only,
never credentials) and preselects `REVIEW_METRICS_MODEL_NAME`. A start request
naming a model that is not such an entry SHALL be rejected with HTTP 400; a request
naming no model SHALL use `REVIEW_METRICS_MODEL_NAME`. Choosing a model does not
bypass the cache rule: a done review is reused unless Force to Review is ticked.
Translations always use `REVIEW_METRICS_MODEL_NAME`. The prompt SHALL NOT be
hard-coded. If a required variable is unset or unresolvable, a review run SHALL
fail with an error naming the missing variable.

#### Scenario: Reviewer picks a model
- **WHEN** the reviewer selects `qwen-plus` in the Model menu and presses Review with Force ticked
- **THEN** the run uses the `qwen-plus` entry and the stored review's model name is `qwen-plus`

#### Scenario: Non-LLM model requested
- **WHEN** a start request names an embedding model such as `bge-m3-llama-cpp`
- **THEN** the request is rejected with HTTP 400 and no run starts

#### Scenario: Prompt env var missing
- **WHEN** `REVIEW_METRICS_PROMPT` is unset and a review is started
- **THEN** the run fails with an error naming `REVIEW_METRICS_PROMPT`

### Requirement: Oversized Documents
If the assembled LLM input exceeds the fixed size limit, the run SHALL fail with an
error stating the document is too large to review, and SHALL NOT send a truncated
document to the LLM.

#### Scenario: Document too large
- **WHEN** a review is started for a document whose assembled input exceeds the limit
- **THEN** the run fails with a "document too large" error and no LLM call is made

### Requirement: Readable Report Presentation
The page SHALL present a completed review as a readable report: summary first, then
tally figures, then separate sections for missed metrics, non-metrics (grouped by
category), and attribute issues (ordered by severity), then recommendations. Metric
IDs in the report SHALL show the snapshot metric name and value.

#### Scenario: Report layout
- **WHEN** a completed review is displayed
- **THEN** the summary, tally, missed metrics, non-metrics, attribute issues and recommendations are shown as distinct sections in that order
