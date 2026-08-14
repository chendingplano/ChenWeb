#!/usr/bin/env python3
"""Unified PDF Parser Service

Polls kb.inputs for unprocessed PDF records and dispatches parsing to the
appropriate backend based on the record's parser_name field.

Required env vars:
  PG_HOST, PG_PORT, PG_DB_NAME, PG_USER_NAME, PG_PASSWORD

Optional env vars:
  STAGING_DIR / PDF_STAGING_DIR   Staging directory for new PDFs
  PDF_REPO_DIR                    Base result directory
  PDF_BACKUP_DIR                  Backup directory for originals
  PDF_POLL_INTERVAL               Seconds between DB polls (default: 10)
  PDF_BATCH_SIZE                  Records per poll (default: 25)
  PDF_PARSER_NAME                 Force all records to use this parser:
                                    "opendata" → opendataloader-pdf (default)
                                    "mineru"   → MinerU
                                  Overrides the per-record parser_name field.
  PDF_DEFAULT_PARSER              Fallback parser when PDF_PARSER_NAME is unset
                                  and the record carries no parser_name (default: opendata)
"""

import json
import logging
import os
import signal
import sys
import time
import asyncio
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

from shared import (
    TIME_FORMAT,
    pg_connect,
    claim_candidates,
    fetch_record_by_id,
    has_parse_success_or_active,
    scan_staging_once,
    find_duplicate_processed_record,
    record_parse_active,
    record_parsed_success,
    record_parsed_failure,
    record_duplicated,
    file_md5,
    copy_file,
    unique_path,
    choose_repo_dir,
    relativize_to_data_home,
    relativize_to_backup_root,
    resolve_repo_path,
)
from parser_base import ParserBackend
from parser_docling import DoclingParser
from parser_opendata import OpenDataParser
from parser_paddle import PaddleParser
from parser_mineru import MineruParser

import psycopg2

try:
    from nats.aio.client import Client as NATS
    from nats.js.errors import NotFoundError
except Exception:  # pragma: no cover - optional runtime dependency
    NATS = None
    NotFoundError = None

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y%m%d %H:%M:%S",
)
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

def _env(key: str, default: str = "") -> str:
    return os.environ.get(key, default).strip()


def _pg_dsn() -> dict:
    return {
        "host": _env("PG_HOST", "127.0.0.1"),
        "port": int(_env("PG_PORT", "5432")),
        "dbname": _env("PG_DB_NAME", "miner"),
        "user": _env("PG_USER_NAME", "admin"),
        "password": _env("PG_PASSWORD"),
    }


def _repo_dirs() -> list[str]:
    raw = _env("DATA_HOME_DIR")
    if raw:
        return [p.strip() for p in raw.split(",") if p.strip()]

    log.error("(MID_2026042601) env var DATA_HOME_DIR not defined!")
    return ["/tmp/pdf-repo"]

def _backup_dir() -> str:
    custom = _env("PDF_BACKUP_DIR")
    if custom:
        return custom

    log.error("(MID_2026042601) PDF_BACKUP_DIR not defined")
    return "/tmp/pdf-backup"

def _staging_dir() -> str:
    return _env("STAGING_DIR") or _env("PDF_STAGING_DIR") or _env("DATA_STAGING_DIR")


def _poll_interval() -> float:
    try:
        return float(_env("PDF_POLL_INTERVAL", "10"))
    except ValueError:
        return 10.0


def _batch_size() -> int:
    try:
        return int(_env("PDF_BATCH_SIZE", "25"))
    except ValueError:
        return 25


def _claim_timeout() -> int:
    """Seconds before a proc_status='active' claim is treated as stale and
    reclaimable (crash recovery). Progress updates keep modify_time fresh while a
    parse is genuinely running, so a live parse is never reclaimed."""
    try:
        return int(_env("PDF_CLAIM_TIMEOUT", "1800"))
    except ValueError:
        return 1800


def _default_parser() -> str:
    return _env("PDF_DEFAULT_PARSER", "opendata")


# ---------------------------------------------------------------------------
# Font-encoding garble detection + OCR fallback
# ---------------------------------------------------------------------------
#
# Some PDFs (notably GB/T standards) embed Latin text in fonts that lack a
# usable ToUnicode CMap. Text-layer extraction then maps those glyphs into the
# CJK Unified Ideographs block instead of ASCII, e.g. "intended application"
# comes out as "犻狀狋犲狀犱犲犱犪狆狆犾犻犮犪狋犻狅狀". The garble is silent: the
# bytes are valid CJK, not U+FFFD, so nothing downstream flags it. The single
# reliable signature is a *run* of consecutive code points in this narrow
# block — real Chinese text never strings several of these rare ideographs
# together, but every Latin letter maps into it, so garbled words produce long
# runs. When detected, we re-parse once with pipeline OCR (reads pixels).

_PASSTHROUGH_LO = 0x72AA  # 'a' in the observed font's CJK passthrough
_PASSTHROUGH_HI = 0x72D6  # 'z' (range covers the full a-z mapping)


def _ocr_fallback_enabled() -> bool:
    return _env("PDF_OCR_FALLBACK", "true").lower() not in {"0", "false", "no", "off"}


def _ocr_fallback_lang() -> str:
    return _env("MINERU_OCR_LANG", "ch")


def _ocr_fallback_min_run() -> int:
    try:
        return max(2, int(_env("PDF_OCR_FALLBACK_MIN_RUN", "4")))
    except ValueError:
        return 4


def _iter_strings(obj) -> "list[str]":
    """Yield every string value found anywhere inside a nested dict/list."""
    if isinstance(obj, str):
        yield obj
    elif isinstance(obj, dict):
        for value in obj.values():
            yield from _iter_strings(value)
    elif isinstance(obj, list):
        for value in obj:
            yield from _iter_strings(value)


def _looks_font_garbled(result: dict, min_run: int = 4) -> bool:
    """True when any extracted string contains a run of >= min_run consecutive
    code points in the CJK passthrough block — the font-encoding garble
    signature. Isolated in-block characters (legitimate rare Chinese, e.g.
    状 U+72B6) do not trip it."""
    for text in _iter_strings(result.get("pages", [])):
        run = 0
        for ch in text:
            if _PASSTHROUGH_LO <= ord(ch) <= _PASSTHROUGH_HI:
                run += 1
                if run >= min_run:
                    return True
            else:
                run = 0
    return False


def _mineru_is_ocr(backend: ParserBackend) -> bool:
    """True when this mineru backend is already pinned to pipeline OCR, so the
    guard does not re-run (or recurse) on an OCR result."""
    return (
        getattr(backend, "_backend", "") == "pipeline"
        and "ocr" in getattr(backend, "_extra_args", [])
    )


def _get_ocr_mineru(cache: dict[str, ParserBackend]) -> ParserBackend:
    key = "mineru::ocr-fallback"
    backend = cache.get(key)
    if backend is None:
        from parser_mineru import make_ocr_parser

        backend = make_ocr_parser(_ocr_fallback_lang())
        cache[key] = backend
    return backend


def _forced_parser() -> str:
    """Return PDF_PARSER_NAME when set; empty string otherwise."""
    return _env("PDF_PARSER_NAME")


def _pipeline_mode() -> str:
    mode = _env("PDF_PIPELINE_MODE", "poll").lower()
    if mode not in {"poll", "jetstream"}:
        return "poll"
    return mode


def _nats_url() -> str:
    return _env("NATS_URL", "nats://127.0.0.1:4222")


def _nats_user() -> str:
    return _env("NATS_USER")


def _nats_pass() -> str:
    return _env("NATS_PASS")


def _nats_token() -> str:
    return _env("NATS_TOKEN")


def _stage_subject() -> str:
    return _env("PDF_STAGE_EVENT_SUBJECT", "kb.pdf.staged")


def _parsed_subject() -> str:
    return _env("PDF_PARSED_EVENT_SUBJECT", "kb.pdf.parsed")


def _stage_durable() -> str:
    return _env("PDF_STAGE_EVENT_DURABLE", "pdf-parser")


def _stage_stream() -> str:
    return _env("PDF_STAGE_EVENT_STREAM")


def _parsed_stream() -> str:
    return _env("PDF_PARSED_EVENT_STREAM")


# ---------------------------------------------------------------------------
# Parser registry and dispatch
# ---------------------------------------------------------------------------

PARSER_REGISTRY: dict[str, type[ParserBackend]] = {
    "opendata": OpenDataParser,
    "paddleocr": PaddleParser,
    "mineru": MineruParser,
    "docling": DoclingParser,
}


def get_backend(name: str, cache: dict[str, ParserBackend]) -> ParserBackend:
    """Get or create a parser backend by name. Backends are cached after init."""
    if name in cache:
        return cache[name]
    cls = PARSER_REGISTRY.get(name)
    if cls is None:
        raise ValueError(
            f"Unknown parser '{name}'. Available: {list(PARSER_REGISTRY.keys())}"
        )
    backend = cls()
    backend.init()
    cache[name] = backend
    return backend


def should_drop_stage_event(exc: Exception) -> bool:
    """Return True when a stage event should be acked instead of retried.

    These are permanent payload/data issues rather than transient runtime
    failures, so JetStream redelivery only creates log spam.
    """
    message = str(exc).lower()
    return any(
        marker in message
        for marker in (
            "missing/invalid record_id",
            "kb.inputs record not found",
        )
    )


def should_skip_existing_parse(raw_status: str, force: bool = False) -> bool:
    """Return True when a stage event should not start a parse.

    Force restart bypasses successful parsed records, but still respects an
    active parser claim so repeated clicks do not launch concurrent parses.
    """
    if force:
        return _has_parse_proc_status(raw_status, "active")
    return has_parse_success_or_active(raw_status)


def _has_parse_proc_status(raw_status: str, *statuses: str) -> bool:
    wanted = {s.strip().lower() for s in statuses if s.strip()}
    if not wanted:
        return False

    raw_status = (raw_status or "").strip()
    if not raw_status or raw_status == "null":
        return False
    try:
        decoded = json.loads(raw_status)
    except json.JSONDecodeError:
        return False

    entries = decoded if isinstance(decoded, list) else [decoded]
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        if str(entry.get("operation", "")).strip().lower() != "parsed":
            continue

        proc_status = str(entry.get("proc_status", "")).strip().lower()
        if not proc_status:
            proc_status = str(entry.get("proc-status", "")).strip().lower()
        if not proc_status:
            proc_status = str(entry.get("status", "")).strip().lower()
        if proc_status in wanted:
            return True

    return False


# ---------------------------------------------------------------------------
# Progress throttle
# ---------------------------------------------------------------------------

def make_throttled_progress(
    db_update: Callable[[int, int, int], None],
    min_interval: float = 3.0,
) -> Callable[[int, int], None]:
    """Wrap a DB progress-update callback with a time-based throttle.

    The callback fires on the first call, then only when >= min_interval
    seconds have elapsed since the last DB write.

    Args:
        db_update: Callable(ms_used, progress_pct, total_pages) that writes to DB.
        min_interval: Minimum seconds between DB writes.

    Returns:
        Throttled callback with signature (page_finished, total_pages).
    """
    state = {"last_write": 0.0, "start": time.time()}

    def throttled(page_finished: int, total_pages: int) -> None:
        now = time.time()
        if state["last_write"] == 0.0 or (now - state["last_write"]) >= min_interval:
            ms_used = int((now - state["start"]) * 1000)
            pct = int(page_finished * 100 / total_pages) if total_pages > 0 else 0
            db_update(ms_used, pct, total_pages)
            state["last_write"] = now

    return throttled


# ---------------------------------------------------------------------------
# Per-record processing
# ---------------------------------------------------------------------------

def _resolve_input_file(
    rec: dict, staging_dir: str, repo_dirs: list[str],
) -> tuple[str, bool]:
    """Locate the PDF to parse, following the Handle Input File workflow.

    Returns (source_path, from_staging).
      - from_staging=True  → file was found in the staging directory and should
        be copied to result/backup dirs then removed from staging after parsing.
      - from_staging=False → file already lives in the result directory (re-process).

    Raises ValueError when the file cannot be located.
    """
    rec_id = rec["id"]
    staging_filename = (rec.get("name") or "").strip()
    result_filename = (rec.get("result_filename") or "").strip()

    # 1. Check staging directory for staging_filename
    if staging_filename and staging_dir:
        staging_path = os.path.join(staging_dir, staging_filename)
        if os.path.isfile(staging_path):
            return staging_path, True

    # 2. Also check file_name (the Go staging service may have already moved
    #    the file to the home/repo directory and recorded it in file_name).
    file_name = (rec.get("file_name") or "").strip()
    resolved_file_name = resolve_repo_path(file_name, repo_dirs)
    if resolved_file_name and os.path.isfile(resolved_file_name):
        return resolved_file_name, False

    resolved_result_name = resolve_repo_path(result_filename, repo_dirs)
    if resolved_result_name and os.path.isfile(resolved_result_name):
        return resolved_result_name, False

    # 3. File not in staging or file_name path.  Check the record's result
    #    directory — look for result_filename first, then fall back to the
    #    original filename (staging_filename or basename of file_name).
    names_to_try: list[str] = []
    if result_filename:
        names_to_try.append(os.path.basename(result_filename))
    if staging_filename:
        names_to_try.append(staging_filename)
    if file_name:
        names_to_try.append(os.path.basename(file_name))

    for repo_dir in repo_dirs:
        group_id = rec_id // 1000
        record_dir = os.path.join(repo_dir, "Artifacts", str(group_id), str(rec_id))
        for name in names_to_try:
            candidate = os.path.join(record_dir, name)
            if os.path.isfile(candidate):
                return candidate, False
        legacy_record_dir = os.path.join(repo_dir, "pdf_parser", str(rec_id))
        for name in names_to_try:
            candidate = os.path.join(legacy_record_dir, name)
            if os.path.isfile(candidate):
                return candidate, False

    tried = ", ".join(names_to_try) if names_to_try else "(none)"
    raise ValueError(
        f"record id={rec_id}: file not in staging dir and not found in result dirs "
        f"(tried: {tried})"
    )


def _process_record(
    conn, rec: dict, backend_cache: dict[str, ParserBackend],
    repo_dirs: list[str], backup_dir: str, staging_dir: str,
    default_parser: str,
    force: bool = False,
) -> dict:
    rec_id = rec["id"]
    raw_status = rec["status"]
    md5_hex = (rec.get("md5") or "").strip()
    forced = _forced_parser()
    parser_name = forced or (rec.get("parser_name") or "").strip() or default_parser

    log.info(
        "(MID_2026041301) received request record id=%s parser=%s name=%s",
        rec_id, parser_name, (rec.get("name") or "").strip(),
    )

    # --- Resolve input file ---------------------------------------------------
    try:
        source_file, from_staging = _resolve_input_file(rec, staging_dir, repo_dirs)
    except ValueError as exc:
        parse_start = datetime.now().strftime(TIME_FORMAT)
        record_parsed_failure(conn, rec_id, raw_status, parse_start, 0, str(exc), parser_name)
        log.error("(MID_2026040703) %s", exc)
        return {
            "record_id": rec_id,
            "type": "pdf",
            "status": "failed",
            "file_format": "json",
            "result_filename": "",
            "error": str(exc),
        }

    # Compute MD5 if missing
    if not md5_hex and os.path.exists(source_file):
        md5_hex = file_md5(source_file)

    # Duplicate check
    if not force:
        duplicate_check_time = datetime.now().strftime(TIME_FORMAT)
        dup_rcd_id = find_duplicate_processed_record(conn, rec_id, md5_hex)
        if dup_rcd_id is not None:
            record_duplicated(conn, rec_id, raw_status, dup_rcd_id, duplicate_check_time)
            log.info("(MID_2026040709) record id=%s marked duplicated (dup_rcd_id=%s)", rec_id, dup_rcd_id)
            return {
                "record_id": rec_id,
                "type": "pdf",
                "status": "failed",
                "file_format": "json",
                "result_filename": "",
                "error": f"duplicated record (dup_rcd_id={dup_rcd_id})",
            }

    parse_start_dt = datetime.now()
    parse_start = parse_start_dt.strftime(TIME_FORMAT)
    total_pages = 0
    result_path = ""

    try:
        backend = get_backend(parser_name, backend_cache)

        # If the file came from staging, copy it to the result dir and backup
        if from_staging:
            repo_dir = choose_repo_dir(repo_dirs)
            group_id = rec_id // 1000
            record_dir = os.path.join(repo_dir, "Artifacts", str(group_id), str(rec_id))
            Path(record_dir).mkdir(parents=True, exist_ok=True)
            repo_pdf_path = os.path.join(record_dir, os.path.basename(source_file))
            if source_file != repo_pdf_path:
                copy_file(source_file, repo_pdf_path)

            Path(backup_dir).mkdir(parents=True, exist_ok=True)
            backup_path = unique_path(backup_dir, os.path.basename(source_file))
            copy_file(source_file, backup_path)
        else:
            # Re-processing or Go-staged input: file is already in its repo dir.
            repo_pdf_path = source_file
            record_dir = str(Path(repo_pdf_path).parent)
            backup_path = (rec.get("backup_filename") or "").strip()

        # Initial in-progress status (idempotent: in poll mode the claim already
        # stamped proc_status=active; this merges/refreshes it).
        raw_status = record_parse_active(conn, rec_id, raw_status, parse_start, 0, 0, parser_name)

        # Throttled progress callback
        def _db_progress(ms_used: int, pct: int, n_pages: int = 0) -> None:
            nonlocal raw_status
            raw_status = record_parse_active(conn, rec_id, raw_status, parse_start, ms_used, pct, parser_name, n_pages)

        throttled = make_throttled_progress(_db_progress, min_interval=3.0)

        log.info("(MID_2026040710) parsing record id=%s parser=%s file=%s", rec_id, parser_name, repo_pdf_path)
        result = backend.parse(repo_pdf_path, record_dir, throttled)

        # Font-encoding garble guard: if a mineru text-layer parse produced the
        # CJK-passthrough signature (Latin mapped into the CJK block), re-parse
        # once with pipeline OCR, which reads pixels and recovers the Latin.
        if (
            parser_name == "mineru"
            and _ocr_fallback_enabled()
            and not _mineru_is_ocr(backend)
            and _looks_font_garbled(result, _ocr_fallback_min_run())
        ):
            log.warning(
                "(MID_2026062301) record id=%s: font-encoding garble detected "
                "(CJK passthrough) — re-parsing with pipeline OCR",
                rec_id,
            )
            try:
                ocr_backend = _get_ocr_mineru(backend_cache)
                ocr_result = ocr_backend.parse(repo_pdf_path, record_dir, throttled)
                if _looks_font_garbled(ocr_result, _ocr_fallback_min_run()):
                    log.warning(
                        "(MID_2026062303) record id=%s: OCR fallback still garbled "
                        "— keeping original parse",
                        rec_id,
                    )
                else:
                    ocr_result["engine"] = "mineru-pipeline-ocr"
                    result = ocr_result
                    log.info(
                        "(MID_2026062302) record id=%s: OCR fallback recovered text",
                        rec_id,
                    )
            except Exception as ocr_exc:
                log.error(
                    "(MID_2026062304) record id=%s: OCR fallback failed (%s) "
                    "— keeping original parse",
                    rec_id, ocr_exc,
                )

        total_pages = result.get("total_pages", 0)

        # Write aggregated result JSON
        result_output = {
            "input_id": rec_id,
            "source_pdf": repo_pdf_path,
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "engine": result.get("engine", parser_name),
            "pages": result.get("pages", []),
        }
        pdf_stem = Path(repo_pdf_path).stem
        result_path = os.path.join(record_dir, f"{pdf_stem}_{parser_name}.json")
        with open(result_path, "w", encoding="utf-8") as f:
            json.dump(result_output, f, indent=2, ensure_ascii=False)

        ms_used = int((datetime.now() - parse_start_dt).total_seconds() * 1000)
        raw_status = record_parsed_success(
            conn, rec_id, raw_status, parse_start, ms_used,
            relativize_to_data_home(result_path),
            relativize_to_data_home(repo_pdf_path),
            relativize_to_backup_root(backup_path),
            parser_name,
            total_pages,
        )

        # Remove source from staging after successful processing
        if from_staging and os.path.exists(source_file):
            os.remove(source_file)

        log.info(
            "(MID_2026040701) processed record id=%s parser=%s pages=%d result=%s",
            rec_id, parser_name, total_pages, result_path,
        )
        return {
            "record_id": rec_id,
            "type": "pdf",
            "status": "success",
            "file_format": "json",
            "result_filename": relativize_to_data_home(result_path),
            "error": "",
        }

    except Exception as exc:
        ms_used = int((datetime.now() - parse_start_dt).total_seconds() * 1000)
        record_parsed_failure(conn, rec_id, raw_status, parse_start, ms_used, str(exc), parser_name)
        log.error("(MID_2026040702) failed to process record id=%s: %s", rec_id, exc)
        return {
            "record_id": rec_id,
            "type": "pdf",
            "status": "failed",
            "file_format": "json",
            "result_filename": "",
            "error": str(exc),
        }


# ---------------------------------------------------------------------------
# Service loop
# ---------------------------------------------------------------------------

class JetStreamBus:
    def __init__(self) -> None:
        self._nc = None
        self._js = None

    async def connect(self) -> None:
        if NATS is None:
            raise RuntimeError("nats-py is not installed. Run: uv pip install nats-py")

        opts = {"servers": [_nats_url()]}
        token = _nats_token()
        if token:
            opts["token"] = token
        else:
            user = _nats_user()
            password = _nats_pass()
            if user or password:
                opts["user"] = user
                opts["password"] = password

        self._nc = NATS()
        await self._nc.connect(**opts)
        self._js = self._nc.jetstream()

        stage_stream = _stage_stream()
        parsed_stream = _parsed_stream()
        if stage_stream:
            await self._ensure_stream(stage_stream, _stage_subject())
        if parsed_stream:
            await self._ensure_stream(parsed_stream, _parsed_subject())

    async def _ensure_stream(self, stream_name: str, subject: str) -> None:
        stream_name = stream_name.strip()
        if not stream_name:
            return
        try:
            await self._js.stream_info(stream_name)
            return
        except NotFoundError:
            pass

        try:
            existing_name = await self._js.find_stream_name_by_subject(subject)
        except NotFoundError:
            existing_name = ""

        if existing_name:
            log.info(
                "reusing jetstream stream name=%s for subject=%s (configured name=%s)",
                existing_name,
                subject,
                stream_name,
            )
            return

        await self._js.add_stream(name=stream_name, subjects=[subject])
        log.info("created jetstream stream name=%s subject=%s", stream_name, subject)

    async def close(self) -> None:
        if self._nc is not None:
            await self._nc.drain()
            await self._nc.close()

    async def publish_parsed(self, result: dict) -> None:
        payload = {
            "record_id": result.get("record_id"),
            "type": "pdf",
            "status": result.get("status", "failed"),
            "file_format": result.get("file_format", "json"),
            "result_filename": result.get("result_filename", ""),
        }
        await self._js.publish(_parsed_subject(), json.dumps(payload).encode("utf-8"))


async def publish_parsed_event_once(result: dict) -> None:
    if NATS is None:
        raise RuntimeError("nats-py is not installed. Run: uv pip install nats-py")

    opts = {"servers": [_nats_url()]}
    token = _nats_token()
    if token:
        opts["token"] = token
    else:
        user = _nats_user()
        password = _nats_pass()
        if user or password:
            opts["user"] = user
            opts["password"] = password

    nc = NATS()
    await nc.connect(**opts)
    js = nc.jetstream()
    payload = {
        "record_id": result.get("record_id"),
        "type": "pdf",
        "status": result.get("status", "failed"),
        "file_format": result.get("file_format", "json"),
        "result_filename": result.get("result_filename", ""),
    }
    await js.publish(_parsed_subject(), json.dumps(payload).encode("utf-8"))
    await nc.drain()
    await nc.close()


async def run_jetstream() -> None:
    dsn = _pg_dsn()
    repo_dirs = _repo_dirs()
    backup_dir = _backup_dir()
    staging_dir = _staging_dir()
    default_parser = _default_parser()

    log.info("(MID_2026040711) starting pdf_parser service in jetstream mode")
    log.info("  stage_subject=%s parsed_subject=%s durable=%s", _stage_subject(), _parsed_subject(), _stage_durable())
    forced = _forced_parser()
    if forced:
        log.info("  PDF_PARSER_NAME=%s (overrides per-record parser_name)", forced)

    for d in repo_dirs:
        Path(d).mkdir(parents=True, exist_ok=True)
    Path(backup_dir).mkdir(parents=True, exist_ok=True)

    backend_cache: dict[str, ParserBackend] = {}
    conn = pg_connect(dsn)
    bus = JetStreamBus()
    await bus.connect()

    stop = asyncio.Event()
    _active_fetch: asyncio.Task | None = None
    loop = asyncio.get_running_loop()

    def _handle_signal() -> None:
        log.info("(MID_2026040712) shutdown signal received")
        stop.set()
        if _active_fetch is not None and not _active_fetch.done():
            _active_fetch.cancel()

    for _sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(_sig, _handle_signal)
        except (NotImplementedError, OSError):
            signal.signal(_sig, lambda s, f: _handle_signal())

    sub = await bus._js.pull_subscribe(_stage_subject(), durable=_stage_durable())

    try:
        while not stop.is_set():
            try:
                _active_fetch = asyncio.ensure_future(sub.fetch(1, timeout=2))
                msgs = await _active_fetch
            except asyncio.CancelledError:
                break
            except Exception:
                msgs = []
            finally:
                _active_fetch = None

            if not msgs:
                continue
            for msg in msgs:
                payload = {}
                try:
                    payload = json.loads(msg.data.decode("utf-8"))
                    rec_id = int(payload.get("record_id") or 0)
                    msg_type = str(payload.get("type", "")).strip().lower()
                    msg_status = str(payload.get("status", "")).strip().lower()
                    force = payload.get("force") is True
                    if rec_id <= 0:
                        raise ValueError("missing/invalid record_id")
                    if msg_type and msg_type != "pdf":
                        await msg.ack()
                        continue
                    if msg_status and msg_status != "success":
                        await msg.ack()
                        continue

                    rec = fetch_record_by_id(conn, rec_id)
                    if not rec:
                        raise ValueError(f"kb.inputs record not found id={rec_id}")

                    # Skip records already parsed/in-progress — handles JetStream
                    # redelivery that occurs when NATS drops between parse completion
                    # and ACK. The lifecycle uses a single "parsed" operation.
                    if should_skip_existing_parse(rec["status"], force=force):
                        log.info(
                            "(MID_2026040716) record id=%s already parsed — acking redelivered message",
                            rec_id,
                        )
                        await msg.ack()
                        continue

                    # Run the synchronous (potentially long) parse in a thread so
                    # the event loop stays live and NATS pings/reconnects can fire.
                    result = await loop.run_in_executor(
                        None,
                        lambda: _process_record(
                            conn, rec, backend_cache,
                            repo_dirs, backup_dir, staging_dir, default_parser,
                            force=force,
                        ),
                    )

                    # ACK before publishing: even if publish fails, the record
                    # won't be re-parsed on the next redelivery.
                    await msg.ack()
                    try:
                        await bus.publish_parsed(result)
                    except Exception as pub_exc:
                        log.error(
                            "(MID_2026040717) publish parsed event failed id=%s: %s",
                            rec_id, pub_exc,
                        )
                except Exception as exc:
                    log.error("(MID_2026040713) jetstream stage event handling failed payload=%s error=%s", payload, exc)
                    if should_drop_stage_event(exc):
                        await msg.ack()
                    else:
                        await msg.nak()
    finally:
        log.info("(MID_2026040715) closing NATS connection")
        try:
            await asyncio.wait_for(bus.close(), timeout=3.0)
        except Exception:
            pass
        conn.close()

def run() -> None:
    dsn = _pg_dsn()
    repo_dirs = _repo_dirs()
    backup_dir = _backup_dir()
    staging_dir = _staging_dir()
    poll_interval = _poll_interval()
    batch_size = _batch_size()
    claim_timeout = _claim_timeout()
    default_parser = _default_parser()

    log.info("(MID_2026040703) starting unified pdf_parser service")
    log.info("  pg_host=%s db=%s", dsn["host"], dsn["dbname"])
    log.info("  repo_dirs=%s", repo_dirs)
    log.info("  backup_dir=%s", backup_dir)
    log.info("  staging_dir=%s", staging_dir if staging_dir else "(not set)")
    log.info("  poll_interval=%ss batch_size=%s claim_timeout=%ss default_parser=%s",
             poll_interval, batch_size, claim_timeout, default_parser)
    forced = _forced_parser()
    if forced:
        log.info("  PDF_PARSER_NAME=%s (overrides per-record parser_name)", forced)

    for d in repo_dirs:
        Path(d).mkdir(parents=True, exist_ok=True)
    Path(backup_dir).mkdir(parents=True, exist_ok=True)

    backend_cache: dict[str, ParserBackend] = {}

    def _stop(_sig, _frame):
        log.info("(MID_2026040704) shutdown signal received")
        sys.exit(0)

    signal.signal(signal.SIGINT, _stop)
    signal.signal(signal.SIGTERM, _stop)

    conn = None
    while True:
        try:
            if conn is None or conn.closed:
                conn = pg_connect(dsn)
                log.info("(MID_2026040705) connected to postgres")

            scan_staging_once(conn, staging_dir, backup_dir)
            # Atomic claim: each returned record is locked (FOR UPDATE SKIP LOCKED)
            # and already stamped proc_status=active in a committed transaction, so
            # no other worker/instance can take it.
            records = claim_candidates(conn, batch_size, claim_timeout)
            if not records:
                log.debug("(MID_2026040706) nothing to process")
            else:
                for rec in records:
                    result = _process_record(
                        conn, rec, backend_cache,
                        repo_dirs, backup_dir, staging_dir, default_parser,
                    )
                    try:
                        asyncio.run(publish_parsed_event_once(result))
                    except Exception as exc:
                        log.error("(MID_2026040714) failed to publish parsed event id=%s error=%s", rec["id"], exc)

        except psycopg2.Error as exc:
            log.error("(MID_2026040707) postgres error: %s — reconnecting", exc)
            try:
                if conn is not None:
                    conn.close()
            except Exception:
                pass
            conn = None
            time.sleep(poll_interval)
            continue
        except Exception as exc:
            log.error("(MID_2026040708) unexpected error: %s", exc, exc_info=True)

        time.sleep(poll_interval)


if __name__ == "__main__":
    if _pipeline_mode() == "jetstream":
        asyncio.run(run_jetstream())
    else:
        run()
