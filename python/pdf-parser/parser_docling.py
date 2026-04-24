"""Docling PDF parser backend.

Uses the local Python Docling package to convert PDFs and writes the exported
document JSON into the parser output directory.
"""

import importlib
import json
import logging
import os
from typing import Any, Callable

from parser_base import ParserBackend

log = logging.getLogger(__name__)


class DoclingParser(ParserBackend):
    name = "docling"

    def __init__(self) -> None:
        self._initialized: bool = False

    def init(self) -> None:
        if self._initialized:
            return

        try:
            importlib.import_module("docling.document_converter")
        except ImportError as exc:
            raise RuntimeError(
                "DoclingParser requires the `docling` package. "
                "Install it with `uv pip install -e \".[docling,dev]\"`."
            ) from exc

        self._initialized = True
        log.info("DoclingParser initialized")

    def _create_converter(self):
        from docling.document_converter import DocumentConverter

        return DocumentConverter()

    def parse(
        self,
        pdf_path: str,
        output_dir: str,
        on_progress: Callable[[int, int], None],
    ) -> dict[str, Any]:
        on_progress(0, 1)

        converter = self._create_converter()
        result = converter.convert(pdf_path)
        raw_result = result.document.export_to_dict()

        pdf_stem = os.path.splitext(os.path.basename(pdf_path))[0]
        json_path = os.path.join(output_dir, f"{pdf_stem}.docling.json")
        with open(json_path, "w", encoding="utf-8") as f:
            json.dump(raw_result, f, ensure_ascii=False, indent=2)

        pages_obj = raw_result.get("pages", {})
        if isinstance(pages_obj, dict):
            page_numbers = sorted(
                int(key) for key in pages_obj.keys() if str(key).isdigit()
            )
            pages = [{"page_number": page_no, "items": []} for page_no in page_numbers]
            total_pages = len(page_numbers)
        else:
            pages = []
            total_pages = 0

        on_progress(1, 1)
        log.info("docling finished: pages=%d output=%s", total_pages, json_path)

        return {
            "pages": pages,
            "total_pages": total_pages,
            "engine": "docling",
            "raw": raw_result,
            "json_path": json_path,
        }
