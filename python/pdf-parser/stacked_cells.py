"""Find where a table cell's stacked lines were glued together, and the rows they split.

MinerU's table VLM writes a cell's lines back to back ("<2.5" over "2.5~<4" becomes
"<2.52.5~<4"). The PDF text layer still has each line at its own height. These pure
functions work on (text, top, bottom) lines, so mineru_cell_repair.py (MinerU HTML) and
table_geometry.py (PDF rows) cut the same rows at the same heights.
"""
import statistics
import unicodedata
from typing import NamedTuple

DASHES = '—–‐‑‒―-－'
VALUE_SYMBOLS = '.~<>≤≥±%'
# Lines sit at the same height when their centres differ by less than this share of the
# row's line spacing.
ALIGN_TOLERANCE = 0.3


class Line(NamedTuple):
    text: str
    top: float
    bottom: float

    @property
    def centre(self):
        return (self.top + self.bottom) / 2


class SplitPlan(NamedTuple):
    k: int
    aligned: tuple  # indexes of the cells whose i-th line goes to sub-row i
    bands: list     # (top, bottom) of each sub-row's lines


def lines_from_spans(spans):
    """Group (text, (x0, y0, x1, y1)) spans into lines by vertical centre, left to right."""
    groups = []
    for text, (x0, y0, x1, y1) in sorted(spans, key=lambda s: ((s[1][1] + s[1][3]) / 2, s[1][0])):
        centre = (y0 + y1) / 2
        if groups and abs(groups[-1]['centre'] - centre) < (y1 - y0) / 2:
            group = groups[-1]
            group['top'], group['bottom'] = min(group['top'], y0), max(group['bottom'], y1)
        else:
            group = {'centre': centre, 'top': y0, 'bottom': y1, 'parts': []}
            groups.append(group)
        group['parts'].append((x0, text))
    lines = [Line(''.join(t for _, t in sorted(g['parts'])).strip(), g['top'], g['bottom']) for g in groups]
    return [line for line in lines if line.text]


def cell_lines(page, box):
    """The text lines inside box (x0, y0, x1, y1) of a PyMuPDF page."""
    spans = [(span['text'], span['bbox'])
             for block in page.get_text('dict', clip=tuple(box))['blocks']
             for line in block.get('lines', []) for span in line['spans']]
    return lines_from_spans(spans)


def _normalized_chars(text):
    """Yield each character's index and normalized form: NFKC, no whitespace, one '-' per dash run."""
    in_dashes = False
    for index, char in enumerate(text):
        piece = ''.join(unicodedata.normalize('NFKC', char).split())
        if piece and all(c in DASHES for c in piece):
            piece = '' if in_dashes else '-'
            in_dashes = True
        elif piece:
            in_dashes = False
        yield index, piece


def normalize(text):
    return ''.join(piece for _, piece in _normalized_chars(text))


def is_value_line(text):
    text = normalize(text)
    return bool(text) and (any(c.isdigit() for c in text) or text == '-')


def _is_value_char(char):
    char = unicodedata.normalize('NFKC', char)[:1]
    return bool(char) and (char.isdigit() or char in VALUE_SYMBOLS or char in DASHES)


def match_lines(text, lines):
    """Cut text into one piece per line when the joined lines are the same text, else None.

    The pieces are slices of text, so they join back to it exactly.
    """
    joined = ''.join(line.text for line in lines)
    if not lines or normalize(text) != normalize(joined):
        return None
    length_at = [0]
    for _, piece in _normalized_chars(joined):
        length_at.append(length_at[-1] + len(piece))
    ends, position = [], 0
    for line in lines[:-1]:
        position += len(line.text)
        ends.append(length_at[position])
    cuts, total, targets = [], 0, iter(ends)
    target = next(targets, None)
    for index, piece in _normalized_chars(text):
        if piece and target is not None and total == target:
            cuts.append(index)
            target = next(targets, None)
        total += len(piece)
    if len(cuts) != len(ends):
        return None
    return [text[a:b] for a, b in zip([0] + cuts, cuts + [len(text)])]


def split_plan(cells):
    """Return how a row splits into sub-rows, or None.

    cells holds each cell's lines, or None for a cell that did not match. A row splits into k
    sub-rows when at least two cells of value lines have k >= 2 lines at the same heights.
    """
    candidates = [(i, lines) for i, lines in enumerate(cells)
                  if lines and len(lines) >= 2 and all(is_value_line(line.text) for line in lines)]
    best = None
    for k in sorted({len(lines) for _, lines in candidates}, reverse=True):
        group = [(i, lines) for i, lines in candidates if len(lines) == k]
        if len(group) < 2:
            continue
        spacing = statistics.median(b.centre - a.centre for _, lines in group for a, b in zip(lines, lines[1:]))
        if spacing <= 0:
            continue
        tolerance = ALIGN_TOLERANCE * spacing

        def close(a, b):
            return all(abs(x.centre - y.centre) < tolerance for x, y in zip(a, b))

        aligned = max(([(i, lines) for i, lines in group if close(lines, anchor)] for _, anchor in group), key=len)
        if len(aligned) >= 2 and (best is None or len(aligned) > len(best[1])):
            best = (k, aligned)
    if best is None:
        return None
    k, aligned = best
    bands = [(min(lines[j].top for _, lines in aligned), max(lines[j].bottom for _, lines in aligned))
             for j in range(k)]
    return SplitPlan(k, tuple(i for i, _ in aligned), bands)


def break_groups(pieces):
    """Merge pieces whose break is a word wrap; keep apart those that would glue two values."""
    groups = [pieces[0]]
    for piece in pieces[1:]:
        before, after = groups[-1].rstrip(), piece.lstrip()
        if before and after and _is_value_char(before[-1]) and _is_value_char(after[0]):
            groups.append(piece)
        else:
            groups[-1] += piece
    return groups
