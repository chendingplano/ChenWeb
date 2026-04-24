# PDF Parser Service

A unified Python service that polls the `kb.inputs` PostgreSQL table for unprocessed PDF records and dispatches parsing to the appropriate backend based on the record's `parser_name` field.

## Supported Parsers

| Name | Engine | How it works |
|---|---|---|
| `opendata` (default) | [opendataloader-pdf](https://github.com/opendataloader-project/opendataloader-pdf) | Invokes the Java CLI JAR as a subprocess |
| `paddleocr` | [PaddleOCR-VL](https://github.com/PaddlePaddle/PaddleOCR) | Python API with optional Apple Metal GPU (MPS) acceleration |
| `mineru` | [MinerU](https://github.com/opendatalab/mineru) | Invokes the `mineru` CLI as a subprocess; reads `*_content_list.json` and groups items by `page_idx` |
| `docling` | [Docling](https://github.com/docling-project/docling) | Uses the Python `docling` package and writes `<pdf_stem>.docling.json` into the record output directory |

## Setup

```bash
cd ChenWeb/python/pdf-parser

# Create venv and install the core service dependencies.
# This is enough for the service itself and the OpenDataLoader backend wrapper.
uv sync

# OpenDataLoader backend:
# No extra Python package is needed here, but you still need Java 11+ and
# opendataloader-pdf-cli.jar available (see prerequisites below).

# MinerU backend:
# Install the external MinerU project/CLI separately, for example:
#   cd ~/Workspace/ThirdParty/mineru
#   mise run install
# Then point this service at the CLI with MINERU_CLI if needed.

# To enable Docling:
uv pip install -e ".[docling]"

# To also use the PaddleOCR backend:
uv pip install -e ".[paddle]"

# To enable both optional backends:
uv pip install -e ".[docling,paddle]"
```

### Prerequisites

- **Python 3.12+**
- **PostgreSQL** with the `kb.inputs` table
- **Java 11+** on PATH (for the opendata backend)
- **opendataloader-pdf JAR** at `~/Workspace/ThirdParty/opendataloader-pdf/` or set `OPENDATA_JAR_PATH`
- **MinerU CLI** installed separately if you want to use `parser_name='mineru'`
- **Docling package** installed if you want to use `parser_name='docling'`

## Running

```bash
# Foreground (from ChenWeb/)
caffeinate -i -s mise ocr-service-start-sync

`caffeinate` applies to macOS only. `-i` prevents idle sleep, `-s` prevents system sleep on AC power.

# Background (from ChenWeb/)
mise ocr-service-start
mise ocr-service-stop
mise ocr-service-status

# Directly
cd ChenWeb/python/pdf-parser
PYTHONPATH=. .venv/bin/python pdf_parser.py
```

## Environment Variables

### Required (PostgreSQL)

| Variable | Default | Description |
|---|---|---|
| `PG_HOST` | `127.0.0.1` | PostgreSQL host |
| `PG_PORT` | `5432` | PostgreSQL port |
| `PG_DB_NAME` | `miner` | Database name |
| `PG_USER_NAME` | `admin` | Database user |
| `PG_PASSWORD` | | Database password |

### Service Configuration

| Variable | Default | Description |
|---|---|---|
| `STAGING_DIR` | | Directory to scan for new PDF files |
| `PDF_REPO_DIR` | `/tmp/pdf-repo` | Base directory for parse results |
| `PDF_BACKUP_DIR` | `/tmp/pdf-backup/pdf_files` | Backup directory for originals |
| `PDF_POLL_INTERVAL` | `10` | Seconds between DB polls |
| `PDF_BATCH_SIZE` | `25` | Records fetched per poll cycle |
| `PDF_DEFAULT_PARSER` | `opendata` | Parser when `parser_name` is not set |
| `PDF_PIPELINE_MODE` | `poll` | `poll` (legacy DB polling) or `jetstream` (consume stage events) |

### JetStream Pipeline

| Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://127.0.0.1:4222` | NATS server URL |
| `NATS_USER` | | Username for NATS auth (optional) |
| `NATS_PASS` | | Password for NATS auth (optional) |
| `NATS_TOKEN` | | Token auth alternative (optional) |
| `PDF_STAGE_EVENT_SUBJECT` | `kb.pdf.staged` | Subject consumed from Step 1 |
| `PDF_STAGE_EVENT_DURABLE` | `pdf-parser` | Durable consumer name |
| `PDF_STAGE_EVENT_STREAM` | | Optional: auto-create stream for stage subject |
| `PDF_PARSED_EVENT_SUBJECT` | `kb.pdf.parsed` | Subject published to Step 3 |
| `PDF_PARSED_EVENT_STREAM` | | Optional: auto-create stream for parsed subject |

### OpenDataLoader-specific

| Variable | Default | Description |
|---|---|---|
| `OPENDATA_JAR_PATH` | auto-detected | Path to `opendataloader-pdf-cli.jar`. Set this environment variable only when you want to force using your .jar file. Currently, it defaults to '~/Workspace/ThirdParty/opendataloader-pdf/python/opendataloader-pdf/src/opendataloader_pdf/jar/opendataloader-pdf-cli.jar' |

### MinerU-specific

| Variable | Default | Description |
|---|---|---|
| `MINERU_CLI` | auto-detected via `PATH` | Absolute path to the `mineru` executable. Set this when the CLI is not on `PATH` — e.g. the project-local uv venv at `~/Workspace/ThirdParty/mineru/.venv/bin/mineru`. |
| `MINERU_BACKEND` | `hybrid-auto-engine` (CLI default) | Passed as `-b` to the CLI. See the backend table below. |
| `MINERU_EXTRA_ARGS` | | Extra whitespace-separated CLI args appended verbatim (e.g. `-l en`, `-m ocr`, `-u http://127.0.0.1:30000`). |

Device selection is **not** a flag on the MinerU 3.x CLI — it is controlled by MinerU's own environment variable `MINERU_DEVICE_MODE` (read by MinerU internally):

| Variable | Value | Description |
|---|---|---|
| `MINERU_DEVICE_MODE` | `mps` / `cuda` / `cpu` | Compute device for local backends. On Apple Silicon (M1–M4) use `mps`. Already set to `mps` in `ThirdParty/mineru/mise.toml`. |

#### MinerU backends (`MINERU_BACKEND` / `-b`)

| Value | Compute | Accuracy | When to use |
|---|---|---|---|
| `pipeline` | Local, classical OCR + layout models | Good, fastest | General docs, max throughput, lowest memory |
| `vlm-auto-engine` | Local VLM (in-process) | High | Best single-box accuracy with GPU/MPS and enough RAM |
| `hybrid-auto-engine` **(default)** | Local, mixes pipeline + VLM | Highest | MinerU's recommended default — "next-gen" solution |
| `vlm-http-client` | Remote VLM over HTTP | High | Requires a separate `mineru-openai-server` (set `-u` via `MINERU_EXTRA_ARGS`) |
| `hybrid-http-client` | Remote VLM + small local work | High | Same as above, with local pipeline assist |

### Docling-specific

Docling does not currently require service-specific environment variables in this wrapper.

To use it:

- install the optional dependency with `uv pip install -e ".[docling]"`
- set `parser_name` to `docling` on the `kb.inputs` record, or make it your default with `PDF_DEFAULT_PARSER=docling`

Docling writes a UTF-8 JSON export to:

```text
{PDF_REPO_DIR}/pdf_parser/{record_id}/{pdf_stem}.docling.json
```

### PaddleOCR-specific

| Variable | Default | Description |
|---|---|---|
| `PDF_USE_VL` | `true` | Use PaddleOCR-VL mode |
| `PDF_MPS` | `false` | Attempt Apple Metal GPU acceleration for torch-backed PaddleOCR-VL builds. Can be unavailable on Paddle-backed builds. |
| `PDF_TIMING` | `false` | Log per-block timing |
| `PDF_VLM_OCR_MAX_PIXELS` | `50176` | Cap on min_pixels for OCR blocks |
| `PDF_QUANTIZE_ENABLED` | `false` | Enable weight quantization |
| `PDF_QUANTIZE_BITS` | `8` | Quantization bit width (4 or 8) |

## How It Works

1. **Input source**:
   - `poll` mode: poll `kb.inputs` for unprocessed rows
   - `jetstream` mode: consume events from `PDF_STAGE_EVENT_SUBJECT`
2. **Dispatch** to the appropriate backend based on `parser_name` (or `PDF_DEFAULT_PARSER`)
3. **Track progress** by updating the `"parsing"` status entry (throttled to one DB write per 3 seconds)
4. **On completion**, replace the `"parsing"` entry with a `"parsed"` entry (success or failure)
5. **Write results** to `{PDF_REPO_DIR}/pdf_parser/{record_id}/`
6. **Publish parsed event** to `PDF_PARSED_EVENT_SUBJECT` for downstream converters

### Status Format

While parsing:
```json
{"operation": "parsing", "progress": "65%", "start_time": "20260407 14:30:00", "ms-used": 12000}
```

When finished:
```json
{"operation": "parsed", "proc-status": "success", "start_time": "20260407 14:30:00", "ms-used": 72000, "error": ""}
```

## Project Structure

```
pdf_parser.py        Main entry: config, service loop, parser dispatch
shared.py            DB polling, status management, file helpers
parser_base.py       Abstract base class for parser backends
parser_docling.py    Docling backend
parser_paddle.py     PaddleOCR backend
parser_opendata.py   opendataloader-pdf backend
parser_mineru.py     MinerU backend
tests/               Unit tests for parser backends and service helpers
```

## Adding a New Parser

1. Create `parser_newname.py` implementing `ParserBackend` from `parser_base.py`
2. Register it in `PARSER_REGISTRY` in `pdf_parser.py`

## Tests

```bash
cd ChenWeb/python/pdf-parser
PYTHONPATH=. .venv/bin/pytest tests/ -v
```
