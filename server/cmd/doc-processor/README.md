# Doc Processor Service

The `doc-processor` control service consumes JetStream messages from `kb.line-file-generated` and runs multiple document processors.

## Processors

1. `structure_analyzer` (`server/api/doc-processing/doc-structure-analyzer.go`)
2. `chunking` (`server/api/doc-processing/chunking.go`)
3. `extract_doc_metadata` (`server/api/doc-processing/extract-doc-metadata.go`)
4. `extract_metrics` (`server/api/doc-processing/extract-metrics.go`)

## Run

From workspace `aas/` root:

```bash
mise doc-processor-run
```

From `aas/server/cmd/doc-processor/`:

```bash
mise doc-processor-run
```

## Background Tasks

```bash
mise doc-processor-start
mise doc-processor-status
mise doc-processor-stop
```

PID/log files:

- `.cache/doc-processor.pid`
- `.cache/doc-processor.log`

## Build

```bash
mise build-doc-processor
```

Binary output:

- `.cache/doc-processor.exe`

## Environment Variables

### NATS

- `NATS_URL` (default: `nats://127.0.0.1:4222`)
- `NATS_USER` / `NATS_PASS` / `NATS_TOKEN` (optional)
- `DOC_PROCESSOR_EVENT_SUBJECT` (default: `kb.line-file-generated`)
- `DOC_PROCESSOR_EVENT_DURABLE` (default: `doc-processor`)
- `DOC_PROCESSOR_EVENT_STREAM` (optional)

### Config

- `DOC_PROCESSOR_CONFIG` (fallbacks: `EXTRACT_DOCMETA_CONFIG`, `FILE_CONVERTER_CONFIG`; default: `../../../config.toml`)

### Metadata Processor

- `EXTRACT_DOCMETA_NUM_PAGES` (default: `2`)
- `EXTRACT_DOCMETA_LLM_NAME` (defaults to `SHARED_LLM_NAME`)
- `EXTRACT_DOCMETA_LLM_API_KEY`
- `EXTRACT_DOCMETA_LLM_BASE_URL`
- `EXTRACT_DOCMETA_LLM_TIMEOUT_SEC`
- `EXTRACT_DOCMETA_PROMPT`
- `PROMPT_DIR`

Rules:
- If `EXTRACT_DOCMETA_LLM_NAME` is set, then `EXTRACT_DOCMETA_LLM_API_KEY`, `EXTRACT_DOCMETA_LLM_BASE_URL`, and `EXTRACT_DOCMETA_LLM_TIMEOUT_SEC` are all required.
- If `EXTRACT_DOCMETA_LLM_NAME` is not set, the processor falls back to `SHARED_LLM_*`.

### Metrics Processor

- `EXTRACT_METRICS_LLM_NAME` (defaults to `SHARED_LLM_NAME`)
- `EXTRACT_METRICS_LLM_API_KEY`
- `EXTRACT_METRICS_LLM_BASE_URL`
- `EXTRACT_METRICS_LLM_TIMEOUT_SEC`
- `EXTRACT_METRICS_PROMPT` (or `PROMPT_FILE_NAME`)

Rules:
- If `EXTRACT_METRICS_LLM_NAME` is set, then `EXTRACT_METRICS_LLM_API_KEY`, `EXTRACT_METRICS_LLM_BASE_URL`, and `EXTRACT_METRICS_LLM_TIMEOUT_SEC` are all required.
- If `EXTRACT_METRICS_LLM_NAME` is not set, the processor falls back to `SHARED_LLM_*`.

### Shared LLM

- `SHARED_LLM_NAME` (required when any processor falls back to shared)
- `SHARED_LLM_API_KEY` (required when any processor falls back to shared)
- `SHARED_LLM_BASE_URL` (required when any processor falls back to shared)
- `SHARED_LLM_TIMEOUT_SEC` (default: `100`)
