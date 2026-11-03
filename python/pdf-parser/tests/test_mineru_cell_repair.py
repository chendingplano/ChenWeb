from bs4 import BeautifulSoup
import fitz

from mineru_cell_repair import repair_content_list

GLUED = ('<table><tr><td>名称</td><td>尺寸mm</td><td>遮光号</td></tr>'
         '<tr><td>手工电弧焊</td><td>&lt;2.52.5~&lt;4&gt;4</td><td>7810</td></tr>'
         '<tr><td>气体保护电弧焊及药芯焊丝电弧焊</td><td>&lt;6060~&lt;160&gt;160</td><td>71010</td></tr>'
         '<tr><td>碳弧焊</td><td>—</td><td>14</td></tr>'
         '<tr><td>气焊</td><td>&lt;33~13&gt;13</td><td>4或5</td></tr></table>')
# The table drawn by make_pdf, in page-normalized 0-1000 coordinates.
TABLE_BBOX = [83, 125, 667, 413]


def make_pdf(path, text=True, rotation=0, footnote=False):
    """Draw a ruled 5 x 3 table whose stacked values have no rule between them, as in GB 9448 表 1.

    With footnote, a sixth row is one cell across the table, as on 表 1's continuation page.
    """
    with fitz.open() as doc:
        page = doc.new_page(width=600, height=800)
        xs, ys = (50, 200, 300, 400), (100, 130, 190, 250, 280, 330)
        for x in xs:
            page.draw_line((x, ys[0]), (x, ys[-1]))
        for y in ys:
            page.draw_line((xs[0], y), (xs[-1], y))
        if text:
            def put(x, y, t):
                page.insert_text((x, y), t, fontsize=9, fontname='china-s')
            put(55, 119, '名称'); put(205, 113, '尺寸'); put(205, 125, 'mm'); put(305, 119, '遮光号')
            put(55, 165, '手工电弧焊')
            for i, (a, b) in enumerate([('<2.5', '7'), ('2.5~<4', '8'), ('>4', '10')]):
                put(205, 148 + 14 * i, a); put(305, 148 + 14 * i, b)
            put(55, 214, '气体保护电弧焊及'); put(55, 228, '药芯焊丝电弧焊')
            for i, (a, b) in enumerate([('<60', '7'), ('60~<160', '10'), ('>160', '10')]):
                put(205, 208 + 14 * i, a); put(305, 208 + 14 * i, b)
            put(55, 268, '碳弧焊'); put(205, 268, '—'); put(305, 268, '14')
            put(55, 308, '气焊')
            for i, a in enumerate(['<3', '3~13', '>13']):
                put(205, 294 + 14 * i, a)
            put(305, 308, '4或5')
        if footnote:
            for x in (xs[0], xs[-1]):
                page.draw_line((x, ys[-1]), (x, 360))
            page.draw_line((xs[0], 360), (xs[-1], 360))
            if text:
                put(55, 348, '注：遮光号不宜低于下限值')
        page.set_rotation(rotation)
        doc.save(path)


def content(html=GLUED, bbox=TABLE_BBOX):
    return {'pages': [{'page_number': 1, 'items': [
        {'type': 'text', 'text': '表 1', 'bbox': [100, 100, 200, 120]},
        {'type': 'table', 'bbox': bbox, 'table_body': html}]}]}


def rows(html):
    soup = BeautifulSoup(html, 'html.parser')
    return [[(cell.decode_contents(), cell.get('rowspan')) for cell in tr.find_all(['td', 'th'])]
            for tr in soup.find_all('tr')]


def repaired(tmp_path, html=GLUED, **pdf):
    pdf_path = tmp_path / 'doc.pdf'
    make_pdf(pdf_path, **pdf)
    doc = content(html)
    report = repair_content_list(doc, str(pdf_path))
    return doc['pages'][0]['items'][1]['table_body'], report


def test_aligned_stacked_row_becomes_sub_rows(tmp_path):
    html, _ = repaired(tmp_path)
    assert rows(html)[1:4] == [
        [('手工电弧焊', '3'), ('&lt;2.5', None), ('7', None)],
        [('2.5~&lt;4', None), ('8', None)],
        [('&gt;4', None), ('10', None)],
    ]


def test_wrapped_name_spans_its_sub_rows_unchanged(tmp_path):
    html, _ = repaired(tmp_path)
    assert rows(html)[4:7] == [
        [('气体保护电弧焊及药芯焊丝电弧焊', '3'), ('&lt;60', None), ('7', None)],
        [('60~&lt;160', None), ('10', None)],
        [('&gt;160', None), ('10', None)],
    ]


def test_single_line_row_is_unchanged(tmp_path):
    html, _ = repaired(tmp_path)
    assert rows(html)[7] == [('碳弧焊', None), ('—', None), ('14', None)]


def test_lone_stacked_value_cell_gets_line_breaks(tmp_path):
    html, _ = repaired(tmp_path)
    assert '<tr><td>气焊</td><td>&lt;3<br>3~13<br>&gt;13</td><td>4或5</td></tr>' in html


def test_header_row_is_untouched(tmp_path):
    html, _ = repaired(tmp_path)
    assert rows(html)[0] == [('名称', None), ('尺寸mm', None), ('遮光号', None)]


def test_report_counts_the_repair(tmp_path):
    _, report = repaired(tmp_path)
    assert report == {'tables': 1, 'paired': 1, 'changed': 1, 'rows_split': 2, 'br_cells': 1,
                      'unmatched_cells': 0, 'skipped': {}}


def test_grid_mismatch_is_skipped_unchanged(tmp_path):
    two_columns = '<table><tr><td>名称</td><td>遮光号</td></tr><tr><td>手工电弧焊</td><td>7810</td></tr></table>'
    html, report = repaired(tmp_path, html=two_columns)
    assert html == two_columns
    assert report['skipped'] == {'grid_mismatch': 1}


def test_table_without_pdf_table_is_skipped(tmp_path):
    pdf_path = tmp_path / 'doc.pdf'
    make_pdf(pdf_path)
    # The top half of the drawn table: it has text, but overlaps the PDF table too little.
    doc = content(bbox=[83, 125, 667, 250])
    report = repair_content_list(doc, str(pdf_path))
    assert doc['pages'][0]['items'][1]['table_body'] == GLUED
    assert report['skipped'] == {'no_match': 1}


def test_page_without_text_layer_is_skipped(tmp_path):
    html, report = repaired(tmp_path, text=False)
    assert html == GLUED
    assert report['skipped'] == {'no_text_layer': 1}


def test_rotated_page_is_skipped(tmp_path):
    html, report = repaired(tmp_path, rotation=90)
    assert html == GLUED
    assert report['skipped'] == {'rotated_page': 1}


def test_cell_with_markup_is_left_alone(tmp_path):
    with_formula = GLUED.replace('<td>7810</td>', '<td>78<eq>10</eq></td>')
    html, report = repaired(tmp_path, html=with_formula)
    assert '<tr><td>手工电弧焊</td><td>&lt;2.5<br>2.5~&lt;4<br>&gt;4</td><td>78<eq>10</eq></td></tr>' in html
    assert report['unmatched_cells'] == 1


def stub_layout_table(monkeypatch, box):
    """Make PyMuPDF's layout model report one table at box (PDF points)."""
    monkeypatch.setattr(fitz.Page, 'get_layout',
                        lambda page, **_: setattr(page, 'layout_information', [(*box, 'table')]), raising=False)


def test_layout_table_box_does_not_replace_the_ruled_table(tmp_path, monkeypatch):
    # On GB 9448 表 1's continuation page the layout model's table box left out the footnote row
    # inside the border, so PyMuPDF 2 dropped the ruled table for a text-line table there.
    stub_layout_table(monkeypatch, (50, 100, 400, 190))
    html, report = repaired(tmp_path)
    assert report['paired'] == 1 and report['skipped'] == {}
    assert rows(html)[1][1] == ('&lt;2.5', None)


def test_full_width_row_below_does_not_hide_the_cells_above(tmp_path):
    pdf_path = tmp_path / 'doc.pdf'
    make_pdf(pdf_path, footnote=True)
    doc = content(GLUED.replace('</table>', '<tr><td colspan="3">注：遮光号不宜低于下限值</td></tr></table>'),
                  bbox=[83, 125, 667, 450])
    report = repair_content_list(doc, str(pdf_path))
    assert '<td>&lt;3<br>3~13<br>&gt;13</td>' in doc['pages'][0]['items'][1]['table_body']
    assert report['unmatched_cells'] == 0
