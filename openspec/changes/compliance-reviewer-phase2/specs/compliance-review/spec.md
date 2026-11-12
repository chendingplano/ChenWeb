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

### Requirement: An evaluation snapshot freezes the searched knowledge base

An evaluation snapshot SHALL record, for each member record, its id, md5, chunk-index generation
and the count of chunk and provision artifacts, and the embedding model of the index. A gold run
SHALL refuse to start when any member's current md5 or generation differs from the manifest, and
SHALL name each member that differs.

#### Scenario: Index changed since freeze
- **WHEN** a member record is re-indexed after the snapshot was recorded
- **THEN** the gold run does not start and reports that record and the old and new generation

### Requirement: Compliance quality is measured against a gold set

The system SHALL keep a versioned compliance gold set whose cases give, per reviewed document,
its activities (including the document-level activity) with line spans, and per activity either
the snapshot records that govern it or an explicit "no grounding expected". Each case SHALL be
labelled by two annotators and adjudicated, and unresolved ambiguity SHALL be recorded as such.
The scorer SHALL report activity span precision and recall, activity-list stability across full,
narrowed and empty search scope, record-level grounding recall, and the precision of "no
grounding" together with the share of those cases where the snapshot held a governing record.
Results SHALL state the number of cases behind each figure.

#### Scenario: Scope stability
- **WHEN** the same document is run against the full snapshot and against the snapshot with the governing records excluded
- **THEN** the scorer reports whether the activity lists are identical and lists any activity that differs

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

### Requirement: Gold labels come from independent model annotators and an adjudicator

Gold labelling SHALL use the model profiles named by `COMPLIANCE_REVIEWER_ANNOTATOR_1` and
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
tables, one that does not exist in `kb.inputs`, or one outside the evaluation snapshot SHALL stop
the run and be named.

#### Scenario: Listed in both
- **WHEN** record 416 appears in both tables
- **THEN** the run stops and names record 416

### Requirement: Organization context is recorded with the gold run

`[compliance-review.organization]` SHALL hold each fact with a value, a status of `asserted`,
`inferred`, `disputed` or `unknown`, and a source; an absent fact SHALL be `unknown`. A gold run
SHALL store a copy of the context it ran with. The reviewer SHALL NOT use the context to decide
applicability in this change.

#### Scenario: Context stored
- **WHEN** a gold run starts with the context table present
- **THEN** the run row holds a copy of the context and later edits to the file do not change it

#### Scenario: No context table
- **WHEN** the table is absent
- **THEN** every fact is recorded as `unknown`
