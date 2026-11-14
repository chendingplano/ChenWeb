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
grounding recall, and the precision of "no grounding" together with the share of those cases where
the snapshot held a governing record. Results SHALL state the number of cases behind each figure.

#### Scenario: Activity-list stability
- **WHEN** the same document is reviewed against two knowledge-base states
- **THEN** the scorer reports whether the activity lists are identical and lists any activity that differs

#### Scenario: Hit on a record added after the snapshot
- **WHEN** a gold activity expects no grounding and the reviewer's only candidates lie on records indexed after the snapshot
- **THEN** the scorer counts the activity as unjudged, not as a false grounding

#### Scenario: Correct abstention
- **WHEN** a gold activity is marked "no grounding expected" and the reviewer reports candidates above the floor
- **THEN** the scorer counts a false grounding for that activity

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
- **WHEN** `max_candidates = 50` and the two pool queries return more than 50 distinct candidates
- **THEN** the pool holds 50, taken alternately from the two queries' rankings

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

The documents a gold run labels SHALL be the record ids listed in `[compliance-review.covered-docs]`
and `[compliance-review.uncovered-docs]` of `doc-review.local.toml`. A record listed in both
tables, or one that is not listed, SHALL stop the run and be named.

#### Scenario: Listed in both
- **WHEN** record 416 appears in both tables
- **THEN** the run stops and names record 416
