## ADDED Requirements

### Requirement: Raw-preserved semantic assertions are created for instantiable families

When an artifact family supports ontology instances, normalization SHALL always create or reuse a
`kb.semantic_assertions` row, whether or not normalization succeeded.

#### Scenario: Unnormalizable artifact still becomes an instance
- **WHEN** an artifact of an instantiable family cannot be fully normalized
- **THEN** a `kb.semantic_assertions` row is created or reused with explicit non-success states

#### Scenario: Admission is not endorsement
- **WHEN** a claim is admitted into `kb.semantic_assertions`
- **THEN** it is represented, and it is not thereby accepted as true or conformant

### Requirement: The cross-family minimum assertion field and state contract

An assertion SHALL carry, directly or through linked validation or outcome records:
`instance_of_term_id`, `class_identity_state_term_id`, `mapping_resolution_state_term_id`,
`value_state_term_id`, `conformance_state_term_id`, `status`, `unsupported_prior_status`, raw
text/payload, normalized value fields when available, processing error details or linked outcome IDs,
and an optional `normalized_against_contract_revision_id`. Artifact-family specs MAY add optional
audit or domain fields but SHALL NOT omit or redefine these axes.

#### Scenario: All required axes are present
- **WHEN** any family writes an assertion
- **THEN** every field and state axis in the minimum contract is present or resolvable through a linked record

#### Scenario: Family cannot redefine an axis
- **WHEN** a family adapter declares its assertion shape
- **THEN** conformance fails if it omits or redefines a contract axis

### Requirement: Independent state dimensions are modelled separately

The system SHALL keep execution, assertion lifecycle, class identity, class definition/capability,
mapping resolution, value, conformance, evidence role, inter-instance relation, and source
authority/truth as independent dimensions that can coexist. The underscore forms SHALL be the
normative machine identifiers and governed-term local names; persisted state, API payloads, dependency
fingerprints, and canonical serialization SHALL use them exactly.

#### Scenario: Coexisting states on one assertion
- **WHEN** an assertion is `represented` with class identity `resolved_existing`, value `datatype_mismatch`, and conformance `contract_violation`
- **THEN** all four states are persisted independently and none overwrites another

#### Scenario: Hyphenated alias is rejected
- **WHEN** a persisted or API machine field receives a hyphenated or natural-language state label
- **THEN** the write is rejected

#### Scenario: Class-identity state and finding term are distinct
- **WHEN** an identity evidence conflict is detected
- **THEN** the class-identity state is `candidate_evidence_conflict` and the processing finding term is `semantic:identity_evidence_conflict`
- **AND** the finding term is not stored in the state field

#### Scenario: Evidence roles use exact identifiers
- **WHEN** an evidence role is persisted or returned
- **THEN** it is exactly `supports` or `contradicts`, with governed term IDs `semantic:evidence_supports` and `semantic:evidence_contradicts`

### Requirement: Mapping resolution and value parsing are independent

Mapping resolution state and value state SHALL be evaluated independently. When the literal itself
parses successfully, its normalized numeric value and `value_state = present` SHALL remain populated
even though bucket or type-dependent fields remain empty. `value_state = unparsed` SHALL be used only
when the literal value itself cannot be parsed.

#### Scenario: Parsed literal with unresolved mapping
- **WHEN** a literal parses but its governed mapping is missing or proposed
- **THEN** `mapping_resolution_state = unresolved`, `value_state = present`, and bucket fields are empty

#### Scenario: Ambiguous mapping does not fan out
- **WHEN** a governed range-type mapping is ambiguous
- **THEN** one assertion is created with `mapping_resolution_state = ambiguous`, a `mapping_ambiguous` finding, and no authoritative bucket
- **AND** candidate buckets and evidence remain in the mapping decision and finding details

### Requirement: Ambiguous classes resolve to one deterministic provisional class

When several classes are plausible, the pipeline SHALL create one deterministic provisional class for
the occurrence or identity cluster, store its ID in `instance_of_term_id`, record
`class_identity_state = ambiguous_candidates`, and record every candidate term, score, method, and
evidence in the class-resolution decision table. No candidate class SHALL populate authoritative
class-derived normalized fields before resolution.

#### Scenario: Ambiguous class produces one instance
- **WHEN** class resolution yields multiple plausible candidates
- **THEN** exactly one assertion exists with a deterministic provisional `instance_of_term_id` and one decision listing all candidates

#### Scenario: Candidate fields stay non-authoritative
- **WHEN** class identity is `ambiguous_candidates`
- **THEN** class-derived normalized fields remain unpopulated

#### Scenario: No existing class resolves
- **WHEN** no existing class matches
- **THEN** the pipeline creates a provisional class rather than dropping the artifact

### Requirement: The assertion payload constraint is value-state aware

The constraint requiring an object reference or literal SHALL be replaced by a constraint requiring
the payload appropriate to the governed value state: `present` requires a normalized literal or
reference; `unparsed`, `datatype_mismatch`, or raw-valued `unknown` require a raw payload; `missing`
requires subject, class, applicability, and evidence but forbids a fabricated object;
`not_applicable` requires explicit non-applicability context.

#### Scenario: Missing value instance is representable
- **WHEN** a source mentions a metric but supplies no value
- **THEN** an assertion with `value_state = missing`, no fabricated literal, and required subject, class, applicability, and evidence is accepted

#### Scenario: Present value without a literal is rejected
- **WHEN** an assertion declares `value_state = present` with neither a normalized literal nor a reference
- **THEN** the write is rejected

#### Scenario: Unparsed value without raw payload is rejected
- **WHEN** an assertion declares `value_state = unparsed` with no raw payload
- **THEN** the write is rejected

### Requirement: Lossless ingestion writes the represented lifecycle status

Lossless ingestion SHALL write `status = 'represented'` and SHALL NOT write `accepted` merely because
parsing or normalization succeeded. A claim selected for governance SHALL enter `candidate`, after
which the existing governed review path applies.

#### Scenario: Successful normalization yields represented
- **WHEN** an artifact normalizes cleanly through the lossless writer
- **THEN** its assertion status is `represented`, not `accepted`

#### Scenario: Governance path is explicit
- **WHEN** a represented claim is selected for governance
- **THEN** it transitions `represented → candidate → in_review` before any accept or reject decision

### Requirement: Evidence loss and restoration preserve the prior lifecycle status

When the final active supporting evidence link is removed from a `represented`, `candidate`,
`in_review`, `deferred`, or `accepted` claim, the system SHALL write that status to
`unsupported_prior_status` and transition to `unsupported`. Restoring qualifying evidence SHALL return
the claim to the recorded status and clear the prior-status field, and SHALL NOT promote a represented
claim to accepted or advance an in-progress governance decision. `rejected` and `superseded` SHALL NOT
transition merely because evidence changes.

#### Scenario: Last evidence lost from an in_review claim
- **WHEN** the final supporting evidence link of an `in_review` claim is removed
- **THEN** `unsupported_prior_status = 'in_review'` and `status = 'unsupported'`

#### Scenario: Evidence restored returns to prior status
- **WHEN** qualifying evidence is restored to an unsupported claim
- **THEN** the claim returns to its `unsupported_prior_status` value and the prior-status field is cleared

#### Scenario: Restoration never escalates governance
- **WHEN** evidence is restored to a claim whose prior status was `represented`
- **THEN** the claim returns to `represented` and is not promoted to `accepted`

#### Scenario: Rejected claims ignore evidence changes
- **WHEN** evidence is lost from or restored to a `rejected` or `superseded` claim
- **THEN** its status does not change

### Requirement: unsupported_prior_status is database-constrained

The database SHALL permit `unsupported_prior_status` only when `status = 'unsupported'`, SHALL require
it for unsupported rows created by evidence loss, and SHALL restrict its value to `represented`,
`candidate`, `in_review`, `deferred`, or `accepted`.

#### Scenario: Prior status on a non-unsupported row is rejected
- **WHEN** a row with `status <> 'unsupported'` sets `unsupported_prior_status`
- **THEN** the write is rejected

#### Scenario: Illegal prior status value is rejected
- **WHEN** `unsupported_prior_status` is set to `rejected` or `superseded`
- **THEN** the write is rejected

### Requirement: Lifecycle transitions are non-identity-bearing revisions

Lifecycle transitions SHALL be assertion-owned, non-identity-bearing changes. They SHALL append a new
assertion revision under the same `claim_id` and atomically advance the claim registry's
`current_assertion_id`. Evidence loss SHALL leave the removed link as deleted history and create the
unsupported revision. Evidence restoration SHALL create a new active evidence link targeting the
restored-status revision rather than undeleting a link to an obsolete revision.

#### Scenario: Transition creates a revision
- **WHEN** an assertion's lifecycle status changes
- **THEN** a new revision is appended under the same `claim_id` and the claim registry's current assertion advances atomically

#### Scenario: Removed link remains history
- **WHEN** evidence loss creates an unsupported revision
- **THEN** the removed evidence link remains as deleted history and is not repointed

#### Scenario: Remaining evidence is relinked, not mutated
- **WHEN** a restored-status revision is created and other active evidence exists
- **THEN** new current links to the new revision are created and prior evidence rows remain immutable history

### Requirement: Lifecycle support is deployed before lossless writes are enabled

The migration SHALL add `represented`, its legal transitions, and `unsupported_prior_status` to the
database and the state-machine implementation before any lossless write is enabled. Every consumer
currently filtering on `status = 'accepted'` SHALL receive an explicit documented policy before
lossless assertions become consumer-visible.

#### Scenario: Schema precedes writers
- **WHEN** the lossless writer gate is evaluated
- **THEN** activation is refused unless `represented` and `unsupported_prior_status` are present in the schema and state machine

#### Scenario: Accepted-status consumers are audited
- **WHEN** the lifecycle migration is deployed
- **THEN** every consumer filtering on `status = 'accepted'` is enumerated with an assigned, documented policy
