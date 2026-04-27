# PDF Parser Service — User Manual

The PDF parser is a two-tier system for ingesting and OCR-processing PDF files.

- **Go staging service** (`server/cmd/pdfparser/`) — watches a staging directory, deduplicates files by MD5, distributes them to backup and repository directories, and records each file in the `kb.inputs` database table.
- **Python OCR service** (`python/pdf-parser/`) — polls `kb.inputs` for unprocessed rows and dispatches PDFs to an OCR backend (OpenDataLoader or PaddleOCR).

The Go service runs first; the Python service picks up where it leaves off.

---

## Quick Start

```bash
# From aas/server/cmd/pdf-parser/

# 1. Start the staging service (runs migrations automatically on startup)
mise pdf-parser-start

# 2. Start the OCR service (background, from aas/)
mise ocr-service-start

# 3. Drop PDFs into the staging directory configured in config.toml
```

> **Note:** The Go staging service must be started before the Python OCR service.
> It runs database migrations automatically on every startup, so there is no
> separate migration step required. The Python OCR service has no migration
> logic and depends on the Go service having already created the schema.

---

## Modes of Operation

| Mode | Flag | Behaviour |
|------|------|-----------|
| **Normal** | _(none)_ | Runs migrations, then polls the staging directory every 10 seconds. Shuts down gracefully on SIGINT/SIGTERM. |
| **Migrate only** | `-migrate-only` | Runs migrations and exits without starting the polling loop. |
| **Verify schema only** | `-verify-schema-only` | Runs migrations, verifies the `kb.inputs.result_filename` column exists, then exits. |

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-config <path>` | `../../../config.toml` | Path to the TOML config file. |
| `-migrate-only` | `false` | Apply migrations and exit. |
| `-verify-schema-only` | `false` | Verify schema compatibility and exit. |

---

## Configuration

### config.toml

The Go service reads its settings from a TOML config file (default `../../../config.toml` relative to the binary).

```toml
[pdf_parser]
enabled = true                       # Enable the service
staging_dir = "Data/kb/staging"      # Directory to watch (relative to config file)
repo_dirs = ["Data/kb/repo-1"]       # Output directories for processed files
backup_dir = "Data/kb/backup"        # Backup directory for originals
delete_from_staging = true           # Remove files from staging after processing
work_dir = "Data/kb/.cache"          # Temp directory for OCR processing

# Python OCR service settings (used by the Python tier only)
poll_interval_seconds = 10
batch_size = 25
python_bin = "python/pdf-parser/.venv/bin/python"    # venv Python interpreter
paddleocr_script = "/path/to/parse_pdf.py"           # PaddleOCR entry script
use_paddleocr_vl = false

[postgres]
host = "127.0.0.1"
port = 5432
max_connections = 10
```

### Environment Variables (Go Service)

These override config file values when set:

| Variable | Required | Description |
|----------|----------|-------------|
| `DATA_STAGING_DIR` | Yes* | Staging directory path |
| `DATA_BACKUP_DIR` | Yes* | Backup directory path |
| `DATA_HOME_DIR` | Yes* | Home/repository directory path |

\* Required only if not specified in `config.toml`.

### Environment Variables (Python OCR Service)

| Variable | Default | Description |
|----------|---------|-------------|
| `PG_HOST` | `127.0.0.1` | PostgreSQL host |
| `PG_PORT` | `5432` | PostgreSQL port |
| `PG_DB_NAME` | `miner` | Database name |
| `PG_USER_NAME` | `admin` | Database user |
| `PG_PASSWORD` | _(none)_ | Database password (**required**) |
| `PDF_REPO_DIR` | `/tmp/pdf-repo` | Output directory |
| `PDF_BACKUP_DIR` | `/tmp/pdf-backup` | Backup directory |
| `PDF_POLL_INTERVAL` | `10` | Poll interval in seconds |
| `PDF_BATCH_SIZE` | `25` | Records per poll cycle |
| `PDF_DEFAULT_PARSER` | `opendata` | Parser backend (`opendata` or `paddle`) |
| `OPENDATA_JAR_PATH` | _(auto-detect)_ | Path to `opendataloader-pdf-cli.jar` |
| `PDF_USE_VL` | `true` | Enable PaddleOCR vision-language mode |
| `PDF_MPS` | `false` | Apple Metal GPU acceleration |
| `PDF_TIMING` | `false` | Detailed timing logs |
| `PDF_QUANTIZE_ENABLED` | `false` | Weight quantization |
| `PDF_QUANTIZE_BITS` | `8` | Quantization bits (4 or 8) |

---

## Mise Tasks

Run these from `aas/server/cmd/pdf-parser/`:

| Task | Description |
|------|-------------|
| `mise pdf-parser-run` | Run the staging service in foreground |
| `mise pdf-parser-start` | Start in background (PID written to `.cache/pdf-parser.pid`) |
| `mise pdf-parser-stop` | Stop the background service |
| `mise pdf-parser-status` | Check if the service is running |
| `mise pdf-parser-migrate` | Run database migrations and exit |
| `mise pdf-parser-verify-schema` | Verify schema compatibility and exit |
| `mise build-pdf-parser` | Build the binary to `.cache/pdf-parser.exe` |

Run these from `aas/` for the Python OCR tier:

| Task | Description |
|------|-------------|
| `mise ocr-service-start` | Start OCR service in background |
| `mise ocr-service-start-sync` | Start OCR service in foreground |
| `mise ocr-service-stop` | Stop the background OCR service |
| `mise ocr-service-kill` | Kill OCR service by process name |
| `mise ocr-service-status` | Check if OCR service is running |

---

## Processing Pipeline

```
 ┌──────────┐    ┌──────────────────────────────────┐    ┌──────────────────┐
 │ Staging  │───▶│  Go Staging Service (10s poll)   │───▶│  kb.inputs row   │
 │   Dir    │    │  • MD5 hash                      │    │  status = []     │
 └──────────┘    │  • Copy to backup dir             │    └────────┬─────────┘
                 │  • Copy to repo dir (dedup)       │             │
                 │  • Delete from staging             │             ▼
                 └──────────────────────────────────┘    ┌──────────────────┐
                                                         │ Python OCR Svc   │
                                                         │ • Poll kb.inputs │
                                                         │ • Dispatch OCR   │
                                                         │ • Write JSON     │
                                                         │ • Update status  │
                                                         └──────────────────┘
```

### File Deduplication

When copying a file to the repo or backup directory:

1. If a file with the same name and same MD5 already exists — **skip** (no duplicate).
2. If a file with the same name but different MD5 exists — rename with a `_N` suffix (e.g., `report_1.pdf`, `report_2.pdf`).
3. If no collision — copy as-is.

---

## Database

### Schema: `kb.inputs`

| Column | Type | Description |
|--------|------|-------------|
| `id` | serial | Primary key |
| `staging_filename` | text | Original filename from staging |
| `type` | text | Always `'pdf'` |
| `file_name` | text | Full path in the repo directory |
| `backup_filename` | text | Full path in the backup directory |
| `result_filename` | text | OCR result JSON filename (e.g., `ocr_rslt_123.json`) |
| `md5` | text | MD5 hex digest |
| `status` | jsonb | Array of operation status objects |
| `error_msg` | text | Error message (if any) |
| `modify_time` | timestamp | Last modification time |

### Status Format

The `status` column is a JSONB array of operation records:

```json
[
  {
    "operation": "parse",
    "time": "20260407 14:30:00",
    "status": "success",
    "error": ""
  }
]
```

### Migrations

Migrations run automatically every time the Go staging service starts (both
project and shared migrations via goose). There is no need to run them manually.

The `-migrate-only` and `-verify-schema-only` flags are available for operational
use cases where you want to apply migrations or check schema compatibility
without starting the polling loop:

```bash
# Apply migrations without starting the service
mise pdf-parser-migrate

# Check that the schema is compatible without starting the service
mise pdf-parser-verify-schema
```

The Python OCR service does not run migrations. It assumes the Go service has
already created the required schema.

---

## External Dependencies

| Dependency | Required By | Notes |
|------------|-------------|-------|
| PostgreSQL | Both tiers | `kb.inputs` table |
| Java 11+ | Python OCR (OpenDataLoader) | Must be on PATH |
| PaddleOCR | Python OCR (Paddle backend) | Python package |
| `opendataloader-pdf-cli.jar` | Python OCR (OpenData backend) | Set via `OPENDATA_JAR_PATH` |

---

## Logging

The Go service uses structured logging via `loggerutil`. Key log codes:

| Code | Area |
|------|------|
| `CWB_PDF_001` | Configuration errors |
| `CWB_PDF_002` | Initialization errors |
| `CWB_PDF_003` | Runtime errors |

When running in background mode, logs are written to `.cache/pdf-parser.log`.

---

## Troubleshooting

**Service won't start — "pdf-parser already running"**
A stale PID file exists. Run `mise pdf-parser-stop` to clean it up, then start again.

**Schema verification fails**
The `result_filename` column is missing. Run `mise pdf-parser-migrate` to apply migrations.

**Files sit in staging without being processed**
1. Check that the service is running: `mise pdf-parser-status`
2. Verify `DATA_STAGING_DIR` (or `pdf_parser.staging_dir` in config) points to the correct directory.
3. Check `.cache/pdf-parser.log` for errors.

**OCR results not appearing**
The Go staging service only moves files and creates database records. The Python OCR service does the actual parsing. Make sure it's running: `mise ocr-service-status`.
