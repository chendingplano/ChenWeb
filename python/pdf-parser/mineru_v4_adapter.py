"""Render MinerU 4 MiddleJson V2 as ChenWeb's page-wrapped content list."""

import json
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any

from mineru.parser import ParseResult
from mineru.render import RenderFormat, render


def _text(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return "".join(_text(part) for part in value)
    if isinstance(value, dict):
        if isinstance(value.get("content"), str):
            return value["content"]
        return _text(value.get("content", []))
    return ""


def _list_leaf_boxes(value: Any) -> list[tuple[str, list[float]]]:
    """Collect text and geometry for list descendants from MiddleJson V2."""
    found: list[tuple[str, list[float]]] = []
    if isinstance(value, dict):
        content = value.get("content")
        bbox = value.get("bbox")
        text = _text(content).strip()
        if text and isinstance(bbox, list) and len(bbox) == 4:
            # MiddleJson uses normalized 0..1 coordinates; Content List V1
            # uses the equivalent 0..1000 page coordinate space.
            found.append((text, [round(float(value) * 1000) for value in bbox]))
        for key, child in value.items():
            if key != "bbox":
                found.extend(_list_leaf_boxes(child))
    elif isinstance(value, list):
        for child in value:
            found.extend(_list_leaf_boxes(child))
    return found


def _add_list_item_boxes(items: list[dict[str, Any]], middle: dict[str, Any]) -> None:
    page_blocks: dict[int, list[Any]] = defaultdict(list)
    for page in middle.get("pages", []):
        page_idx = page.get("page_idx")
        if isinstance(page_idx, int):
            page_blocks[page_idx].extend(page.get("blocks", []))

    for item in items:
        if item.get("type") != "list" or not item.get("list_items"):
            continue
        candidates: list[tuple[str, list[float]]] = []
        for block in page_blocks.get(item["page_idx"], []):
            if isinstance(block, dict) and block.get("type") == "list":
                candidates.extend(_list_leaf_boxes(block))
        boxes: list[list[float]] = []
        for entry in item["list_items"]:
            text = str(entry).strip()
            match = next((bbox for leaf, bbox in candidates if leaf == text), None)
            if match is None:
                boxes = []
                break
            boxes.append(match)
            candidates = [(leaf, bbox) for leaf, bbox in candidates if bbox != match]
        if len(boxes) == len(item["list_items"]):
            item["list_item_bboxes"] = boxes


def convert(middle_path: Path, output_path: Path) -> None:
    raw = middle_path.read_text(encoding="utf-8")
    result = ParseResult.from_json(raw)
    middle = json.loads(raw)
    content_list = render(result.middle_json, RenderFormat.CONTENT_LIST)
    if not isinstance(content_list, list):
        raise ValueError("MinerU CONTENT_LIST renderer did not return a list")

    pages_map: dict[int, list[dict[str, Any]]] = defaultdict(list)
    supported = {
        "text", "list", "table", "image", "equation", "header", "footer",
        "page_number", "page_footnote", "chart", "code", "index",
    }
    fields = {
        "type", "text", "text_level", "bbox", "list_items", "list_item_bboxes",
        "table_body", "table_caption", "table_footnote", "image_caption",
        "image_footnote", "img_path",
    }
    for source in content_list:
        if not isinstance(source, dict):
            continue
        page_idx = source.get("page_idx")
        if not isinstance(page_idx, int) or page_idx < 0:
            raise ValueError(f"Invalid MinerU page_idx: {page_idx!r}")
        kind = str(source.get("type", "text")).lower()
        item = {key: source[key] for key in fields if key in source}
        if kind not in supported:
            text = _text(source.get("text", source.get("content", "")))
            if not text:
                continue
            item = {"type": "text", "text": text}
            if "bbox" in source:
                item["bbox"] = source["bbox"]
        elif kind in {"chart", "code", "index", "page_footnote"}:
            text = _text(source.get("text", source.get("content", source.get("list_items", []))))
            if text:
                item = {"type": "text", "text": text}
                if "bbox" in source:
                    item["bbox"] = source["bbox"]
            else:
                continue
        elif "type" not in item:
            item["type"] = kind
        pages_map[page_idx].append(item)

    # Enrich each page's items while carrying the renderer's zero-based page index.
    for page_idx, page_items in pages_map.items():
        enriched = [item | {"page_idx": page_idx} for item in page_items]
        _add_list_item_boxes(enriched, middle)
        for item, normalized in zip(page_items, enriched):
            if "list_item_bboxes" in normalized:
                item["list_item_bboxes"] = normalized["list_item_bboxes"]

    page_count = max((page.get("page_idx", -1) for page in middle.get("pages", [])), default=-1) + 1
    page_count = max(page_count, max(pages_map, default=-1) + 1)
    pages = [
        {"page_number": page_idx + 1, "items": pages_map.get(page_idx, [])}
        for page_idx in range(page_count)
    ]
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps({"pages": pages}, ensure_ascii=False), encoding="utf-8")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: mineru_v4_adapter.py MIDDLE_JSON OUTPUT_JSON")
    convert(Path(sys.argv[1]), Path(sys.argv[2]))
