## ADDED Requirements

### Requirement: Physical geometry records sub-rows
`table_geometry.py` SHALL apply the same sub-row rule as the stacked-cell repair to every PyMuPDF
table row it writes. For a row that the rule splits into k sub-rows, the physical row SHALL keep its
band `coords` and `cells` and SHALL also carry `sub_rows`: k entries in top-to-bottom order, each
with `coords` spanning the table's width and the vertical extent of that sub-row's lines, and
`cells` holding, per column, the cell's line for that sub-row or, for a cell that is not split, the
cell's whole text. Rows the rule does not split SHALL carry no `sub_rows`. The extractor id SHALL
be `pymupdf-tables-v2`.

#### Scenario: Split row carries sub-rows
- **WHEN** `table_geometry.py` processes page 12 of record 100's PDF
- **THEN** the 手工电弧焊 row has four `sub_rows`, the first with cells `手工电弧焊`, `—`, `<2.5`, `<60`, `7`, `—`

#### Scenario: Single-line row has no sub-rows
- **WHEN** a row's cells are all single-line
- **THEN** the physical row has no `sub_rows` field

#### Scenario: Old physical geometry is regenerated on conversion
- **WHEN** the file converter converts a document whose `.pdf-table-geometry.json` has extractor `pymupdf-tables-v1`
- **THEN** it treats the file as stale and regenerates it

#### Scenario: Old physical geometry still builds
- **WHEN** the canonical geometry builder reads a `pymupdf-tables-v1` file of a document that has not been converted again
- **THEN** it builds the companion from it, as before, so the document keeps its highlighting

### Requirement: Canonical rows match physical sub-rows
When it maps ChenWeb table rows to physical rows, the canonical geometry builder SHALL match a
physical row that has `sub_rows` either as its band or as its sub-rows, never both. It SHALL use the
sub-rows when the sum, over the sub-rows, of the best exact cell-text score any ChenWeb row reaches
is greater than the band's best exact score; otherwise it SHALL use the band. The chosen rows are
then matched like any physical row. A ChenWeb row matched to a sub-row SHALL get that sub-row's box
and cell boxes. A ChenWeb row matched to the band SHALL get the band's box, as before. The builder
SHALL accept both `pymupdf-tables-v1` and `pymupdf-tables-v2` files. Its `Algorithm` SHALL change so
canonical companions built by the previous algorithm are rebuilt.

#### Scenario: Re-parsed document highlights a sub-row
- **WHEN** a re-parsed document's ChenWeb row is `手工电弧焊 | — | <2.5 | <60 | 7 | ——`
- **THEN** it matches the first sub-row of the physical 手工电弧焊 row and gets that sub-row's box

#### Scenario: Document parsed before the change still highlights
- **WHEN** a document's ChenWeb row still holds the glued cell `<2.52.5~<44~6.4>6.4`
- **THEN** it matches the physical band row, as it did before this change
