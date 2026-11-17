## ADDED Requirements

### Requirement: Tie detection uses every top-scored candidate
Object reconciliation SHALL order candidates by score, highest first, before deciding. A tie
SHALL be two or more candidates sharing the top score, whatever order the database returned them
in. The non-tie branches SHALL use the highest-scored candidate.

#### Scenario: Higher score listed second
- **WHEN** the database returns one candidate scored 0.85 followed by one scored 1.0
- **THEN** reconciliation matches the 1.0 candidate and does not create a new node

#### Scenario: Tie behind a lower score
- **WHEN** the database returns candidates scored 0.85, 1.0 and 1.0, in that order
- **THEN** reconciliation treats the two 1.0 candidates as tied

### Requirement: Ties are linked to the tie-break pick
Reconciliation SHALL link a tied artifact object to the candidate chosen by the deterministic
tie-break when the tie remains after the LLM adjudicator (if configured) and the existing
re-check against merged nodes. The tie-break runs over the tied candidates only, with `reconcile_status =
'ambiguous_resolved'` and `ext_info.reconcile_method = 'tie_break_deterministic'`. It SHALL NOT
save the artifact object with an empty `object_id` because of a tie. A tie the LLM resolves with
sufficient confidence SHALL keep the LLM's result and SHALL NOT take the tie-break.

#### Scenario: Tie without LLM adjudication
- **WHEN** an artifact object ties between two nodes and no LLM adjudicator is configured
- **THEN** the saved artifact object has the tie-break pick's `object_id`, status
  `ambiguous_resolved` and method `tie_break_deterministic`

#### Scenario: LLM declines
- **WHEN** the LLM adjudicator answers below the confidence threshold for a tie
- **THEN** the artifact object is linked to the tie-break pick

#### Scenario: LLM resolves
- **WHEN** the LLM adjudicator resolves a tie with sufficient confidence
- **THEN** the artifact object keeps the LLM's choice and no attention item is recorded for it

#### Scenario: Tie-break ignores lower-scored candidates
- **WHEN** two candidates tie at 1.0 and a third scored 0.85 shares more names with the object
- **THEN** the pick is one of the two 1.0 candidates

### Requirement: Each tie raises an ambiguous-object-name attention item
Whenever a tie is linked by the tie-break, the system SHALL record an attention item of type
`ambiguous-object-name` whose dedupe key is the sorted tied `object_id`s, and whose `details`
hold the object name, every tied candidate as seen at detection time (object ID, canonical name,
object type, score, method), the picked object ID, the LLM outcome (`not_configured`,
`unresolved` or `failed`), and the most recent occurrences (record ID, artifact type, artifact
ID, object name, time), capped at 50. It SHALL log `object reconciliation tie broken` at INFO
level with the item ID, and SHALL NOT log `object reconciliation ambiguous` at WARN level. A
failure to record the item SHALL be logged at ERROR level and SHALL NOT fail the record's
processing.

#### Scenario: Same tie in two documents
- **WHEN** records 279 and 383 each produce an artifact object tying the same two nodes
- **THEN** one open attention item exists for that node pair, with `occurrence_count` 2 and both
  occurrences in `details`

#### Scenario: Different names, same tied nodes
- **WHEN** two differently named artifact objects tie the same set of nodes
- **THEN** both are recorded on the same attention item

#### Scenario: Attention store unavailable
- **WHEN** recording the attention item fails
- **THEN** the artifact object is still saved linked to the tie-break pick, and an ERROR is logged

### Requirement: Resolve Ambiguous Objects page works from attention items
The Resolve Ambiguous Objects page SHALL list open `ambiguous-object-name` attention items with
title, occurrence count and last-seen time. Selecting an item SHALL show its occurrences, the
tied candidates recorded at detection, and the existing artifact-object detail and live
candidate panels for the newest occurrence that still has a `kb.artifact_objects` row. The page
SHALL offer Resolve and Dismiss with an optional note, and SHALL keep its existing actions. All
new text SHALL be translated in English and Chinese.

#### Scenario: Reviewing a tie
- **WHEN** an admin opens the page and selects the 管路 item
- **THEN** the page shows the occurrences, the tied nodes and the artifact-object detail, and the
  admin can merge the nodes and then resolve the item

#### Scenario: Source rows gone
- **WHEN** no occurrence of an item still has a `kb.artifact_objects` row
- **THEN** the page shows the stored evidence and offers only Resolve and Dismiss

#### Scenario: Item closed
- **WHEN** the admin resolves or dismisses an item
- **THEN** it leaves the list and the next item is selected
