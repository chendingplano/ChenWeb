# Document-processor benchmark schema and retention

Migrations `20260713000003_create_doc_benchmark_tables.sql` and `20260713000004_extend_benchmark_workspace_ownership.sql` create the V1 benchmark schema under `kb`. Migration `00003` preserves the historical baseline contract; `00004` upgrades it to the canonical `canceled` lifecycle and crash-safe workspace ownership contract. Rolling `00004` down restores the exact `00003` boundary. Rolling `00003` down removes only benchmark objects and never production knowledge rows.

## Tables

| Table | Ownership and purpose |
|---|---|
| `kb.benchmark_experiments` | One immutable raw experiment request and resolved dataset/file/case-set identity. `raw_request_hash` is the resume key. |
| `kb.benchmark_runs` | One named variant under an experiment, including requested/resolved config, scorer/pricing snapshots, hashes, provenance, lifecycle, usage, and runtime summaries. |
| `kb.benchmark_case_runs` | One `(run, case_id, repetition)` sampling unit with applicability, tags, upstream hash, derived outcome, and exactly one selected terminal attempt. |
| `kb.benchmark_case_attempts` | Append-only execution/rescore history with leases, source execution, immutable input ID snapshot, telemetry, failure taxonomy, timing, and capture verification. |
| `kb.benchmark_workspaces` | Persistent authority for one execution attempt’s input, canonical work/evidence roots and paths, nonce/marker hashes, verification, and cleanup recovery state. |
| `kb.benchmark_scores` | Nullable scalar/additive score rows owned by exactly one attempt or run, with processor/scorer/version/direction/aggregation metadata. |
| `kb.benchmark_artifacts` | Hashed input/evidence/diagnostic/report references owned by exactly one attempt or run. Verified artifacts are immutable. |

## Core constraints

- Experiments are unique by raw request hash; runs by `(experiment_id, variant_name)`; case runs by `(run_id, case_id, repetition)`; attempts by `(case_run_id, attempt_number)`.
- A selected attempt must belong to the same case run. The partial unique selected-attempt index prevents one attempt from being selected by multiple cases.
- A rescore must reference an execution attempt in the same case and cannot own a workspace or invoke production processing.
- Only non-terminal attempts can heartbeat or accept result rows. Terminal run/case/attempt payloads are immutable.
- Scores and artifacts have exactly one owner (`attempt_id` XOR `run_id`). Selected/terminal scores and verified artifacts cannot be changed or deleted.
- Workspace input ownership is unique and is bound transactionally to both the workspace and the execution attempt’s immutable snapshot.
- Cleanup queries are parameterized by the workspace-owned numeric input ID. The transaction deletes `kb.metrics`, then `kb.chunks`, then the exact `kb.inputs` row.

Canonical final lifecycles are:

- run: `queued`, `running`, `succeeded`, `failed`, `canceled`;
- case: `pending`, `running`, `success`, `processor_failed`, `timed_out`, `invalid_output`, `infrastructure_failed`, `scorer_failed`, `canceled`;
- attempt: `queued`, `leased`, `running`, `succeeded`, `failed`, `canceled`;
- workspace cleanup: `pending`, `active`, `error`, `db_pending`, `files_pending`, `cleaned`.

## Retention model

`BENCHMARK_WORK_ROOT` is disposable. `BENCHMARK_EVIDENCE_ROOT` is immutable and has no automatic V1 expiry. The roots must be absolute after resolution, non-overlapping, non-symlink directory trees. Each allocation stores a nonce and filesystem identities so a replaced root or forged marker cannot authorize recursive deletion.

After verified capture, cleanup commits production-row/input deletion in one ownership-locked transaction, removes the work directory, and marks the workspace `cleaned`. Attempts, selected results, scores, input ID snapshots, evidence files, hashes, and artifact rows remain. `clean` is never an evidence purge.

Unverified evidence can be discarded only with an explicit attempt ID and `--discard-unverified`. Broad filename-, timestamp-, tenant-, or parser-based deletion is forbidden.

## Useful audit queries

Experiment and variant status:

```sql
SELECT e.id AS experiment_id, e.name, e.dataset_id, e.dataset_version,
       r.id AS run_id, r.variant_name, r.lifecycle
FROM kb.benchmark_experiments e
JOIN kb.benchmark_runs r ON r.experiment_id = e.id
WHERE e.id = $1
ORDER BY r.variant_name;
```

Selected outcomes and attempt history:

```sql
SELECT c.case_id, c.repetition, c.lifecycle AS case_lifecycle,
       a.attempt_number, a.kind, a.lifecycle AS attempt_lifecycle,
       a.failure_kind, a.capture_verified, a.runtime_ms
FROM kb.benchmark_case_runs c
LEFT JOIN kb.benchmark_case_attempts a ON a.case_run_id = c.id
WHERE c.run_id = $1
ORDER BY c.case_id, c.repetition, a.attempt_number;
```

Cleanup recovery queue:

```sql
SELECT w.execution_attempt_id, w.cleanup_state, w.cleanup_error,
       w.canonical_dir, w.verified, w.cleaned_at
FROM kb.benchmark_workspaces w
WHERE w.cleanup_state <> 'cleaned'
ORDER BY w.created_at;
```

Report-visible scores must follow only the selected attempt (or the verified source execution when the selected attempt is a rescore). Application code uses `SQLStore.ReportScores` and `SQLStore.ReportArtifacts`; ad-hoc reporting should preserve the same join rule.

## Out of scope for v1

MLflow, LangSmith, Promptfoo, web UI/public API, CI/release gating, PDF parsing/OCR, human-annotated production documents, LLM-as-judge/open semantic grading/arbitrary unit conversion, combined pipeline-wide score, automatic prompt optimization, automatic retention/purge of verified evidence, processors beyond `chunking`/`extract_metrics`, and multi-host distributed scheduling.
