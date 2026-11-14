## ADDED Requirements

### Requirement: A document's own copies are never grounding candidates

The Compliance Review reviewer SHALL exclude from its knowledge-base search the reviewed record
and every other record whose `kb.inputs.md5` equals the reviewed record's. A reviewed record with
no md5 SHALL exclude only itself.

#### Scenario: Duplicate upload is excluded
- **WHEN** record 416 is reviewed and record 198 has the same md5
- **THEN** the search request excludes both 416 and 198 and no reported candidate belongs to either

#### Scenario: No md5
- **WHEN** the reviewed record has an empty md5
- **THEN** only the reviewed record is excluded

### Requirement: Candidates below a relevance floor are not reported

The reviewer SHALL apply `min_cosine` from `[reviewers.compliance]` as the search's minimum cosine
similarity. When the setting is absent or 0, no floor SHALL apply. Every reported candidate SHALL
show its cosine similarity. An activity whose search completes with no candidate at or above the
floor SHALL get the `compliance_no_grounding` row, never an empty candidates row.

#### Scenario: Floor removes all candidates
- **WHEN** `min_cosine` is set and every result for an activity scores below it
- **THEN** the activity's row has `FindingType` `compliance_no_grounding` and the fixed sentence stating that this does not mean the activity is unregulated

#### Scenario: No floor configured
- **WHEN** `min_cosine` is absent
- **THEN** the reviewer reports the engine's results as today, each with its cosine

#### Scenario: Search did not run
- **WHEN** the search call fails
- **THEN** the chunk fails with the error and no no-grounding row is written for that activity

### Requirement: An evaluation snapshot stamps the whole indexed knowledge base

An evaluation snapshot SHALL record, for every record indexed for search, its id, md5, chunk-index
generation and the count of chunk and provision artifacts, and the embedding model of the index.
Pooling and saving gold SHALL refuse a snapshot when any of its records changed md5, generation or
counts or disappeared, or the embedding model changed, and SHALL name each difference. Records
indexed after the snapshot SHALL be allowed and SHALL be left out of gold pools.

#### Scenario: Index changed since the snapshot
- **WHEN** a snapshot record is re-indexed after the snapshot was recorded
- **THEN** pooling does not start and reports that record and the old and new generation

#### Scenario: Record added since the snapshot
- **WHEN** a record is indexed after the snapshot was recorded
- **THEN** the snapshot still verifies and the record is not in any pool built against it

### Requirement: Compliance quality is measured against a gold set

The system SHALL keep a versioned compliance gold set whose cases give, per reviewed document,
its activities (including the document-level activity) with line spans, and per activity either
the snapshot records that govern it or an explicit "no grounding expected". Each run SHALL record
how it was labelled (the coding agent alone, or the model panel) and the scorer SHALL report it;
ambiguity SHALL be recorded as such. The scorer SHALL report activity span precision and recall,
activity-list stability between runs made against different knowledge-base states, record-level
grounding recall, candidate precision over the reported passages the gold judged, and the precision
of "no grounding" together with the share of those cases where the snapshot held a governing record.
A reported passage the gold never judged (on a record added after the snapshot, or outside the
pool) SHALL be counted as unjudged, never as wrong. Results SHALL state the number of cases behind
each figure.

#### Scenario: Activity-list stability
- **WHEN** the same document is reviewed against two knowledge-base states
- **THEN** the scorer reports whether the activity lists are identical and lists any activity that differs

#### Scenario: Hit on a record added after the snapshot
- **WHEN** a gold activity expects no grounding and the reviewer's only candidates lie on records indexed after the snapshot
- **THEN** the scorer counts the activity as unjudged, not as a false grounding

#### Scenario: Correct abstention
- **WHEN** a gold activity is marked "no grounding expected" and the reviewer reports candidates above the floor, at least one of which the gold judged
- **THEN** the scorer counts a false grounding for that activity

#### Scenario: Reported passage outside the pool
- **WHEN** the reviewer reports a passage on a snapshot record that the gold never judged for the covered activities
- **THEN** the scorer counts it as unjudged (not in the pool), leaves it out of candidate precision and false grounding, and says that `compliance-gold augment` would judge it

#### Scenario: Candidate precision with a coarse row
- **WHEN** one reviewer row covers gold activities A and B and reports a passage that governs A and is unrelated to B
- **THEN** the passage counts as `governs` for candidate precision

### Requirement: A baseline is kept with the gold run

The scorer's saved gold run SHALL include the same documents scored for the current legal,
regulatory and standards compliance reviewers and for the Compliance Review reviewer with and
without the exclusion and floor, so later phases compare against it.

#### Scenario: Baseline recorded
- **WHEN** the baseline run completes
- **THEN** it is stored with the snapshot id, rules version, model and per-reviewer scores

### Requirement: First-iteration gold labels come from the coding agent

The first iteration's gold SHALL be labelled by the coding agent following the skill
`compliance-review-benchmark`, one immutable run per document, recording the agent's model, the
rules version and its SHA-256, and the agent's audit notes. Saving SHALL refuse a pooled candidate
without exactly one label, a label outside `governs`, `topical`, `unrelated`, `ambiguous`, a
`governs` or `ambiguous` label without a reason, and a rules file whose SHA-256 differs from the
skill's `versions.json`. Scores SHALL say that the labels come from one agent, with no
inter-annotator agreement.

#### Scenario: Unlabelled candidate
- **WHEN** the agent's labels leave one pooled candidate without a label
- **THEN** nothing is saved and the error names that activity and candidate

#### Scenario: Edited rules
- **WHEN** a released rules file was edited after release
- **THEN** saving stops and asks for a new rules version

### Requirement: The candidate pool per activity is capped by configuration

`[reviewers.compliance].max_candidates` in `doc-review.local.toml` SHALL cap the candidates pooled
for one gold activity; absent SHALL mean 100, and a value that is not positive SHALL be an error. The
gold run SHALL store the cap it used. The Compliance Review reviewer's own search SHALL NOT read it.

#### Scenario: Cap applied
- **WHEN** `max_candidates = 50` and the pool's rankings return more than 50 distinct candidates
- **THEN** the first pass holds 50, taken alternately from the rankings (each query's fused, keyword and vector order)

### Requirement: The pool is wider than the reviewer's search

The gold pool SHALL apply no relevance floor and SHALL search, per activity, the source text, the
name with its object and up to six agent-written queries, each in its fused, keyword and vector
ranking. Documents the activity cites (`cited_docs`) SHALL be resolved edition-insensitively to
snapshot records and searched within; unresolved citations SHALL be listed. After labelling,
expansion SHALL search within every record with a `governs` label, and saving SHALL refuse an
activity with a `governs` record that was not expanded. Each judgement SHALL record which list
first found its passage.

#### Scenario: Cited standard in the knowledge base
- **WHEN** an activity cites `GB/T 19095-2019` and the snapshot holds record 1234 whose `doc_no` is `GB/T 19095-2008`
- **THEN** the pool searches within record 1234 and adds its passages with `found_by` `cited:GB/T 19095-2019`

#### Scenario: Cited document not held
- **WHEN** an activity cites a document no snapshot record identifies as
- **THEN** the pool file lists the citation as unresolved and pooling continues

#### Scenario: Governing record not expanded
- **WHEN** the labels give an activity a `governs` passage on record 1234 and the pool never expanded record 1234 for it
- **THEN** saving stops and asks for `compliance-gold expand`

### Requirement: A gold run can be augmented with a reviewer run's unjudged passages

`compliance-gold augment` SHALL write a pool of the reviewer's reported passages on snapshot records
that a gold run never judged, per covered gold activity. Saving its labels SHALL create a new run
holding the base run's activities and judgements plus the new judgements, linked to the base run and
the review run, and SHALL refuse a different snapshot or rules version.

#### Scenario: Augmented run
- **WHEN** gold run K never judged passage `chunk/77` that review run 101 reported for a covered activity
- **THEN** `augment` puts `chunk/77` in the pool file, and saving its label creates a new run with K's judgements, the new one (`found_by` `review:101`), `base_run_id` K and `review_run_id` 101

### Requirement: Model-panel labels, when used, come from independent model annotators and an adjudicator

When the deferred model panel (`compliance-gold panel`) is used, gold labelling SHALL use the model profiles named by `COMPLIANCE_REVIEWER_ANNOTATOR_1` and
`COMPLIANCE_REVIEWER_ANNOTATOR_2` as independent annotators and the profile named by
`COMPLIANCE_ADJUDICATOR` as adjudicator, each resolved through `MODEL_DEF_FILE`. An unset variable
SHALL fail the labelling run naming the variable, and the two annotators SHALL NOT be the same
profile. Each stored label SHALL carry its source (`llm:` and the profile) and an adjudicated case
SHALL carry the adjudicator's rationale or an ambiguity mark. Scores SHALL be reported as
LLM-adjudicated.

#### Scenario: Missing variable
- **WHEN** `COMPLIANCE_ADJUDICATOR` is unset
- **THEN** the labelling run stops before any model call and names `COMPLIANCE_ADJUDICATOR`

#### Scenario: Same profile twice
- **WHEN** both annotator variables name the same profile
- **THEN** the labelling run stops and reports that the annotators must differ

### Requirement: Gold documents are listed in configuration

The documents a gold run labels SHALL be the record ids listed in the one table
`[compliance-review.gold-docs]` of `doc-review.local.toml`. A record that is not listed, a key
that is not a record id, or a record listed twice SHALL stop the run and be named. The retired
tables `[compliance-review.covered-docs]` and `[compliance-review.uncovered-docs]` SHALL be refused
with a message naming the new table. Whether an activity has grounding SHALL come from its labels,
and `show`, `save` and the scorer SHALL report how many gold activities have a governing passage and
how many have none.

#### Scenario: Not listed
- **WHEN** the agent pools record 416 and `[compliance-review.gold-docs]` does not list it
- **THEN** pooling stops and names record 416 and the table

#### Scenario: Retired table
- **WHEN** `doc-review.local.toml` still has `[compliance-review.covered-docs]`, even empty
- **THEN** every command that reads the config stops and says the tables were merged into `[compliance-review.gold-docs]`

#### Scenario: Mix reported
- **WHEN** a gold run is saved with 60 activities, 41 of which have a `governs` passage
- **THEN** `save` reports 41 with a governing passage and 19 with none
