#!/usr/bin/env python3
"""Parse a sample PDF without claiming a database record or publishing events."""

import argparse
import json
import logging
from pathlib import Path

import fitz
from parser_mineru import MineruParser, make_ocr_parser

cli = argparse.ArgumentParser(description=__doc__)
cli.add_argument("input", type=Path)
cli.add_argument("output", type=Path)
cli.add_argument("--ocr", action="store_true")
args = cli.parse_args()
logging.basicConfig(level=logging.INFO)
backend = make_ocr_parser() if args.ocr else MineruParser()
backend.init()
result = backend.parse(str(args.input), str(args.output), lambda done, total: None)
with fitz.open(args.input) as source:
    assert result["total_pages"] == len(source), "Incorrect page count"
items = [item for page in result["pages"] for item in page["items"]]
text = " ".join(str(item.get("text", "")) for item in items).strip()
assert len(text) > 10, "The sample produced no useful text"
assert Path(result["content_list_path"]).is_file(), "Content list was not saved"
summary = {
    "pages": result["total_pages"], "items": len(items),
    "images": result["image_count"], "tier": backend._tier,
    "text_preview": text[:240], "content_list": result["content_list_path"],
}
(args.output / "smoke-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))
print(json.dumps(summary, ensure_ascii=False))
