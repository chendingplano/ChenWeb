"""Re-segment MinerU table cells whose stacked lines were glued together.

MinerU's table VLM has no line break inside a cell, so a cell listing <2.5, 2.5~<4, 4~6.4
and >6.4 on four lines arrives as "<2.52.5~<44~6.4>6.4". This pass pairs each MinerU table
with the PyMuPDF table at the same place, cuts each data cell at its PDF lines when they
spell the same text, and splits rows whose stacked values line up (see stacked_cells.py).
Header rows, cells whose text does not match, and tables without a matching PDF grid are
left as MinerU wrote them. No model calls are made.

Called by parser_mineru.py on the normalized content list:
    report = repair_content_list(content_list, pdf_path)
"""
from collections import Counter

from bs4 import BeautifulSoup
from bs4.dammit import EntitySubstitution
from bs4.formatter import HTMLFormatter
import fitz

from stacked_cells import break_groups, cell_lines, match_lines, split_plan

MIN_IOU = 0.8
# bs4's minimal escaping, with <br> written as MinerU and ChenWeb write it (not <br/>).
_FORMATTER = HTMLFormatter(entity_substitution=EntitySubstitution.substitute_xml, void_element_close_prefix='')


def _page_rect(page, bbox):
    """A content-list bbox (page-normalized 0-1000) in PDF points, or None."""
    if not isinstance(bbox, (list, tuple)) or len(bbox) != 4:
        return None
    r = page.rect
    x0, y0, x1, y1 = (float(v) / 1000 for v in bbox)
    return fitz.Rect(r.x0 + x0 * r.width, r.y0 + y0 * r.height, r.x0 + x1 * r.width, r.y0 + y1 * r.height)


def _iou(a, b):
    b = fitz.Rect(b)
    w, h = min(a.x1, b.x1) - max(a.x0, b.x0), min(a.y1, b.y1) - max(a.y0, b.y0)
    if w <= 0 or h <= 0:
        return 0.0
    inter = w * h
    return inter / (a.width * a.height + b.width * b.height - inter)


def _span(cell, name):
    try:
        return max(int(cell.get(name, 1)), 1)
    except ValueError:
        return 1


def _html_grid(soup):
    """Rows of (cell, column, rowspan, colspan), the column count, and the rows a span touches."""
    occupied, rows, spanned = set(), [], set()
    for r, tr in enumerate(soup.find_all('tr')):
        col, cells = 0, []
        for cell in tr.find_all(['td', 'th'], recursive=False):
            while (r, col) in occupied:
                col += 1
            rowspan, colspan = _span(cell, 'rowspan'), _span(cell, 'colspan')
            occupied.update((r + dr, col + dc) for dr in range(rowspan) for dc in range(colspan))
            if rowspan > 1 or colspan > 1:
                spanned.update(range(r, r + rowspan))
            cells.append((cell, col, rowspan, colspan))
            col += colspan
        rows.append(cells)
    return rows, max((c + 1 for _, c in occupied), default=0), spanned


def _header_rows(rows):
    """Leading <th> rows, else the first row, as ParseTableGrid decides."""
    count = 0
    while count < len(rows) and any(cell.name == 'th' for cell, *_ in rows[count]):
        count += 1
    return max(count, 1)


def _single_cells(table):
    """The PDF box of each unmerged cell, by (row, column).

    A row's band ends where its shortest cell ends and a column's where its narrowest cell
    ends, so a merged cell is one whose box reaches past either. A None in the next row is no
    evidence: it may be a full-width footnote's colspan, not a rowspan from above.
    """
    bottoms = [min((box[3] for box in row.cells if box), default=0) for row in table.rows]
    rights = [min((row.cells[c][2] for row in table.rows if row.cells[c]), default=0)
              for c in range(table.col_count)]
    return {(r, c): box for r, row in enumerate(table.rows) for c, box in enumerate(row.cells)
            if box and box[3] <= bottoms[r] + 1 and box[2] <= rights[c] + 1}


def _break(soup, cell, pieces):
    groups = break_groups(pieces)
    if len(groups) < 2:
        return 0
    cell.clear()
    for i, group in enumerate(groups):
        if i:
            cell.append(soup.new_tag('br'))
        cell.append(group)
    return 1


def _repair_table(page, table, html):
    """Return (new HTML or None when unchanged, counts, skip reason or None)."""
    soup = BeautifulSoup(html, 'html.parser')
    rows, ncols, spanned = _html_grid(soup)
    if len(rows) != table.row_count or ncols != table.col_count:
        return None, Counter(), 'grid_mismatch'
    counts = Counter()
    single = _single_cells(table)
    for r in range(_header_rows(rows), len(rows)):
        cells, pieces, lines = rows[r], {}, {}
        for cell, col, rowspan, colspan in cells:
            if rowspan > 1 or colspan > 1 or not cell.get_text().strip():
                continue
            box = single.get((r, col))
            found = None
            if box is not None and cell.find(True) is None:
                found_lines = cell_lines(page, box)
                found = match_lines(cell.get_text(), found_lines)
            if found is None:
                counts['unmatched_cells'] += 1
                continue
            pieces[col], lines[col] = found, found_lines
        plan = None if r in spanned else split_plan([lines.get(c) for c in range(ncols)])
        if plan is None:
            for cell, col, _, _ in cells:
                if col in pieces:
                    counts['br_cells'] += _break(soup, cell, pieces[col])
            continue
        anchor = cells[0][0].parent
        new_rows = []
        for _ in range(plan.k - 1):
            new_row = soup.new_tag('tr')
            anchor.insert_after(new_row)
            anchor = new_row
            new_rows.append(new_row)
        for cell, col, _, _ in cells:
            if col in plan.aligned:
                first, *rest = pieces[col]
                cell.string = first
                for new_row, piece in zip(new_rows, rest):
                    extra = soup.new_tag(cell.name)
                    extra.string = piece
                    new_row.append(extra)
            else:
                cell['rowspan'] = str(plan.k)
                if col in pieces:
                    counts['br_cells'] += _break(soup, cell, pieces[col])
        counts['rows_split'] += 1
    changed = counts['rows_split'] or counts['br_cells']
    return (soup.decode(formatter=_FORMATTER) if changed else None), counts, None


def repair_content_list(content, pdf_path):
    """Repair the content list's MinerU tables in place and return what was done.

    New table HTML is written back only after every table has been processed, so an exception
    leaves the content list as it was.
    """
    report = Counter()
    skipped = Counter()
    changes = []
    with fitz.open(pdf_path) as doc:
        for page_entry in content.get('pages', []):
            items = [item for item in page_entry.get('items', [])
                     if item.get('type') == 'table' and item.get('table_body')]
            if not items:
                continue
            report['tables'] += len(items)
            index = page_entry.get('page_number', 0) - 1
            if not 0 <= index < doc.page_count:
                skipped['no_match'] += len(items)
                continue
            page = doc[index]
            if page.rotation:
                skipped['rotated_page'] += len(items)
                continue
            pdf_tables = None
            for item in items:
                rect = _page_rect(page, item.get('bbox'))
                if rect is None:
                    skipped['no_match'] += 1
                    continue
                if not page.get_text('text', clip=rect).strip():
                    skipped['no_text_layer'] += 1
                    continue
                if pdf_tables is None:
                    pdf_tables = page.find_tables().tables
                best = max(pdf_tables, key=lambda t: _iou(rect, t.bbox), default=None)
                if best is None or _iou(rect, best.bbox) < MIN_IOU:
                    skipped['no_match'] += 1
                    continue
                html, counts, reason = _repair_table(page, best, item['table_body'])
                if reason:
                    skipped[reason] += 1
                    continue
                report['paired'] += 1
                report.update(counts)
                if html is not None:
                    report['changed'] += 1
                    changes.append((item, html))
    for item, html in changes:
        item['table_body'] = html
    result = {key: report[key] for key in ('tables', 'paired', 'changed', 'rows_split', 'br_cells', 'unmatched_cells')}
    result['skipped'] = dict(skipped)
    return result
