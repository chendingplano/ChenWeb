## ADDED Requirements

### Requirement: Approving the QUDT resource writes governed quantity terms
When an operator approves the `qudt` terminology resource on the Review External Resources page, the
system SHALL, in addition to the existing keyword-lexicon import, parse the same downloaded QUDT artifact
and write `quantity_kind`, `unit`, and `dimension` term rows into `kb.ontology_terms` for the `quantity`
module, with a preferred label into `kb.ontology_term_labels` and an exact source-IRI mapping into
`kb.ontology_mappings`, for every QUDT resource not already represented by an existing term in that module.

#### Scenario: First-time approve populates all three term kinds
- **WHEN** an operator approves the `qudt` resource for the first time (no prior `quantity` module terms exist)
- **THEN** `kb.ontology_terms` gains rows with `module_id='quantity'` for `term_kind` values `quantity_kind`, `unit`, and `dimension`, each with a `kb.ontology_term_labels` preferred-label row and a `kb.ontology_mappings` row with `relation='exact'` pointing at the source QUDT IRI

#### Scenario: Existing keyword-lexicon import behavior is preserved
- **WHEN** an operator approves the `qudt` resource
- **THEN** the existing keyword-lexicon staging tables (`kb.keyword_sources`, `kb.keyword_source_artifacts`, `kb.keyword_catalog_entries`, `kb.keyword_catalog_labels`, `kb.keyword_catalog_relations`) are populated exactly as before this change

### Requirement: Approved quantity terms are released and activated
After writing new `quantity` module term rows, the system SHALL create a new `kb.ontology_module_releases`
row for the `quantity` module containing those terms and SHALL activate it as the module's current active
release, so the terms are immediately queryable at `status='included_in_release'`.

#### Scenario: New terms trigger a release and activation
- **WHEN** approving `qudt` writes at least one new term that was not present before
- **THEN** a new `kb.ontology_module_releases` row is created for `module_id='quantity'` with a version higher than any prior release, and `kb.ontology_active_releases` is updated so this new release is the sole active release for the `quantity` module

#### Scenario: No new terms means no new release
- **WHEN** approving `qudt` writes zero new terms (every parsed term already exists in the `quantity` module)
- **THEN** no new `kb.ontology_module_releases` row is created and the previously active release, if any, remains active and unchanged

### Requirement: Re-approving identical QUDT content is idempotent
Approving the `qudt` resource with content whose checksum matches a previously-approved import SHALL NOT
create duplicate term, label, or mapping rows, and SHALL NOT create an empty or redundant module release.

#### Scenario: Re-approve with unchanged content
- **WHEN** an operator approves the `qudt` resource again with an artifact whose checksum matches the last successfully-approved artifact
- **THEN** no new rows are inserted into `kb.ontology_terms`, `kb.ontology_term_labels`, or `kb.ontology_mappings`, and no new `quantity` module release is created

### Requirement: Ontology write failures are safely retryable
If the governed-term write fails after the keyword-lexicon import for the same approve request has already
succeeded, the system SHALL return an error for the approve request while leaving the keyword-lexicon
state intact, and a subsequent approve of the same resource SHALL retry only the incomplete portion of the
work without duplicating already-written data.

#### Scenario: Ontology write fails after keyword-lexicon import succeeds
- **WHEN** the keyword-lexicon import for an approve request completes successfully but the subsequent governed-term write fails
- **THEN** the approve request returns an error, the keyword-lexicon tables retain the successfully-imported data, and no partial or inconsistent `kb.ontology_terms`/`kb.ontology_term_labels`/`kb.ontology_mappings` rows are left behind for the terms that failed to write

#### Scenario: Retry after a failed ontology write completes the work
- **WHEN** an operator re-approves the same `qudt` resource after a prior approve failed partway through the governed-term write
- **THEN** the keyword-lexicon checksum short-circuit skips re-importing already-imported keyword-lexicon data, and the governed-term write resumes by inserting only the terms still missing from `kb.ontology_terms`

### Requirement: Governed-term writes are scoped to the QUDT resource
Approving a terminology resource other than `qudt` SHALL NOT write to `kb.ontology_terms`,
`kb.ontology_term_labels`, `kb.ontology_mappings`, or trigger a `quantity` module release.

#### Scenario: Approving a non-QUDT resource
- **WHEN** an operator approves a terminology resource whose source is not `qudt`
- **THEN** only the existing keyword-lexicon import runs, and no `kb.ontology_*` tables are written and no module release is created
