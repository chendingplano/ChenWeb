## Context

Tables reach ChenWeb as one HTML line per table. The PDF parser (`python/pdf-parser`) runs
`mineru-kit parse --tier standard --ocr-mode ocr` in the `chenweb-pdf-parser` container,
`mineru_v4_adapter.py` turns MinerU's middle JSON into `<stem>_content_list.json`, and the Go
converter writes the `.txt` line file that every processor reads. `ParseTableGrid` turns a table
line into numbered rows (`123#r1: …`) for LLMs and for row citations.

At the `standard` tier MinerU sends each table crop to its VLM (effort `high`). The VLM answers in
OTSL, which has a token per cell and per row but none for a line break inside a cell, so stacked
values come out glued. This was confirmed on record 100 by capturing the raw OTSL (`<fcel><2.52.5~<44~6.4>6.4<fcel>…<fcel>781011`)
before `convert_otsl_to_html`. The `basic` tier (wired/wireless table models, which join a cell's
lines with `""` in `unet_table/utils_table_recover.py:_cell_text`) and the `advanced` tier glue the
same cells.

The separators survive in the PDF. On record 100, PyMuPDF `page.find_tables()` (already used by
`table_geometry.py` for viewer highlighting) returns 11 × 6 and 3 × 6 grids, the same as MinerU's.
Each stacked cell reads as separate lines, `<2.5 ⏎ 2.5~<4 ⏎ 4~6.4 ⏎ >6.4`, and the sub-row lines sit
at identical heights across columns (321.4 / 335.7 / 349.9 / 364.2 in two different columns).
Wrapped text (`气体保护电弧焊及 ⏎ 药芯焊丝电弧焊`) sits at other heights.

Constraints: no LLM calls outside `shared/go/api/llm` (the reason `mineru_table_repair.py` was
commented out on 2026-10-27); the container ships PyMuPDF 1.28.2 and BeautifulSoup 4.15 but not
pdfminer; MinerU's source in `ThirdParty/mineru` stays unpatched.

## Goals / Non-Goals

**Goals:**
- Stacked values in bordered tables of text-layer PDFs arrive as separate values: as sub-rows when
  they line up across columns, otherwise with `<br>` where glued values would run together.
- Never alter MinerU's characters; only add separators and row breaks, and only on an exact match.
- Keep PDF row highlighting working for split rows and for documents parsed before the change.

**Non-Goals:**
- Borderless tables (PyMuPDF finds no grid) and scanned PDFs (no text layer). These stay glued.
- Rotated pages (skipped in this version).
- Re-parsing existing documents in bulk.
- Changing `ParseTableGrid`, row-ID rules or how `<br>` is rendered (it stays a space).
- Patching MinerU or its VLM.

## Decisions

### D1. Repair in the PDF parser, after the adapter
The repair runs in `parser_mineru.py` on the normalized content list, right after the adapter and
the `img_path` clean-up and before `_promote_result_pair`. Only there are both the PDF and MinerU's
table HTML at hand. Once the content list is written, the separators are gone for every consumer
(converter, `.txt`, LLMs, UI, search).
*Alternatives:* fix in Go (`ParseTableGrid`, converter): too late, the information is lost. Patch
MinerU's VLM post-processing: the VLM never emits a separator, so there is nothing to keep.

### D2. PyMuPDF `find_tables()` for the grid, the text layer for lines
The PDF grid comes from `find_tables()`, the same detector `table_geometry.py` uses. A cell's lines
are the text lines inside its rectangle (`page.get_text("dict", clip=cell)`), with spans grouped by
line centre, since PyMuPDF splits `<2.5` into spans `<2.` and `5` at the same height.
*Alternatives:* pdfminer ruling lines plus text lines: works (verified), but pdfminer is not in the
container. Image-based ruling detection, as in the old repair: needs pixels, and gives no text.

### D3. Verify, never copy, PDF text
A cell is re-segmented only when its PDF lines, joined, equal MinerU's text after NFKC, whitespace
removal and dash-run collapse. The line boundaries are then mapped back onto MinerU's own string,
so MinerU's characters stay (its `——` stays `——` though the PDF has `—`). The VLM's text is
treated as the content because prod parses in OCR mode, and some PDFs' text layers are garbled.
A mismatch costs nothing: the cell stays as it was. Cells with child elements (`<eq>`) are skipped.

### D4. Sub-rows first, `<br>` second
A row splits when two or more cells are made of value lines (a digit, or only dashes) with the same
line count k ≥ 2 at the same heights. Other cells are written once with `rowspan=k`, which
`ParseTableGrid` already copies into every covered row. Each sub-row then holds one value per cell,
so the model reads `手工电弧焊 | — | <2.5 | <60 | 7 | ——` and a citation like `123#r3` names one
assertion. The value-line test keeps two wrapped prose cells from being mistaken for sub-rows. Cells
that do not take part get `<br>` only at value/value junctions (`7` | `10`), not at word wraps
(`及` | `药`), so wrapped Chinese text does not gain a space. Rows that already have rowspan or
colspan are not split: merging two span structures is guesswork.
*Alternatives:* `<br>` everywhere: smallest change, but the model must still align columns by
position, and prose gains spaces. Copy shared cells instead of `rowspan`: same result after
`ParseTableGrid`, but the HTML would no longer look like the PDF.

### D5. Headers untouched
Header rows (`<th>` rows, else row 0, as `ParseTableGrid` decides) are never changed. Header text
such as `板厚mm` reads fine glued, and splitting a header would change the column labels.

### D6. One shared module for the rule
`stacked_cells.py` holds the line grouping, the matching and the split rule as pure functions over
`(text, line centres)` data. `mineru_cell_repair` applies them to MinerU HTML (parsed and rewritten
with BeautifulSoup). `table_geometry.py` applies the split rule to PDF rows, so both sides cut the
same rows at the same heights. Unit tests cover the rule without a PDF; integration tests use
synthetic PDFs drawn with PyMuPDF, as `test_table_geometry.py` does, with the built-in `china-s`
font for Chinese text.

### D7. Geometry: additive `sub_rows`, version bumps, no `TableGridVersion` change
Physical rows gain an optional `sub_rows` list. The Go builder matches such a row either as its band
or as its sub-rows, never both: the sub-rows when their exact (`score`) matches sum higher than the
band's. A document parsed before the change still has glued text, so its band scores exactly and
its sub-rows do not; a re-parsed document is the other way round. Offering both at once would let
the Chinese-only fallback (`scoreReadable`) tie the band with every sub-row. A garbled text layer
scores neither exactly, so the band is kept, which is right because the repair could not match
those cells either. Bumping `pymupdf-tables-v1` → `v2` makes the file converter regenerate physical
geometry when it converts a document. The builder accepts both ids, because the viewer builds
companions on request (`kbhandler/table_geometry_handler.go`) from whatever physical file a
document has, and documents that are not re-parsed keep their v1 files. Bumping `Algorithm`
(v4 → v5) rebuilds canonical companions. `ParseTableGrid` does not change, so `TableGridVersion` stays as it is; re-parsed
documents also change `LineHash`, which rebuilds their companions anyway.

### D8. No feature flag
The repair is on for every MinerU parse. Every table is verified cell by cell and passes through on
any doubt, and rollback is a redeploy of the previous parser image.

## Risks / Trade-offs

- [PyMuPDF and MinerU disagree on a grid, e.g. a missing inner rule] → shape check; the table is
  skipped and counted as `grid_mismatch`.
- [Coincidental alignment of unrelated value cells] → requires ≥2 value cells with equal line counts
  at matching heights, within a tolerance of 30 % of line spacing. Tests include a near-miss row.
- [Row IDs shift for re-parsed documents] → expected; re-parsing re-runs downstream processors. Old
  extractions of the same record become stale, as with any re-parse.
- [Text layer present but garbled] → exact match fails, cells stay glued. The log counts unmatched
  cells per table so this shows up.
- [Old repair module `mineru_table_repair.py` stays commented out] → untouched; this change makes
  no model calls.
- [Slower parse] → one `find_tables()` per page with a MinerU table; small next to a CPU VLM run
  (record 100: 31 pages in 13.7 min).

## Migration Plan

1. Build and deploy the `chenweb-pdf-parser` image (worker code only; MinerU unchanged).
2. Deploy `server`, `doc-processor` and `parser-result-converter`, the binaries that contain the
   geometry builder and file converter, with the v2 extractor and v5 algorithm.
3. Re-parse record 100, run `extract_metrics_simple`, and check 表 1: 26 data rows on page 12, and
   metrics for every welding method.
4. Rollback: redeploy the previous image and binaries. Documents parsed in between keep their
   split rows, which `ParseTableGrid` reads like any other table.

## Resolved Questions

- Already-parsed documents are left as they are (decided 2026-11-03). They are not searched for or
  re-parsed; a document gets the repair only when someone re-parses it.
- Borderless tables are worth a follow-up change (decided 2026-11-03), for example with
  `find_tables(strategy="text")`. They stay out of scope here.
