## ADDED Requirements

### Requirement: Downstream stages continue with capability-aware behavior

A downstream processor SHALL NOT treat a flagged instance as nonexistent. It SHALL perform the
operations supported by the instance and its class capabilities and SHALL record an explicit outcome
for operations it cannot perform. "Do nothing" SHALL NOT be an acceptable terminal behavior.

#### Scenario: Flagged instance is still processed
- **WHEN** a downstream stage encounters an instance with non-success semantic states
- **THEN** it performs every operation its capabilities support

#### Scenario: Unsupported operation records a reason
- **WHEN** a downstream stage cannot perform its semantic operation
- **THEN** it records why, preserves its inputs, and leaves a retryable outcome when appropriate

### Requirement: Search indexes raw and normalized representations

Search SHALL index both raw and normalized text when available, so a flagged instance is findable by
its raw wording and by its normalized wording.

#### Scenario: Flagged instance found by raw wording
- **WHEN** a user searches using the source's exact wording for an unparsed metric
- **THEN** the flagged instance is returned

#### Scenario: Normalized instance found by normalized wording
- **WHEN** a normalized value exists
- **THEN** the instance is findable by its normalized wording as well

### Requirement: Comparison records an explicit no-verdict

Comparison SHALL record `no_verdict` or `incomparable_with` with a reason when required normalization
or class capabilities are missing, and SHALL NOT drop unsupported instances.

#### Scenario: Missing normalization yields no_verdict
- **WHEN** a comparison rule requires a normalized numeric interval and the value is unparsed
- **THEN** the comparison records `no_verdict` with a reason and the instance is not dropped

#### Scenario: Incomparable instances are recorded
- **WHEN** two instances cannot be compared due to absent capabilities
- **THEN** an `incomparable_with` relation with a reason is recorded

### Requirement: Observed class profiles include outliers without granting authority

Observed class profile aggregation SHALL include malformed, represented, and outlier observations and
SHALL NOT promote them into the authoritative class contract.

#### Scenario: Outlier included in observed profile
- **WHEN** a malformed observation exists for a class
- **THEN** it appears in the observed profile aggregation

#### Scenario: Outlier does not become a contract rule
- **WHEN** an observed profile is used to derive an authoritative contract
- **THEN** malformed and outlier observations are not promoted into it

### Requirement: Review Document displays flagged instances with their states

Review Document SHALL display the claim, its raw value, its independent states, its errors, class
confidence, and evidence, rather than omitting flagged instances.

#### Scenario: Flagged metric is visible
- **WHEN** a metric has an unresolved mapping or an unparsed value
- **THEN** Review Document displays it with its raw value, states, errors, and evidence

#### Scenario: Findings are not shown as document failures
- **WHEN** a document processed with semantic findings is displayed
- **THEN** the derived "completed with findings" label is not presented as a processing failure

### Requirement: Completeness checks distinguish absence from missing value

Completeness checks SHALL distinguish an absent artifact from a present artifact with a missing value.

#### Scenario: Missing value is not absence
- **WHEN** a source mentions a metric but supplies no value
- **THEN** completeness reports a present artifact with a missing value, not an absent artifact

### Requirement: Every consumer has an explicit assigned lifecycle policy

Governance decisions and profile-rule evaluation requiring endorsed truth SHALL continue to require
`accepted` assertions. Observed class-profile aggregation SHALL include represented, malformed, and
outlier observations. Search, semantic discovery, diagnostic projections, and Review Document SHALL
expose represented assertions with their warnings. Every other consumer SHALL choose and document one
behavior. No consumer SHALL silently treat `represented` as `accepted` or as absent.

#### Scenario: Governance stays accepted-only
- **WHEN** a governance decision or profile-rule evaluation runs
- **THEN** it considers only `accepted` assertions

#### Scenario: Discovery exposes represented with warnings
- **WHEN** search, semantic discovery, diagnostics, or Review Document returns results
- **THEN** represented assertions are included and carry their warnings

#### Scenario: Undocumented consumer fails certification
- **WHEN** a consumer has no documented lifecycle policy
- **THEN** it fails the reader compatibility certification

#### Scenario: Consumers can filter explicitly
- **WHEN** a consumer chooses to include, warn about, or exclude findings
- **THEN** that choice is expressed through explicit policy rather than implicit filtering

### Requirement: Readers tolerate both legacy and new states before writers are enabled

APIs, semantic projection, search, comparison, Review Document, reports, and retry tooling SHALL
tolerate both legacy assertions and every new raw-preserved, ambiguous, missing, represented, and
unsupported state. Consumer APIs SHALL add state fields before changing default filtering behavior.
Dual-read behavior SHALL be certified against a reader compatibility suite, including
represented/unsupported restoration and assertion redirects, before any writer gate is enabled.

#### Scenario: Legacy assertion still readable
- **WHEN** a dual-read consumer encounters a pre-existing legacy assertion
- **THEN** it renders correctly with no new state fields required

#### Scenario: New state readable before writers exist
- **WHEN** a dual-read consumer encounters a raw-preserved, missing-value, or unsupported assertion
- **THEN** it renders correctly with the new state exposed

#### Scenario: Certification gates the writer
- **WHEN** a writer gate is enabled before all required consumers pass dual-read certification
- **THEN** activation is refused

#### Scenario: Old consumers never meet new writers
- **WHEN** mixed-version operation occurs
- **THEN** the only permitted sequence is new dual-read consumers with old writers, followed by gated new writers

### Requirement: Failure and finding affect downstream branches differently

After a genuine upstream execution failure, only the affected dependency branch SHALL stop and enter
operational retry. After a semantic finding, capable downstream branches SHALL continue and incapable
operations SHALL persist an explicit no-result reason.

#### Scenario: Execution failure stops one branch
- **WHEN** an upstream processor fails operationally
- **THEN** only the dependent branch stops and enters operational retry

#### Scenario: Semantic finding does not stop branches
- **WHEN** an upstream stage completes with findings
- **THEN** capable downstream branches continue and incapable operations record an explicit no-result reason
