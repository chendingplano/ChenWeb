## ADDED Requirements

### Requirement: Metrics record the table rows they came from
The system SHALL store, in `kb.metrics.source_table_rows`, the table line and row IDs a metric was extracted from, with a row hash computed by the system for each row.

#### Scenario: LLM cites a valid row
- **WHEN** the enrich LLM returns `source_table_rows: ["116#r1"]` for a metric and row `r1` exists on line 116
- **THEN** the stored value is `[{"line":116,"rows":["r1"],"row_hash":{"r1":"<12 hex>"}}]`, with the hash computed from the grid

#### Scenario: LLM cites a row that does not exist
- **WHEN** the LLM cites `116#r9` and the table has 3 data rows
- **THEN** `r9` is dropped and row selection falls through to evidence matching

#### Scenario: Merged metrics keep all rows
- **WHEN** metric A absorbs metric B during merge, and they reference different rows of the same table
- **THEN** the surviving metric's `source_table_rows` contains the union of both rows

### Requirement: Table-sourced metric_context is built from the matched rows
For a metric whose spans cover a `table` line, the system SHALL replace `metric_context` with the table caption followed by each matched row rendered as `[<row id>] <column label>: <value> | …`, in the document's language.

#### Scenario: Record 416 metric 比能耗
- **WHEN** the metric 比能耗 is extracted from Table 1 of record 416
- **THEN** `metric_context` is `表1 易腐垃圾及其他垃圾主要处理模式` followed by `[r1] 序号: 1 | 垃圾类型: 易腐垃圾 | 处理模式: 机器成肥 | 技术要求: 采用机械成肥设备，…比能耗… | 适用范围: 人口密度高，有机肥需求量较大的农村地区。`

#### Scenario: Row chosen by evidence when no row is cited
- **WHEN** the LLM cites no rows and exactly one data row contains the metric's name
- **THEN** that row is used

#### Scenario: Ambiguous evidence
- **WHEN** no row is cited, several rows tie on evidence, and the table has more than 5 data rows
- **THEN** the LLM's `metric_context` is kept and a warning is logged with the metric ID, line and table size

#### Scenario: Small table
- **WHEN** no row can be matched and the table has 5 or fewer data rows
- **THEN** all data rows are included

#### Scenario: Length caps
- **WHEN** a cell is longer than 400 characters, or the whole context is longer than 2000 characters
- **THEN** the text is truncated and ends with `…`

#### Scenario: Non-table metrics are unaffected
- **WHEN** a metric's spans cover no `table` line
- **THEN** its `metric_context` is the LLM's value, as today

### Requirement: Neighbor rows are provided at read time only
The system SHALL NOT store neighbor rows in `metric_context` or `search_document`. It SHALL return the header, the matched rows and one data row above and below each matched row when a metric's context is read for review or display.

#### Scenario: Search does not match neighbor rows
- **WHEN** a user searches metrics for `太阳能辅助堆肥`
- **THEN** metric `416_mtc_31` (比能耗, row r1) is not matched through its `metric_context`

#### Scenario: Doc review hydration
- **WHEN** a matched metric with `source_table_rows` is hydrated for a metrics review prompt
- **THEN** its table line is presented as header + matched rows + ±1 data rows in the numbered-row format, with matched rows marked, instead of raw HTML

#### Scenario: Metric list API
- **WHEN** `GET /kb/metrics?input_record_id=N` returns a metric with `source_table_rows`
- **THEN** the metric includes `table_context` entries with `line`, `caption`, `columns`, and `rows` entries of `{id, cells, full_width, header, matched}`

#### Scenario: Row at the edge of the table
- **WHEN** the matched row is the first data row
- **THEN** the window contains the header, the matched row and the next data row only

### Requirement: Existing metrics can be backfilled without LLM calls
The system SHALL provide a command that rebuilds `metric_context`, `source_table_rows` and `search_document` for existing table-sourced metrics from stored lines, and that supports `--dry-run` and `--record-id`.

#### Scenario: Dry run
- **WHEN** the backfill runs with `--dry-run --record-id 416`
- **THEN** it prints the before/after context for affected metrics and a count of matched, whole-table and unmatched metrics, and writes nothing

#### Scenario: English context untouched
- **WHEN** the backfill updates a metric
- **THEN** `metric_context_en` is unchanged

#### Scenario: Idempotent
- **WHEN** the backfill runs twice on the same data
- **THEN** the second run changes no rows
