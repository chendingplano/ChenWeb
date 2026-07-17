## ADDED Requirements

### Requirement: Findings record their generating run
Every finding persisted to `kb.doc_review_findings` SHALL carry the ID of the review run that produced it, stored as `run_id` in the `metadata` JSONB column, mirroring the value already stored in the row's real `run_id` column.

#### Scenario: Finding saved during a review run
- **WHEN** `ReviewFindingsSQLStore.SaveFindings` persists a finding for run `runID`
- **THEN** the persisted finding's `metadata.run_id` equals `runID`

#### Scenario: Run ID surfaced on the API response
- **WHEN** a client fetches findings through the doc-review findings API
- **THEN** each `FindingItem` in the response includes `run_id` matching the value stored in `metadata.run_id`, without the client needing to parse `metadata`

#### Scenario: Historical findings without a run ID in metadata
- **WHEN** a finding row was persisted before this change and has no `run_id` key in `metadata`
- **THEN** the API SHALL return that finding with `run_id` as zero rather than erroring
