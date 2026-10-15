"""Recover tables that MinerU's VLM truncated at its output limit.

MinerU recognises each table crop in one VLM request. With the llama.cpp engine
that request gets an 8192-token slot (the GGUF's training context), and the
image tokens use part of it. A large table (record 371, Table E.1: 50 rows x 36
columns per page) therefore stops after about 26 rows. MinerU keeps the partial
output and only logs TRUNCATION_MARKER, and the OTSL-to-HTML step closes the
table cleanly, so nothing downstream can tell rows are missing.

A larger context is not a fix: past about 10k positions the model silently
merges and skips rows. Instead, a truncated table is recognised again in row
bands, with the header repeated on top of each band, and the rows are joined.
Bands are cut on the table's horizontal ruling lines, so a table without them
is reported as unrepaired.

Run with MinerU's Python, like mineru_v4_adapter.py:
    python mineru_table_repair.py <content_list.json> <images_root>
It rewrites the content list in place and prints a JSON report to stdout.
"""

import io
import json
import logging
import os
import re
import sys
from pathlib import Path
from typing import Callable

import numpy as np
from PIL import Image

log = logging.getLogger(__name__)

# Logged by mineru_vl_utils when a VLM request ends with finish_reason=length.
TRUNCATION_MARKER = "The output was truncated due to length limit"
# A truncated table filled most of the token budget, so its HTML is long. Shorter
# tables are not worth a re-recognition pass.
MIN_CANDIDATE_CHARS = 4000

# recognize(image) -> (table html, finished). finished is False when the model
# stopped at its length limit.
Recognizer = Callable[[Image.Image], tuple[str, bool]]

_ROW_RE = re.compile(r"<tr\b.*?</tr>", re.S)


class TableSplitError(RuntimeError):
    pass


def _line_spans(gray: np.ndarray) -> list[tuple[int, int]]:
    """(first, last) pixel row of each horizontal ruling line."""
    dark = np.flatnonzero((gray < 128).mean(axis=1) > 0.5)
    spans: list[tuple[int, int]] = []
    for y in dark.tolist():
        if spans and y - spans[-1][1] <= 2:
            spans[-1] = (spans[-1][0], y)
        else:
            spans.append((y, y))
    return spans


def row_separators(gray: np.ndarray) -> list[int]:
    """Centre y of each horizontal ruling line, top to bottom."""
    return [(first + last) // 2 for first, last in _line_spans(gray)]


def header_row_count(html: str) -> int:
    """Header depth, taken from the tallest rowspan in the first row."""
    rows = _ROW_RE.findall(html)
    if not rows:
        return 1
    return max((int(n) for n in re.findall(r'rowspan="(\d+)"', rows[0])), default=1)


def _split_bands(image: Image.Image, header_rows: int) -> tuple[Image.Image, Image.Image]:
    """Cut the body at the ruling line nearest its middle; both bands get the header."""
    spans = _line_spans(np.asarray(image.convert("L")))
    if len(spans) < header_rows + 3:
        raise TableSplitError(f"only {len(spans)} ruling lines; cannot split below a {header_rows}-row header")
    header_end = spans[header_rows][1] + 1
    inner = spans[header_rows + 1:-1]
    middle = (header_end + spans[-1][0]) / 2
    cut_end = min(inner, key=lambda span: abs((span[0] + span[1]) / 2 - middle))[1] + 1
    width, height = image.size
    header = image.crop((0, 0, width, header_end))
    bands = []
    for top, bottom in ((header_end, cut_end), (cut_end, height)):
        band = Image.new(image.mode, (width, header_end + bottom - top), "white")
        band.paste(header, (0, 0))
        band.paste(image.crop((0, top, width, bottom)), (0, header_end))
        bands.append(band)
    return bands[0], bands[1]


def _merge(first: str, second: str, header_rows: int) -> str:
    rows = _ROW_RE.findall(first) + _ROW_RE.findall(second)[header_rows:]
    return "<table>" + "".join(rows) + "</table>"


def _recognize_split(image: Image.Image, recognize: Recognizer, header_rows: int) -> str:
    """Recognise each band, splitting again any band that is still truncated."""
    merged = ""
    for band in _split_bands(image, header_rows):
        html, finished = recognize(band)
        if not finished:
            html = _recognize_split(band, recognize, header_rows)
        merged = _merge(merged, html, header_rows) if merged else html
    return merged


def repair_table(image: Image.Image, recognize: Recognizer, header_rows: int) -> str | None:
    """Full table HTML rebuilt from bands, or None when the whole crop fits."""
    _, finished = recognize(image)
    if finished:
        return None
    return _recognize_split(image, recognize, header_rows)


def repair_content_list(doc: dict, images_root: str, recognize: Recognizer) -> list[dict]:
    """Repair truncated tables in a page-wrapped content list, in place."""
    report = []
    for page in doc.get("pages", []):
        for item in page.get("items", []):
            body = item.get("table_body") or ""
            if item.get("type") != "table" or not item.get("img_path") or len(body) < MIN_CANDIDATE_CHARS:
                continue
            entry = {
                "page_number": page.get("page_number"),
                "img_path": item["img_path"],
                "rows_before": len(_ROW_RE.findall(body)),
            }
            try:
                with Image.open(os.path.join(images_root, item["img_path"])) as image:
                    html = repair_table(image, recognize, header_row_count(body))
            except (TableSplitError, OSError) as exc:
                entry.update(status="unrepaired", reason=str(exc))
            else:
                if html is None:
                    entry["status"] = "complete"
                else:
                    item["table_body"] = html
                    entry.update(status="repaired", rows_after=len(_ROW_RE.findall(html)))
            report.append(entry)
    return report


def _llama_cpp_recognizer() -> Recognizer | None:
    """MinerU's table request on its own llama.cpp model; None for other engines."""
    from mineru.config import config
    from mineru.model.registry import vlm_model_repo
    from mineru.model.vlm.selector import resolve_vlm_engine
    from mineru_llama_cpp import Engine
    from mineru_vl_utils.mineru_client import DEFAULT_PROMPTS, DEFAULT_SAMPLING_PARAMS
    from mineru_vl_utils.post_process.otsl2html import convert_otsl_to_html
    from mineru_vl_utils.vlm_client.llama_cpp_engine_client import LlamaCppEngineVlmClient
    from mineru_vl_utils.vlm_client.utils import get_image_data_url

    if config.model.vlm.server_url or resolve_vlm_engine() != "llama-cpp":
        return None
    repo = vlm_model_repo("llama-cpp")
    model_dir = Path(repo.ensure())
    engine = Engine(model_dir / repo.paths["main"], model_dir / repo.paths["mmproj"], n_parallel=1)
    client = LlamaCppEngineVlmClient(engine, use_tqdm=False)
    params = client.build_llama_cpp_sampling_params(DEFAULT_SAMPLING_PARAMS["table"])

    def recognize(image: Image.Image) -> tuple[str, bool]:
        buf = io.BytesIO()
        image.convert("RGB").save(buf, format="PNG")
        messages = client.build_messages([get_image_data_url(buf.getvalue(), "png")], DEFAULT_PROMPTS["table"])
        result = engine.generate(messages, params)
        return convert_otsl_to_html(result.content), result.finish_reason == "stop"

    return recognize


def main(argv: list[str]) -> int:
    content_list_path, images_root = argv
    with open(content_list_path, encoding="utf-8") as f:
        doc = json.load(f)
    recognize = _llama_cpp_recognizer()
    if recognize is None:
        print(json.dumps({"skipped": "VLM engine is not local llama-cpp", "tables": []}))
        return 0
    report = repair_content_list(doc, images_root, recognize)
    if any(entry["status"] == "repaired" for entry in report):
        with open(content_list_path, "w", encoding="utf-8") as f:
            json.dump(doc, f, ensure_ascii=False)
    print(json.dumps({"tables": report}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
