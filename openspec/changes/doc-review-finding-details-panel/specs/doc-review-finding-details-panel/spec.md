## ADDED Requirements

### Requirement: LINES list is hidden only in the doc-review-report embed
When `DocStructureView` is rendered by the doc-review-report page, the sidebar SHALL show the Finding Details panel instead of the LINES list. When `DocStructureView` is rendered by any other page (standalone Document Structure page, Knowledge page), the sidebar SHALL continue to show the LINES list exactly as before this change.

#### Scenario: Doc review report hides the LINES list
- **WHEN** a user opens `/home3/doc-review-report/:id`
- **THEN** the Document Structure panel's sidebar shows the Finding Details panel, not the LINES list, search box, or line-list settings button

#### Scenario: Standalone Document Structure page is unaffected
- **WHEN** a user opens `/home3/doc-structure`
- **THEN** the sidebar shows the LINES list, search box, and settings button exactly as before this change

### Requirement: Finding block shows the selected finding's fields
The Finding Details panel SHALL show a foldable block, expanded by default, titled to identify it as the Finding block, containing the following fields of the currently selected finding: `artifact_id`, `aspect`, `severity`, `finding_type`, `title`, `description`, `suggestion`, `confidence`, `location`, `reference_doc`, and a `metadata` field rendered as a button.

#### Scenario: No finding selected
- **WHEN** no finding has been clicked yet on the doc-review-report page
- **THEN** the Finding Details panel shows an empty/placeholder state instead of the four blocks

#### Scenario: Finding selected
- **WHEN** a user clicks a finding in the left findings list
- **THEN** the Finding block updates to show that finding's `artifact_id`, `aspect`, `severity`, `finding_type`, `title`, `description`, `suggestion`, `confidence`, `location`, and `reference_doc` as name-value pairs

#### Scenario: Metadata dialog is recursive
- **WHEN** a user clicks the `metadata` button in the Finding block
- **THEN** a dialog opens showing the finding's `metadata` JSON in name-value form, with nested objects rendered recursively with indentation rather than as a single stringified value

### Requirement: Artifact block shows the source artifact and its object node
When the selected finding's `artifact_id` is non-empty, the Finding Details panel SHALL show a foldable block, expanded by default, with the artifact record (metric, provision, or inventory item, per `artifact_id`'s type) in name-value form. If the artifact is linked to a `kb.object_nodes` record, the block SHALL also show that record's `canonical_name`.

#### Scenario: Finding with an artifact
- **WHEN** the selected finding has a non-empty `artifact_id`
- **THEN** the Artifact block shows that artifact's fields in name-value form

#### Scenario: Finding without an artifact
- **WHEN** the selected finding's `artifact_id` is empty
- **THEN** the Artifact block is not shown

#### Scenario: Artifact linked to an object node
- **WHEN** the selected artifact is linked to a `kb.object_nodes` record
- **THEN** the Artifact block includes that record's `canonical_name`

### Requirement: Doc Review Log block shows the matching review-log row
The Finding Details panel SHALL show a foldable block, folded by default, containing the `kb.doc_review_logs` row where `unit_key` equals the selected finding's `artifact_id`, or equals `artifact_id` followed by a `#`-prefixed suffix (the format written by artifact-scoped reviewers, e.g. `"244_mtc_1#31082"`), and `run_id` equals the finding's `run_id`. It SHALL show `unit_key`, `matched_units`, `findings`, `outcome`, and `detail` as name-value pairs, with `matched_units`, `findings`, and `detail` each rendered as a button that opens a dialog.

#### Scenario: Matching log row exists
- **WHEN** a `kb.doc_review_logs` row exists with `unit_key` equal to `finding.artifact_id` (optionally followed by a `#suffix`) and `run_id = finding.run_id`
- **THEN** the Doc Review Log block shows that row's `unit_key`, `matched_units`, `findings`, `outcome`, and `detail`, with dialog buttons for `matched_units`, `findings`, and `detail`

#### Scenario: No matching log row
- **WHEN** no `kb.doc_review_logs` row matches the selected finding's `artifact_id` (with or without a `#suffix`) and `run_id`
- **THEN** the Doc Review Log block shows an empty state instead of fields

### Requirement: LLM Calls block lists the review-log's LLM usage events
The Finding Details panel SHALL show a foldable block, folded by default, listing the `public.llm_usage_event` rows referenced by the matched `kb.doc_review_logs` row's `detail.llm_usage_event_ids` array. Clicking a list entry SHALL open a dialog showing that row's fields in name-value form.

#### Scenario: LLM usage events present
- **WHEN** the matched `kb.doc_review_logs` row's `detail.llm_usage_event_ids` is a non-empty array
- **THEN** the LLM Calls block lists one entry per id, and clicking an entry opens a dialog with that `llm_usage_event` row's fields

#### Scenario: No LLM usage events
- **WHEN** `detail.llm_usage_event_ids` is empty, missing, or no matching `kb.doc_review_logs` row exists
- **THEN** the LLM Calls block shows an empty state instead of a list
