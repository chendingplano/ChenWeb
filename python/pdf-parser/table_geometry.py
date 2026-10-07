"""Extract physical PDF table borders for canonical line geometry companions.

Run in the parser environment: python table_geometry.py /path/to/document.pdf
No canonical line numbers are assigned here; the Go converter owns those IDs.
"""
import argparse
import hashlib
import json
import logging
import os
from pathlib import Path
import tempfile

import fitz

log = logging.getLogger(__name__)


def extract_table_geometry(pdf_path):
    pdf_path = Path(pdf_path)
    result = {'version': 1, 'extractor': 'pymupdf-tables-v1',
              'pdf_sha256': hashlib.sha256(pdf_path.read_bytes()).hexdigest(), 'tables': []}
    with fitz.open(pdf_path) as document:
        for page_number, page in enumerate(document, 1):
            # Table detection uses unrotated page space. Normalize after detection
            # against the displayed CropBox, using the PDF's intrinsic rotation.
            rotation = page.rotation
            page.set_rotation(0)
            matrix = fitz.Matrix(1, 1).prerotate(rotation)
            display = page.rect * matrix

            def coords(box):
                rect = fitz.Rect(box) * matrix
                return [(rect.x0-display.x0)/display.width*1000,
                        (rect.y0-display.y0)/display.height*1000,
                        (rect.x1-display.x0)/display.width*1000,
                        (rect.y1-display.y0)/display.height*1000]

            for table in page.find_tables().tables:
                texts = table.extract()
                x_edges = sorted({x for box in table.cells if box for x in (box[0], box[2])})
                rows = []
                for i, row in enumerate(table.rows):
                    explicit = [box for box in row.cells if box]
                    if not explicit:
                        continue
                    # Ignore rowspan height when determining the individual band.
                    top = max(box[1] for box in explicit)
                    bottom = min(box[3] for box in explicit)
                    if bottom <= top:
                        continue
                    cells = []
                    for col, box in enumerate(row.cells):
                        text = texts[i][col] or ''
                        if box is None and col+1 < len(x_edges):
                            cx = (x_edges[col]+x_edges[col+1])/2
                            cy = (top+bottom)/2
                            box = next((b for b in table.cells if b and b[0] < cx < b[2] and b[1] < cy < b[3]), None)
                            if box:
                                text = page.get_textbox(fitz.Rect(box))
                        cells.append({'text': ' '.join(text.split()), 'coords': coords(box) if box else None})
                    rows.append({'coords': coords((table.bbox[0], top, table.bbox[2], bottom)), 'cells': cells})
                result['tables'].append({'page': page_number, 'rotation': rotation, 'coords': coords(table.bbox), 'rows': rows})
            page.set_rotation(rotation)
    return result


def write_table_geometry(pdf_path):
    pdf_path = Path(pdf_path)
    output = pdf_path.with_suffix('.pdf-table-geometry.json')
    data = extract_table_geometry(pdf_path)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode='w', encoding='utf-8', suffix='.tmp',
                                         dir=output.parent, delete=False) as stream:
            temporary = stream.name
            json.dump(data, stream, ensure_ascii=False, indent=2)
            stream.write('\n')
        os.chmod(temporary, 0o644)
        os.replace(temporary, output)
    finally:
        if temporary and os.path.exists(temporary):
            os.unlink(temporary)
    log.info('(20261007-641) extracted PDF table geometry: pdf=%s tables=%d companion=%s',
             pdf_path, len(data['tables']), output)
    return output


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('pdf', type=Path)
    args = parser.parse_args()
    write_table_geometry(args.pdf)
