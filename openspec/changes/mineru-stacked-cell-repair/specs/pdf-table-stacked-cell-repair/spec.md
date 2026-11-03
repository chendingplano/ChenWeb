## ADDED Requirements

### Requirement: Repair runs after MinerU without model calls
The PDF parser SHALL run the stacked-cell repair on the MinerU content list after
`mineru_v4_adapter.py` has written it and before the result is promoted. The repair SHALL read only
the source PDF and the content list, and it SHALL make no LLM or VLM calls. If the repair raises,
the parser SHALL keep the content list as MinerU produced it, log a warning, and finish the parse
successfully.

#### Scenario: Repair error does not fail the parse
- **WHEN** the repair raises while processing a document
- **THEN** the parse succeeds with the unrepaired content list and a warning is logged

### Requirement: MinerU tables are paired with PDF tables by place and shape
For each MinerU table item on an unrotated page, the repair SHALL look for a PyMuPDF
`find_tables()` table on the same page whose bounding box overlaps the item's bounding box (both in
page-normalized 0–1000 coordinates, intersection over union at least 0.8) and whose row and column
counts equal the MinerU table's grid. A table without such a match SHALL be left unchanged.

#### Scenario: Matching grid is paired
- **WHEN** MinerU's 表 1 on page 12 is an 11 × 6 grid and PyMuPDF finds an 11 × 6 table at the same place
- **THEN** the two tables are paired and the repair continues

#### Scenario: Different grid is skipped
- **WHEN** PyMuPDF finds a table at the same place with a different number of rows or columns
- **THEN** the MinerU table is left unchanged and counted as skipped with reason `grid_mismatch`

#### Scenario: No text layer is skipped
- **WHEN** the page has no extractable text inside the table's box
- **THEN** the MinerU table is left unchanged and counted as skipped with reason `no_text_layer`

#### Scenario: Rotated page is skipped
- **WHEN** the table's page has a non-zero rotation
- **THEN** the MinerU table is left unchanged and counted as skipped with reason `rotated_page`

### Requirement: A cell is re-segmented only on an exact text match
For each data cell of a paired table, the repair SHALL take the PDF cell's text lines from top to
bottom and compare their concatenation with the MinerU cell text, after normalizing both by NFKC,
removing whitespace, and collapsing runs of dash characters (`—`, `–`, `-`, `－`) into one. Only
when they are equal SHALL the cell's line breaks be used. The repair SHALL NOT change, add or remove
any character of the MinerU text other than the separators and row breaks defined below.

#### Scenario: Glued cell matches its PDF lines
- **WHEN** the MinerU cell is `<2.52.5~<44~6.4>6.4` and the PDF cell lines are `<2.5`, `2.5~<4`, `4~6.4`, `>6.4`
- **THEN** the cell is re-segmented into those four lines

#### Scenario: Doubled dash still matches
- **WHEN** the MinerU cell is `——101214` and the PDF cell lines are `—`, `10`, `12`, `14`
- **THEN** the cell is re-segmented, and its first line keeps MinerU's text `——`

#### Scenario: Different text is left alone
- **WHEN** the joined PDF lines differ from the MinerU cell text after normalization
- **THEN** the cell keeps MinerU's text unchanged

#### Scenario: Content is preserved
- **WHEN** any repaired table has its inserted `<br>` removed and its split rows merged back by column
- **THEN** every cell's text equals the MinerU text it came from

### Requirement: Aligned stacked cells become sub-rows
A data row SHALL be split into k rows (k ≥ 2) when at least two of its re-segmented cells have k
lines each, every line of those cells is a value line (it contains a digit, or consists only of
dash characters), and their i-th lines sit at the same height, for every i. Two lines sit at the
same height when their vertical centres differ by less than 30 % of the median line spacing in that
row. Each aligned cell SHALL contribute its i-th line to the i-th new row. Every other cell of the row
SHALL be written once, in the first new row, with `rowspan="k"` and its original text. A row that
already contains or is covered by a cell with rowspan or colspan greater than 1 SHALL NOT be split.

#### Scenario: Manual arc welding row is split
- **WHEN** row 手工电弧焊 has aligned four-line cells for 焊条尺寸, 电弧电流, 最低遮光号 and 推荐遮光号
- **THEN** it becomes four rows; the first reads `手工电弧焊 | — | <2.5 | <60 | 7 | ——`, and `手工电弧焊` and `—` carry `rowspan="4"`

#### Scenario: Wrapped name stays one cell
- **WHEN** row 气体保护电弧焊及药芯焊丝电弧焊 has a two-line name cell whose lines do not sit at the heights of its four-line value cells
- **THEN** the row is split into four rows and the name cell is written once, with `rowspan="4"` and the text `气体保护电弧焊及药芯焊丝电弧焊`

#### Scenario: Gas cutting row on the continuation page is split
- **WHEN** row 气割 has aligned three-line cells for 板厚 (`<25`, `25~150`, `>150`) and 推荐遮光号 (`3或4`, `4或5`, `5或6`)
- **THEN** it becomes three rows and the single-line cells carry `rowspan="3"`

#### Scenario: Single stacked cell is not split
- **WHEN** a data row has only one multi-line cell
- **THEN** the row is not split

#### Scenario: Two wrapped prose cells are not split
- **WHEN** two cells of a data row each wrap their text onto two lines at the same heights and neither line holds a digit
- **THEN** the row is not split and both cells keep their glued text

#### Scenario: Cell with markup is not re-segmented
- **WHEN** a MinerU data cell contains child elements, such as an inline formula `<eq>…</eq>`
- **THEN** the cell is treated as not matched and keeps its HTML unchanged

### Requirement: Unaligned stacked values get a line break marker
For a re-segmented cell that does not take part in a row split, the repair SHALL insert `<br>`
between two of its lines exactly when the last character of the upper line and the first character
of the lower line are both value characters: a digit or one of `. ~ < > ≤ ≥ ± % —`. Other line
breaks SHALL be left glued, because they are wrapped words.

#### Scenario: Values glued across a break get a marker
- **WHEN** an unaligned cell has lines `7`, `10`, `10`
- **THEN** its HTML text becomes `7<br>10<br>10`

#### Scenario: Wrapped text is not marked
- **WHEN** an unaligned cell has lines `气体保护电弧焊及` and `药芯焊丝电弧焊`
- **THEN** its text stays `气体保护电弧焊及药芯焊丝电弧焊`

### Requirement: Header rows are not changed
The repair SHALL leave every header row unchanged: the `<th>` rows of the table, or its first row
when it has none. This is the same header rule `ParseTableGrid` uses.

#### Scenario: Two-line header cell is untouched
- **WHEN** the header cell `板厚mm` has PDF lines `板厚` and `mm`
- **THEN** the header cell keeps the text `板厚mm` and the header row is not split

### Requirement: Repair outcome is logged per document
After each document, the parser SHALL log one line with a unique log code giving: MinerU tables
seen, tables paired, tables changed, rows split, cells given `<br>`, and skipped tables by reason
(`no_match`, `grid_mismatch`, `no_text_layer`, `rotated_page`).

#### Scenario: Record 100 report
- **WHEN** record 100 is parsed
- **THEN** the log line shows two tables paired and changed and eight rows split: seven on page 12 (all but 焊炬硬钎焊, 焊炬软钎焊 and 碳弧焊, whose cells are single-line) and 气割 on page 13; page 12's 10 data rows become 26
