## ADDED Requirements

### Requirement: Table lines parse into a normalized row grid
The system SHALL parse the HTML content of a `table` line into a grid where every `rowspan` and `colspan` cell is expanded, so that each row has one value per column.

#### Scenario: Rowspan cells are repeated into each covered row
- **WHEN** the table on record 416 line 116 is parsed, where 序号 and 垃圾类型 cells carry `rowspan="2"` over the 太阳能辅助堆肥 and 厌氧产沼发酵 rows
- **THEN** both of those rows contain `1` in the 序号 column and `易腐垃圾` in the 垃圾类型 column

#### Scenario: Colspan cells are repeated into each covered column
- **WHEN** a header cell has `colspan="2"`
- **THEN** both grid columns under it carry that cell's text as part of their column label

#### Scenario: Malformed HTML does not break processing
- **WHEN** a table line's content cannot be parsed as a table
- **THEN** the parser returns an error and callers use the raw line content, as they do today

### Requirement: Rows have stable logical IDs and hashes
The system SHALL give header rows the IDs `h0, h1, …` and data rows the IDs `r1, r2, …` in document order, and SHALL compute a row hash from the normalized cell text of each row.

#### Scenario: Header detection without th tags
- **WHEN** a table has no `<th>` cells and its first row has no `colspan > 1` or `rowspan > 1` cell
- **THEN** the first row is `h0` and the next row is `r1`

#### Scenario: Multi-level header
- **WHEN** a table's first row contains a `colspan > 1` cell
- **THEN** the first two rows are header rows `h0` and `h1`, and each column label joins its header cells top-down with `/`

#### Scenario: Rowspan header cell
- **WHEN** a header row has a `rowspan="2"` cell (record 753 表2: `标准工况条件 | tAl | Δt1 | Δtsub` over a unit row `°C | K | K`)
- **THEN** the row it reaches is also a header row, so the unit row is `h1` and the first condition row `SC1` is `r1`

#### Scenario: IDs are deterministic
- **WHEN** the same table HTML is parsed twice
- **THEN** the row IDs and row hashes are identical

#### Scenario: Changed row content changes the hash
- **WHEN** any cell text in a row changes, after NFKC normalization
- **THEN** that row's hash changes

### Requirement: Chunk input renders tables as numbered rows
`canonicalChunkInputText` SHALL render the content of each `table` line as one row per text line, formatted `<line>#<row id>: <cell> | <cell> | …`, and SHALL leave line numbers, line types and all non-table lines unchanged.

#### Scenario: Table line rendering
- **WHEN** a chunk containing record 416 line 116 is serialized
- **THEN** the line's content starts with `116#h0: 序号 | 垃圾类型 | 处理模式 | 技术要求 | 适用范围` followed by `116#r1: 1 | 易腐垃圾 | 机器成肥 | …`, and its `line_number` is still 116

#### Scenario: Byte-identical across processors
- **WHEN** two different chunk processors serialize the same chunk
- **THEN** they produce byte-identical text

#### Scenario: Unparseable table falls back to raw content
- **WHEN** a table line cannot be parsed
- **THEN** its content is sent as the original HTML

### Requirement: Existing span addressing is unchanged
The system SHALL NOT change the format of `source_line_spans` or the computation of `line_range` for any artifact type.

#### Scenario: Table metric spans
- **WHEN** a metric is extracted from a row of the table on line 116
- **THEN** its `source_line_spans` is `["116"]` and its row reference is stored only in `source_table_rows`
