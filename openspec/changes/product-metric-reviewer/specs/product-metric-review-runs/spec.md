## ADDED Requirements

### Requirement: Review request and run lifecycle

The system SHALL persist a review request holding the profile id, the pinned `profile_version`, the
requested artifact types, optional filters, notes, and requester identity; and SHALL persist one run
per execution of that request, holding `run_number`, `status` ∈ {`pending`, `running`, `completed`,
`failed`}, start and end times, result counts, and an error message when failed. A request MAY have
many runs.

#### Scenario: Creating a review starts a run

- **WHEN** a user submits a review for profile 7 at version 3 with `artifact_types = ["metric"]`
- **THEN** a request is created and a run with `run_number` 1 and `status` `pending` is created
- **AND** the run transitions to `running` and then to `completed` with its counts populated

#### Scenario: Run failure is recorded, not lost

- **WHEN** retrieval errors partway through a run
- **THEN** the run's `status` becomes `failed` with a non-empty `error_message`
- **AND** the request remains re-runnable

#### Scenario: Run against a draft profile is refused

- **WHEN** a review is requested for a profile whose `status` is `draft`
- **THEN** the request is rejected with `CWB_KB_PMR_030`
- **AND** no run is created

### Requirement: Result persistence with provenance

Each result row SHALL persist `run_id`, `artifact_type`, `artifact_id`, `source_row_id`,
`input_record_id`, the matched `node_id`, `tier`, `score`, `paths`, `inclusion_reason`, and the
artifact's source line spans. Results SHALL be readable filtered by node, tier, path, document, and
artifact type, and SHALL be sortable by score.

#### Scenario: Result carries citable provenance

- **WHEN** a completed run's results are read
- **THEN** each row names its document and carries the source line spans needed to cite it

#### Scenario: Results filtered by node

- **WHEN** results are requested with `node_id` set to the `Display` node
- **THEN** only results whose matched node is `Display` are returned

#### Scenario: Results filtered by tier

- **WHEN** results are requested with `tier=document_scope` excluded
- **THEN** only `direct`, `part`, and `aspect` results are returned

### Requirement: Scoped document set persistence

Each run SHALL persist the document scope set it computed: `input_record_id`, fused score, matching
node ids, matching paths, and the document kind used for the boost. The set SHALL be readable
independently of the results.

#### Scenario: Reviewer inspects why a document was in scope

- **WHEN** a run's scoped documents are read
- **THEN** each row names the nodes that matched it and the paths that matched
- **AND** a document scoped only by a `storage_requirement` product record shows the `storage`
  aspect node as its matching node

### Requirement: Coverage and gap reporting

Each completed run SHALL produce a report containing a per-node coverage table — node id, label,
kind, grounding state, artifact count, document count — and an explicit gap list of accepted nodes
with zero artifacts. The report SHALL count attributed results (tiers `direct`, `part`, `aspect`)
separately from `document_scope` results. The report SHALL be stored on the run as structured JSON
and as rendered Markdown.

#### Scenario: Node with no metrics is reported as a gap

- **WHEN** the accepted node `Humidifier` matched no artifacts in a completed run
- **THEN** the run's report lists `Humidifier` in its gaps with its grounding state

#### Scenario: Coverage separates attributed from scoped-only counts

- **WHEN** a run's report is read
- **THEN** each node's attributed count excludes `document_scope` results
- **AND** the run-level totals report both counts

#### Scenario: Report is available in both forms

- **WHEN** a completed run is read
- **THEN** both the structured report and the rendered Markdown are present on the run

### Requirement: Re-run and diff

The system SHALL support re-running a request, creating a new run with the next `run_number` under
the same request. A run SHALL be readable as a diff against the previous run of the same request,
reporting artifacts added, artifacts removed, and artifacts whose tier changed. A diff SHALL report
whether the compared runs used different `profile_version` values.

#### Scenario: New documents surface as added artifacts

- **WHEN** a request is re-run after new documents were ingested
- **THEN** the diff against the previous run lists the newly matched artifacts as added

#### Scenario: Diff flags a profile change

- **WHEN** two compared runs used `profile_version` 3 and 4
- **THEN** the diff reports the version difference so scope changes are not read as corpus changes

#### Scenario: Stable corpus yields an empty diff

- **WHEN** a request is re-run with no corpus change and no profile edit
- **THEN** the diff reports no additions, no removals, and no tier changes

### Requirement: Read and export endpoints

The system SHALL expose endpoints to create and list profiles and runs; to read one run with its
counts and report; to read a run's results, scoped documents, and diff; and to read the configured
aspect vocabulary with locale resolution. Results SHALL be exportable in a tabular form carrying
metric identity, value, unit, document, line spans, matched node, tier, and inclusion reason.

#### Scenario: Aspect vocabulary is served with locale fallback

- **WHEN** the aspect vocabulary is requested with `?lang=zh_cn` and an aspect has no `name_zh_cn`
- **THEN** that aspect's `name_en` is returned in its place

#### Scenario: Export carries citation columns

- **WHEN** a completed run's results are exported
- **THEN** every exported row includes its document and source line spans
