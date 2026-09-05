## ADDED Requirements

### Requirement: Contract synthesis requires unambiguous, multi-document agreement
The system SHALL promote a metric class contract from `identity_only` to `partially_defined` only when its recorded, non-malformed observed-profile evidence agrees on exactly one value datatype and one unit across at least two distinct documents. The system SHALL NOT promote a contract from a single document's evidence, and SHALL NOT promote when more than one distinct (datatype, unit) pair is observed.

#### Scenario: Two documents agree, contract is promoted
- **WHEN** a metric class has `present`-state observed-profile evidence naming the same numeric
  datatype and the same unit term from two different documents
- **THEN** the system SHALL append a `partially_defined` contract revision declaring that datatype
  and unit, with `synthesis_method` recording that it was automatic

#### Scenario: Single document is insufficient
- **WHEN** a metric class has `present`-state observed-profile evidence from only one document
- **THEN** the system SHALL leave the contract `identity_only` and SHALL NOT append a revision

#### Scenario: Conflicting evidence blocks promotion
- **WHEN** a metric class has `present`-state observed-profile evidence naming two different units
  across documents
- **THEN** the system SHALL leave the contract `identity_only` and SHALL NOT guess a unit

#### Scenario: Malformed evidence is excluded from the agreement check
- **WHEN** a metric class has observed-profile evidence in a non-`present` observation state
  (`unparsed`, `missing`, or an exception)
- **THEN** that evidence SHALL NOT count toward the unambiguous-agreement determination, though it
  SHALL remain visible in the observed profile

### Requirement: Synthesis never automatically reverses a promoted contract
Once a contract has been promoted beyond `identity_only`, the system SHALL NOT automatically revert or overwrite it in response to later contradicting evidence. A specific occurrence that contradicts an already-promoted contract SHALL remain visible for review through that occurrence's own conformance state, not through a silent, unreviewable rewrite of the contract.

#### Scenario: Later contradicting evidence does not revert a promoted contract
- **WHEN** a metric class's contract is already `partially_defined` and a new observation names a
  different unit
- **THEN** the contract SHALL remain unchanged and SHALL NOT be replaced by a new automatic revision

#### Scenario: A contradicting occurrence is flagged on the occurrence, not hidden
- **WHEN** a metric instance's resolved unit disagrees with its class's already-`partially_defined`
  contract
- **THEN** that instance's own conformance state SHALL record the disagreement (per
  `metric-capability-validation`'s per-instance conformance requirement), rather than the
  disagreement being absorbed into the contract or lost
