import json
import sys

import pytest
from types import SimpleNamespace

from mineru_v4_adapter import convert, _add_caption_boxes


def test_convert_preserves_caption_boxes(monkeypatch, tmp_path):
    caption = "表 2 水源式热水机的试验工况"
    middle = {"pages": [{"page_idx": 7, "blocks": [{
        "type": "table", "content": [
            {"type": "table_caption", "bbox": [.28, .65, .62, .67],
             "content": [{"type": "text", "content": caption}]},
            {"type": "table_body", "bbox": [.103, .678, .881, .884]},
        ],
    }]}]}
    rendered = [{"type": "table", "page_idx": 7,
                 "bbox": [103, 678, 881, 884], "table_caption": [caption]}]
    monkeypatch.setitem(sys.modules, "mineru.parser", SimpleNamespace(
        ParseResult=SimpleNamespace(from_json=lambda raw: SimpleNamespace(middle_json=middle))))
    monkeypatch.setitem(sys.modules, "mineru.render", SimpleNamespace(
        RenderFormat=SimpleNamespace(CONTENT_LIST="content_list"), render=lambda *args: rendered))
    source, target = tmp_path / "middle.json", tmp_path / "output.json"
    source.write_text(json.dumps(middle))
    convert(source, target)
    item = json.loads(target.read_text())["pages"][7]["items"][0]
    assert item["table_caption_bboxes"] == [[280, 650, 620, 670]]
    assert item["bbox"] == [103, 678, 881, 884]


def test_caption_boxes_do_not_guess_missing_or_ambiguous_geometry():
    item = {"type": "table", "page_idx": 0, "table_caption": ["Table 1"]}
    block = {"type": "table", "content": [{"type": "table_caption",
             "content": "Table 1", "bbox": [.1, .2, .3, .4]}]}
    for blocks in ([], [block, block]):
        _add_caption_boxes([item], {"pages": [{"page_idx": 0, "blocks": blocks}]})
        assert "table_caption_bboxes" not in item


@pytest.mark.parametrize("field", ["image_caption", "image_footnote"])
def test_convert_preserves_image_annotation_boxes(monkeypatch, tmp_path, field):
    text = "b) 不带水泵"
    middle = {"pages": [{"page_idx": 21, "blocks": [{
        "type": "image", "content": [
            {"type": field, "bbox": [.61, .38, .72, .40],
             "content": [{"type": "text", "content": text}]},
            {"type": "image_body", "bbox": [.499, .138, .836, .340]},
        ],
    }]}]}
    rendered = [{"type": "image", "page_idx": 21, "img_path": "image.jpg",
                 "bbox": [499, 138, 836, 340], field: [text]}]
    monkeypatch.setitem(sys.modules, "mineru.parser", SimpleNamespace(
        ParseResult=SimpleNamespace(from_json=lambda raw: SimpleNamespace(middle_json=middle))))
    monkeypatch.setitem(sys.modules, "mineru.render", SimpleNamespace(
        RenderFormat=SimpleNamespace(CONTENT_LIST="content_list"), render=lambda *args: rendered))
    source, target = tmp_path / "middle.json", tmp_path / "output.json"
    source.write_text(json.dumps(middle))
    convert(source, target)
    item = json.loads(target.read_text())["pages"][21]["items"][0]
    assert item[field + "_bboxes"] == [[610, 380, 720, 400]]
    assert item["bbox"] == [499, 138, 836, 340]


def test_image_footnotes_match_their_own_picture():
    items = [{"type": "image", "page_idx": 21, "image_footnote": [text]}
             for text in ("a) 自带水泵", "b) 不带水泵")]
    blocks = [{"type": "image", "content": [{"type": "image_footnote",
               "content": item["image_footnote"][0], "bbox": box}]}
              for item, box in zip(items, ([.2, .4, .3, .42], [.6, .4, .7, .42]))]
    _add_caption_boxes(items, {"pages": [{"page_idx": 21, "blocks": blocks[::-1]}]})
    assert items[0]["image_footnote_bboxes"] == [[200, 400, 300, 420]]
    assert items[1]["image_footnote_bboxes"] == [[600, 400, 700, 420]]
