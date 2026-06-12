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
from typing import Any, Callable

from parser_base import ParserBackend

log = logging.getLogger(__name__)


def _env(key: str, default: str = "") -> str:
    return os.environ.get(key, default).strip()


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
        on_progress(0, 1)

        cmd: list[str] = [self._cli, "-p", pdf_path, "-o", output_dir]
        if self._backend:
            cmd += ["-b", self._backend]
        if self._extra_args:
            cmd += self._extra_args

        log.info("running mineru: %s", " ".join(cmd))
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
        for line in proc.stdout:
            line = line.rstrip()
            if not line:
                continue
            log.info("[mineru] %s", line)
            tail.append(line)
            if len(tail) > 50:
                tail.pop(0)
        rc = proc.wait()
        if rc != 0:
            raise RuntimeError(
                f"mineru exited {rc}: " + "\n".join(tail[-20:])
            )

        # Locate content_list.json. MinerU nests it under
        # <output_dir>/<pdf_stem>/<backend>_auto/<pdf_stem>_content_list.json.
        pdf_stem = os.path.splitext(os.path.basename(pdf_path))[0]
        candidates = sorted(glob.glob(
            os.path.join(output_dir, pdf_stem, "*", f"{pdf_stem}_content_list.json")
        ))
        if not candidates:
            candidates = sorted(glob.glob(
                os.path.join(output_dir, "**", "*_content_list.json"),
                recursive=True,
            ))
        if not candidates:
            raise FileNotFoundError(
                f"mineru: no *_content_list.json found under {output_dir}"
            )

        content_list_path = candidates[0]
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
