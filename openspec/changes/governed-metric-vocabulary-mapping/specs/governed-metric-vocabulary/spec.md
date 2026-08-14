## ADDED Requirements

### Requirement: `value_range_type` classification is a governed, DB-backed lookup
`kb.metrics.value_range_type` classification into a canonical bucket (`lower_bound`/
`upper_bound`/`exact`/`range`/`qualitative`/`limit_absent`) SHALL be sourced from
`kb.metric_value_range_type_map` rather than a hardcoded switch, using the row's `status` to
decide authority: only `status='approved'` rows classify a metric; `'proposed'` and `'ambiguous'`
rows both leave the metric unparsed.

#### Scenario: Raw value matches an approved mapping
- **WHEN** a `value_range_type` string normalizes to a `raw_value` with `status='approved'` in `kb.metric_value_range_type_map`
- **THEN** the metric is classified using that row's `canonical_bucket`, identical to today's hardcoded-switch outcome for the equivalent seeded synonym

#### Scenario: Raw value has no row at all
- **WHEN** a normalized `value_range_type` string has never been seen before
- **THEN** a new row is auto-inserted with `status='proposed'`, `occurrence_count=1`, and the metric is left unparsed

#### Scenario: Raw value matches an existing proposed or ambiguous row
- **WHEN** a normalized `value_range_type` string matches an existing `raw_value` row with `status='proposed'` or `status='ambiguous'`
- **THEN** no duplicate row is inserted; `occurrence_count` increments and `last_seen_record_id` updates on the existing row, and the metric is left unparsed

### Requirement: A cutover-time seed preserves today's classification behavior exactly
The creation migration SHALL seed `kb.metric_value_range_type_map` with every synonym currently
recognized by the hardcoded switch (as `status='approved'` with the matching `canonical_bucket`)
and every known direction-ambiguous string (as `status='ambiguous'`), so that no previously-working
classification and no previously-silent ambiguous deferral changes into a new failure on cutover.

#### Scenario: A previously-recognized synonym after cutover
- **WHEN** `extract_metrics` or `associate_semantics` processes a metric whose `value_range_type` was recognized by the pre-cutover Go switch
- **THEN** it still classifies to the same `canonical_bucket` post-cutover, via the seeded `'approved'` row

#### Scenario: A known ambiguous string after cutover
- **WHEN** `extract_metrics` or `associate_semantics` processes a metric whose `value_range_type` is a known direction-ambiguous string (e.g. `threshold`, `target`, `tolerance`)
- **THEN** it is seeded as `status='ambiguous'` and does not register as a new `'proposed'` row or trigger a processor failure

### Requirement: `extract_metrics` detects and flags an unmapped `value_range_type` at extraction time
After persisting a record's `kb.metrics` rows, `extract_metrics`'s Phase C indexing step SHALL
check each row's `value_range_type` against the governed mapping and, on a miss, flag the specific
row and fail the record's `extract_metrics` status — without altering what was extracted or
persisted.

#### Scenario: A metric's value_range_type has no approved mapping
- **WHEN** `MetricsProcessor.PostProcessIndex` finds a `kb.metrics` row whose `value_range_type` lookup result is `'proposed'`
- **THEN** that row's `value_range_type_error` column is set to a short message identifying the unmapped string, a `kb.metric_value_range_type_map` proposal is upserted, and the metric row itself is otherwise unchanged from what was extracted

#### Scenario: One or more misses occurred in a run
- **WHEN** `MetricsProcessor.PostProcessIndex` finds at least one unmapped `value_range_type` among a record's metrics
- **THEN** it writes one `kb.doc_proc_logs` row (`entry_type='assertion_mapping_miss'`, `doc_proc_name='extract_metrics'`) summarizing the run and returns a non-nil error, which (per the `phase-c-failure-propagation` capability) causes `extract_metrics`'s status for that record to end as `"failed"`

#### Scenario: No misses occurred in a run
- **WHEN** every metric row's `value_range_type` resolves to `'approved'` or `'ambiguous'`/`'absent'`
- **THEN** `PostProcessIndex` returns nil and no `assertion_mapping_miss` log row is written, unchanged from today

#### Scenario: extract_metrics does not drop unclassifiable metrics
- **WHEN** a metric's `value_range_type` cannot be mapped
- **THEN** the metric row is still persisted exactly as extracted; only the `value_range_type_error` flag and the failure status differ from a mapped metric

### Requirement: `normalize_assertions` waits for `extract_metrics`'s mapping check before reading metrics
`normalize_assertions`'s Phase C pass SHALL not read a record's `kb.metrics` rows until
`extract_metrics`'s Phase C mapping check (when invoked in the same run) has finished, so it never
observes a row before its `value_range_type_error` flag or governed-map proposal is written.

#### Scenario: Both processors invoked in the same run
- **WHEN** a record's pipeline run invokes both `extract_metrics` and `normalize_assertions`
- **THEN** `normalize_assertions`'s Phase C pass does not start until `extract_metrics`'s Phase C pass has completed for that record

### Requirement: `associate_semantics` fails only when a candidate is blocked on ungoverned vocabulary
`associate_semantics` SHALL distinguish a deferral caused by an unreviewed (`'proposed'`)
`value_range_type` mapping from all other deferral reasons, and SHALL fail its run only when at
least one candidate was deferred for that specific reason.

#### Scenario: A candidate's value_range_type lookup is 'proposed'
- **WHEN** `AssociateSemantics.processMetric` processes a candidate whose `proposed_payload.value_range_type_lookup` is `"proposed"`
- **THEN** the candidate is deferred as today, `AssociateReport.MappingMisses` increments, and after the run one `kb.doc_proc_logs` row (`entry_type='assertion_mapping_miss'`, `doc_proc_name='associate_semantics'`) is written and `Run` returns a non-nil error

#### Scenario: A candidate's value_range_type lookup is 'ambiguous' or 'absent'
- **WHEN** `AssociateSemantics.processMetric` processes a candidate whose lookup tag is `"ambiguous"` or `"absent"`
- **THEN** the candidate is deferred as today with no change to `MappingMisses`, and this alone does not cause `Run` to return an error

#### Scenario: No candidate hit an ungoverned mapping
- **WHEN** a run has zero `MappingMisses`
- **THEN** `Run` returns a nil error, unchanged from today's behavior, regardless of how many candidates were deferred for other reasons
