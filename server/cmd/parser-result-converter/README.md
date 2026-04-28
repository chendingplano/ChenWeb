# Parser Result Converter Service

The `parser-result-converter` service consumes JetStream messages for parsed PDFs and converts parser JSON output into canonical `.txt` Line Files used by the downstream document-processing pipeline.

## What It Does

- Subscribes to JetStream subject `kb.pdf.parsed`
- Expects payload fields:
  - `record_id`
  - `result_filename`
  - `file_format`
  - optional: `type`, `status`
- Loads `kb.inputs` by `record_id`
- Validates record state before conversion:
  - `type == pdf`
  - `status` contains parsed success (`operation=parsed`, `proc-status=success`)
- Uses converter by `parser_name`:
  - empty / `opendata`: converts JSON to `.txt`
  - `paddleocr`: currently returns not-implemented
  - `mineru`: currently returns not-implemented
  - `docline`: currently returns not-implemented
- Appends a `converted` status entry to `kb.inputs.status` with success/failed
- Publishes completion event to subject `kb.line-file-generated`
- Removes repeated non-empty lines that appear on every page by default

## Run

From workspace `ChenWeb/` root:

```bash
mise parser-result-converter-run
```

From `ChenWeb/server/cmd/parser-result-converter/`:

```bash
mise parser-result-converter-run
```

## Background Tasks

From workspace `ChenWeb/` root:

```bash
mise parser-result-converter-start
mise parser-result-converter-status
mise parser-result-converter-stop
```

PID/log files:

- `.cache/parser-result-converter.pid`
- `.cache/parser-result-converter.log`

## Build

From workspace `ChenWeb/` root:

```bash
mise build-parser-result-converter
```

Binary output:

- `.cache/parser-result-converter.exe`

## Environment Variables

- `NATS_URL` (default: `nats://127.0.0.1:4222`)
- `NATS_USER` (optional; username for authenticated NATS connection)
- `NATS_PASS` (optional; password for authenticated NATS connection)
- `NATS_TOKEN` (optional; token auth alternative to user/pass)
- `PARSER_RESULT_CONVERTER_SUBJECT`, `PDF_RESULT_CONVERTER_SUBJECT`, and `PDF_CONVERTER_SUBJECT` are ignored by this service.
  - Subscription subject is fixed to `kb.pdf.parsed`.
- `PARSER_RESULT_CONVERTER_DURABLE` (fallback: `PDF_RESULT_CONVERTER_DURABLE`, `PDF_CONVERTER_DURABLE`, default: `parser-result-converter`)
- `PARSER_RESULT_CONVERTER_STREAM` (fallback: `PDF_RESULT_CONVERTER_STREAM`, `PDF_CONVERTER_STREAM`, optional)
  - when set, service ensures the stream exists before subscribing
  - stream retention must be `LimitsPolicy` (not `WorkQueuePolicy`) because this service uses `DeliverNew`
- `PARSER_RESULT_CONVERTER_AUTO_RECREATE_STREAM` (fallback: `PDF_RESULT_CONVERTER_AUTO_RECREATE_STREAM`, `PDF_CONVERTER_AUTO_RECREATE_STREAM`, optional)
  - default: `true`
  - when enabled, and the subject owner stream is `WorkQueuePolicy`, service will auto-recreate that stream as `LimitsPolicy`
  - set to `false` / `0` / `no` / `off` to disable
  - safety guard: auto-recreate only runs when the stream has exactly one subject and it is `kb.pdf.parsed`
- `PARSER_RESULT_CONVERTER_CONFIG` (fallback: `FILE_CONVERTER_CONFIG`, default: `../../../config.toml`)
- `LINE_FILE_REMOVE_REPEAT_LINES` (default: enabled)
  - repeated non-empty content that appears on every page is removed from generated Line Files
  - set `LINE_FILE_REMOVE_REPEAT_LINES=false` to keep those repeated lines
- `LINE_FILE_REMOVE_REPEAT_PERCENT` (default: `85`)
  - when repeat-line removal is enabled, a line is removed if the same non-empty content appears on at least this percent of pages

Legacy fallback variables (for backward compatibility):

- `PDF_CONVERTER_SUBJECT`
- `PDF_RESULT_CONVERTER_SUBJECT`
- `PDF_RESULT_CONVERTER_DURABLE`
- `PDF_RESULT_CONVERTER_STREAM`
- `PDF_RESULT_CONVERTER_AUTO_RECREATE_STREAM`
- `PDF_CONVERTER_DURABLE`
- `PDF_CONVERTER_STREAM`
- `FILE_CONVERTER_CONFIG`

## NATS Auth Behavior

- If `nets-server` runs without `NATS_USER`/`NATS_PASS` (and without `NATS_TOKEN`), clients can connect without credentials.
- If `nets-server` runs with `NATS_USER`/`NATS_PASS`, all clients (publishers/subscribers), including this converter service, must provide matching credentials or connection will be rejected.
- Setting `NATS_USER`/`NATS_PASS` configures access for the running NATS server process. It is not a separate user-account provisioning workflow.

## Running with shared `nets-server`

Start JetStream server in shared repo:

```bash
cd ~/Workspace/shared/go/cmd/nets-server
mise run nets-server-run
```

Then start converter service in `ChenWeb`:

```bash
cd ~/Workspace/ChenWeb
export NATS_URL=nats://127.0.0.1:4222
export NATS_USER=admin
export NATS_PASS='your-password'
# optional stream bootstrap
export PARSER_RESULT_CONVERTER_STREAM=kb_inputs
mise parser-result-converter-run
```

## Message Filtering

Messages are ignored unless:

- `type` is absent or `pdf`
- `status` is absent or `success`

This lets publishers include routing metadata while keeping converter behavior strict.
