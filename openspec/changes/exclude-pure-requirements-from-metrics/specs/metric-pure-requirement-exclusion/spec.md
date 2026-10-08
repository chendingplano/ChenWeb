## ADDED Requirements

### Requirement: extract_metrics stores only rows with a measurable property
`extract_metrics` SHALL NOT save to `kb.metrics` any row whose statement kind is
`inspection_requirement` or `delegated_requirement`. Statement kind is defined by the rules in
spec `metric-statement-kind`, applied on the server after `value_range_type` is canonicalized.
Every other kind SHALL be saved as before. This applies to both the sequential and chunk-batch
save paths, with `force_clear` true or false.

#### Scenario: Inspection requirement is excluded
- **WHEN** enrichment returns a row with `value_class = requirement` and `value_range_type = qualitative`
- **THEN** that row SHALL NOT be saved to `kb.metrics`

#### Scenario: Delegated requirement is excluded
- **WHEN** enrichment returns a row with `value_class = reference`, `value_range_type = qualitative` and tag `cited_doc:CJJ 52`
- **THEN** that row SHALL NOT be saved to `kb.metrics`

#### Scenario: Numeric criterion is kept
- **WHEN** enrichment returns a row with `value_class = requirement` and `value_range_type = upper_bound`
- **THEN** that row SHALL be saved

#### Scenario: Value-open requirement is kept
- **WHEN** enrichment returns a row with `value_class = requirement` and `value_range_type = limit_absent`
- **THEN** that row SHALL be saved

#### Scenario: Non-canonical range type is canonicalized before classifying
- **WHEN** enrichment returns a row with `value_class = requirement` and a `value_range_type` synonym that canonicalizes to `upper_bound`
- **THEN** that row SHALL be saved

#### Scenario: Metric ids stay contiguous
- **WHEN** a `force_clear` run excludes some rows
- **THEN** the saved rows SHALL be numbered `<record>_mtc_1` … `<record>_mtc_N` with no gaps

### Requirement: Excluded rows are recorded
Every excluded row SHALL be saved to `kb.metrics_dropped` with `drop_stage = statement_kind` and
its kind as `drop_reason` (amended 2026-10-08 by change `metric-row-soft-drop-decision-model`,
spec `metric-row-soft-drop`). When a run sets aside at least one row, `extract_metrics` SHALL write
one `kb.doc_proc_logs` entry with `doc_proc_name = extract_metrics` and
`activity_name = drop_metric_rows`, listing every set-aside row with its `kind`, `metric_name`,
`subject`, `threshold_or_target`, `context` and `source_line_spans`. A run that sets aside
nothing SHALL NOT write this entry. The `exclude_pure_requirements` entry is no longer written.

#### Scenario: Excluded rows are stored and logged
- **WHEN** a run excludes two inspection requirements and one delegated requirement
- **THEN** three rows with `drop_stage = statement_kind` SHALL be saved to `kb.metrics_dropped`
- **AND** one `drop_metric_rows` entry SHALL list them

#### Scenario: Nothing excluded
- **WHEN** a run sets aside no rows
- **THEN** no `drop_metric_rows` entry SHALL be written

### Requirement: Default metric prompts do not request pure requirements
The default `EXTRACT_METRIC_CANDIDATES_PROMPT` SHALL be `prompt-extract-metric-candidates-v11.md`
and the default `ENRICH_METRICS_PROMPT` SHALL be `prompt-enrich-metrics-v8.md`. Neither prompt
SHALL instruct the model to emit a clause that has no measurable property. Both SHALL keep a
requirement that names a measurable quantity, with or without a stated value.

#### Scenario: Defaults without environment overrides
- **WHEN** the metrics processor is built with neither prompt variable set
- **THEN** it SHALL load `prompt-extract-metric-candidates-v11.md` and `prompt-enrich-metrics-v8.md`
