## ADDED Requirements

### Requirement: Metric occurrence cardinality is exact

One atomic current `kb.metrics` occurrence SHALL have exactly one active supporting assertion link;
one active semantic-processing outcome envelope for each adapter-declared required semantic stage,
containing zero or more typed findings; one active class-resolution decision; zero or one current
normalized value representation inside its assertion; and one raw occurrence revision/fingerprint
preserved independently of normalization.

#### Scenario: Every current metric has one supporting link
- **WHEN** the corpus report runs after cutover
- **THEN** every current metric occurrence has exactly one current supporting assertion link and the required stage outcome set

#### Scenario: Many occurrences converge on one assertion
- **WHEN** several metric occurrences share a canonical claim identity
- **THEN** they converge on the same assertion, each with its own evidence link

#### Scenario: One occurrence does not fan out
- **WHEN** a metric occurrence is processed
- **THEN** it does not produce several current metric assertions
- **AND** a compound extraction is first split into atomic metric rows

### Requirement: The metric supporting-link invariant is database-enforced

The database SHALL enforce at most one active supporting link per atomic metric occurrence through the
partial unique index `uq_assertion_evidence_current_metric_support` on
`(artifact_type, artifact_id, input_record_id)` where
`artifact_type = 'metric' AND evidence_role = 'supports' AND deleted = false`. Existing duplicates
SHALL be resolved through an auditable backfill before the index is created.

#### Scenario: Second supporting link is rejected
- **WHEN** a second active supporting evidence link is written for the same metric occurrence
- **THEN** the database rejects the write

#### Scenario: Generic artifacts are unaffected
- **WHEN** a non-metric artifact supports several assertions
- **THEN** the index does not apply and the writes succeed

#### Scenario: Non-supporting metric evidence is unaffected
- **WHEN** a metric retains contradicting or deleted evidence relationships
- **THEN** the index does not apply to them

#### Scenario: Duplicates resolved before index creation
- **WHEN** the index migration runs
- **THEN** pre-existing duplicate current supporting links have already been resolved auditably and non-current duplicates remain as history

### Requirement: Metric semantic writes occur in one atomic transaction

For each metric semantic stage, one atomic transaction SHALL include the idempotent mapping
observation/upsert keyed by the distinct source occurrence; class and class-resolution decision
preparation; canonical claim find-or-create and assertion creation/reuse; supersession of any prior
current supporting evidence link and insertion or restoration of the new link; the mandatory outcome
envelope, complete child-finding set, and validation/error records; and projection/retry invalidation
records. The raw artifact occurrence SHALL already be durably committed before the transaction begins.

#### Scenario: Rollback leaves no partial state
- **WHEN** the metric semantic transaction rolls back at any write boundary
- **THEN** no new assertion, current evidence link, resolution decision, mapping observation count, or outcome exists for that attempt
- **AND** the previously committed raw metric artifact remains

#### Scenario: Retry does not double-count occurrences
- **WHEN** the transaction is retried after a rollback
- **THEN** the mapping observation count for that distinct source occurrence is not incremented again

#### Scenario: Run status is derived after artifact transactions
- **WHEN** a run finishes or crashes
- **THEN** run status and aggregate finding summaries are recomputed from committed outcomes

### Requirement: Proposed and ambiguous mappings no longer fail processors

A proposed or ambiguous range-type mapping SHALL NOT cause `extract_metrics` or `associate_semantics`
to return a processor error, SHALL NOT set `has_failed_proc`, and SHALL NOT enter the failed-processor
retry queue. `associate_semantics` SHALL remain a vocabulary backstop but SHALL NOT defer the metric
out of the knowledge base.

#### Scenario: Proposed mapping completes the run
- **WHEN** a record's metrics include a range type whose governed mapping status is `proposed`
- **THEN** `associate_semantics` returns a completed run with a finding summary and no aggregate error

#### Scenario: Mapping miss stays out of the failed queue
- **WHEN** a mapping is missing for a metric
- **THEN** the record does not enter `has_failed_proc` or the failed-processor retry queue

#### Scenario: Metric remains in the knowledge base
- **WHEN** `associate_semantics` encounters ungoverned range-type vocabulary
- **THEN** the metric materializes as a raw-preserved assertion rather than being deferred out

### Requirement: The metric value-range disposition table is normative

Every identifiable metric SHALL materialize exactly one current semantic instance and stage outcome
envelope with the states, dispositions, findings, and retry triggers defined for its mapping and value
condition.

#### Scenario: Approved mapping
- **WHEN** the range-type mapping status is `approved`
- **THEN** execution is `completed`, mapping is `resolved`, the parsed literal is `present`, the authoritative bucket is populated, and the disposition is `semantic:normalized`
- **AND** retry triggers on a changed source or mapping revision

#### Scenario: Proposed mapping
- **WHEN** the range-type mapping status is `proposed`
- **THEN** execution is `completed` with a finding summary, mapping is `unresolved`, the parsed literal remains `present` (otherwise `unparsed`), bucket fields are empty, the disposition is `semantic:raw_preserved`, and a `mapping_unresolved` finding exists
- **AND** retry triggers when the mapping becomes approved or ambiguous or the source changes

#### Scenario: Ambiguous mapping
- **WHEN** the range-type mapping status is `ambiguous`
- **THEN** execution is `completed` with a finding summary, mapping is `ambiguous`, candidate buckets are non-authoritative, the disposition is `semantic:raw_preserved`, and a `mapping_ambiguous` finding exists
- **AND** retry triggers when the mapping decision or source context changes

#### Scenario: Absent range-type field
- **WHEN** the metric has no `value_range_type`
- **THEN** execution is `completed`, mapping is `not_required` when inapplicable and otherwise `unresolved`, the value remains independently `present`, `missing`, or `unparsed`, and `value_missing` or `mapping_unresolved` findings appear only when the class expects the field
- **AND** retry triggers on a source or class-contract revision change

#### Scenario: Malformed or unparseable literal
- **WHEN** the metric literal cannot be parsed
- **THEN** execution is `completed` with a finding summary, mapping is evaluated independently, the value is `unparsed`, the exact raw literal is retained, the disposition is `semantic:raw_preserved`, and an `unparsed` finding exists
- **AND** retry triggers on parser, mapping, source, or contract changes

#### Scenario: Recognized special value
- **WHEN** the metric carries a recognized special value
- **THEN** execution is `completed`, mapping is `resolved` when governed, the value is `present`, `unknown`, or `not_applicable` per contract, and the disposition is `semantic:normalized` when supported and otherwise `semantic:raw_preserved`
- **AND** retry triggers on source, mapping, parser, or contract changes

### Requirement: The governed mapping workflow and artifact flagging are preserved

The governed `kb.metric_value_range_type_map` table, raw-value discovery, occurrence counts,
proposed/approved/ambiguous mapping states, artifact-level `value_range_type_error`, operator
visibility, and genuine Phase-C execution-failure reporting SHALL remain unchanged. The raw metric
SHALL be persisted exactly as extracted and `value_range_type_error` SHALL be preserved as a finding.

#### Scenario: Mapping upsert still increments occurrences
- **WHEN** an unseen raw range type is encountered
- **THEN** the proposed mapping row is upserted and its occurrence count increments idempotently per distinct source occurrence

#### Scenario: Artifact error becomes a finding
- **WHEN** `value_range_type_error` is set on a metric
- **THEN** it is preserved on the artifact and surfaced as a finding on the stage outcome

#### Scenario: Genuine Phase-C failure still fails
- **WHEN** a Phase-C post-process indexer fails operationally
- **THEN** the processor still reports a failed execution status

### Requirement: The metric lossless writer is gated

The new metric semantic transaction SHALL be enabled only behind the named
`LOSSLESS_SEMANTIC_WRITES_METRIC` gate, and only after reader certification and confirmation that the
coordinated class, claim-registry, redirect, `represented` lifecycle, and metric support-index
foundations are active in shadow mode.

#### Scenario: Gate defaults off
- **WHEN** the system starts without explicit configuration
- **THEN** `LOSSLESS_SEMANTIC_WRITES_METRIC` is disabled and the legacy writer runs

#### Scenario: Activation requires prerequisites
- **WHEN** the gate is enabled before reader certification or before the coordinated foundations are active
- **THEN** activation is refused

#### Scenario: Rollback preserves committed rows
- **WHEN** the gate is disabled after lossless writes have occurred
- **THEN** the legacy writer is restored and committed raw-preserved assertions and outcome history are not deleted
