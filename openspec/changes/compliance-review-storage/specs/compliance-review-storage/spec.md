## ADDED Requirements

### Requirement: Finding summary
Every compliance finding SHALL keep its identity, record and run references, pass, aspect,
severity, finding type, title, review status, confidence, processing status, location,
source artifact and a machine-readable `line_start` and `line_end` in
`kb.doc_review_findings`. A normalized compliance finding SHALL have `storage_version = 2`
and SHALL store `activity_count`, `search_result_count`, `selected_candidate_count`,
`relevant_candidate_count`, `judged_candidate_count` and `verdict_counts` with it. Its
metadata SHALL hold only `schema_version`, `result_kind` and `storage_version`.

#### Scenario: Counters of a grouped finding
- **WHEN** a finding groups two activities and one candidate is judged for both
- **THEN** `activity_count` is 2 and `judged_candidate_count` counts that candidate twice

#### Scenario: Metadata stays small
- **WHEN** a normalized compliance finding is saved
- **THEN** its metadata contains no activities, traces, verdicts, language copies or models

### Requirement: Description holds no judgements
The description of a normalized compliance finding SHALL hold only text about the finding
as a whole. It SHALL NOT contain verdict rows, verdict counts, activity lines or a list of
judged candidates. An assessment finding's description SHALL be empty.

#### Scenario: Assessment finding
- **WHEN** the reviewer saves a finding with four compatible verdicts
- **THEN** its details description is empty and the four verdicts are candidate rows

#### Scenario: No-grounding finding
- **WHEN** no candidate was left after search and relevance
- **THEN** the description is the sentence saying so and that it does not mean the activity is unregulated

### Requirement: Separately stored records
Compliance Review SHALL store each finding's full content and execution details in
`kb.doc_review_finding_details`, each activity in `kb.doc_review_compliance_activities`,
each activity–candidate pair sent to the relevance check in
`kb.doc_review_compliance_candidates`, each search in `kb.doc_review_compliance_searches`
and each returned result in `kb.doc_review_compliance_search_results`. Configured prompts,
models, assessment date and settings of a run SHALL be stored once in
`kb.doc_review_compliance_run_context`; the model that answered each stage SHALL be stored
with the finding's details. An unchecked-activities notice SHALL have no activity record.

#### Scenario: One candidate in two searches
- **WHEN** two searches of one activity return the same candidate
- **THEN** there are two search-result rows and one candidate row for that activity

#### Scenario: Configured and stage models
- **WHEN** the correction model answered a finding's judgement
- **THEN** the run context names the configured primary, correction and fallback models and the finding's stage models name the correction model for judgement

### Requirement: Candidate assessment status
Every candidate row SHALL carry a status: `rejected`, `relevance_failed`, `pending`,
`judgement_failed` or `judged`. Only a `judged` row SHALL carry a verdict and binding. A
missing verdict SHALL NOT be read as compliant or irrelevant. A failure that names a
candidate SHALL be stored with that candidate; a broader failure SHALL stay in the
finding's error message.

#### Scenario: Judgement batch fails
- **WHEN** the judgement of a batch of kept candidates fails after three tries
- **THEN** each of those candidate rows has status `judgement_failed` and the error, and the finding is marked failed

#### Scenario: Relevance call fails
- **WHEN** an activity's relevance call fails
- **THEN** each candidate found for that activity has a row with status `relevance_failed`

### Requirement: Global activity identity
Each activity SHALL receive an `activity_id` from the shared ID manager sequence
`compliance_activity_id`, unique across all reviews. The local label (`a1`, `a2`, …) SHALL
be kept as `local_label`, unique within its finding. Searches and candidates SHALL
reference `activity_id` through foreign keys. A rerun SHALL create new activity IDs.

#### Scenario: Same label in two findings
- **WHEN** findings 11970 and 11869 both have an activity labelled `a1`
- **THEN** the two activities have different activity IDs

### Requirement: Atomic save
Compliance Review SHALL save a normalized finding's summary, details, activities,
searches, results and candidates in one database transaction. A failure SHALL leave none of that finding's
records. The run context SHALL exist before the run's findings are saved and SHALL be
reused by them.

#### Scenario: Insert fails midway
- **WHEN** inserting a finding's candidate rows fails
- **THEN** the finding row, its details, activities and searches are not in the database

### Requirement: Compact report loading
`GET /doc-review/requests/:id` SHALL return compliance findings as compact entries with
identity, pass, aspect, severity, finding type, title, location, line range, review
status, processing status and storage version, without description, evidence, suggestion
or metadata. `GET /doc-review/reports/:id?view=summary` SHALL return the report without
its finding arrays.

#### Scenario: Report opens
- **WHEN** the report view of run 116 opens
- **THEN** no response contains a compliance description, candidate, search or metadata payload

### Requirement: Paginated compliance summaries
`GET /doc-review/runs/:run_id/findings?aspect=compliance` SHALL return at most `limit`
(default 25, at most 100) finding summaries in the API's severity order with ID as
tie-breaker, and a cursor for the next page. Summaries SHALL include the counters and no
description, evidence, candidate or search data. The query SHALL NOT join child tables.

#### Scenario: Second page
- **WHEN** the client requests the page after cursor `4:11900` with limit 25
- **THEN** the response holds the next 25 findings after that position and a new cursor, or no cursor when none remain

### Requirement: Details on demand
Selecting a finding SHALL load its full content, run-context and stage models, a page of
activity summaries and a page of candidate summaries. A candidate's evidence (quotes,
numeric check, verification, draft verdict) SHALL be loaded only when that candidate is
expanded. Search summaries SHALL be loaded only when search history is opened, and a
search's results only as a bounded page. No details request SHALL call a model.

#### Scenario: Select a finding
- **WHEN** the user selects finding 11970
- **THEN** the details response holds one activity and judged candidate summaries, and no search results

#### Scenario: Large group
- **WHEN** the user selects finding 11869 with 48 activities
- **THEN** activities and candidates arrive in pages with cursors

### Requirement: Audit export
`GET /doc-review/findings/:id/export` SHALL return the complete record assembled on the
server: summary, details, run context, activities with searches, results and candidates,
and translations. For a legacy finding it SHALL return its stored columns and metadata.

#### Scenario: Export a migrated finding
- **WHEN** finding 11970 is exported after migration
- **THEN** the export holds 1 activity, 2 searches, 60 search results and 30 candidate rows

### Requirement: Reports and scorer read candidate rows
Reports SHALL render a normalized compliance finding's verdicts and activities from its
candidate and activity rows. The compliance scorer SHALL read judged candidates of
normalized findings from candidate rows with status `judged`, and SHALL keep parsing
descriptions only for legacy findings. A run with a failed compliance finding SHALL
still be refused.

#### Scenario: Score before and after migration
- **WHEN** run 116 is scored before and after migration
- **THEN** both scores are identical

### Requirement: Legacy migration
`compliance-storage-migrate` SHALL migrate legacy compliance findings in bounded batches,
one transaction per finding, and SHALL switch a finding's `storage_version` only after
validation passes. Validation SHALL fail when a local label does not resolve, a verdict
has no kept relevance decision, counts differ, a verdict row or judged-list entry in the
description has no matching judged candidate row, or a canonical-language copy differs
from the finding. A language copy identical to the canonical content SHALL be dropped;
a copy in another language SHALL become a translation row. Missing historical
configuration SHALL NOT be filled from current settings; differing run context between
findings SHALL be kept as review flags. Findings of unsupported shapes SHALL remain
legacy and readable. A retry SHALL reuse existing activity IDs.

#### Scenario: Run 116
- **WHEN** run 116 is migrated
- **THEN** it keeps 126 findings and 244 activities, creates no translation rows and one run context with no review flags, and finding 11869 keeps 48 activities with distinct IDs

#### Scenario: Unsupported shape
- **WHEN** a run-98 finding of type `compliance_grounding_candidates` is met
- **THEN** it is reported as unsupported and keeps `storage_version = 1`

#### Scenario: Validation failure
- **WHEN** a finding's relevance list names activity `a3` that the finding does not have
- **THEN** that finding's transaction rolls back and its legacy payload stays intact

### Requirement: Strip after recovery copy
`compliance-storage-migrate --strip` SHALL act only on validated (`storage_version = 2`)
findings, SHALL first write a recovery copy of each original row, and SHALL then reduce
metadata to markers, set the description column to an empty string, and clear evidence,
suggestion and cited documents.

#### Scenario: Strip without recovery directory
- **WHEN** `--strip` is given without `--recovery-dir`
- **THEN** the command exits with a usage error and changes nothing

### Requirement: Deletion
Deleting a run's findings SHALL remove their details, translations, activities,
searches, results and candidates. Deleting a run SHALL remove its run context. Marking a
finding deleted SHALL remove nothing and SHALL record a correction action. Correction
actions SHALL survive the removal of the findings they mention.

#### Scenario: Restarted run
- **WHEN** a run restarts and its previous findings are deleted by run ID
- **THEN** no activity, candidate, search or result row of those findings remains

### Requirement: Bounded rendering and highlighting
The report view SHALL render compliance summaries 25 at a time. Clicking the Compliance
folder SHALL highlight every compliance finding in the PDF from the compact entries,
without loading descriptions, evidence or candidate records.

#### Scenario: Expand Compliance
- **WHEN** the user expands Compliance in run 116
- **THEN** 25 summaries render with a control to load more, and all 126 findings are highlighted
