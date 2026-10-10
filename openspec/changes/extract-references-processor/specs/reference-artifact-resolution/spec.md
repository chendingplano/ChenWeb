## ADDED Requirements

### Requirement: Artifact families resolve references through a registered resolver
The reference layer SHALL define a resolver interface (`Family`, `Takes`, `HasArtifacts`,
`Pending`, `Resolve`, `Apply`). Each artifact family SHALL register one implementation. The
reference layer SHALL NOT write artifact tables, and artifact extractors SHALL NOT parse references.
Withdrawal SHALL need no operation of its own: `Apply` replaces a mention's previous links and
rows, and deleting a mention SHALL remove them by foreign-key cascade.

#### Scenario: A family takes only its own references
- **WHEN** a mention has `target_content = method`
- **THEN** the metric resolver's `Takes` returns false and nothing is written for it

### Requirement: Step 5 — the reference extractor resolves outgoing references
After storing a record's mentions, `extract_references` SHALL run, for every registered family,
`Resolve` and `Apply` for each mention the family takes whose holding is `held` and whose target
record already has artifacts of that family.

#### Scenario: Target already processed
- **WHEN** document A is processed by `extract_references` after document B (which A cites) was processed by `extract_metrics`
- **THEN** A's metric references to B are resolved in the same run

### Requirement: Step 6 — an artifact extractor resolves incoming references
After saving a record's artifacts, an artifact extractor SHALL resolve, with its own family's
resolver, every mention from other records whose holding is that record and that the family takes.
A failure in step 6 SHALL be logged and SHALL NOT fail the extractor.

#### Scenario: Target processed later
- **WHEN** document A was processed by `extract_references` before document B had metrics, and B is then processed by `extract_metrics`
- **THEN** A's metric references to B are resolved at the end of B's `extract_metrics` run

### Requirement: Holding events also trigger resolution
When `extract_references` attaches a record to its resource, it SHALL find other records' mentions
of that resource that become `held`, and resolve them for every family whose `HasArtifacts` is
true for the record.

#### Scenario: Target ingested last
- **WHEN** document A cites B, B is processed by `extract_metrics` first and by `extract_references` last
- **THEN** A's metric references to B are resolved when B's `extract_references` run attaches B to its resource

### Requirement: Whoever comes second resolves, without duplicates
Each event SHALL commit its own writes before it looks for the other side. Links SHALL be unique on
(mention, family, target artifact key). Re-resolving a mention SHALL replace its previous links and
added rows.

#### Scenario: Both sides resolve the same mention
- **WHEN** step 5 and step 6 both resolve the same mention
- **THEN** there is one link per target artifact and one set of resolved rows

### Requirement: Links record outcome and provenance
Every candidate or resolved target SHALL be stored in `kb.reference_artifact_links` with:
- `artifact_family`;
- target record, artifact id and stable artifact key;
- `rank`, `score`, `match_method`, `status` (`candidate`, `resolved`, `confirmed`, `rejected`,
  `stale`);
- the policy version.

The mention SHALL carry the family outcome (`metric_state`).

#### Scenario: Not confident
- **WHEN** no candidate reaches the policy threshold
- **THEN** the mention's state is `needs_review` and up to five candidates are stored with status `candidate`
