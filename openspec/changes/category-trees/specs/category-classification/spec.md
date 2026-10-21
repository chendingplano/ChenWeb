## ADDED Requirements

### Requirement: Proposals are evidence only
Classification SHALL read the artifact's LLM category proposals (for metrics,
`category_paths_en`, falling back to `category_paths`, plus the category keys in
`metric_categories_en`, falling back to `metric_categories`), pass them to the decision model, and
store them with the classification record. Proposals SHALL never be bound directly.

#### Scenario: Proposal names a missing category
- **WHEN** a metric proposes "Physical > Colour" and the tree has no such category
- **THEN** no category is created and the proposal is kept in the classification record

### Requirement: Verification walk
The system SHALL walk the artifact's tree from the root breadth-first. For each inner node it
SHALL ask the decision model about its ordinary children in batches of at most R (default 20),
one request per batch, and classify each child's answer as match (P ≥ match threshold), no-match
(P ≤ no-match threshold) or uncertain. Only supplied child IDs SHALL be accepted. When any child
of a parent matches, each matched child SHALL be visited; when every child is a definite
no-match, the parent's "others" leaf SHALL join the direct memberships; a reached ordinary leaf
SHALL join the direct memberships.

#### Scenario: Match in one batch only
- **WHEN** a parent has 25 children, one child in the second batch matches and every other answer is a definite no-match
- **THEN** the matched child is visited and the parent's "others" leaf is not a membership

#### Scenario: Multiple branches
- **WHEN** two children of the root match and each leads to a matching leaf
- **THEN** both leaves become direct memberships

#### Scenario: Nothing fits at the root
- **WHEN** every child of the root is a definite no-match
- **THEN** the root's "others" leaf is the only direct membership

### Requirement: Failures and uncertainty are never non-matches
A failed request (after retries), an invalid response, or an uncertain answer SHALL NOT be
treated as a no-match. If any parent is unresolved, the attempt SHALL end as `failed` (a request
failed) or `in_review` (uncertain answers or exhausted request budget), with the reason
recorded, and SHALL NOT publish or change any binding.

#### Scenario: A batch fails
- **WHEN** one of a parent's two batch requests fails after retries and the other has no match
- **THEN** the attempt is `failed`, the artifact is not put into "others", and its previously published bindings are unchanged

#### Scenario: Budget exhausted
- **WHEN** the walk reaches the per-artifact request limit before finishing
- **THEN** the attempt is `in_review` with reason `budget_exhausted` and nothing is published

### Requirement: Publication
A completed walk SHALL replace the artifact's bindings with its direct memberships and mark the
classification `published`, in one transaction. Each binding SHALL record the node, artifact
type and ID, record ID, classification ID and tree sequence number. The artifact's associations
SHALL be its direct memberships plus every ancestor of them, derived from parent links and
deduplicated.

#### Scenario: Associations
- **WHEN** an artifact is published with leaves A/B/x and A/C/y
- **THEN** reading its bindings returns x and y as direct memberships and A, B, C and the root as ancestors, each once

#### Scenario: Category members
- **WHEN** a user lists the members of inner node A
- **THEN** every artifact with a direct membership in A's subtree is returned

### Requirement: Classification records
Every attempt SHALL be stored in `kb.category_classifications` with artifact type and ID, record
ID, tree ID and sequence number, status (`pending`, `published`, `failed`, `in_review`,
`unclassified`), reason, proposals, decision evidence (per request: parent, child IDs,
probabilities, outcome), and model and prompt references.

#### Scenario: Evidence kept
- **WHEN** an attempt is published
- **THEN** its record holds every decision request's probabilities and the model and prompt references

### Requirement: Running classification
The API SHALL classify all current artifacts of a record for an artifact type in the background,
removing first the bindings and classifications of that record whose artifacts no longer exist.
It SHALL also reclassify artifacts with stale bindings. Classification SHALL fail with a clear
error when the artifact type has no bound tree or no decision model is configured.

#### Scenario: Re-extracted record
- **WHEN** a record's metrics were re-extracted with new IDs and classification is run for the record
- **THEN** bindings of the old metric IDs are deleted and the new metrics are classified

#### Scenario: No decision model configured
- **WHEN** `CATEGORY_DECISION_MODEL` is unset and classification is requested
- **THEN** the profile `deepseek-flash-decision-model` is used

### Requirement: Review queue
The system SHALL list the latest attempt per artifact whose status is `failed` or `in_review`,
with its reason. A reviewer SHALL be able to `retry` (start a new attempt), `close` (set
`unclassified`; publish nothing), or `resolve` by supplying leaf node IDs of the current tree,
which are published with the reviewer recorded in the evidence.

#### Scenario: Resolve with a non-leaf
- **WHEN** a reviewer resolves an attempt with an inner node's ID
- **THEN** the request is rejected and nothing is published

#### Scenario: Close
- **WHEN** a reviewer closes an attempt
- **THEN** its status becomes `unclassified`, it leaves the queue, and the artifact's bindings are unchanged

### Requirement: Others report
The API SHALL report every "others" leaf of a tree with its number of direct members and its
parent, largest first, so reviewers can see where the tree lacks categories.

#### Scenario: Report
- **WHEN** three artifacts are filed under `root-others`
- **THEN** the report lists `root-others` with a count of 3
