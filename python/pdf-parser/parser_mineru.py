"""MinerU PDF parser backend.

Invokes MinerU Kit from ~/Workspace/ThirdParty/mineru and adapts its ZIP result
to ChenWeb's persisted page-wrapped content-list shape.
"""

import json
import logging
import os
import platform
import re
import shutil
import subprocess
import threading
import time
import tempfile
import zipfile
from typing import Any, Callable

from parser_base import ParserBackend

log = logging.getLogger(__name__)

# Matches "X/Y" in tqdm output like "Processing pages: 60%|██| 3/5 [00:06<00:04]"
_PAGE_PROGRESS_RE = re.compile(r'\b(\d+)/(\d+)\b')
LINUX_PHASE_NAMES = (
    "Layout Predict", "MFR Predict", "Table-ocr det", "Table-ocr rec ch",
    "Table-wireless Predict", "Table-wired Predict", "OCR-det ch",
    "OCR-rec Predict", "Processing pages",
)
MAC_PHASE_NAMES = ("Layout Preparation", "Extract Preparation", "Post Processing", "Processing pages")
_PHASE_PROGRESS_RE = re.compile(
    r'(' + '|'.join(re.escape(name) for name in (*LINUX_PHASE_NAMES, *MAC_PHASE_NAMES, "Predict")) + r'):\s*\d+%\|[^|]*\|\s*(\d+)/(\d+)'
)


def _processing_pages(line: str) -> tuple[int, int] | None:
    if "Processing pages:" not in line:
        return None
    match = _PAGE_PROGRESS_RE.search(line)
    if not match:
        return None
    done, total = int(match.group(1)), int(match.group(2))
    return (done, total) if total > 0 else None


class PhaseTracker:
    """Turn host-specific MinerU tqdm updates into ordered phase snapshots."""

    def __init__(self, system: str | None = None) -> None:
        self.mac = (system or platform.system()) == "Darwin"
        self.names = MAC_PHASE_NAMES if self.mac else LINUX_PHASE_NAMES
        self.phases = [dict(name=name, progress=0, status="pending") for name in self.names]
        self.started: dict[str, float] = {}
        self.predict_phase = 0

    def update(self, line: str) -> tuple[list[dict], bool] | None:
        match = _PHASE_PROGRESS_RE.search(line)
        if not match:
            return None
        name, done, total = match.group(1), int(match.group(2)), int(match.group(3))
        if total <= 0:
            return None
        if self.mac:
            if name in MAC_PHASE_NAMES[:2]:
                self.predict_phase = MAC_PHASE_NAMES.index(name)
                # The preparation counter finishes before its costly Predict pass.
                # Keep the combined phase active until Predict reaches 100%.
                done = 0
            elif name == "Predict":
                name = MAC_PHASE_NAMES[self.predict_phase]
            elif name not in MAC_PHASE_NAMES:
                return None
        elif name not in LINUX_PHASE_NAMES:
            return None
        phase = self.phases[self.names.index(name)]
        now = time.monotonic()
        if name not in self.started:
            self.started[name] = now
        progress = min(100, int(done * 100 / total))
        complete = done >= total
        if phase["status"] == "complete":
            return None
        if phase["progress"] == progress and phase["status"] == ("complete" if complete else "active"):
            return None
        phase["progress"] = progress
        phase["status"] = "complete" if complete else "active"
        if complete:
            phase["elapsed_seconds"] = round(now - self.started[name], 2)
        return [dict(item) for item in self.phases], complete or progress == 0

_DEFAULT_MINERU_CLI_PATHS = [
    os.path.expanduser("~/Workspace/ThirdParty/mineru/.venv/bin/mineru-kit"),
]


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


def _terminate_and_wait(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        return
    try:
        proc.terminate()
    except OSError:
        pass
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()


def _promote_result_pair(
    content_src: str,
    images_src: str,
    content_dst: str,
    images_dst: str,
    staging_dir: str,
) -> None:
    """Replace a previously successful JSON/assets pair, rolling back on failure."""
    backup_dir = os.path.join(staging_dir, "previous")
    os.makedirs(backup_dir, exist_ok=True)
    backups: list[tuple[str, str]] = []
    promoted: list[str] = []
    try:
        for destination in (content_dst, images_dst):
            if os.path.exists(destination):
                backup = os.path.join(backup_dir, os.path.basename(destination))
                os.replace(destination, backup)
                backups.append((backup, destination))
        for source, destination in ((content_src, content_dst), (images_src, images_dst)):
            os.replace(source, destination)
            promoted.append(destination)
    except Exception:
        for destination in reversed(promoted):
            if os.path.isdir(destination):
                shutil.rmtree(destination)
            elif os.path.exists(destination):
                os.remove(destination)
        for backup, destination in reversed(backups):
            if os.path.exists(backup):
                os.replace(backup, destination)
        raise


class MineruParser(ParserBackend):
    name = "mineru"

    def __init__(self) -> None:
        self._cli: str = ""
        self._python: str = ""
        self._tier = "standard"
        self._ocr_mode = "auto"
        self._initialized: bool = False
        self.on_phase_progress: Callable[[list[dict], bool], None] | None = None

    def init(self) -> None:
        if self._initialized:
            return

        cli = _env("MINERU_CLI")
        if cli and os.path.isfile(cli):
            self._cli = cli
        else:
            self._cli = shutil.which("mineru-kit") or ""
            if not self._cli:
                for candidate in _DEFAULT_MINERU_CLI_PATHS:
                    if os.path.isfile(candidate):
                        self._cli = candidate
                        break
        if not self._cli:
            raise RuntimeError(
                "MineruParser requires the `mineru-kit` CLI on PATH. "
                "Install via ~/Workspace/ThirdParty/mineru (`mise run install`) "
                "or set MINERU_CLI."
            )
        configured_python = _env("MINERU_PYTHON")
        if configured_python:
            self._python = os.path.abspath(os.path.expanduser(configured_python))
            if not os.path.isfile(self._python) or not os.access(self._python, os.X_OK):
                raise RuntimeError(f"MINERU_PYTHON is not an executable file: {self._python}")
        else:
            cli_path = os.path.realpath(self._cli)
            cli_dir = os.path.dirname(cli_path)
            self._python = next(
                (path for path in (os.path.join(cli_dir, "python"), os.path.join(cli_dir, "python3"))
                 if os.path.isfile(path) and os.access(path, os.X_OK)),
                "",
            )
            if not self._python:
                raise RuntimeError(
                    "Cannot find the MinerU virtualenv Python beside MINERU_CLI. "
                    "Set MINERU_PYTHON to the Python executable from the MinerU 4 environment."
                )

        self._initialized = True
        log.info(
            "MineruParser initialized: cli=%s python=%s tier=%s",
            self._cli, self._python, self._tier,
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
        total_pages_hint = pdf_page_count
        on_progress(0, total_pages_hint)

        os.makedirs(output_dir, exist_ok=True)
        pdf_stem = os.path.splitext(os.path.basename(pdf_path))[0]
        content_list_path = os.path.join(output_dir, f"{pdf_stem}_content_list.json")
        images_dst_dir = os.path.join(output_dir, "images")
        cmd: list[str] = [
            self._cli, "parse", pdf_path, "--pages", "all", "--tier", self._tier,
            "--format", "zip", "-o",
        ]
        if self._ocr_mode != "auto":
            cmd += ["--ocr-mode", self._ocr_mode]

        log.info("running MinerU Kit: %s", " ".join(cmd + ["<result.zip>"]))

        # MinerU is a long-running subprocess. tqdm uses \r (not \n) for
        # progress updates, so line-by-line reading can block for minutes
        # without yielding anything. A heartbeat thread calls on_progress
        # every 15 s so ms_used keeps advancing in the DB.
        #
        # psycopg2 connections are not thread-safe, so we guard all
        # on_progress calls with a non-blocking lock: the heartbeat skips
        # a tick if the main thread is currently doing a DB write.
        _progress_lock = threading.Lock()
        phase_tracker = PhaseTracker()
        # Shared mutable state for heartbeat thread (dict is GIL-safe).
        _state: dict[str, int] = {"pages_done": 0, "total": total_pages_hint}
        _stop_heartbeat = threading.Event()
        # Raised by on_progress when the user aborts. The heartbeat runs on a
        # background thread, so the exception can't just propagate — it's
        # stashed here and re-raised on the main thread once proc exits.
        _stop_exc: list[BaseException] = []

        # Stream output line-by-line so the stdout/stderr pipe can never fill
        # and deadlock a long-running mineru subprocess. Keep last N lines for
        # the error message on non-zero exit.
        with tempfile.TemporaryDirectory(prefix=".mineru-run-", dir=output_dir) as temp_dir:
            staged_content_path = os.path.join(temp_dir, os.path.basename(content_list_path))
            staged_images_dir = os.path.join(temp_dir, "images")
            result_zip = os.path.join(temp_dir, "result.zip")
            proc: subprocess.Popen | None = None
            heartbeat_thread: threading.Thread | None = None
            tail: list[str] = []
            try:
                proc = subprocess.Popen(
                    cmd + [result_zip],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    bufsize=1,
                )

                def _heartbeat() -> None:
                    while not _stop_heartbeat.wait(timeout=15.0):
                        if _progress_lock.acquire(blocking=False):
                            try:
                                on_progress(_state["pages_done"], _state["total"])
                            except Exception as exc:
                                _stop_exc.append(exc)
                                try:
                                    proc.terminate()
                                except OSError:
                                    pass
                            finally:
                                _progress_lock.release()

                heartbeat_thread = threading.Thread(target=_heartbeat, daemon=True)
                heartbeat_thread.start()

                assert proc.stdout is not None
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

                    # Only Processing pages counts pages. Other phase totals can
                    # be much larger (e.g. 303 extraction items for a 16-page PDF).
                    for seg in segments or [line]:
                        phase_update = phase_tracker.update(seg)
                        if phase_update and self.on_phase_progress:
                            with _progress_lock:
                                self.on_phase_progress(*phase_update)
                        page_progress = _processing_pages(seg)
                        if page_progress:
                            x, y = page_progress
                            _state["pages_done"] = x
                            if y > _state["total"]:
                                _state["total"] = y
                            break

                if _stop_exc:
                    raise _stop_exc[0]
                rc = proc.wait()
                if rc != 0:
                    raise RuntimeError(f"mineru exited {rc}: " + "\n".join(tail[-20:]))

                try:
                    with zipfile.ZipFile(result_zip) as archive:
                        middle_members = [
                            info for info in archive.infolist()
                            if os.path.basename(info.filename) == "middle_json.json"
                        ]
                        if len(middle_members) != 1:
                            raise RuntimeError(
                                f"MinerU ZIP must contain one middle_json.json (found {len(middle_members)})"
                            )
                        middle_json_path = os.path.join(temp_dir, "middle_json.json")
                        with archive.open(middle_members[0]) as source, open(middle_json_path, "wb") as target:
                            shutil.copyfileobj(source, target)
                        os.makedirs(staged_images_dir, exist_ok=True)
                        image_count = 0
                        for info in archive.infolist():
                            parts = info.filename.replace("\\", "/").split("/")
                            if "images" not in parts or info.is_dir():
                                continue
                            image_index = parts.index("images")
                            relative = parts[image_index + 1:]
                            if not relative or any(part in {"", ".", ".."} for part in relative):
                                continue
                            target_path = os.path.join(staged_images_dir, *relative)
                            os.makedirs(os.path.dirname(target_path), exist_ok=True)
                            with archive.open(info) as source, open(target_path, "wb") as target:
                                shutil.copyfileobj(source, target)
                            image_count += 1
                    adapter = os.path.join(os.path.dirname(__file__), "mineru_v4_adapter.py")
                    subprocess.run(
                        [self._python, adapter, middle_json_path, staged_content_path],
                        check=True, capture_output=True, text=True,
                    )
                except (OSError, zipfile.BadZipFile, subprocess.CalledProcessError) as exc:
                    detail = getattr(exc, "stderr", "")
                    raise RuntimeError(f"MinerU output adaptation failed: {exc} {detail}") from exc
                with open(staged_content_path, "r", encoding="utf-8") as f:
                    normalized = json.load(f)
                pages = normalized.get("pages")
                if not isinstance(pages, list):
                    raise ValueError("MinerU adapter output is missing pages")
                for page in pages:
                    for item in page.get("items", []):
                        image_path = item.get("img_path")
                        if image_path:
                            clean = str(image_path).replace("\\", "/").split("/")
                            if "images" in clean:
                                clean = clean[clean.index("images") + 1:]
                            else:
                                clean = clean[-1:]
                            if clean and all(part not in {"", ".", ".."} for part in clean):
                                item["img_path"] = "images/" + "/".join(clean)
                with open(staged_content_path, "w", encoding="utf-8") as f:
                    json.dump(normalized, f, ensure_ascii=False)
                total_pages = len(pages)
                on_progress(max(total_pages, 1), max(total_pages, 1))
                _promote_result_pair(
                    staged_content_path,
                    staged_images_dir,
                    content_list_path,
                    images_dst_dir,
                    temp_dir,
                )
            except BaseException:
                if proc is not None:
                    _terminate_and_wait(proc)
                raise
            finally:
                _stop_heartbeat.set()
                if heartbeat_thread is not None and heartbeat_thread.ident is not None:
                    heartbeat_thread.join(timeout=5)
                if proc is not None and proc.stdout is not None:
                    proc.stdout.close()

        log.info(
            "mineru finished: pages=%d images=%d content_list=%s",
            total_pages, image_count, content_list_path,
        )

        return {
            "pages": pages,
            "total_pages": total_pages,
            "engine": "mineru",
            "content_list_path": content_list_path,
            "images_dir": images_dst_dir if image_count > 0 else "",
            "image_count": image_count,
        }


def make_ocr_parser() -> "MineruParser":
    """Build a MinerU parser using the basic tier with OCR forced."""
    parser = MineruParser()
    parser.init()
    parser._tier = "basic"
    parser._ocr_mode = "ocr"
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


def annotate_list_item_bboxes(content_list: list[Any], middle_json: Any) -> int:
    """Attach a per-item bbox array to each "list" entry in content_list.

    MinerU's content_list.json collapses an entire list block into one shared
    bbox plus a `list_items` string array, so every exploded list-item line
    ends up with the same (too-large) bounding box downstream. middle_json
    keeps the original per-physical-line span bboxes that content_list was
    derived from. This pairs each content_list "list" item with the
    corresponding middle_json "list" para_block on the same page, in
    document order, and — only when the pairing is verified by both item
    count and leading text — attaches `list_item_bboxes`: one bbox per
    `list_items` entry, computed as the union of that item's line bboxes
    (list items that wrap multiple physical lines get one combined box).

    Returns the number of list items annotated.
    """
    pages = middle_json.get("pdf_info") if isinstance(middle_json, dict) else None
    if not isinstance(pages, list):
        return 0

    cl_lists_by_page: dict[int, list[dict[str, Any]]] = {}
    for item in content_list:
        if not isinstance(item, dict) or item.get("type") != "list":
            continue
        page_idx = item.get("page_idx")
        if not isinstance(page_idx, int):
            continue
        cl_lists_by_page.setdefault(page_idx, []).append(item)

    annotated = 0
    for page_idx, cl_lists in cl_lists_by_page.items():
        if page_idx < 0 or page_idx >= len(pages):
            continue
        page = pages[page_idx]
        para_blocks = page.get("para_blocks") if isinstance(page, dict) else None
        if not isinstance(para_blocks, list):
            continue
        mj_lists = [b for b in para_blocks if isinstance(b, dict) and b.get("type") == "list"]

        for cl_item, mj_block in zip(cl_lists, mj_lists):
            list_items = cl_item.get("list_items")
            sub_blocks = mj_block.get("blocks")
            if not isinstance(list_items, list) or not isinstance(sub_blocks, list):
                continue
            if not list_items or len(list_items) != len(sub_blocks):
                continue
            if not _list_item_text_matches(list_items[0], sub_blocks[0]):
                continue

            # content_list.json and middle_json use different coordinate
            # scales for the same page (middle_json is raw layout-detection
            # pixel space; content_list is MinerU's own normalized space).
            # Both give a bbox for this same list block, so derive the
            # per-axis affine map between the two spaces from that pair and
            # apply it to every sub-block bbox — otherwise the "correct"
            # per-item boxes land in the wrong coordinate system entirely.
            transform = _affine_params(cl_item.get("bbox"), mj_block.get("bbox"))
            if transform is None:
                continue

            bboxes: list[list[float]] = []
            for sub_block in sub_blocks:
                bbox = _union_bbox_from_lines(sub_block)
                if bbox is None:
                    bboxes = []
                    break
                bboxes.append(_apply_affine(bbox, transform))
            if not bboxes:
                continue

            cl_item["list_item_bboxes"] = bboxes
            annotated += len(bboxes)

    return annotated


def _affine_params(content_bbox: Any, middle_bbox: Any) -> tuple[float, float, float, float] | None:
    """Derive the per-axis (scale, offset) mapping middle_json's coordinate
    space onto content_list's, from two bboxes known to describe the same
    region in both spaces. Returns (scale_x, offset_x, scale_y, offset_y)."""
    if not (isinstance(content_bbox, list) and len(content_bbox) == 4):
        return None
    if not (isinstance(middle_bbox, list) and len(middle_bbox) == 4):
        return None
    try:
        cx1, cy1, cx2, cy2 = (float(v) for v in content_bbox)
        mx1, my1, mx2, my2 = (float(v) for v in middle_bbox)
    except (TypeError, ValueError):
        return None
    dx = mx2 - mx1
    dy = my2 - my1
    if dx <= 0 or dy <= 0:
        return None
    scale_x = (cx2 - cx1) / dx
    scale_y = (cy2 - cy1) / dy
    offset_x = cx1 - scale_x * mx1
    offset_y = cy1 - scale_y * my1
    return (scale_x, offset_x, scale_y, offset_y)


def _apply_affine(bbox: list[float], params: tuple[float, float, float, float]) -> list[float]:
    scale_x, offset_x, scale_y, offset_y = params
    x1, y1, x2, y2 = bbox
    return [
        x1 * scale_x + offset_x,
        y1 * scale_y + offset_y,
        x2 * scale_x + offset_x,
        y2 * scale_y + offset_y,
    ]


def _list_item_text_matches(list_item_text: Any, sub_block: dict[str, Any]) -> bool:
    lines = sub_block.get("lines")
    if not isinstance(lines, list):
        return False
    parts: list[str] = []
    for line in lines:
        spans = line.get("spans") if isinstance(line, dict) else None
        if not isinstance(spans, list):
            continue
        for span in spans:
            if isinstance(span, dict):
                parts.append(str(span.get("content", "")))
    sub_text = "".join(parts)
    # content_list wraps inline equations in `$...$`, while middle.json stores
    # equation spans without those delimiters. Strip the wrappers before the
    # leading-text guard so otherwise matching math-bearing lists can pair.
    a = re.sub(r"\s+", "", str(list_item_text).replace("$", ""))[:20]
    b = re.sub(r"\s+", "", sub_text)[:20]
    return bool(a) and a == b


def _union_bbox_from_lines(sub_block: dict[str, Any]) -> list[float] | None:
    lines = sub_block.get("lines")
    if not isinstance(lines, list) or not lines:
        return None
    x1s: list[float] = []
    y1s: list[float] = []
    x2s: list[float] = []
    y2s: list[float] = []
    for line in lines:
        bbox = line.get("bbox") if isinstance(line, dict) else None
        if not isinstance(bbox, list) or len(bbox) != 4:
            return None
        try:
            x1, y1, x2, y2 = (float(v) for v in bbox)
        except (TypeError, ValueError):
            return None
        x1s.append(x1)
        y1s.append(y1)
        x2s.append(x2)
        y2s.append(y2)
    return [min(x1s), min(y1s), max(x2s), max(y2s)]
