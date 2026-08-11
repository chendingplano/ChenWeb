## ADDED Requirements

### Requirement: Term candidates get a wording-invariant identity key
When `CreateCandidate` inserts a row with `candidate_kind = 'term'` whose `proposed_payload`
contains a non-empty `label`, the system SHALL compute a deterministic `identity_key` from
`proposed_module_id`, the payload's `term_kind`, and the normalized set `{label} ∪ aliases`
(case-folded, trimmed, deduplicated, order-independent), and store it on the row. Rows for any
other `candidate_kind`, or `term` rows with no usable `label`, SHALL have `identity_key = NULL`.

#### Scenario: Same names, different casing/order, produce the same identity key
- **WHEN** two `term` candidates are created with `proposed_module_id="measurement"`, `term_kind="metric_definition"`, and `label`/`aliases` such that one has `label="种子发芽指数"`, `aliases=["发芽指数"]` and the other has `label="发芽指数"`, `aliases=["种子发芽指数"]`
- **THEN** both rows are stored with the same non-null `identity_key`

#### Scenario: Different term_kind values do not collide
- **WHEN** two `term` candidates share the same `proposed_module_id` and the same `label`, but one has `term_kind="metric_definition"` and the other `term_kind="concept"`
- **THEN** the two rows have different `identity_key` values

#### Scenario: Non-term candidates are unaffected
- **WHEN** a candidate is created with `candidate_kind` of `axiom`, `label`, `mapping`, `profile`, `profile_rule`, or `module_change`
- **THEN** the row's `identity_key` is `NULL`

### Requirement: Identity-key matches are recorded as a soft signal, never block or merge
After a new candidate row is created (not reused via the existing fingerprint match) with a
non-null `identity_key`, the system SHALL look up other rows sharing the same `identity_key` whose
`status` is not `rejected` or `superseded`, and SHALL append a match entry — carrying
`match_type: "candidate"`, the other row's id, and `matched_on: "identity_key"` — to the
`candidate_matches` column of both the new row and each matched row. This process SHALL NOT
prevent the insert, change any row's `status`, or modify any row's `proposed_payload`.

#### Scenario: A new candidate matches one existing candidate
- **WHEN** a `term` candidate is created whose `identity_key` matches an existing candidate's `identity_key`, and the existing candidate's `status` is `discovered`
- **THEN** the new row's `candidate_matches` includes an entry referencing the existing candidate's id, and the existing candidate's `candidate_matches` includes an entry referencing the new row's id, and the existing candidate's `status` remains `discovered`

#### Scenario: A matching candidate that is rejected or superseded is not matched against
- **WHEN** a `term` candidate is created whose `identity_key` matches only candidates with `status` in (`rejected`, `superseded`)
- **THEN** no match entry is added to either the new row or the rejected/superseded rows

#### Scenario: An exact fingerprint reuse does not trigger identity-key matching
- **WHEN** `CreateCandidate` is called with a payload/source/module combination whose fingerprint already exists, so the existing row is returned instead of a new one being created
- **THEN** no identity-key lookup or `candidate_matches` update occurs as a result of this call

#### Scenario: Insert succeeds even when a match is found
- **WHEN** a new candidate's `identity_key` matches one or more existing non-terminal-status candidates
- **THEN** the new row is still created successfully and returned to the caller, with no error and no change to its own `status`
