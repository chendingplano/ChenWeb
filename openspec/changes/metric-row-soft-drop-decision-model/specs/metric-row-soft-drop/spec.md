## ADDED Requirements

### Requirement: Dropped rows are stored, not discarded
Every row that `extract_metrics` sets aside SHALL be saved to `kb.metrics_dropped` and SHALL NOT
be saved to `kb.metrics`. This covers rows set aside for a drop-reason tag (`drop_stage =
llm_tag`), for a pure-requirement statement kind (`statement_kind`), for stating no value
(`no_value`), and by the open-value decision (`decision_model`). Each saved row SHALL carry `input_record_id`, `drop_id`,
`candidate_id`, `drop_stage`, `drop_reason`, `event_id` and the full enriched row as
`row_data`. This applies to the sequential and chunk-batch save paths.

#### Scenario: Pure requirement is stored as dropped
- **WHEN** enrichment returns a row with `value_class = requirement` and `value_range_type = qualitative`
- **THEN** the row SHALL be saved to `kb.metrics_dropped` with `drop_stage = statement_kind` and `drop_reason = inspection_requirement`
- **AND** it SHALL NOT be saved to `kb.metrics`

#### Scenario: Row tagged with a drop reason is stored as dropped
- **WHEN** enrichment returns a row whose `reasoning_tags` contain `applicability_scope`
- **THEN** the row SHALL be saved to `kb.metrics_dropped` with `drop_stage = llm_tag` and `drop_reason = applicability_scope`

#### Scenario: Duplicate tagged rows from overlapping chunks
- **WHEN** two batches return the same tagged row (same dedup key)
- **THEN** `kb.metrics_dropped` SHALL hold one row for it

### Requirement: A metric states a value
After the pure-requirement stage, `extract_metrics` SHALL set aside every row whose
`metric_value` is empty or a placeholder (`-`, `—`, `–`, `/`, `n/a`, `na`, `none`, `无`), with
`drop_stage = no_value` and `drop_reason = no_value`, whatever its `value_class`, tags or
`formula_or_definition`. The check SHALL run on Pass 2 rows, so the full row is kept.

#### Scenario: Value left open
- **WHEN** enrichment returns "比能耗" with `value_range_type = limit_absent` and no `metric_value`
- **THEN** it SHALL be set aside with `drop_reason = no_value` and not reach the decision model

#### Scenario: Formula without a value
- **WHEN** enrichment returns "标准排热量" with `formula_or_definition` "标准排热量 = (主测法 + 校核方法) / 2" and no `metric_value`
- **THEN** it SHALL be set aside with `drop_reason = no_value`

#### Scenario: Zero is a value
- **WHEN** enrichment returns a row with `metric_value = "0"`
- **THEN** it SHALL be kept

### Requirement: Dropped rows have their own ids
A dropped row SHALL get `drop_id = <record_id>_drp_<seqno>`, with `seqno` starting at 1 per
record, and SHALL NOT get a `metric_id`. Live rows SHALL be numbered `<record_id>_mtc_1` …
`<record_id>_mtc_N` with no gaps, whatever was dropped.

#### Scenario: Ids on a force_clear run
- **WHEN** a `force_clear` run produces 17 rows and sets aside 4 of them
- **THEN** `kb.metrics` SHALL hold `_mtc_1` … `_mtc_13` and `kb.metrics_dropped` SHALL hold `_drp_1` … `_drp_4`

#### Scenario: Merge mode appends dropped rows
- **WHEN** a merge-mode run (`force_clear = false`) sets aside a row for a record whose highest drop id is `_drp_4`
- **THEN** the new row SHALL get `_drp_5` and the existing dropped rows SHALL be kept

### Requirement: Re-extraction clears dropped rows
`DeleteMetricsByInputRecordID` SHALL delete the record's rows in `kb.metrics_dropped` as well as
in `kb.metrics`.

#### Scenario: force_clear re-extraction
- **WHEN** a record with dropped rows is re-extracted with `force_clear`
- **THEN** its previous `kb.metrics_dropped` rows SHALL be deleted before the new ones are saved

### Requirement: Every drop is logged
When a run sets aside at least one row, `extract_metrics` SHALL write one `kb.doc_proc_logs` entry
with `doc_proc_name = extract_metrics` and `activity_name = drop_metric_rows`. Its `extra_info`
SHALL contain `num_dropped`, a count per `drop_stage` and a count per `drop_reason`. Its artifact
SHALL list every dropped row with `drop_id`, `candidate_id`, `drop_stage`, `drop_reason`, `kind`,
`metric_name`, `subject`, `threshold_or_target`, `context`, `source_line_spans` and `decision`.
A run that sets aside nothing SHALL NOT write this entry. `exclude_pure_requirements` entries
SHALL no longer be written.

#### Scenario: Mixed drops in one run
- **WHEN** a run sets aside one tagged row, two pure requirements and one decision-model row
- **THEN** one `drop_metric_rows` entry SHALL exist with `num_dropped = 4` and four rows in its artifact

#### Scenario: Nothing dropped
- **WHEN** a run sets aside no rows
- **THEN** no `drop_metric_rows` entry SHALL be written

### Requirement: Dropped rows are invisible outside two pages
Dropped rows SHALL NOT be returned by search, indexing, connected-artifact building, category
indexing, ontology harvest, semantic assertions, reviewers, explorers, or any metrics API called
without `include_dropped=true`. Only System Admin → LLM → Metrics → Benchmark and Knowledge
System → Metrics SHALL request and show them.

#### Scenario: Default metrics API
- **WHEN** a client lists a record's metrics without `include_dropped`
- **THEN** the response SHALL contain only `kb.metrics` rows, unchanged from before this change

#### Scenario: Knowledge System metrics with dropped rows
- **WHEN** a user turns on "Show dropped" on Knowledge System → Metrics
- **THEN** dropped rows SHALL be listed, each marked as dropped with its `drop_id`, stage and reason, in English or Chinese per the locale

#### Scenario: Benchmark score ignores dropped rows
- **WHEN** the Benchmark page scores a record that has dropped rows
- **THEN** the dropped rows SHALL be shown, marked as dropped, and SHALL NOT count toward the score

### Requirement: Scoring reads dropped pure requirements from the table
The `score-extract-metrics` skill SHALL read a run's dropped pure requirements from
`kb.metrics_dropped` (`drop_stage = statement_kind`, matched by the run's `event_id`). For a run
with no such rows it SHALL fall back to that run's `exclude_pure_requirements` log entry.

#### Scenario: Run after this change
- **WHEN** the skill scores a run that saved dropped rows
- **THEN** its excluded list SHALL come from `kb.metrics_dropped`

#### Scenario: Run before this change
- **WHEN** the skill scores a run that has an `exclude_pure_requirements` entry and no dropped rows
- **THEN** its excluded list SHALL come from that entry
