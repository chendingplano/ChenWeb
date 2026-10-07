import hashlib
import json

import fitz
import pytest

from table_geometry import extract_table_geometry, write_table_geometry


def make_pdf(path, rotation=0):
    with fitz.open() as doc:
        for page_number in range(2):
            page = doc.new_page(width=300, height=400)
            for x in (40, 120, 240):
                page.draw_line((x, 40), (x, 130))
            for y in (40, 70, 130):
                page.draw_line((40, y), (240, y))
            page.draw_line((120, 100), (240, 100))
            for x, y, text in [(45, 60, 'Group'), (125, 60, 'Value'),
                               (45, 85, 'shared'), (125, 85, f'row{page_number}a'),
                               (125, 115, f'row{page_number}b')]:
                page.insert_text((x, y), text, fontsize=10)
            page.set_rotation(rotation)
        doc.save(path)


def test_extracts_rows_cells_and_rowspan_bands(tmp_path):
    pdf = tmp_path / 'sample.pdf'
    make_pdf(pdf)
    result = extract_table_geometry(pdf)
    assert result['pdf_sha256'] == hashlib.sha256(pdf.read_bytes()).hexdigest()
    assert [t['page'] for t in result['tables']] == [1, 2]
    rows = result['tables'][0]['rows']
    assert rows[1]['coords'] == pytest.approx([40/300*1000, 175, 800, 250])
    assert rows[2]['coords'] == pytest.approx([40/300*1000, 250, 800, 325])
    assert rows[2]['cells'][0]['text'] == 'shared'
    assert rows[2]['cells'][0]['coords'] == rows[1]['cells'][0]['coords']
    assert rows[2]['cells'][1]['text'] == 'row0b'


def test_companion_is_atomic_json_and_uses_displayed_page_rotation(tmp_path):
    pdf = tmp_path / 'rotated.pdf'
    make_pdf(pdf, 90)
    output = write_table_geometry(pdf)
    assert output == tmp_path / 'rotated.pdf-table-geometry.json'
    result = json.loads(output.read_text())
    assert len(result['tables']) == 2
    assert result['tables'][0]['rows'][0]['coords'] == pytest.approx([825, 40/300*1000, 900, 800])
    assert not list(tmp_path.glob('*.tmp'))


def test_cropbox_coordinates_are_relative_to_displayed_page(tmp_path):
    pdf = tmp_path / 'cropped.pdf'
    make_pdf(pdf)
    with fitz.open(pdf) as doc:
        for page in doc:
            page.set_cropbox(fitz.Rect(20, 20, 280, 370))
        doc.saveIncr()
    result = extract_table_geometry(pdf)
    assert result['tables'][0]['rows'][0]['coords'] == pytest.approx(
        [20/260*1000, 20/350*1000, 220/260*1000, 50/350*1000])


def test_colspan_columns_share_the_same_physical_cell(tmp_path):
    pdf = tmp_path / 'colspan.pdf'
    with fitz.open() as doc:
        page = doc.new_page(width=300, height=400)
        for x in (40, 240):
            page.draw_line((x, 40), (x, 130))
        page.draw_line((120, 70), (120, 130))
        for y in (40, 70, 100, 130):
            page.draw_line((40, y), (240, y))
        page.insert_text((45, 60), 'Title', fontsize=10)
        for x, y, text in [(45,85,'Group'),(125,85,'Value'),(45,115,'Item'),(125,115,'50')]:
            page.insert_text((x,y),text,fontsize=10)
        doc.save(pdf)
    cells = extract_table_geometry(pdf)['tables'][0]['rows'][0]['cells']
    assert cells[0]['text'] == cells[1]['text'] == 'Title'
    assert cells[0]['coords'] == cells[1]['coords']
