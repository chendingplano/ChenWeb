"""Shared infrastructure for the unified PDF parser service.

DB connection, status management, file helpers, staging scanner.
"""

import json
import hashlib
import logging
import os
import shutil
from datetime import datetime
from pathlib import Path
from typing import Any

import psycopg2

log = logging.getLogger(__name__)

TIME_FORMAT = "%Y%m%d %H:%M:%S"

# Fixed operation identifier for the PDF parser stage. It NEVER changes across the
# record lifecycle — it is the stable dedup key. The lifecycle state lives in the
# entry's `proc_status` field: "active" (in-progress) -> "success" / "fail", plus
# "duplicated". Because the operation is constant, every status write upserts the
# same single entry, so a record can never accumulate two "parsed" entries.
# See docs/superpowers/specs/2026-04-07-unified-pdf-parser-service-design.md
PARSE_OPERATION = "parsed"

# ---------------------------------------------------------------------------
# Status helpers
# ---------------------------------------------------------------------------

def decode_status(raw: str | None) -> list[dict]:
    """Parse the status JSONB column into a list of dicts."""
    raw = (raw or "").strip()
    if not raw or raw == "null":
        return []
    try:
        data = json.loads(raw)
        if isinstance(data, dict):
            return [data]
        if isinstance(data, list):
            return data
    except json.JSONDecodeError:
        pass
    return []


def has_operation(raw_status: str, *operations: str) -> bool:
    """Check if any of the given operations exist in the status array."""
    entries = decode_status(raw_status)
    ops = {op.strip().lower() for op in operations}
    return any(e.get("operation", "").strip().lower() in ops for e in entries)


def upsert_status(raw_status: str, operation: str, patch: dict) -> str:
    """Update an existing status entry by operation name, or append a new one.

    Merges patch into the existing entry (preserving fields not in patch).
    """
    entries = decode_status(raw_status)
    op = operation.strip().lower()
    idx = next(
        (i for i, e in enumerate(entries) if e.get("operation", "").strip().lower() == op),
        None,
    )

    merged = {"operation": operation}
    merged.update(patch)

    if idx is None:
        entries.append(merged)
    else:
        current = dict(entries[idx]) if isinstance(entries[idx], dict) else {}
        current.update(merged)
        entries[idx] = current

    return json.dumps(entries)


# ---------------------------------------------------------------------------
# File helpers
# ---------------------------------------------------------------------------

def file_md5(path: str) -> str:
    """Compute MD5 hex digest of a file."""
    hasher = hashlib.md5()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def copy_file(src: str, dst: str) -> None:
    """Copy src to dst, creating parent directories as needed."""
    Path(dst).parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dst)


def unique_path(directory: str, filename: str) -> str:
    """Return a path in directory that does not yet exist."""
    base = Path(filename).stem
    ext = Path(filename).suffix
    candidate = Path(directory) / filename
    i = 1
    while candidate.exists():
        candidate = Path(directory) / f"{base}_{i}{ext}"
        i += 1
    return str(candidate)


def resolve_source_path(file_name: str, staging_dir: str) -> str:
    """Resolve a source file path, joining with staging_dir if relative."""
    file_name = (file_name or "").strip()
    if not file_name:
        return ""
    if os.path.isabs(file_name):
        return file_name
    if staging_dir:
        return os.path.join(staging_dir, file_name)
    return file_name


def relativize_to_data_home(path: str, data_home_dir: str | None = None) -> str:
    """Store repo paths relative to DATA_HOME_DIR when possible."""
    path = (path or "").strip()
    if not path:
        return ""
    if not os.path.isabs(path):
        return path
    home = (data_home_dir or os.environ.get("DATA_HOME_DIR", "")).strip()
    if not home:
        return path
    try:
        rel = os.path.relpath(path, home)
    except ValueError:
        return path
    if rel == "." or rel.startswith(".."+os.sep) or rel == "..":
        return path
    return rel


def relativize_to_backup_root(path: str, backup_dir: str | None = None) -> str:
    """Store backup paths relative to the parent of DATA_BACKUP_DIR when possible.

    Example:
      DATA_BACKUP_DIR=/Users/cding/Apps/Backup
      /Users/cding/Apps/Backup/file.pdf -> Backup/file.pdf
    """
    path = (path or "").strip()
    if not path:
        return ""
    if not os.path.isabs(path):
        return path

    configured_backup = (backup_dir or os.environ.get("DATA_BACKUP_DIR", "")).strip()
    if not configured_backup:
        return path

    base = os.path.dirname(configured_backup)
    if not base:
        return path

    try:
        rel = os.path.relpath(path, base)
    except ValueError:
        return path
    if rel == "." or rel.startswith(".."+os.sep) or rel == "..":
        return path
    return rel


def resolve_repo_path(path: str, repo_dirs: list[str]) -> str:
    """Resolve a stored file path against repo dirs when it is relative."""
    path = (path or "").strip()
    if not path:
        return ""
    if os.path.isabs(path):
        return path
    for repo_dir in repo_dirs:
        candidate = os.path.join(repo_dir, path)
        if os.path.exists(candidate):
            return candidate
    if repo_dirs:
        return os.path.join(repo_dirs[0], path)
    return path


def resolve_backup_path(path: str, backup_dir: str | None = None) -> str:
    """Resolve a stored backup path against the parent of DATA_BACKUP_DIR."""
    path = (path or "").strip()
    if not path:
        return ""
    if os.path.isabs(path):
        return path

    configured_backup = (backup_dir or os.environ.get("DATA_BACKUP_DIR", "")).strip()
    if not configured_backup:
        return path

    base = os.path.dirname(configured_backup)
    if not base:
        return path
    return os.path.join(base, path)


_repo_dir_cache: str | None = None


def choose_repo_dir(repo_dirs: list[str]) -> str:
    """Return the repo dir with the least total bytes (cached after first call)."""
    global _repo_dir_cache
    if _repo_dir_cache is not None and _repo_dir_cache in repo_dirs:
        return _repo_dir_cache

    def _size(d: str) -> int:
        total = 0
        try:
            for f in Path(d).rglob("*"):
                if f.is_file():
                    total += f.stat().st_size
        except OSError:
            pass
        return total

    best = min(repo_dirs, key=_size)
    Path(best).mkdir(parents=True, exist_ok=True)
    _repo_dir_cache = best
    return best


# ---------------------------------------------------------------------------
# DB connection
# ---------------------------------------------------------------------------

def pg_connect(dsn: dict):
    """Connect to PostgreSQL with autocommit=False."""
    conn = psycopg2.connect(**dsn)
    conn.autocommit = False
    return conn


# ---------------------------------------------------------------------------
# DB queries
# ---------------------------------------------------------------------------

def claim_candidates(conn, batch_size: int, claim_timeout: int) -> list[dict]:
    """Atomically claim eligible PDF records in a single transaction.

    Eligibility:
      - type = 'pdf', AND
      - no `operation = "parsed"` entry at all, OR a stale claim: an entry with
        `proc_status = "active"` whose row modify_time is older than claim_timeout
        seconds (a worker that died mid-parse).

    The SELECT takes `FOR UPDATE SKIP LOCKED`, so two concurrent workers / service
    instances can never claim the same row. Each claimed row is immediately stamped
    `proc_status = "active"` and the whole batch is committed before the (slow)
    parse begins, so the claim is visible to everyone else. This replaces the old
    check-then-act `fetch_candidates`, which left a TOCTOU window between selecting
    a row and marking it in-progress.
    """
    sql = """
        SELECT id,
               COALESCE(staging_filename, ''),
               COALESCE(file_name, ''),
               COALESCE(parser_name, ''),
               COALESCE(status::text, '[]'),
               COALESCE(md5, ''),
               COALESCE(result_filename, ''),
               COALESCE(backup_filename, '')
        FROM kb.inputs
        WHERE LOWER(type) = 'pdf'
          AND (
              status IS NULL
              OR NOT jsonb_path_exists(status, '$[*] ? (@.operation == "parsed")')
              OR (
                  jsonb_path_exists(status, '$[*] ? (@.operation == "parsed" && @.proc_status == "active")')
                  AND modify_time < NOW() - make_interval(secs => %s)
              )
          )
        ORDER BY id ASC
        LIMIT %s
        FOR UPDATE SKIP LOCKED
    """
    claimed: list[dict] = []
    start_time = datetime.now().strftime(TIME_FORMAT)
    with conn.cursor() as cur:
        cur.execute(sql, (claim_timeout, batch_size))
        rows = cur.fetchall()

        for r in rows:
            rec = {
                "id": r[0],
                "name": r[1],
                "file_name": r[2],
                "parser_name": r[3],
                "status": r[4],
                "md5": r[5],
                "result_filename": r[6],
                "backup_filename": r[7],
            }
            # Stamp proc_status=active in the same locked transaction so the claim
            # is durable and visible once committed.
            rec["status"] = upsert_status(
                rec["status"], PARSE_OPERATION,
                {
                    "proc_status": "active",
                    "progress": "0%",
                    "start_time": start_time,
                    "ms_used": 0,
                },
            )
            cur.execute(
                "UPDATE kb.inputs SET status = %s::jsonb, modify_time = NOW() WHERE id = %s",
                (rec["status"], rec["id"]),
            )
            claimed.append(rec)
    conn.commit()
    return claimed


def fetch_record_by_id(conn, rec_id: int) -> dict | None:
    """Fetch one kb.inputs record by id."""
    sql = """
        SELECT id,
               COALESCE(staging_filename, ''),
               COALESCE(file_name, ''),
               COALESCE(parser_name, ''),
               COALESCE(status::text, '[]'),
               COALESCE(md5, ''),
               COALESCE(result_filename, ''),
               COALESCE(backup_filename, '')
        FROM kb.inputs
        WHERE id = %s
        LIMIT 1
    """
    with conn.cursor() as cur:
        cur.execute(sql, (rec_id,))
        row = cur.fetchone()
    conn.commit()
    if not row:
        return None
    return {
        "id": row[0],
        "name": row[1],
        "file_name": row[2],
        "parser_name": row[3],
        "status": row[4],
        "md5": row[5],
        "result_filename": row[6],
        "backup_filename": row[7],
    }


def has_md5_record(conn, md5_hex: str) -> bool:
    """Check if any record with this MD5 exists."""
    sql = "SELECT 1 FROM kb.inputs WHERE md5 = %s LIMIT 1"
    with conn.cursor() as cur:
        cur.execute(sql, (md5_hex,))
        found = cur.fetchone() is not None
    conn.commit()
    return found


def find_duplicate_processed_record(conn, rec_id: int, md5_hex: str) -> int | None:
    """Find another record with same MD5 that was successfully parsed.

    Only records whose status contains a "parsed" entry with
    ``proc_status == "success"`` count as a true duplicate.  Failures are not
    counted, so a transient failure (e.g. a shutdown mid-parse) on one copy still
    lets another copy be retried.  Records merely marked "duplicated" are likewise
    ignored so that when every copy is "duplicated" (e.g. the original was
    deleted), one of them will still be processed.
    """
    if not md5_hex:
        return None
    sql = """
        SELECT id
        FROM kb.inputs
        WHERE id <> %s
          AND md5 = %s
          AND jsonb_path_exists(
              COALESCE(status, '[]'::jsonb),
              '$[*] ? (@.operation == "parsed" && @.proc_status == "success")'
          )
        ORDER BY id ASC
        LIMIT 1
    """
    with conn.cursor() as cur:
        cur.execute(sql, (rec_id, md5_hex))
        row = cur.fetchone()
    conn.commit()
    return row[0] if row else None


def insert_staged_pdf_record(conn, file_path: str, md5_hex: str) -> None:
    """Insert a new kb.inputs record for a staged PDF file."""
    sql = """
        INSERT INTO kb.inputs (
            staging_filename, type, file_name, status, md5
        ) VALUES (
            %s, 'pdf', %s, %s::jsonb, %s
        )
    """
    with conn.cursor() as cur:
        cur.execute(sql, (os.path.basename(file_path), file_path, json.dumps([]), md5_hex))
    conn.commit()


# ---------------------------------------------------------------------------
# DB status updates
# ---------------------------------------------------------------------------

def record_parse_active(
    conn, rec_id: int, raw_status: str,
    start_time: str, ms_used: int, progress_pct: int,
) -> str:
    """Upsert the single 'parsed' entry to proc_status='active' with progress."""
    new_status = upsert_status(
        raw_status, PARSE_OPERATION,
        {
            "proc_status": "active",
            "progress": f"{progress_pct}%",
            "start_time": start_time,
            "ms_used": ms_used,
        },
    )
    sql = """
        UPDATE kb.inputs
        SET status = %s::jsonb, modify_time = NOW()
        WHERE id = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (new_status, rec_id))
    conn.commit()
    return new_status


def record_parsed_success(
    conn, rec_id: int, raw_status: str,
    start_time: str, ms_used: int,
    result_filename: str, file_name: str, backup_filename: str,
    parser_name: str, num_pages: int,
) -> str:
    """Upsert the single 'parsed' entry to proc_status='success'."""
    new_status = upsert_status(
        raw_status, PARSE_OPERATION,
        {
            "proc_status": "success",
            "progress": "100%",
            "parser_name": parser_name,
            "start_time": start_time,
            "ms_used": ms_used,
            "num_pages": num_pages,
            "error": "",
        },
    )
    sql = """
        UPDATE kb.inputs
        SET status          = %s::jsonb,
            result_filename = %s,
            file_name       = %s,
            backup_filename = %s,
            parser_name     = %s,
            error_msg       = NULL,
            modify_time     = NOW()
        WHERE id = %s
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (new_status, result_filename, file_name, backup_filename, parser_name, rec_id),
        )
    conn.commit()
    return new_status


def record_parsed_failure(
    conn, rec_id: int, raw_status: str,
    start_time: str, ms_used: int, error: str, parser_name: str,
) -> str:
    """Upsert the single 'parsed' entry to proc_status='fail'."""
    new_status = upsert_status(
        raw_status, PARSE_OPERATION,
        {
            "proc_status": "fail",
            "parser_name": parser_name,
            "start_time": start_time,
            "ms_used": ms_used,
            "error": error,
        },
    )
    sql = """
        UPDATE kb.inputs
        SET status    = %s::jsonb,
            error_msg = %s,
            modify_time = NOW()
        WHERE id = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (new_status, error, rec_id))
    conn.commit()
    return new_status


def record_duplicated(
    conn, rec_id: int, raw_status: str, dup_rcd_id: int, start_time: str,
) -> str:
    """Mark a record as duplicated (another record with same MD5 was already parsed)."""
    new_status = upsert_status(
        raw_status, PARSE_OPERATION,
        {
            "proc_status": "duplicated",
            "dup_rcd_id": dup_rcd_id,
            "start_time": start_time,
        },
    )
    sql = """
        UPDATE kb.inputs
        SET status    = %s::jsonb,
            error_msg = NULL,
            modify_time = NOW()
        WHERE id = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (new_status, rec_id))
    conn.commit()
    return new_status


# ---------------------------------------------------------------------------
# Staging scanner
# ---------------------------------------------------------------------------

def scan_staging_once(conn, staging_dir: str, backup_dir: str) -> None:
    """Scan staging directory for new PDFs, dedup by MD5, insert records."""
    if not staging_dir:
        return
    Path(staging_dir).mkdir(parents=True, exist_ok=True)

    for entry in sorted(Path(staging_dir).iterdir()):
        if not entry.is_file() or entry.suffix.lower() != ".pdf":
            continue

        src_path = str(entry)
        md5_hex = file_md5(src_path)

        if has_md5_record(conn, md5_hex):
            Path(backup_dir).mkdir(parents=True, exist_ok=True)
            backup_path = os.path.join(backup_dir, os.path.basename(src_path))
            if not os.path.exists(backup_path):
                copy_file(src_path, backup_path)
            if os.path.abspath(src_path) != os.path.abspath(backup_path):
                os.remove(src_path)
            log.info("duplicate staged pdf skipped (md5): %s", src_path)
            continue

        insert_staged_pdf_record(conn, src_path, md5_hex)
        log.info("new staged pdf registered: %s md5=%s", src_path, md5_hex)
