## ADDED Requirements

### Requirement: Every candidate of an assessed activity has a verdict and a summary

For every activity with candidates, Compliance Review SHALL give each candidate exactly one verdict and one summary. Candidates are the retrieved artifacts and any dependency artifacts. The verdict SHALL be one of:

- `compliant`: the document shows that the activity meets what the candidate requires of it;
- `compatible`: nothing in the document conflicts with the candidate, but the document does not state enough to show compliance;
- `not_compliant`: the document states something that conflicts with what the candidate requires;
- `undetermined`: the candidate can be used, but the reviewer could not reach a verdict;
- `unqualified`: the candidate cannot be used to judge this activity's compliance.

The summary SHALL be written in the activity's language. It SHALL explain how the candidate applies to the activity: what the candidate requires and under which conditions, what the document states about it, and what is missing or in conflict. For `unqualified`, the summary SHALL say why the candidate cannot be used. This applies whether or not any requirement was extracted for the activity.

#### Scenario: The document is silent on a candidate's conditions
- **WHEN** the activity states that live fish are stored in a container, and a candidate requires live sea fish to be stored in a sealed container with 60% air and 40% water at 5–10 °C
- **THEN** the candidate's verdict is `compatible`, and its summary says the candidate applies if the fish are sea fish and that the document states none of the container, air/water or temperature conditions

#### Scenario: A candidate states no requirement
- **WHEN** a candidate is a definition or background passage with no requirement for the activity
- **THEN** its verdict is `unqualified` and its summary gives the reason

#### Scenario: No requirement was extracted for the activity
- **WHEN** an activity has candidates but the requirements stage extracts none
- **THEN** screening is skipped, and every candidate still receives a verdict (`unqualified` or `undetermined`) and a summary

### Requirement: Requirements are linked to every candidate that expresses them

The requirements stage SHALL return, for each requirement, every candidate of the source being read that expresses that requirement (`artifact_keys`, at least one). A key that is not a candidate of that source SHALL be rejected. A candidate is linked to the requirements whose keys include it.

#### Scenario: A metric and a provision express one criterion
- **WHEN** a metric and a provision of the same source state the same criterion
- **THEN** one requirement is extracted, both candidates are linked to it, and both verdicts rest on its assessment

### Requirement: Strong verdicts rest on verified requirement results

A candidate's verdict SHALL be consistent with the assessments of its linked requirements. "Required" means binding `required`.

- `not_compliant` SHALL require a linked required requirement that is `applicable` and `contradicted`. Such a requirement SHALL force `not_compliant`.
- `compliant` SHALL require at least one linked required requirement that is `applicable` and `supported`, and every other linked required requirement `supported` or `not_applicable`.
- A candidate with no linked requirement SHALL be `unqualified` or `undetermined`.
- `unqualified` SHALL NOT be used when a linked required requirement is `applicable`.

The reviewer SHALL check these rules on the model's draft inside the call's correction retries, naming the candidate and the field in each correction. After requirement validation, numeric checks and verification, it SHALL re-evaluate the `compliant` and `not_compliant` rules on the final results. A verdict that no longer holds SHALL become `undetermined`, with the cause recorded. Code SHALL never raise a verdict to `compliant` or `not_compliant`. Candidates linked only to recommendations or unknown-binding requirements SHALL NOT be `compliant` or `not_compliant`.

#### Scenario: A drafted verdict contradicts the model's own assessment
- **WHEN** the draft marks a candidate `compatible` while a linked required requirement is drafted `applicable` and `contradicted`
- **THEN** the next request to the same model names the candidate, the verdict and the requirement, and asks for `not_compliant` or a corrected assessment; a corrected answer is accepted

#### Scenario: A supporting requirement is downgraded after the call
- **WHEN** a candidate is drafted `compliant`, but its supporting requirement becomes `insufficient_evidence` because the source context is incomplete
- **THEN** the candidate's verdict becomes `undetermined`, and its note records the cause

#### Scenario: Recommendation only
- **WHEN** a candidate's only linked requirement has binding `recommended`
- **THEN** its verdict is not `compliant` or `not_compliant`, and its summary says the candidate only recommends

### Requirement: Verdicts and summaries are verified

The verifier SHALL return one check per candidate, with a reason. It SHALL check that:

- the verdict follows from the requirement assessments and evidence;
- the summary states the requirement, the conditions, the document's statements and the gaps without invented facts;
- an `unqualified` candidate really cannot judge the activity, given the candidate's text.

A rejected verdict SHALL become `undetermined`. The draft summary is kept, and the verifier's reason is kept as the candidate's note.

#### Scenario: The verifier rejects an unqualified verdict
- **WHEN** the verifier finds that a candidate marked `unqualified` states a requirement on the activity's object
- **THEN** the verdict becomes `undetermined`, and the note holds the verifier's reason

### Requirement: Verdicts are stored and shown per candidate

Each assessed row SHALL store one `related_artifacts` entry per candidate. The entry holds the artifact id, the record, the relationship (the verdict), the summary, and an optional note. Entries SHALL be ordered `not_compliant`, `compatible`, `compliant`, `undetermined`, `unqualified`. The row's `artifact_fields` SHALL keep each candidate's draft verdict, final verdict, summary, requirement IDs, verification and note, with metadata version 2.

The row text SHALL begin with verdict counts and one line per candidate. It SHALL keep the existing candidate lines in the format the scorer reads. The verdict lines SHALL NOT match that format.

The Finding Details panel SHALL show each candidate's verdict as a translated label (English and Chinese) beside its summary, and the note under the summary. Rows written before this change SHALL render as before.

#### Scenario: Viewing an activity in the panel
- **WHEN** a user opens an assessed compliance row in Finding Details
- **THEN** each candidate shows its translated verdict label, its summary, any note, and a link to its source

#### Scenario: Scoring an assessed row
- **WHEN** `compliance-score` reads a row written after this change
- **THEN** it finds the same candidate passages as before, and does not count the verdict lines as passages
