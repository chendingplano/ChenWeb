## ADDED Requirements

### Requirement: The metric resolver takes criterion references only
The metric family resolver SHALL take a mention only when its `target_content` is `criterion` or
`criteria_set`.

#### Scenario: Method reference
- **WHEN** a mention says "rated power determined in accordance with GB/T 755" (`target_content = method`)
- **THEN** the metric resolver does not take it

### Requirement: Matching is deterministic and versioned
The resolver SHALL score the target record's own metrics (not resolved rows), and its dropped
definition rows, against the mention's named quantity using the `metric_reference_match/v1` policy:
- name similarity in Chinese and English;
- a subject bonus;
- a locator bonus.

It SHALL resolve when the top score is at least 0.8 and leads the next by at least 0.1. For
`criteria_set`, it SHALL resolve every candidate in the located table or section. Otherwise it
SHALL mark `needs_review` with the top five candidates scoring at least 0.3, or `no_match` with
`type_doubtful` when no target metric scores 0.3 (nothing in the target resembles the quantity).

#### Scenario: Clear match
- **WHEN** A requires "粪大肠菌群数" to meet GB 7959, and GB 7959 has exactly one metric named "粪大肠菌群数"
- **THEN** the mention is `resolved` to that metric

#### Scenario: Target holds no metrics of that kind
- **WHEN** the target record has been processed by `extract_metrics` but none of its metrics resembles the named quantity
- **THEN** the mention is `no_match` with `type_doubtful`

#### Scenario: Target not processed yet
- **WHEN** the target record is held but `extract_metrics` has not processed it
- **THEN** the mention is `pending_target` until step 6 runs on the target

### Requirement: Resolved metrics are written into the referencing document
For each resolved target metric, the resolver SHALL insert a row into the referencing record's
`kb.metrics`:
- `metric_id` `<rec>_rfm_<n>`;
- value columns copied from the target metric;
- name, subject, context, section, line spans and `source_table_rows` from the referencing text;
- `ref_mention_id`, `ref_input_record_id`, `ref_metric_id` and `ref_metric_key` set.

A dropped target row SHALL be linked but SHALL NOT produce a `kb.metrics` row.

#### Scenario: Resolved row
- **WHEN** a mention of record 900 resolves to metric `753_mtc_12`
- **THEN** record 900 gets a `kb.metrics` row `900_rfm_<n>` with `ref_metric_id = 753_mtc_12` and the value of `753_mtc_12`

### Requirement: extract_metrics never removes resolved rows
`extract_metrics` SHALL exclude rows with `ref_mention_id` set from its delete, its merge
comparison and its metric-object snapshot.

#### Scenario: Re-run of the referencing document's metrics
- **WHEN** `extract_metrics` re-runs with force clear on a record that has resolved rows
- **THEN** its own rows are replaced and its resolved rows remain

### Requirement: Every kb.metrics reader has an explicit default
Every query that reads `kb.metrics` SHALL exclude resolved rows (`ref_mention_id IS NULL`), except
the metric list API, which SHALL return them with their `ref_*` fields. The benchmark scoring
scripts SHALL exclude them.

#### Scenario: Review and Phase D
- **WHEN** a record has resolved rows and its metrics are reviewed or normalized into assertions
- **THEN** the resolved rows are not read

#### Scenario: Guard test
- **WHEN** a new non-test Go query reading `kb.metrics` is added without `ref_mention_id` and is not allowlisted
- **THEN** the guard test fails
