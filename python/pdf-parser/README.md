# PDF Parser Service

A unified Python service that polls the `kb.inputs` PostgreSQL table for unprocessed PDF records and dispatches parsing to the appropriate backend based on the record's `parser_name` field.

## Supported Parsers

| Name | Engine | How it works |
|---|---|---|
| `opendata` (default) | [opendataloader-pdf](https://github.com/opendataloader-project/opendataloader-pdf) | Invokes the Java CLI JAR as a subprocess |
| `paddleocr` | [PaddleOCR-VL](https://github.com/PaddlePaddle/PaddleOCR) | Python API with optional Apple Metal GPU (MPS) acceleration |
| `mineru` | [MinerU](https://github.com/opendatalab/mineru) 4.0.9 | Invokes `mineru-kit parse` for a ZIP result and adapts it to ChenWeb's stable page-wrapped JSON |
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

# MinerU 4.0.9 backend:
# Install the external MinerU project/CLI separately:
#   cd ~/Workspace/ThirdParty/mineru
#   mise run install
# This installs mineru-kit in ThirdParty/mineru/.venv and creates the default
# ~/.mineru/config.yaml from the checked-in template when no config exists.
# Set MINERU_CLI only when the CLI is installed elsewhere.

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
- **MinerU 4.0.9 CLI** installed separately if you want to use `parser_name='mineru'`
- **Docling package** installed if you want to use `parser_name='docling'`

## Running

```bash
# Foreground (from ChenWeb/)
mise ocr-service-start-sync

# Background (from ChenWeb/)
mise ocr-service-start
mise ocr-service-stop
mise ocr-service-status

# Directly
cd ChenWeb/python/pdf-parser
PYTHONPATH=. .venv/bin/python pdf_parser.py
```

### One-off WeVisDoc-4B run on a PDF (Apple silicon)

The standalone runner renders each PDF page to a temporary 200-DPI PNG, then
passes the pages to WeVisDoc-4B through the MLX-VLM environment in
`ThirdParty/mlx-vlm/.venv`. It loads the model once per PDF and writes
`<pdf-name>.wevisdoc.md` beside the input file. Per-page inference times are
printed to stderr. The first run downloads the Hugging Face checkpoint; later
runs use the local cache.

```sh
cd ChenWeb/python/pdf-parser
.venv/bin/python wevisdoc_mlx_pdf.py /path/to/input.pdf
```

Use `--output` to choose another Markdown path, `--dpi` to change rendering
resolution, or `--max-tokens` to allow longer output for dense pages. The
default generation limit is 5,000 tokens per page. The prompt is stored at
`ChenWeb/prompts/prompt-wevisdoc-page-to-markdown-v1.md`.

The service does **not** keep the Mac awake. If a long batch must run while you are
away, keep the Mac awake from the workspace root (macOS only):

```bash
mise start caffeinate   # before the batch
mise check caffeinate   # is it running?
mise stop caffeinate    # afterwards, so the Mac can sleep again
```

These tasks live in `Workspace/mise.toml`; no ChenWeb module starts or stops
`caffeinate` itself.

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
| `PDF_BACKUP_DIR` | `/tmp/pdf-backup` | Backup directory for originals |
| `PDF_POLL_INTERVAL` | `10` | Seconds between DB polls |
| `PDF_BATCH_SIZE` | `25` | Records fetched per poll cycle |
| `PDF_DEFAULT_PARSER` | `opendata` | Parser when `parser_name` is not set |
| `PDF_PIPELINE_MODE` | `poll` | `poll` (legacy DB polling) or `jetstream` (consume stage events) |
| `PDF_OCR_FALLBACK` | `true` | Auto-detect font-encoding garble in MinerU output and re-parse with MinerU 4 basic OCR. See [Font-encoding garble OCR fallback](#font-encoding-garble-ocr-fallback). |
| `PDF_OCR_FALLBACK_MIN_RUN` | `4` | Min run of consecutive CJK-passthrough chars that triggers the fallback |

### JetStream Pipeline

| Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://127.0.0.1:4222` | NATS server URL |
| `NATS_USER` | | Username for NATS auth (optional) |
| `NATS_PASS` | | Password for NATS auth (optional) |
| `NATS_TOKEN` | | Token auth alternative (optional) |
| `PDF_STAGE_EVENT_SUBJECT` | `kb.pdf.staged` | Subject consumed from Step 1 |
| `PDF_STAGE_EVENT_DURABLE` | `pdf-parser` | Durable consumer name |
| `PDF_STAGE_EVENT_STREAM` | | Preferred stream name for the stage subject; an existing stream owning the subject is reused |
| `PDF_PARSED_EVENT_SUBJECT` | `kb.pdf.parsed` | Subject published to Step 3 |
| `PDF_PARSED_EVENT_STREAM` | | Preferred stream name for the parsed subject; an existing stream owning the subject is reused |

On startup, the service first checks each configured stream name. If that name
does not exist, it looks up the JetStream stream that already owns the required
subject before creating anything. This allows restarts after a machine crash
and preserves streams whose names were created by an older configuration.
Only a JetStream `not found` response triggers this fallback; authentication,
authorization, timeout, and other NATS errors stop startup with their original
error.

### OpenDataLoader-specific

| Variable | Default | Description |
|---|---|---|
| `OPENDATA_JAR_PATH` | auto-detected | Path to `opendataloader-pdf-cli.jar`. Set this environment variable only when you want to force using your .jar file. Currently, it defaults to '~/Workspace/ThirdParty/opendataloader-pdf/python/opendataloader-pdf/src/opendataloader_pdf/jar/opendataloader-pdf-cli.jar' |

### MinerU-specific

| Variable | Default | Description |
|---|---|---|
| `MINERU_CLI` | auto-detected via `PATH`, then `~/Workspace/ThirdParty/mineru/.venv/bin/mineru-kit` | Absolute path to the MinerU Kit CLI. Set this to override auto-detection or use an installation outside the default workspace. |
| `MINERU_CONFIG` | `~/.mineru/config.yaml` | MinerU 4 YAML configuration path. The ThirdParty `mise run install` setup creates it from `mineru.template.yaml` if absent. |

Normal parsing uses the `standard` tier and explicitly processes all pages. MinerU
selects the available local runtime/device from its configuration. The garble
fallback reruns the complete document with the `basic` tier and OCR forced. MinerU
4 removed the old backend, method, and language flags; the legacy
`MINERU_BACKEND`, `MINERU_EXTRA_ARGS`, and `MINERU_DEVICE_MODE` options are not used.

Each parse produces a MinerU ZIP result internally. The parser adapts its
MiddleJson V2 through MinerU 4's Content List V1 renderer and writes ChenWeb's
existing `<pdf-stem>_content_list.json` shape:

```json
{"pages": [{"page_number": 1, "items": [{"type": "text", "text": "..."}]}]}
```

Page numbers are one-based, and image references point to files under the
adjacent `images/` directory. The Go converter and stored-result contract stay
unchanged.

#### Font-encoding garble OCR fallback

Some PDFs — notably Chinese GB/T standards — embed Latin text in fonts that lack
a usable `ToUnicode` CMap. Text-layer extraction then maps those glyphs into the
CJK Unified Ideographs block instead of ASCII, so e.g. *"intended application"*
comes out as `犻狀狋犲狀犱犲犱犪狆狆犾犻犮犪狋犻狅狀`. The corruption is **silent**:
the bytes are valid CJK (not `U+FFFD`), so nothing downstream flags it, and the
standard tier can reproduce it when it reuses the text layer.

After every MinerU parse the service scans the result for this signature — a
**run** of `PDF_OCR_FALLBACK_MIN_RUN` (default 4) consecutive code points in the
passthrough block `U+72AA–U+72D6`. A run, not a count: isolated in-block
characters (legitimate rare ideographs such as `状` U+72B6) do not trip it, but
garbled Latin words always do. On a hit, the document is re-parsed **once** with
`mineru-kit parse` using `--pages all --tier basic --ocr-mode ocr`. This reads
rendered pixels and recovers the Latin; the OCR result is verified clean before
it replaces the original (`engine` becomes `mineru-basic-ocr`). If OCR fails or
is still garbled, the original parse is kept and the event logged. Set
`PDF_OCR_FALLBACK=false` to disable.

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
   - For `mineru`, after parsing, auto-detect font-encoding garble and re-parse with forced basic-tier OCR if needed (see [Font-encoding garble OCR fallback](#font-encoding-garble-ocr-fallback))
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

## Table geometry companions

After a successful parse, PyMuPDF extracts physical table row/cell borders into
`<pdf-stem>.pdf-table-geometry.json` beside the source PDF. Geometry failure is
logged without discarding parsed text. On conversion reruns, the Go converter also
extracts physical geometry when it is missing or stale, using the same Python
module and environment (see the converter README for deployment overrides). It maps those boxes to
canonical table/row/cell IDs in `<line-stem>.table-geometry.json`; the PDF viewer
retrieves that companion through the input's table-geometry API.

To backfill an existing PDF without re-running its parser:

```sh
.venv/bin/python table_geometry.py /path/to/document.pdf
```

The API builds/refreshes the canonical companion on its next request. Line/PDF/
physical geometry hashes prevent stale geometry reuse. See the PDF Viewer capsule's
`table-geometry-design.md` for the coordinate and reference contracts.

New PDF table geometry companions store coordinates as integers, rounded to the nearest integer. The Go line converter and canonical geometry builder apply the same output rule. Existing artifacts are not migrated.
