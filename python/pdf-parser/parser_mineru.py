"""MinerU PDF parser backend.

Invokes the `mineru` CLI (from ~/Workspace/ThirdParty/mineru) as a subprocess.
MinerU writes a directory tree under the output dir:

    <output_dir>/<pdf_stem>/<backend>_auto/
        <pdf_stem>_content_list.json   ← canonical structured output
        <pdf_stem>.md
        <pdf_stem>_middle.json
        <pdf_stem>_model.json
        <pdf_stem>_layout.pdf
        images/...

We locate `*_content_list.json` after the run and return its entries as pages.
"""

import glob
import json
import logging
import os
import re
import shutil
import subprocess
import threading
from typing import Any, Callable

from parser_base import ParserBackend

log = logging.getLogger(__name__)

# Matches "X/Y" in tqdm output like "Processing pages: 60%|██| 3/5 [00:06<00:04]"
_PAGE_PROGRESS_RE = re.compile(r'\b(\d+)/(\d+)\b')


def _env(key: str, default: str = "") -> str:
    return os.environ.get(key, default).strip()


def _get_pdf_page_count(pdf_path: str) -> int:
    """Return the page count of the PDF, or 0 if it cannot be determined."""
    try:
        import fitz  # PyMuPDF — available in the parser venv
        with fitz.open(pdf_path) as doc:
            return len(doc)
    except Exception as exc:
        log.debug("_get_pdf_page_count fitz failed: %s", exc)
    return 0


class MineruParser(ParserBackend):
    name = "mineru"

    def __init__(self) -> None:
        self._cli: str = ""
        self._backend: str = ""
        self._extra_args: list[str] = []
        self._initialized: bool = False

    def init(self) -> None:
        if self._initialized:
            return

        cli = _env("MINERU_CLI") or shutil.which("mineru") or ""
        if not cli:
            raise RuntimeError(
                "MineruParser requires the `mineru` CLI on PATH. "
                "Install via ~/Workspace/ThirdParty/mineru (`mise run install`) "
                "or set MINERU_CLI."
            )
        self._cli = cli
        self._backend = _env("MINERU_BACKEND")

        extra = _env("MINERU_EXTRA_ARGS")
        if extra:
            self._extra_args = extra.split()

        self._initialized = True
        log.info(
            "MineruParser initialized: cli=%s backend=%s",
            self._cli, self._backend or "(default)",
        )

    def parse(
        self,
        pdf_path: str,
        output_dir: str,
        on_progress: Callable[[int, int], None],
    ) -> dict[str, Any]:
        # Use the real page count so the dashboard shows accurate total from
        # the start rather than the placeholder value of 1.
        pdf_page_count = _get_pdf_page_count(pdf_path)
        total_pages_hint = max(pdf_page_count, 1)
        on_progress(0, total_pages_hint)

        # Clear any prior mineru output for this PDF. MinerU nests per-run output
        # under <output_dir>/<pdf_stem>/<backend>_<method>/; across re-parses (and
        # the OCR-fallback's second pass) these accumulate, wasting disk and — before
        # the mtime-based selection below — risking returning a stale run's content.
        # The source PDF lives at <output_dir>/<pdf_stem>.pdf (a file), not this dir,
        # so removing the directory never touches the input.
        pdf_stem = os.path.splitext(os.path.basename(pdf_path))[0]
        nested_output = os.path.join(output_dir, pdf_stem)
        if os.path.isdir(nested_output):
            shutil.rmtree(nested_output, ignore_errors=True)

        cmd: list[str] = [self._cli, "-p", pdf_path, "-o", output_dir]
        if self._backend:
            cmd += ["-b", self._backend]
        if self._extra_args:
            cmd += self._extra_args

        log.info("running mineru: %s", " ".join(cmd))

        # MinerU is a long-running subprocess. tqdm uses \r (not \n) for
        # progress updates, so line-by-line reading can block for minutes
        # without yielding anything. A heartbeat thread calls on_progress
        # every 15 s so ms_used keeps advancing in the DB.
        #
        # psycopg2 connections are not thread-safe, so we guard all
        # on_progress calls with a non-blocking lock: the heartbeat skips
        # a tick if the main thread is currently doing a DB write.
        _progress_lock = threading.Lock()
        # Shared mutable state for heartbeat thread (dict is GIL-safe).
        _state: dict[str, int] = {"pages_done": 0, "total": total_pages_hint}
        _stop_heartbeat = threading.Event()

        def _heartbeat() -> None:
            while not _stop_heartbeat.wait(timeout=15.0):
                if _progress_lock.acquire(blocking=False):
                    try:
                        on_progress(_state["pages_done"], _state["total"])
                    except Exception as exc:
                        log.debug("heartbeat on_progress error: %s", exc)
                    finally:
                        _progress_lock.release()

        heartbeat_thread = threading.Thread(target=_heartbeat, daemon=True)
        heartbeat_thread.start()

        # Stream output line-by-line so the stdout/stderr pipe can never fill
        # and deadlock a long-running mineru subprocess. Keep last N lines for
        # the error message on non-zero exit.
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
        tail: list[str] = []
        assert proc.stdout is not None
        try:
            for raw_line in proc.stdout:
                # tqdm uses \r to overwrite the current line; take the last
                # non-empty segment so log output is clean.
                segments = [s for s in raw_line.rstrip('\n').split('\r') if s.strip()]
                line = segments[-1] if segments else raw_line.rstrip()
                if not line:
                    continue
                log.info("[mineru] %s", line)
                tail.append(line)
                if len(tail) > 50:
                    tail.pop(0)

                # Extract page progress from tqdm lines, e.g.
                # "Processing pages: 60%|██| 3/5 [00:06<00:04]"
                for seg in segments or [line]:
                    m = _PAGE_PROGRESS_RE.search(seg)
                    if m:
                        x, y = int(m.group(1)), int(m.group(2))
                        if y > 0:
                            _state["pages_done"] = x
                            if y > _state["total"]:
                                _state["total"] = y
                            break
        finally:
            _stop_heartbeat.set()
            heartbeat_thread.join(timeout=5)

        rc = proc.wait()
        if rc != 0:
            raise RuntimeError(
                f"mineru exited {rc}: " + "\n".join(tail[-20:])
            )

        content_list_path = _find_content_list(output_dir, pdf_stem)
        with open(content_list_path, "r", encoding="utf-8") as f:
            content_list = json.load(f)

        middle_json_path = content_list_path.replace("_content_list.json", "_middle.json")
        equation_image_count = 0
        if os.path.isfile(middle_json_path) and isinstance(content_list, list):
            try:
                with open(middle_json_path, "r", encoding="utf-8") as f:
                    middle_json = json.load(f)
                equation_image_count = annotate_equation_image_paths(content_list, middle_json)
            except Exception as exc:
                log.warning("mineru: failed to annotate equation image paths: %s", exc)

        # Copy images from MinerU's nested output dir to output_dir/images/ so
        # that img_path values ("images/foo.png") in the content list resolve
        # correctly relative to the aggregated result JSON at output_dir/.
        images_src_dir = os.path.join(os.path.dirname(content_list_path), "images")
        images_dst_dir = os.path.join(output_dir, "images")
        image_count = 0
        if os.path.isdir(images_src_dir):
            os.makedirs(images_dst_dir, exist_ok=True)
            for img_file in os.listdir(images_src_dir):
                src = os.path.join(images_src_dir, img_file)
                if os.path.isfile(src):
                    shutil.copy2(src, os.path.join(images_dst_dir, img_file))
                    image_count += 1
            log.info("mineru: copied %d image(s) to %s", image_count, images_dst_dir)

        # Group content items by page_idx so downstream consumers see pages.
        pages_map: dict[int, list[Any]] = {}
        for item in content_list if isinstance(content_list, list) else []:
            if not isinstance(item, dict):
                continue
            page_idx = int(item.get("page_idx", 0))
            pages_map.setdefault(page_idx, []).append(item)

        if pages_map:
            total_pages = max(pages_map.keys()) + 1
            pages = [
                {"page_number": i + 1, "items": pages_map.get(i, [])}
                for i in range(total_pages)
            ]
        else:
            total_pages = 0
            pages = []

        on_progress(max(total_pages, 1), max(total_pages, 1))
        log.info(
            "mineru finished: pages=%d images=%d equation_images=%d content_list=%s",
            total_pages, image_count, equation_image_count, content_list_path,
        )

        return {
            "pages": pages,
            "total_pages": total_pages,
            "engine": "mineru",
            "content_list_path": content_list_path,
            "images_dir": images_dst_dir if image_count > 0 else "",
            "image_count": image_count,
        }


def _find_content_list(output_dir: str, pdf_stem: str) -> str:
    """Return the content_list.json this run produced.

    MinerU nests output under <output_dir>/<pdf_stem>/<backend>_<method>/, e.g.
    hybrid_auto/, hybrid_ocr/, ocr/. When several coexist (e.g. the OCR-fallback
    re-parse, or leftovers from a backend/method change), we must select the one
    written by the current run — the most recently modified — rather than the
    alphabetically first, which would silently return a stale, possibly garbled
    parse. Raises FileNotFoundError if none is found.
    """
    candidates = glob.glob(
        os.path.join(output_dir, pdf_stem, "*", f"{pdf_stem}_content_list.json")
    )
    if not candidates:
        candidates = glob.glob(
            os.path.join(output_dir, "**", "*_content_list.json"),
            recursive=True,
        )
    if not candidates:
        raise FileNotFoundError(
            f"mineru: no *_content_list.json found under {output_dir}"
        )
    return max(candidates, key=os.path.getmtime)


def make_ocr_parser(lang: str = "ch") -> "MineruParser":
    """Build a MineruParser pinned to the `pipeline` backend with forced OCR.

    Used as a fallback when the normal (text-layer) extraction yields
    font-encoding garble — e.g. GB/T standard PDFs whose Latin glyphs live in a
    font without a usable ToUnicode CMap, so text extraction maps them into the
    CJK block. The `pipeline` backend OCRs rendered pixels and sidesteps the
    broken encoding; `hybrid`/`vlm` backends reuse the text layer and would
    reproduce the garble, so they are deliberately not used here.
    """
    parser = MineruParser()
    parser.init()
    parser._backend = "pipeline"
    parser._extra_args = ["-m", "ocr", "-l", lang]
    return parser


def annotate_equation_image_paths(content_list: list[Any], middle_json: Any) -> int:
    equation_images = _extract_equation_images(middle_json)
    if not equation_images:
        return 0

    by_content: dict[str, list[str]] = {}
    for equation in equation_images:
        key = _normalize_equation_text(equation.get("content", ""))
        image_path = _normalize_mineru_image_path(equation.get("image_path", ""))
        if key and image_path:
            by_content.setdefault(key, []).append(image_path)

    changed = 0
    for item in content_list:
        if not isinstance(item, dict):
            continue
        if str(item.get("type", "")).strip().lower() != "equation":
            continue
        if str(item.get("img_path", "")).strip():
            continue

        key = _normalize_equation_text(str(item.get("text", "")))
        candidates = by_content.get(key) or []
        if not candidates:
            continue

        item["img_path"] = candidates.pop(0)
        changed += 1

    return changed


def _extract_equation_images(value: Any) -> list[dict[str, str]]:
    found: list[dict[str, str]] = []

    def walk(node: Any) -> None:
        if isinstance(node, dict):
            node_type = str(node.get("type", "")).strip().lower()
            image_path = str(node.get("image_path", "")).strip()
            content = str(node.get("content", "")).strip()
            if image_path and content and "equation" in node_type:
                found.append({"content": content, "image_path": image_path})
            for child in node.values():
                walk(child)
        elif isinstance(node, list):
            for child in node:
                walk(child)

    walk(value)
    return found


def _normalize_equation_text(text: str) -> str:
    text = text.strip()
    text = re.sub(r"^\s*(\$\$|\\\[)", "", text)
    text = re.sub(r"(\$\$|\\\])\s*$", "", text)
    return re.sub(r"\s+", "", text)


def _normalize_mineru_image_path(image_path: str) -> str:
    image_path = image_path.strip()
    if not image_path:
        return ""
    if image_path.startswith("images/"):
        return image_path
    return "images/" + image_path
