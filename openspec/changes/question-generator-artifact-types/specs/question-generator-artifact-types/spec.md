## ADDED Requirements

### Requirement: One purpose per search artifact type
The question generator SHALL accept exactly these purposes, each bound to one search artifact type:
`search_metrics` (metric), `search_chunks` (chunk), `search_summaries` (summary), `search_topics`
(topic), `search_scene_blocks` (scene_block), `search_provisions` (provision), `search_products`
(product), `search_inventory_items` (inventory_item), `search_semantic_projections`
(semantic_projection), `search_knowledge` (knowledge), `search_entities` (entity) and
`search_relations` (relation), plus `search_mixed`, bound to all types. Every bound type SHALL be
one of the search registry's partition types. Any other purpose SHALL be rejected with HTTP 400
before a run is created.

#### Scenario: New purpose accepted
- **WHEN** an admin starts a run with purpose `search_provisions`
- **THEN** the server creates the run with that purpose and generates questions whose targets are provisions

#### Scenario: Unknown purpose rejected
- **WHEN** an admin starts a run with purpose `search_tables`
- **THEN** the server returns HTTP 400 and creates no run

### Requirement: Metric purpose unchanged
`search_metrics` SHALL keep its current behaviour: targets from `kb.metrics`, the metric prompt
(`PROMPT_GENERATE_METRIC_QUESTIONS`, default `prompt-question-generator-search-metrics-v3.md`), the
same model input shape and the metric name-leak check. Its questions SHALL also record
`artifact_type = 'metric'` and `artifact_id` equal to `metric_id`.

#### Scenario: Metric run after the change
- **WHEN** an admin generates 3 questions for record 416 with purpose `search_metrics`
- **THEN** the model receives the same metric sets as before, and each saved question has `metric_id`, `metric_snapshot`, `artifact_type = 'metric'` and `artifact_id = metric_id`

### Requirement: Registry targets for non-metric purposes
For any purpose other than `search_metrics`, the targets of a document SHALL be the
`kb.search_artifacts` rows of the purpose's artifact type for that document, one per distinct
`artifact_id`. A row SHALL be eligible only when it has at least one source line span, every span
is well formed with both endpoints present among the document's non-empty canonical lines, and its
source context (below) is at most 20,000 characters. Ineligible rows SHALL never be sent to the
model.

#### Scenario: Stale span excluded
- **WHEN** a summary row cites span `900-950` and the document's canonical lines end at line 800
- **THEN** the row is not eligible and is never sampled

#### Scenario: Oversized context excluded
- **WHEN** a level-0 summary spans 1,465 lines whose formatted context exceeds 20,000 characters
- **THEN** the summary is not eligible, while shorter summaries of the same document remain eligible

### Requirement: Mixed purpose
`search_mixed` SHALL draw targets from the registry rows of every search artifact type, including
`metric` rows, with the same eligibility rules. For each target slot, the generator SHALL pick an
artifact type uniformly at random among the types that still have unsampled eligible targets in
the document, then a random unsampled target of that type. Only when every type is exhausted SHALL
the pools be refilled, as metric sampling does. Each saved question SHALL keep `purpose =
search_mixed` and store the actual type and ID of its target. A random batch SHALL sample documents
that have registry rows of any type.

#### Scenario: Types mixed within one document
- **WHEN** a `search_mixed` run asks for 4 targets from a document with 2 eligible provisions and 300 eligible entities
- **THEN** each target first draws provision or entity with equal probability, so provisions are not crowded out by the larger entity pool

#### Scenario: Mixed question storage
- **WHEN** a `search_mixed` run accepts a question about relation `365_rel_705`
- **THEN** the question is saved with `purpose = search_mixed`, `artifact_type = relation` and `artifact_id = 365_rel_705`

### Requirement: Target source context
Each sampled target SHALL be sent with a context of canonical lines: its span lines, up to ten
actual lines before its first span line and up to ten actual lines after its last span line, with
gaps between disjoint spans excluded. Context line IDs SHALL be listed as compressed ranges, as for
metrics.

#### Scenario: Disjoint spans
- **WHEN** a semantic projection cites spans `1-4` and `22-35`
- **THEN** its context contains lines 1–4, 22–35 and up to ten lines after 35, and none of lines 5–21

### Requirement: Artifact question model input and prompt
Non-metric purposes SHALL use the prompt named by `PROMPT_GENERATE_ARTIFACT_QUESTIONS`, default
`prompt-question-generator-search-artifacts-v1.md`. The model input SHALL contain document
metadata, run ID, record ID, purpose, description, previous questions and one set per sampled
target. Each set SHALL give the target's artifact type and ID, primary and secondary labels, snippet,
semantic payload with empty values removed, source line spans, context and context line ranges, plus
three question slots (one, two and multi-hop) each with an independently assigned language. The
prompt SHALL tell the model what a question about each artifact type asks for and SHALL keep the
hop, language, single-target and citation rules of the metric prompt.

#### Scenario: Summary run input
- **WHEN** a `search_summaries` run samples two summaries from record 387
- **THEN** the model input has two sets, each with `artifact_type = "summary"`, three question slots and the summary's labels, payload and context

### Requirement: Artifact question validation
Questions for non-metric purposes SHALL pass the existing structural validation (hop types,
reasoning steps, citations, duplicates). Each question SHALL cite only lines within its target's
context, at least one cited line SHALL fall within the target's spans, and its set, hop type and
language SHALL match an open slot. A two-hop or multi-hop question SHALL be rejected when it
contains the target's primary label of two or more characters (case-insensitive), and for
inventory items also its secondary label. Invalid output SHALL get one correction attempt with the
same targets, and valid questions SHALL be saved before that attempt, as for metrics.

#### Scenario: Citation outside the target
- **WHEN** a question for provision `416_prv_40` (span `116`) cites only line 300
- **THEN** the question is rejected and the correction request reports it

#### Scenario: Label leak in a two-hop question
- **WHEN** a two-hop question for inventory item `371_inv_11` (labels `秒表`, `stopwatch`) contains "stopwatch"
- **THEN** the question is rejected as naming its target

### Requirement: Target storage
`testbed.generated_questions` and `testbed.generated_questions_archive` SHALL have `artifact_type`,
`artifact_id` and `artifact_snapshot` columns. Each non-metric question SHALL store its target's type,
ID and a snapshot of the registry row (type, ID, labels, snippet, source title, spans and semantic
payload), with `metric_id` empty and `metric_snapshot` `{}`. The migration SHALL backfill existing
metric questions with `artifact_type = 'metric'` and `artifact_id = metric_id`. Archiving SHALL move
the new columns with the rest of the row.

#### Scenario: Existing metric questions after migration
- **WHEN** the migration runs on a database with 30 `search_metrics` questions
- **THEN** all 30 have `artifact_type = 'metric'` and `artifact_id` equal to their `metric_id`

#### Scenario: Archive keeps the target
- **WHEN** an admin archives a `search_products` question
- **THEN** its archive row has the same `artifact_type`, `artifact_id` and `artifact_snapshot`

### Requirement: Document sampling and capacity by type
A random batch run for a non-metric purpose SHALL sample, in random order, documents in `kb.inputs`
that have registry rows of the purpose's type, and skip documents with no eligible target. When no
document can produce the remaining questions, the run SHALL fail with an error naming the artifact
type. A single-document request with `num_questions_to_generate` SHALL count that type's eligible
targets. When too few exist, the server SHALL return HTTP 409 with `available_targets`,
`available_questions`, `requested_questions` and `confirmation_required` before creating a run, and
the confirmation flow SHALL work as for metrics. `available_targets` SHALL replace
`available_metrics` for every purpose, including `search_metrics`.

#### Scenario: No indexed rows of the type
- **WHEN** an admin starts a batch run with purpose `search_knowledge` and no knowledge rows are indexed
- **THEN** the run fails with an error saying no document with knowledge artifacts can generate the questions

#### Scenario: Too few targets in one document
- **WHEN** 30 questions are requested for a record with 4 eligible provisions
- **THEN** the server returns HTTP 409 with `available_targets = 4` and `available_questions = 12`, and creates no run

### Requirement: LLM call attribution
Artifact question calls SHALL record `call_reason = question_generation_<purpose>` (for example
`question_generation_search_summaries`) with their own call location, and carry the run's LLM run ID
like metric question calls.

#### Scenario: Usage log of a topic run
- **WHEN** a `search_topics` run makes a model call
- **THEN** its usage event has `call_reason = question_generation_search_topics` and the run's LLM run ID

### Requirement: Page support for all purposes
The Question Generator page SHALL offer all thirteen purposes (Mix included) in the generation
dialog and the purpose filter, with English and Chinese labels. Count fields, hints, the plan line and the no-capacity
message SHALL refer to targets of the selected type instead of metrics. Each question in the list
and card views SHALL show its target as artifact type and artifact ID. Historical purposes without a
label SHALL still display as their raw value.

#### Scenario: Generate summary questions from the page
- **WHEN** an admin selects Search Summaries, enters record 387 and 9 questions, and submits
- **THEN** the dialog shows a plan of 3 targets and 9 questions, the run starts, and each saved question card shows "Summary · 387_sum_…" as its target

#### Scenario: Chinese interface
- **WHEN** the interface language is Chinese
- **THEN** every purpose option, count label, hint and target label is shown in Chinese
