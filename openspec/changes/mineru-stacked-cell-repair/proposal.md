## Why

MinerU's table model writes the lines of a multi-line table cell back to back, with no separator. In
record 100 (GB 9448—2025, 表 1) the electrode sizes `<2.5`, `2.5~<4`, `4~6.4`, `>6.4` arrive as
`<2.52.5~<44~6.4>6.4` and the minimum shades 7, 8, 10, 11 as `781011`. Metric extraction then has
to guess where one value ends and the next begins. It guesses differently from run to run (record
100's chunk 8 put 推荐遮光号 10、12、14 on the wrong sub-rows), and the glued text is what users see in
metric context and what search indexes.

The cause sits in MinerU itself: the prod tier (`standard`) sends tables to a VLM whose OTSL output
has no in-cell line break, and the `basic` and `advanced` tiers glue the same cells. No setting
fixes it. The PDF's own text layer still has every value as a separate, positioned line, and
PyMuPDF's `find_tables()` already recovers the same cell grid (11 × 6 for 表 1, as MinerU sees it).

## What Changes

- After MinerU, the PDF parser compares each MinerU table with the PyMuPDF table at the same place
  on the page. A data cell whose PDF lines join up to exactly its text is re-segmented at those
  line breaks. The cell's characters never change. Only separators or row breaks are added.
- A data row whose re-segmented cells stack their lines at the same heights is split into one
  row per height. Single-line cells span the new rows with `rowspan`. 表 1's 手工电弧焊 row becomes
  four rows, the first reading `手工电弧焊 | — | <2.5 | <60 | 7 | —`.
- A multi-line cell that does not line up with others gets a `<br>` only where the break falls
  between two value characters (digits, `~ < > ± % —`). Wrapped prose (`气体保护电弧焊及` /
  `药芯焊丝电弧焊`) and header rows are left as they are.
- A table is repaired only when the PDF has a text layer at that table, PyMuPDF finds a matching
  grid, and the page is not rotated. Everything else passes through unchanged and is counted in
  the parser log.
- The physical table geometry (`table_geometry.py`, `<pdf>.pdf-table-geometry.json`) records the
  sub-row bands of each split row. The canonical geometry builder matches a ChenWeb row against a
  physical row or one of its sub-rows, so PDF highlighting keeps working for split rows and for
  documents parsed before this change.
- No model calls. The repair uses PyMuPDF only, which the parser environment already ships.

Re-parsed documents get more table rows than before. Their row IDs (`r1`, `r2`, …) shift, and
earlier metric extractions must be re-run to cite them. Documents that are not re-parsed are
unaffected.

## Capabilities

### New Capabilities
- `pdf-table-stacked-cell-repair`: the PDF parser re-segments MinerU table cells whose lines were
  glued together, using the PDF text layer, by splitting aligned sub-rows or inserting `<br>`.
- `pdf-table-geometry-sub-rows`: physical table geometry records sub-row bands, and the canonical
  geometry builder matches ChenWeb rows against them.

### Modified Capabilities
(none)

## Impact

- `python/pdf-parser/`: new repair module, called from `parser_mineru.py` after
  `mineru_v4_adapter.py` writes the content list. `table_geometry.py` emits sub-rows and its
  extractor id moves to `pymupdf-tables-v2`. New tests in `python/pdf-parser/tests/`.
- `server/api/file-converters/geometry.go`: accepts the `pymupdf-tables-v2` extractor id, so
  existing physical geometry files are regenerated.
- `server/api/documentgeometry/geometry.go`: `PhysicalRow` gains sub-rows; row matching tries them;
  `Algorithm` moves to v5 so canonical companions rebuild.
- `server/api/doc-processing/table_grid.go` is unchanged. Rowspan cells are already copied into
  every row they cover, so `TableGridVersion` stays the same.
- Prod: a new `chenweb-pdf-parser` image (worker code only, MinerU unchanged) plus the `server`,
  `doc-processor` and `parser-result-converter` binaries. Record 100 needs a re-parse, then `extract_metrics_simple` again.
