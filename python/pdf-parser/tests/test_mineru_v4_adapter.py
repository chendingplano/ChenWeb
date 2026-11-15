import json
import sys
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
