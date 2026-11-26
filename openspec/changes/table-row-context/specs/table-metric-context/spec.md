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

#### Scenario: Unique value under the metric header repairs an incorrect citation
- **WHEN** exactly one non-full-width data row has the metric value in the column identified by the metric name
- **THEN** that row takes precedence over text evidence and saved row citations, including citations with matching row hashes
- **AND** the reference is narrowed to the value cell
- **AND** missing values, ambiguous headers and duplicate column values do not override the existing selection rules
- **AND** record 825 metric `825_mtc_307` selects `157#r7:c6` (wet-bulb temperature `1`) rather than notes row `r12`

#### Scenario: Labeled conditions distinguish repeated metric values
- **WHEN** no specific row was selected and several data rows contain the same exact value under the metric header
- **THEN** a row is selected if it is the only candidate that satisfies every labeled condition in the cells under the corresponding headers
- **AND** a condition such as `机组类型：水源式（低温余热式）` can match adjacent type and subtype cells under one spanning header
- **AND** record 404 metric `404_mtc_60` receives the cell citation `149#r9:c4`

#### Scenario: A quantity is embedded in a table cell
- **WHEN** there is no matching row from the existing evidence or citations, and exactly one data row contains the complete quantity with its unit
- **THEN** that row is cited with its hash, and the value and unit must match together without matching part of a longer number
- **AND** the matcher accepts a range that repeats its unit, such as `100mm~200mm`, and an explicit single-digit limit written in Chinese, such as `不应有多于两处`
- **AND** prose that gives the applicability of the limit, such as an inspection area of 0.1 m², remains condition evidence

#### Scenario: A metric name combines a row label and column heading
- **WHEN** the metric name is an exact row label from any column followed by a column heading and exactly one cell at that intersection agrees with the extracted value
- **THEN** the system can narrow a missing citation or a row-only citation covering the whole table to that cell
- **AND** labels after index or identifier columns are supported, while multiple matching intersections remain ambiguous
- **AND** the cell stays selected when the stored citation is replayed
- **AND** record 404 metric `404_mtc_19` cites `214#r2:c2` for the average wet-bulb temperature variation

#### Scenario: A cited header groups a quantity before its unit
- **WHEN** an explicitly cited header cell contains the complete metric value enclosed in parentheses followed by its unit, such as `(240±8%)V`
- **THEN** that header cell and its row hash are retained in the context and citation
- **AND** full-width parentheses are equivalent to ordinary parentheses
- **AND** a partial number, missing tolerance, or unit that is only a prefix of another unit does not match

#### Scenario: Ambiguous evidence
- **WHEN** no row is cited, several rows tie on evidence, and the table has more than 5 data rows
- **THEN** the LLM's `metric_context` is kept and a warning is logged with the metric ID, line and table size

#### Scenario: Uncited full-width footnote
- **WHEN** a large table has no row match by the existing evidence or cited-row rules, and exactly one full-width row contains both the scalar metric value with its unit and a condition clause
- **THEN** that footnote row is selected as evidence, with a computed row hash
- **AND** a digit inside a longer number does not count as the metric value
- **AND** ambiguous footnotes remain unmatched

#### Scenario: Simple extraction recovers omitted citations before truncation
- **WHEN** simple extraction omits table row citations and deterministic row selection can recover them
- **THEN** the recovered references are saved before building the source text for context and threshold evidence, so unrelated table rows do not displace the matched footnote at the text limit

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
