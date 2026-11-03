from stacked_cells import (
    Line,
    break_groups,
    is_value_line,
    lines_from_spans,
    match_lines,
    normalize,
    split_plan,
)


def stack(texts, first_centre, spacing=14.3, height=10.0):
    return [Line(t, first_centre + i * spacing - height / 2, first_centre + i * spacing + height / 2)
            for i, t in enumerate(texts)]


def test_spans_at_one_height_form_one_line():
    spans = [('<2.', (252, 316, 262, 327)), ('5', (262, 316, 268, 327)),
             ('2.', (243, 330, 250, 341)), ('5~<4', (250, 330, 277, 341))]
    assert [l.text for l in lines_from_spans(spans)] == ['<2.5', '2.5~<4']


def test_normalize_drops_whitespace_and_collapses_dashes():
    assert normalize(' —— 10 ～ 12 ') == '-10~12'


def test_value_line_needs_a_digit_or_only_dashes():
    assert is_value_line('3或4')
    assert is_value_line('—')
    assert not is_value_line('药芯焊丝电弧焊')


def test_glued_cell_is_cut_at_its_pdf_lines():
    lines = stack(['<2.5', '2.5~<4', '4~6.4', '>6.4'], 321.4)
    assert match_lines('<2.52.5~<44~6.4>6.4', lines) == ['<2.5', '2.5~<4', '4~6.4', '>6.4']


def test_doubled_dash_keeps_minerus_text():
    lines = stack(['—', '10', '12', '14'], 321.4)
    assert match_lines('——101214', lines) == ['——', '10', '12', '14']


def test_pieces_join_back_to_the_original_text():
    text = '4 或 55或6'
    pieces = match_lines(text, stack(['4或5', '5或6'], 100))
    assert pieces == ['4 或 5', '5或6']
    assert ''.join(pieces) == text


def test_different_text_does_not_match():
    assert match_lines('781012', stack(['7', '8', '10', '11'], 321.4)) is None


def manual_arc_row():
    return [stack(['手工电弧焊'], 343), stack(['—'], 343),
            stack(['<2.5', '2.5~<4', '4~6.4', '>6.4'], 321.4),
            stack(['<60', '60~<160', '160~<250', '250~550'], 321.4),
            stack(['7', '8', '10', '11'], 321.4),
            stack(['—', '10', '12', '14'], 321.4)]


def test_aligned_value_cells_split_the_row():
    plan = split_plan(manual_arc_row())
    assert plan.k == 4
    assert plan.aligned == (2, 3, 4, 5)


def test_split_bands_follow_the_aligned_lines():
    plan = split_plan(manual_arc_row())
    tops = [round(top, 1) for top, _ in plan.bands]
    assert tops == [316.4, 330.7, 345.0, 359.3]


def test_wrapped_name_is_not_aligned():
    row = [stack(['气体保护电弧焊及', '药芯焊丝电弧焊'], 394, spacing=14), stack(['—'], 407),
           stack(['—'], 407),
           stack(['<60', '60~<160', '160~<250', '250~500'], 387),
           stack(['7', '10', '10', '10'], 387),
           stack(['—', '11', '12', '14'], 387)]
    plan = split_plan(row)
    assert plan.k == 4
    assert plan.aligned == (3, 4, 5)


def test_wrapped_prose_at_same_heights_does_not_split():
    row = [stack(['焊接与切割', '安全要求'], 200), stack(['应符合规定', '并定期检查'], 200)]
    assert split_plan(row) is None


def test_near_miss_heights_do_not_split():
    row = [stack(['<60', '60~<160'], 100, spacing=14), stack(['7', '10'], 107, spacing=14)]
    assert split_plan(row) is None


def test_one_stacked_cell_does_not_split():
    row = [stack(['气焊'], 100), stack(['<3', '3~13', '>13'], 86), stack(['—'], 100)]
    assert split_plan(row) is None


def test_unmatched_cells_are_ignored():
    row = [None, stack(['<25', '25~150', '>150'], 180), stack(['3或4', '4或5', '5或6'], 180)]
    plan = split_plan(row)
    assert plan.aligned == (1, 2)


def test_breaks_only_between_values():
    assert break_groups(['7', '10', '10']) == ['7', '10', '10']
    assert break_groups(['气体保护电弧焊及', '药芯焊丝电弧焊']) == ['气体保护电弧焊及药芯焊丝电弧焊']
    assert break_groups(['板厚', 'mm']) == ['板厚mm']
    assert break_groups(['3或4', '4或5']) == ['3或4', '4或5']
