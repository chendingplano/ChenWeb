import json
import re

import numpy as np
import pytest
from PIL import Image

from mineru_table_repair import (
    TableSplitError,
    header_row_count,
    repair_content_list,
    repair_table,
    row_separators,
)

ROW_H = 20


def _ruled_table(header_rows: int, data_rows: int, width: int = 200) -> Image.Image:
    """White table with a full-width ruling line between rows. Each row carries a
    small grey marker whose level encodes its id (0 for header rows)."""
    n = header_rows + data_rows
    a = np.full((n * ROW_H + 1, width), 255, dtype=np.uint8)
    for r in range(n + 1):
        a[r * ROW_H, :] = 0
    for r in range(n):
        level = 200 if r < header_rows else 140 + (r - header_rows + 1)
        a[r * ROW_H + 5:r * ROW_H + 15, 5:15] = level
    return Image.fromarray(a)


def _fake_recognizer(capacity: int, calls: list):
    """Reads row ids back from the markers; emits at most `capacity` rows and
    reports truncation when the image holds more."""
    def recognize(image):
        a = np.asarray(image.convert("L"))
        seps = row_separators(a)
        rows = []
        for top, bottom in zip(seps, seps[1:]):
            level = int(a[(top + bottom) // 2, 10])
            rows.append("<tr><td>H</td></tr>" if level == 200 else f"<tr><td>{level - 140}</td></tr>")
        calls.append(len(rows))
        return "<table>" + "".join(rows[:capacity]) + "</table>", len(rows) <= capacity
    return recognize


def _cells(html: str) -> list[str]:
    return re.findall(r"<td[^>]*>(.*?)</td>", html)


def test_row_separators_finds_each_ruling_line():
    seps = row_separators(np.asarray(_ruled_table(1, 3).convert("L")))
    assert seps == [0, 20, 40, 60, 80]


def test_header_row_count_follows_first_row_rowspan():
    html = '<table><tr><td rowspan="3">No</td><td colspan="2">In</td></tr><tr><td>a</td></tr></table>'
    assert header_row_count(html) == 3
    assert header_row_count("<table><tr><td>No</td></tr></table>") == 1


def test_untruncated_table_is_left_alone():
    assert repair_table(_ruled_table(1, 3), _fake_recognizer(10, []), 1) is None


@pytest.mark.parametrize("header_rows", [1, 2])
def test_truncated_table_is_rebuilt_from_bands_in_order(header_rows):
    calls = []
    html = repair_table(_ruled_table(header_rows, 10), _fake_recognizer(5, calls), header_rows)
    assert _cells(html) == ["H"] * header_rows + [str(i) for i in range(1, 11)]
    assert max(calls[1:]) <= 5 + header_rows  # every band after the first pass is small


def test_table_without_ruling_lines_cannot_be_split():
    blank = Image.new("L", (200, 200), 255)
    with pytest.raises(TableSplitError):
        repair_table(blank, lambda image: ("<table><tr><td>1</td></tr></table>", False), 1)


def test_repair_content_list_replaces_only_truncated_large_tables(tmp_path):
    (tmp_path / "images").mkdir()
    _ruled_table(1, 10).save(tmp_path / "images" / "big.png")
    _ruled_table(1, 2).save(tmp_path / "images" / "small.png")
    truncated = "<table><tr><td>H</td></tr>" + "<tr><td>x</td></tr>" * 400 + "</table>"
    doc = {"pages": [{"page_number": 1, "items": [
        {"type": "table", "img_path": "images/big.png", "table_body": truncated},
        {"type": "table", "img_path": "images/small.png", "table_body": "<table><tr><td>H</td></tr></table>"},
        {"type": "text", "text": "x"},
    ]}]}
    calls = []
    report = repair_content_list(doc, str(tmp_path), _fake_recognizer(5, calls))
    items = doc["pages"][0]["items"]
    assert _cells(items[0]["table_body"]) == ["H"] + [str(i) for i in range(1, 11)]
    assert items[1]["table_body"] == "<table><tr><td>H</td></tr></table>"
    assert [r["img_path"] for r in report] == ["images/big.png"]
    assert report[0]["status"] == "repaired" and report[0]["rows_after"] == 11
    json.dumps(report)


def test_repair_content_list_reports_tables_it_cannot_split(tmp_path):
    (tmp_path / "images").mkdir()
    Image.new("L", (200, 200), 255).save(tmp_path / "images" / "plain.png")
    body = "<table><tr><td>H</td></tr>" + "<tr><td>x</td></tr>" * 400 + "</table>"
    doc = {"pages": [{"page_number": 1, "items": [
        {"type": "table", "img_path": "images/plain.png", "table_body": body},
    ]}]}
    report = repair_content_list(doc, str(tmp_path), lambda image: (body, False))
    assert doc["pages"][0]["items"][0]["table_body"] == body
    assert report[0]["status"] == "unrepaired"
