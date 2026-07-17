# Document-processor benchmark operations

This guide is the runbook for the ChenWeb document-processor benchmark described by ADR `2026071301`. V1 scores both `chunking` and `extract_metrics`; metric extraction automatically runs its production `static_analyzer → chunking → extract_metrics` dependency closure.

## What a benchmark run does

Each committed case has two immutable files:

- `input.lines.txt`: the canonical seven-column line file given to the production controller;
- `expected.json`: the reviewed gold chunks and/or metrics for that input.

The runner verifies the dataset and per-file hashes, creates one logical case run per repetition and variant, seeds a temporary `kb.inputs` row, invokes the real controller, captures the persisted `.chunks`/`.metrics` files and `kb.chunks`/`kb.metrics` rows, reconciles those sources, scores them against `expected.json`, and stores a verified evidence bundle. Reports are generated from the selected stored attempts. They never call the model again.

The checked-in starter corpus is:

```text
benchmark/doc-processors/datasets/doc-processors-synthetic-core/1.0.0/
├── manifest.json
└── cases/<case-id>/
    ├── input.lines.txt
    └── expected.json
```

The starter experiment is `benchmark/doc-processors/experiments/example.toml`. A dataset version is immutable: changing any manifest, input, or expected byte requires a new SemVer directory.

## Prerequisites

Run commands from the ChenWeb repository root. Before a live run:

1. Configure `config.toml`, `.env`, the shared-library configuration, PostgreSQL, prompts, model definitions, and provider credentials exactly as for the document processor.
2. Set `ARTIFACT_DIR` to the production artifact root used by chunking and metric extraction.
3. Set a benchmark-only knowledge-store ID with `BENCHMARK_STORE_ID` or `--store-id`.
4. Choose two distinct, non-overlapping local roots for disposable work and immutable evidence.
5. Commit the working copy. A dirty run is rejected unless `--allow-dirty` is explicit.

Typical environment:

```sh
export ARTIFACT_DIR="$PWD/Data/kb/artifacts"
export BENCHMARK_STORE_ID=1
export BENCHMARK_WORK_ROOT="$PWD/.benchmark/work"
export BENCHMARK_EVIDENCE_ROOT="$PWD/.benchmark/evidence"
```

`run`, `compare`, `report`, and `clean` load ChenWeb configuration, initialize the project/shared database handles, and apply pending Goose migrations. `validate` is filesystem-only and performs no database or model work.

## Validate before spending model tokens

```sh
go run ./server/cmd/doc-benchmark validate \
  --experiment benchmark/doc-processors/experiments/example.toml
```

Success prints one JSON object containing the dataset ID/version/hash, raw experiment request hash, and processor case-set hashes. Validation rejects unknown TOML fields, unsafe paths and symlinks, stale line references, mismatched expected sections, unknown tags/processors, duplicate IDs, and invalid sampling settings.

## Run or resume

```sh
go run ./server/cmd/doc-benchmark run \
  --config config.toml \
  --experiment benchmark/doc-processors/experiments/example.toml \
  --artifact-root "$ARTIFACT_DIR" \
  --store-id "$BENCHMARK_STORE_ID"
```

Success returns the stable experiment ID and its variant names. Repeating the same command reuses the experiment/run/case identities and fills only missing or retryable work. A changed experiment file has a new request hash and therefore creates a new experiment identity.

Useful flags:

- `--datasets-root`, `--work-root`, `--evidence-root`, and `--artifact-root` override their defaults;
- `--tenant-id` and `--store-id` isolate temporary `kb.inputs` rows;
- `--owner` labels leases for recovery;
- `--allow-dirty` permits exploratory work and reports it as non-reproducible.

The command currently initializes and executes variants serially in one CLI process. Case attempt ownership, retry safety, and database uniqueness remain transactional. Do not run variants that depend on conflicting process-global configuration concurrently; use separate CLI invocations until the isolated-worker scheduler is enabled.

## Report and compare

Render all stored variant vectors:

```sh
go run ./server/cmd/doc-benchmark report \
  --experiment-id <uuid> \
  --format json \
  --output .benchmark/reports/<uuid>.json

go run ./server/cmd/doc-benchmark report \
  --experiment-id <uuid> \
  --format markdown \
  --output .benchmark/reports/<uuid>.md
```

Render paired case/repetition deltas:

```sh
go run ./server/cmd/doc-benchmark compare \
  --experiment-id <uuid> \
  --baseline metrics-baseline \
  --candidate metrics-alt \
  --format markdown
```

Comparison uses exact stored case/repetition pairs and refuses incompatible identities. `--allow-incompatible` produces a prominently marked, non-gating exploratory report instead of a winner claim. Reports show completion/failure counts before quality, processor vectors, nullable metrics, paired deltas, provenance hashes, and an explicit non-gating status. There is no combined pipeline-wide score.

## Attempt lifecycle and recovery

Case outcomes are `success`, `processor_failed`, `timed_out`, `invalid_output`, `infrastructure_failed`, `scorer_failed`, or `canceled`.

- A processor or invalid-output result is a visible quality outcome and is selected without silently dropping the case.
- Infrastructure, stale-lease, timeout, and scorer failures may retry up to `max_attempts`.
- If capture was verified, a retry is a `rescore`: it re-verifies the bundle and reruns reconciliation/scoring without creating an input or invoking the model.
- If capture was not verified, a retry is a fresh execution attempt.
- The latest real failure classification is retained when the attempt budget is exhausted.
- Canceling is terminal for that attempt; rerun the same experiment to resume permitted missing work.

Evidence identity includes exact input bytes/hash, expected JSON, resolved runtime JSON/hash, and serialized scorer JSON/hash. Rescore refuses any mismatch and never trusts a previously serialized canonical `actual`; it reconstructs actual output from the raw captured database/file evidence.

## Cleanup

Normal cleanup is automatic after a verified terminal attempt unless `retain_workspaces = true`. To retry retained or `files_pending` cleanup:

```sh
go run ./server/cmd/doc-benchmark clean --experiment-id <uuid>
```

For an interrupted attempt that never produced verified evidence, the only permitted destructive recovery is explicit and attempt-scoped:

```sh
go run ./server/cmd/doc-benchmark clean \
  --attempt-id <attempt-uuid> \
  --discard-unverified
```

Cleanup locks the ownership row, deletes only production rows tied to the captured input ID, deletes that input, validates the allocation marker/root identities, removes only the owned work directory, and retains the workspace audit row as `cleaned`. It never removes verified evidence. A database failure rolls back; a filesystem failure remains `files_pending` for safe retry.

## Live and integration verification

Pure tests need no database:

```sh
go test -race ./server/api/doc-benchmark ./server/cmd/doc-benchmark -count=1
go test ./benchmark/doc-processors/generator -count=1
```

Database migration/concurrency tests use `TEST_DATABASE_URL` and otherwise skip explicitly. Provider-backed execution is intentionally opt-in with `BENCHMARK_LIVE_INTEGRATION=1` in addition to normal credentials; it can consume tokens and incur cost.

## Troubleshooting

- **Working copy is dirty:** commit with `jj`, or use `--allow-dirty` only for exploratory results.
- **Artifact not found:** verify `ARTIFACT_DIR` is identical to the production runtime’s artifact directory and writable.
- **Runtime hash mismatch:** the resolved config bytes changed or a runtime returned a non-canonical hash. Do not bypass this in production.
- **Case file hash changed:** restore the committed dataset or publish a new dataset version; never edit a version in place.
- **`scorer_failed`:** inspect the verified evidence artifact, correct deterministic scorer code, and rerun to create a rescore attempt.
- **`files_pending`:** rerun `clean` for the experiment or attempt after fixing permissions; do not manually delete around the ownership guard.
- **No quality score:** inspect `processor_success`, case lifecycle, and verified evidence. Failed cases intentionally remain visible rather than acquiring a fabricated zero for every quality metric.

## Out of scope for v1

- MLflow
- LangSmith
- Promptfoo
- web UI/public API
- CI/release gating
- PDF parsing/OCR
- human-annotated production documents
- LLM-as-judge/open semantic grading/arbitrary unit conversion
- combined pipeline-wide score
- automatic prompt optimization
- automatic retention/purge of verified evidence
- processors beyond `chunking`/`extract_metrics`
- and multi-host distributed scheduling.
