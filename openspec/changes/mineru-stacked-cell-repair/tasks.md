## 1. Shared rule (`python/pdf-parser/stacked_cells.py`)

- [x] 1.1 Add pure functions: group a cell's PyMuPDF spans into lines by vertical centre; normalize text for matching (NFKC, no whitespace, dash runs collapsed); test a value line (has a digit, or only dashes)
- [x] 1.2 Add `match_lines(cell_text, lines)`: return the line boundaries mapped onto the original cell string when the normalized join equals the normalized text, else `None`
- [x] 1.3 Add `split_plan(row_cells)`: given each cell's matched lines with centres, return k and the aligned cell set when ≥2 value-line cells share k ≥ 2 lines within 30 % of the row's median line spacing; else no split
- [x] 1.4 Add the `<br>` rule: insert at a boundary only when both neighbouring characters are value characters (digit or `. ~ < > ≤ ≥ ± % —`)
- [x] 1.5 Unit tests without a PDF: record 100 cells (`<2.52.5~<44~6.4>6.4`, `781011`, `——101214` vs `—`), wrapped name not aligned, two wrapped prose cells at the same heights (no split), near-miss heights (no split), mismatched text (`None`), content preserved after removing separators

## 2. MinerU table repair (`python/pdf-parser/mineru_cell_repair.py`)

- [x] 2.1 Parse a content-list `table_body` with BeautifulSoup into rows of cells (text, rowspan, colspan, has-child-elements); find header rows by the `ParseTableGrid` rule (`<th>` rows, else row 0)
- [x] 2.2 Pair each MinerU table with a `find_tables()` table on the same unrotated page: bbox IoU ≥ 0.8 in page-normalized 0–1000 coordinates and equal row/column counts; record skip reasons `no_match`, `grid_mismatch`, `no_text_layer`, `rotated_page`
- [x] 2.3 For data rows: match plain-text cells (1.2), skip rows with spans, apply `split_plan` (1.3) and write sub-rows with `rowspan` for the other cells; apply the `<br>` rule (1.4) to matched cells that are not split
- [x] 2.4 Rewrite `table_body` only for changed tables; return a per-document report (tables seen/paired/changed, rows split, cells with `<br>`, skips by reason, unmatched cells)
- [x] 2.5 Integration tests with synthetic PDFs drawn by PyMuPDF (`china-s` font), as in `test_table_geometry.py`: a ruled table with a 4-line stacked row, a wrapped-name row and a single-line row; a grid mismatch; a page without text; a rotated page; a `<eq>` cell
- [x] 2.6 Find ruled tables with `find_tables(use_layout=False)` here and in `table_geometry.py`; pin PyMuPDF 2.1 in `pyproject.toml` and the Dockerfile; ship `table_geometry.py` in `prepare-context.py`'s file list (record 103, 表 1 continuation page, 2026-11-04)

## 3. Parser wiring (`python/pdf-parser/parser_mineru.py`)

- [x] 3.1 Call the repair on the normalized content list after the `img_path` clean-up and before writing the staged content list; catch any exception, keep the unrepaired list and log a warning with a new log code
- [x] 3.2 Log the report from 2.4 as one line per document with a new log code
- [x] 3.3 Extend `tests/test_parser_mineru.py`: the repair runs on a staged content list, and a repair exception leaves the parse result intact

## 4. Physical geometry sub-rows (`python/pdf-parser/table_geometry.py`)

- [x] 4.1 For each written row, run the same `split_plan` on the PDF cells' lines and add `sub_rows` (coords across the table width and the sub-row's line extent; one cell per column, the line for split cells and the whole text otherwise)
- [x] 4.2 Change the extractor id to `pymupdf-tables-v2`
- [x] 4.3 Tests in `tests/test_table_geometry.py`: split row carries `sub_rows` with the right coords and cells; single-line rows have none; extractor id is v2

## 5. Go geometry (`server/api/documentgeometry`, `server/api/file-converters`)

- [x] 5.1 Add `SubRows []PhysicalRow` (`json:"sub_rows,omitempty"`) to `PhysicalRow`
- [x] 5.2 Accept `pymupdf-tables-v2` in `file-converters/geometry.go:physicalGeometryCurrent` and in `documentgeometry.Build`'s extractor check
- [x] 5.3 In row matching, score a canonical row against each physical row and each of its sub-rows; a sub-row match takes the sub-row's box and cell boxes
- [x] 5.4 Bump `Algorithm` to `canonical-table-geometry-v5`
- [x] 5.5 Tests in `geometry_test.go`: split canonical rows match sub-rows; a glued canonical row still matches its band; v1 physical files are rejected as stale
- [x] 5.6 `go test ./server/api/documentgeometry/... ./server/api/file-converters/... ./server/api/doc-processing/...` and `go vet` pass (2026-11-03: geometry + file-converters pass; doc-processing has one unrelated failure, `TestKBMetricsReadersExcludeResolvedRows` flags committed `kbhandler/question_generator_worker.go:133`)

## 6. End-to-end check on record 100 (local)

- [x] 6.1 Run the parser on record 100's PDF locally (`--tier standard --ocr-mode ocr`) and confirm the report: two tables paired and changed, eight rows split, page 12's 10 data rows become 26
- [x] 6.2 Convert to a line file and check `renderTableLineContent` output for 表 1: one value per cell in every split row, `手工电弧焊` and `—` copied into its four sub-rows
- [x] 6.3 Build the geometry companion for the repaired line file and check that 表 1's sub-rows get their own boxes

## 7. Docs and deploy

- [x] 7.1 Document the repair (what it changes, when it skips, the log codes) in `python/pdf-parser/container/README.md` and in the PDF-parser section of devdoc `2026090701-devdoc-deploy-start-system-onto.md`
- [ ] 7.2 With the user's go-ahead: build and deploy the `chenweb-pdf-parser` image and the `server`, `doc-processor` and `parser-result-converter` binaries to onto.bzton.cn
- [ ] 7.3 With the user's go-ahead: re-parse record 100 on prod, run `extract_metrics_simple`, and confirm metrics for every welding method in 表 1
