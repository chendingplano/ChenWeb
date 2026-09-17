## ADDED Requirements

### Requirement: Copy metadata from the original onto a newly detected duplicate
When a new `kb.inputs` row is matched as an MD5 duplicate of an existing, already-successfully-parsed `kb.inputs` row, the system SHALL copy `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, and `parser_name` from the matched row onto the new row as part of marking it duplicated.

#### Scenario: New upload matches a fully-parsed original
- **WHEN** a newly staged file's MD5 matches an existing `kb.inputs` row whose `parse_state` is `parsed_success`
- **THEN** the new row is marked `duplicated` (status JSON records `proc_status: "duplicated"` and `dup_rcd_id`)
- **AND** the new row's `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, and `parser_name` are set to the matched row's values for those columns

### Requirement: Never copy metadata from a row that is itself a duplicate
The system SHALL NOT copy metadata onto a new duplicate row from a matched row that is not, at the time of the copy, in `parse_state = 'parsed_success'` — in particular, a matched row that is itself marked duplicated.

#### Scenario: Matched row is no longer a valid non-duplicate original
- **WHEN** a new row is being marked duplicated against a matched row id
- **AND** that matched row's current `parse_state` is not `parsed_success` (e.g. it has itself since been marked duplicated, or deleted)
- **THEN** the new row is still marked `duplicated` with the recorded `dup_rcd_id`
- **AND** none of the new row's `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, or `parser_name` columns are modified by the copy step
