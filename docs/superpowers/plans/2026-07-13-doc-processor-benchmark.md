# Doc Processor Benchmark Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a ChenWeb-native v1 benchmark that deterministically validates, executes, scores, compares, reports, resumes, and cleans benchmark runs for `chunking` and `extract_metrics` through the production document-processing path.

**Architecture:** A new `docbenchmark` package owns immutable dataset/config contracts, processor-specific adapters and scorers, SQL persistence, attempt lifecycle, evidence capture, and report generation. A thin `doc-benchmark` CLI runs an orchestrator that launches isolated worker subprocesses; workers initialize the existing production `ControlService`, seed temporary `kb.inputs`, capture canonical outputs, score them, and safely clean disposable state. The implementation follows the approved design in `docs/superpowers/specs/2026-07-13-doc-processor-benchmark-design.md`; MLflow, LangSmith, Promptfoo, web UI, release gating, and processors other than the two named above remain out of scope for v1.

**Tech Stack:** Go 1.25, PostgreSQL, Goose SQL migrations, `database/sql`, `go-toml/v2`, `shopspring/decimal`, standard-library `flag`/`os/exec`, `sqlmock`, deterministic JSON/TOML and SHA-256.

---

## File map

New package `server/api/doc-benchmark` (Go package name `docbenchmark`):

- `types.go`: lifecycle enums and immutable experiment/run/case/attempt value types.
- `dataset.go`: manifest/gold structs, strict JSON loading, applicability rules, and semantic validation.
- `dataset_hash.go`: raw-file dataset/per-file/case-set hashing with the versioned length-framed format.
- `experiment.go`: TOML parsing, defaults, explicit-variant expansion, requested overrides, and policy validation.
- `chunk_score.go`: chunk canonicalization, invariant checks, boundary/coverage scoring, diagnostics.
- `metric_normalize.go`: text, decimal, range, percent, and unit normalization.
- `metric_match.go`: deterministic maximum-total-weight bipartite matching and tie breaking.
- `metric_score.go`: extraction metrics, hard invariants, and diagnostics.
- `aggregate.go`: run/slice aggregation and paired comparison statistics.
- `report.go`: deterministic JSON and Markdown reports.
- `store.go`: persistence interfaces and shared SQL scan/transaction helpers.
- `store_runs.go`: experiments, immutable variants/runs, and logical case runs.
- `store_attempts.go`: transactional claims, leases, attempts, workspaces, and terminalization.
- `store_results.go`: immutable scores, artifacts, telemetry, and report reads.
- `workspace.go`: safe work/evidence path allocation, marker/nonces, hashing, and cleanup guards.
- `adapter.go`: processor-adapter contract and canonical capture types.
- `chunk_adapter.go`: seed/capture/cleanup for production chunking outputs.
- `metric_adapter.go`: seed/capture/cleanup for production metric outputs.
- `runner.go`: case-attempt state machine, execution/rescore routing, heartbeats, capture, score, and cleanup.
- `worker.go`: worker request/ready/result JSON protocol and one-variant runtime initialization.
- `orchestrator.go`: matrix scheduling, process isolation, permits, resume, and aggregation.

Existing production package `server/api/doc-processing`:

- `control.go`: expose a synchronous error-returning entry point that delegates to the existing production handler.
- `runtime.go`: centralize the production processor graph now assembled in `server/cmd/doc-processor/main.go` and expose redacted benchmark snapshots.
- `fix-size-chunking.go`: expose the exact production raw-line byte-size calculation.
- Focused `_test.go` files verify these narrow exported seams without changing processor behavior.

Other files:

- `project_migrations/20260713000001_create_doc_benchmark_tables.sql`: benchmark experiment/run/attempt/workspace/score/artifact schema and rollback.
- `server/cmd/doc-processor/main.go`: use the shared production runtime builder.
- `server/cmd/doc-benchmark/main.go`: CLI/bootstrap only.
- `server/cmd/doc-benchmark/main_test.go`: command parsing and exit behavior.
- `benchmark/doc-processors/datasets/synthetic-v1/**`: deterministic v1 manifest, line fixtures, and gold labels.
- `benchmark/doc-processors/generator/**`: deterministic structured case templates and fixture generator.
- `benchmark/doc-processors/experiments/example.toml`: runnable example limited to `chunking` and `extract_metrics`.
- `ChenWeb/docs/doc-processor-benchmark-operations.md`: operator workflow, safety, troubleshooting, and explicit v1 exclusions.
- `KnowledgeStore/Capsules/coding-capsules/doc-processor/+CAPSULE.md`: link the implemented benchmark and summarize the production seam; update in its own repository/commit after confirming that repository is clean.

## Chunk 1: Deterministic contracts and scorers

### Task 1: Dataset manifest, applicability, and canonical hash

**Files:**
- Create: `server/api/doc-benchmark/types.go`
- Create: `server/api/doc-benchmark/dataset.go`
- Create: `server/api/doc-benchmark/dataset_hash.go`
- Test: `server/api/doc-benchmark/dataset_test.go`

- [ ] **Step 1: Write failing filesystem/schema tests for raw `manifest.json`.** Cover valid/invalid SemVer 2.0.0 dataset versions, schema-v1/unsupported schema, duplicate case IDs, restricted ASCII path/case-ID characters, missing files, absolute/`..`/symlink escapes, duplicate normalized references, unsupported processors, and empty applicability. Assert errors contain the case ID and JSON field path.
- [ ] **Step 2: Write failing semantic-gold tests.** Reject unknown or repeated manifest tags, processor lists that do not exactly match expected JSON sections, stale/invalid chunk line references, out-of-range metric source spans, duplicate gold IDs, and malformed protected groups. Experiment filter tag de-duplication/sorting is tested separately in Task 2.
- [ ] **Step 3: Write failing raw hash tests.** Assert the stream prefix `chenweb-doc-benchmark-dataset-v1\n`, big-endian entry count, UTF-8 byte-sorted cleaned paths, and big-endian path/content length frames. Prove directory enumeration order is irrelevant, while formatting-only `manifest.json` changes, source bytes, and expected bytes each change the dataset hash.
- [ ] **Step 4: Write failing provenance-hash tests.** Assert raw SHA-256 for every input/expected path and processor case-set hashes over sorted applicable `(case_id,repetition)` units; filters and repetitions must change the appropriate case-set hash.
- [ ] **Step 5: Run `go test ./server/api/doc-benchmark -run 'Test(Load|Validate|DatasetHash|FileHashes|CaseSetHash)' -count=1`.** Expected: FAIL because the package/types do not exist.
- [ ] **Step 6: Implement strict structs/loading and root-confined reads in `dataset.go`.** Define only the two v1 processors and checked-in tag vocabulary; preserve raw file bytes for hashing; validate all manifest, path, applicability, line/span, protected-group, and expected-section rules with deterministic sorted errors.
- [ ] **Step 7: Implement `dataset_hash.go`.** Stream raw `manifest.json` plus the unique referenced raw files using the exact prefix/count/path/content framing; separately compute/store per-file SHA-256 and processor case-set hashes. Do not canonicalize JSON before hashing.
- [ ] **Step 8: Re-run the focused tests.** Expected: PASS, including deterministic error ordering and formatting-sensitive hashing.
- [ ] **Step 9: Commit with `jj describe -m 'feat: add benchmark dataset contract' && jj new`.**

### Task 2: Experiment parsing, defaults, matrix expansion, and policy

**Files:**
- Create: `server/api/doc-benchmark/experiment.go`
- Test: `server/api/doc-benchmark/experiment_test.go`

- [ ] **Step 1: Write failing tests for the exact example in spec section 7.** Assert resolved experiment defaults and deterministic lexical expansion of the explicit `[[variants]]` list into runs, unique variant names, processor subset validation, repetitions, timeouts, attempt lease, max attempts, both parallelism settings, `allow_upstream_variation=false`, and validated/de-duplicated/byte-sorted filter tags. Do not invent Cartesian axes.
- [ ] **Step 2: Write failing policy tests.** Accept only allowlisted prompt/model/config overrides for `chunking` and `extract_metrics`; reject API keys, arbitrary environment variables, unknown processors/fields, duplicate variants, `attempt_lease <= timeout`, and invalid duration/count values before any DB write.
- [ ] **Step 3: Write failing requested-intent hash tests.** Assert original TOML/raw request hash, materialized experiment defaults, canonical requested override JSON, file/case-set hashes, and rejection of secret-shaped keys. Authoritative prompt/model/default resolution remains a Task 9 worker-handshake responsibility.
- [ ] **Step 4: Run `go test ./server/api/doc-benchmark -run 'Test(Experiment|Expand|Override|Snapshot)' -count=1`.** Expected: FAIL with undefined experiment APIs.
- [ ] **Step 5: Implement strict TOML decoding and explicit-variant expansion.** Use `toml.Decoder.DisallowUnknownFields`, the exact section-7 allowlists, `time.ParseDuration`, stable explicit variant ordering, and a redaction walk that rejects secret-shaped keys rather than masking them. Persist requested intent separately from the later production `ResolvedConfig()` snapshot.
- [ ] **Step 6: Re-run the focused tests.** Expected: PASS.
- [ ] **Step 7: Commit with `jj describe -m 'feat: resolve benchmark experiment matrices' && jj new`.**

### Task 3: Chunking scorer using production byte semantics

**Files:**
- Modify: `server/api/doc-processing/fix-size-chunking.go`
- Test: `server/api/doc-processing/fix-size-chunking_test.go`
- Create: `server/api/doc-benchmark/chunk_score.go`
- Test: `server/api/doc-benchmark/chunk_score_test.go`

- [ ] **Step 1: Add a failing production-package test for `ChunkLineRawByteSize(Line)`.** Assert it matches the internal `lineRawByteSize` for ASCII, tabs, Unicode, and an explicitly populated `Raw` field.
- [ ] **Step 2: Run `go test ./server/api/doc-processing -run TestChunkLineRawByteSize -count=1`.** Expected: FAIL because the exported seam does not exist.
- [ ] **Step 3: Add the one-line exported wrapper and rerun the test.** Expected: PASS without modifying internal chunk behavior.
- [ ] **Step 4: Write failing primary/empty-set scorer tests.** Cover exact sequences, one shifted boundary, one-chunk documents, eligible-lines with empty output, no-eligible-lines with empty/non-empty output, and every explicit boundary/overlap precision-recall-F1 empty-denominator rule. Assert the primary vector and additive TP/FP/FN rows.
- [ ] **Step 5: Run `go test ./server/api/doc-benchmark -run TestScoreChunks -count=1`.** Expected: FAIL with undefined scorer.
- [ ] **Step 6: Implement canonical ordered set/rate scoring.** Parse only explicit normal/overlap rows, exclude case-folded `toc` from eligible lines, use physical source order, and compute exact pass, boundary, coverage/missing/extra/duplicate/reordered, and overlap metrics with their exact empty rules and additive components.
- [ ] **Step 7: Write failing invariant tests.** Cover sequence gaps, TOC/unknown normal lines, missing/extra/duplicate/reordered lines, no first-chunk overlap, overlap not owned by the immediately preceding normal payload, non-final payload below 80%, the fixed 20%-of-chunk-size/one-line overlap cap independent of configured desired overlap, and protected groups with both `never` and `expected` policies.
- [ ] **Step 8: Implement stable rule IDs and diagnostics.** Use `ChunkLineRawByteSize`, source-order maps, resolved chunk settings, protected-group declarations, cases-with-any plus raw violation counts, and diagnostics containing line numbers and artifact hashes.
- [ ] **Step 9: Add mutation tests.** Starting from exact fixtures, independently shift a boundary, remove/add/duplicate/reorder a line, corrupt overlap, create a sequence gap, and split a protected list; assert intended metrics/rules change and unrelated values stay stable.
- [ ] **Step 10: Run `go test ./server/api/doc-benchmark ./server/api/doc-processing -run 'Test(ScoreChunks|ChunkInvariants|ChunkLineRawByteSize|ChunkScoreMutations)' -count=1`.** Expected: focused tests PASS; note unrelated pre-existing summary-ID failures only if running the whole doc-processing package.
- [ ] **Step 11: Commit with `jj describe -m 'feat: score benchmark chunk output' && jj new`.**

### Task 4: Metric normalization and deterministic optimal matching

**Files:**
- Create: `server/api/doc-benchmark/metric_normalize.go`
- Create: `server/api/doc-benchmark/metric_match.go`
- Create: `server/api/doc-benchmark/metric_score.go`
- Test: `server/api/doc-benchmark/metric_normalize_test.go`
- Test: `server/api/doc-benchmark/metric_match_test.go`
- Test: `server/api/doc-benchmark/metric_score_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write failing normalization tables.** Cover NFKC/case-fold/Unicode whitespace and edge punctuation, maximal Unicode letter-number tokens, exact base-10 decimal/scientific equality without float64, unparsable text fallback, all fixed unit aliases, unknown-unit textual equality, absent versus empty versus value, sorted/deduplicated spans, and no unit conversion.
- [ ] **Step 2: Run `go test ./server/api/doc-benchmark -run TestNormalizeMetric -count=1`.** Expected: FAIL.
- [ ] **Step 3: Implement normalization with `shopspring/decimal`.** Preserve `absent`, `empty`, and non-empty states; exact decimal values/text fallback; the fixed checked-in alias table; token Jaccard 0 if either side is empty; and reason-coded invalid spans. Make the dependency direct through `go mod tidy`.
- [ ] **Step 4: Write failing edge/matching tests.** Assert eligibility only for intersecting source sets or two exact non-empty fields; exact 0.35/0.20/0.15/0.20/0.10 components; 0.60 threshold; a greedy counterexample; rectangular/forbidden graphs; and equal-optimum tie breaking by the lexicographically smallest ordered `(gold_id,prediction_input_index)` list, preserving canonical DB input index under permutations.
- [ ] **Step 5: Run `go test ./server/api/doc-benchmark -run TestMatchMetrics -count=1`.** Expected: FAIL.
- [ ] **Step 6: Implement deterministic optimal bipartite matching.** Build integer-scaled eligible edge weights, solve maximum total weight, then fix pairs in `(gold_id,prediction_input_index)` lexical order only when the remaining graph can still attain the same optimum. Never derive order from maps.
- [ ] **Step 7: Write failing detection/classification tests.** Cover both-empty, predictions-empty, gold-empty detection rules; accepted/unmatched rows; duplicate versus unsupported classification using best eligible edge to all gold including assigned gold; and zero-prediction rates.
- [ ] **Step 8: Write failing matched-metric tests.** Cover value/unit/value-unit, gold-specified stable fields, explicit/implicit labels, pooled grounding TP/FP/FN, zero-denominator nulls, absent-versus-empty assertions, invalid numeric text, unsupported spans, and deterministic diagnostics/component weights.
- [ ] **Step 9: Write upstream-attribution/version tests.** Wrong chunks still retain end-to-end extraction scores plus `upstream_invalid`; only conditional attribution aggregates exclude them. Serialize the exact aliases/weights/eligibility/threshold/tie rule into normalization/scorer hashes and assert any behavior change changes version/hash.
- [ ] **Step 10: Implement scoring per sections 10.2–10.4.** Emit directions, aggregation kinds, nullable values, additive components/numerators/denominators, accepted pairs, all unmatched classifications, field diffs, artifact/scorer hashes, and separate upstream validity.
- [ ] **Step 11: Run `go test ./server/api/doc-benchmark -run 'Test(NormalizeMetric|MetricEdges|MatchMetrics|ScoreMetrics|MetricVersions)' -count=1`.** Expected: PASS.
- [ ] **Step 12: Commit with `jj describe -m 'feat: score extracted benchmark metrics' && jj new`.**

### Task 5: Aggregation, paired comparison, and deterministic reports

**Files:**
- Create: `server/api/doc-benchmark/aggregate.go`
- Create: `server/api/doc-benchmark/report.go`
- Test: `server/api/doc-benchmark/aggregate_test.go`
- Test: `server/api/doc-benchmark/report_test.go`

- [ ] **Step 1: Write failing aggregation-kind tests.** Separately assert binary/rate macro mean/median/population SD, pooled TP/FP/FN micro with post-pool empty rules, matched-field correct/eligible micro with null zero denominator, raw violation/failure totals plus cases-with-any, and operational count/sum/mean/median/population SD. Every aggregate carries numerator, denominator, non-null units, applicable total, and aggregation kind.
- [ ] **Step 2: Write failing slice/upstream tests.** Apply identical formulas per canonical tag; exclude not-applicable/missing from denominators; retain end-to-end metric scores under bad chunks while excluding only conditional attribution; keep stable ordering.
- [ ] **Step 3: Write failing comparison tests.** Pair exact `(case_id,repetition)` sampling units and require equal dataset hash, processor case-set hash, and scorer/normalization versions. With `allow_upstream_variation=false`, reject different upstream chunk hashes; when true, label deltas end-to-end pipeline comparisons and never present conditional extraction scores as isolated evidence about the metric component. Target processor configurations may differ by design. Assert pooled-micro headline deltas, `paired_macro_diagnostic` distributions/coverage, and incomplete-pair diagnostics.
- [ ] **Step 4: Test `allow_incompatible`.** Default comparison rejects incompatible inputs; override produces prominent warnings, preserves provenance, and suppresses winner/win-tie-loss language.
- [ ] **Step 5: Write golden JSON/Markdown report tests.** Freeze clock/IDs/pricing and assert provenance (dataset/code/prompt/config/scorer/normalization), completion/failures before quality, vectors/slices/sample sizes, lowest cases/artifact links, paired deltas, latency and input/output/cache/hit/miss tokens, versioned pricing/cost, Pareto data, and explicit incomplete/incompatible/non-gating text.
- [ ] **Step 6: Run `go test ./server/api/doc-benchmark -run 'Test(Aggregate|Compare|Render)' -count=1`.** Expected: FAIL.
- [ ] **Step 7: Implement aggregation and comparison first.** Dispatch by declared aggregation kind, retain additive components and coverage, enforce compatibility identities, and compute only the spec-defined headline/diagnostic deltas.
- [ ] **Step 8: Implement deterministic reports.** Sort collections, use canonical JSON, escape Markdown, make versioned pricing an input, display quality only after completion coverage, and never calculate an overall winner/composite.
- [ ] **Step 9: Re-run focused tests and `go test ./server/api/doc-benchmark -count=1`.** Expected: PASS.
- [ ] **Step 10: Commit with `jj describe -m 'feat: aggregate and report benchmark scores' && jj new`.**

## Chunk 2: Persistence, safety, and production adapters

### Task 6: Goose schema and migration verification

**Files:**
- Create: `project_migrations/20260713000001_create_doc_benchmark_tables.sql`
- Create: `server/api/doc-benchmark/migration_test.go`

- [ ] **Step 1: Write a failing migration integration test.** When `TEST_DATABASE_URL` is set, migrate up, query exact catalog constraints/indexes, insert the minimal experiment→run→case→attempt→workspace/score/artifact graph, verify every uniqueness/FK/check/partial index below, migrate down, and assert all seven tables are gone. Skip with an explicit reason when no test DB is configured.
- [ ] **Step 2: Run `go test ./server/api/doc-benchmark -run TestBenchmarkMigration -count=1`.** Expected: FAIL because the migration is absent (or SKIP when no DB is configured).
- [ ] **Step 3: Create experiment/run/case tables.** Experiments carry UUID ID, name, dataset ID/version/hash, raw request TOML/hash, resolved experiment/file-hash/case-set-hash JSON, created/updated `TIMESTAMPTZ`, and unique request hash. Runs carry experiment FK, unique `(experiment_id,variant_name)`, lifecycle check, requested/resolved/config/prompt/scorer/pricing JSON+hashes, Git/jj/executable/dirty provenance, concurrency/usage/runtime aggregates, timestamps, and a trigger rejecting immutable payload changes after terminal lifecycle. Case runs carry run FK, unique `(run_id,case_id,repetition)`, applicability/tags/upstream hash, lifecycle, nullable selected-attempt ID, timestamps, and terminal-payload immutability; after attempts exist, enforce composite `(selected_attempt_id,id) REFERENCES benchmark_case_attempts(id,case_run_id)` so cross-case selection fails.
- [ ] **Step 4: Create attempt/workspace tables.** Attempts carry UUID ID, case-run FK, unique `(case_run_id,attempt_number)`, kind check (`execution|rescore`), nullable source-execution FK with a check requiring it only for rescore, immutable nullable `input_record_id_snapshot`, lifecycle/failure checks, lease/heartbeat/timing/telemetry/provider/model JSON, capture verification fields, and a trigger allowing only heartbeat/non-terminal progress before terminal append-only state. Workspaces carry unique execution-attempt FK, unique non-null `input_record_id REFERENCES kb.inputs(id) ON DELETE SET NULL`, canonical directory, allocation nonce, cleanup-state/error/timestamps, with kind/ownership enforced by store transaction.
- [ ] **Step 5: Create score/artifact tables and indexes.** Scores carry exactly one non-null owner (`attempt_id` XOR `run_id`), processor/scorer/version/metric/slice/direction/aggregation kind, nullable numeric value, additive component, numerator/denominator/non-null/applicable counts, and uniqueness per owner/metric/slice/component. Database triggers reject score update/delete once its attempt/run owner is terminal or selected. Artifacts carry exactly one owner, kind/path/SHA-256/size/verified/metadata and uniqueness per owner/kind/path; triggers reject updates/deletes after verification. Add partial unique selected-attempt index, experiment/run lookup, lifecycle/lease, case diagnostics, processor/metric comparison, workspace cleanup, and owner lookup indexes; integration tests directly reject cross-case selection and score update/delete.
- [ ] **Step 6: Add reverse Goose down.** Drop triggers/functions/indexes/tables in exact reverse dependency order using `IF EXISTS` without touching `kb.inputs`, production chunks, metrics, logs, or objects.
- [ ] **Step 7: Verify with a configured DB or inspect via the migration test.** Expected: up/down PASS; without DB, test SKIP and SQL remains covered later by startup/integration verification.
- [ ] **Step 8: Commit with `jj describe -m 'feat: add benchmark persistence schema' && jj new`.**

### Task 7: SQL store and transactional attempt claims

**Files:**
- Create: `server/api/doc-benchmark/store.go`
- Create: `server/api/doc-benchmark/store_runs.go`
- Create: `server/api/doc-benchmark/store_attempts.go`
- Create: `server/api/doc-benchmark/store_results.go`
- Test: `server/api/doc-benchmark/store_runs_test.go`
- Test: `server/api/doc-benchmark/store_attempts_test.go`
- Test: `server/api/doc-benchmark/store_results_test.go`
- Test: `server/api/doc-benchmark/store_concurrency_test.go`

- [ ] **Step 1: Write sqlmock tests for experiment/run/case creation and reads.** Require explicit columns, canonical JSON, UTC, enum validation, deterministic order, unique-content resume, authoritative runtime snapshot attachment, and rejection of completed run/case payload mutation.
- [ ] **Step 2: Implement shared store contracts and run/case SQL.** Use typed value structs, explicit statements/transactions, affected-row checks, and no dynamic identifiers; expose immutable snapshot/report reads separately from mutations.
- [ ] **Step 3: Write sqlmock tests for claim SQL.** Require `SELECT ... FOR UPDATE`, stale lease closure, verified-capture lookup, max-attempt enforcement, exact `execution`/`rescore` source/input snapshot fields, and selected terminalization in one transaction.
- [ ] **Step 4: Implement claim/attempt/workspace SQL.** Only heartbeats and declared progress fields may update a non-terminal attempt; kind/source/input/config snapshots never update; terminalization and selected-attempt choice are idempotent only for the same payload.
- [ ] **Step 5: Write result-store tests.** Cover nullable/additive scores, exactly-one-owner artifacts/scores, source-execution artifact following for rescore, verified immutability, telemetry, cleanup state transitions, and stable report query ordering.
- [ ] **Step 6: Implement immutable result reads/writes.** Batch insert scores/artifacts only before terminal selection, reject post-terminal mutation, and keep captured evidence/report records after workspace cleanup.
- [ ] **Step 7: Add real-PostgreSQL concurrency integration tests.** Under `TEST_DATABASE_URL`, start two transactions claiming the same logical case; assert exactly one attempt row/number is allocated, the loser observes claimed/non-stale state, stale closure plus retry is atomic, and uniqueness prevents duplicate case/run identity. Skip explicitly without DB.
- [ ] **Step 8: Run `go test ./server/api/doc-benchmark -run 'TestSQLStore|TestClaim|TestConcurrentClaim' -count=1`.** Expected: PASS for sqlmock and PASS/SKIP for DB concurrency.
- [ ] **Step 9: Commit with `jj describe -m 'feat: persist benchmark attempt lifecycle' && jj new`.**

### Task 8: Work/evidence allocation and cleanup guards

**Files:**
- Create: `server/api/doc-benchmark/workspace.go`
- Test: `server/api/doc-benchmark/workspace_test.go`

- [ ] **Step 1: Write failing filesystem safety tests.** Cover canonical distinct work/evidence roots, symlinked roots/components, path traversal IDs, wrong/missing marker, nonce mismatch, root replacement after allocation, partial capture cleanup, evidence immutability, and retry after `db_pending`/`files_pending`.
- [ ] **Step 2: Write capture durability failure-injection tests.** Interrupt after partial creation, copy, file sync, rename, final-file hash/size, directory sync, and before verified-state commit; no interrupted path may become verified/scoreable, and retry must either resume safely or replace only that attempt's partial file.
- [ ] **Step 3: Run `go test ./server/api/doc-benchmark -run 'Test(Allocate|Capture|Cleanup)' -count=1`.** Expected: FAIL.
- [ ] **Step 4: Implement allocation.** Canonicalize distinct roots, reject overlap, validate safe ID components, create directories without following symlinks, and persist an allocation marker with attempt ID/nonce/canonical root identity.
- [ ] **Step 5: Implement durable capture.** Copy to attempt-scoped `.partial`, sync it, rename within the evidence directory, hash/size the final named file, sync the directory where supported, then commit verified artifact/state; ordering is explicit and injectable for tests.
- [ ] **Step 6: Implement guarded cleanup.** Re-read persistent ownership and marker, revalidate roots/ancestors/nonce, delete only registered disposable paths, never verified evidence, and expose only the exact-attempt unverified-discard path.
- [ ] **Step 7: Re-run focused tests.** Expected: PASS on normal, adversarial, and every injected interruption.
- [ ] **Step 8: Commit with `jj describe -m 'feat: guard benchmark evidence and cleanup' && jj new`.**

### Task 9: Narrow production runtime seams

**Files:**
- Create: `server/api/doc-processing/runtime.go`
- Test: `server/api/doc-processing/runtime_test.go`
- Modify: `server/api/doc-processing/control.go`
- Test: `server/api/doc-processing/control_test.go`
- Modify: `server/cmd/doc-processor/main.go`

- [ ] **Step 1: Write a failing control-service test for `RunEvent(ctx, payload) error`.** Assert it invokes the same parse/preflight/run/processor path as `HandleEvent` and returns the internal error synchronously.
- [ ] **Step 2: Add only the delegating exported method and rerun the focused test.** Expected: PASS.
- [ ] **Step 3: Write failing runtime-builder tests.** With environment/model fixtures, request `chunking` plus `extract_metrics`; assert one production `ControlService`, exact dependency closure, unchanged processor selection, exact adapter allowlists, and authoritative `ResolvedConfig()` containing all production defaults, chunk settings, prompt paths/content hashes, model reference/definition hashes/parameters/provider/model, concurrency/seed support, and no API keys/tokens.
- [ ] **Step 4: Move the processor graph construction from `server/cmd/doc-processor/main.go` to `NewProductionRuntime`.** Preserve existing defaults, LLM clients, stores, dependency ordering, logging, and filtering; keep command-only usage-sink/bootstrap code in `main.go`.
- [ ] **Step 5: Make `doc-processor` use the builder and run `go test ./server/cmd/doc-processor ./server/api/doc-processing -run 'Test(RunEvent|ProductionRuntime)' -count=1`.** Expected: focused tests PASS.
- [ ] **Step 6: Run `go test ./server/cmd/config -count=1` and `go build ./server/cmd/doc-processor`.** Expected: PASS. Record the known unrelated summary-ID test failures if the full doc-processing suite is run; do not change them in this task.
- [ ] **Step 7: Commit with `jj describe -m 'refactor: share production doc processor runtime' && jj new`.**

### Task 10: Production chunking and metric adapters

**Files:**
- Create: `server/api/doc-benchmark/adapter.go`
- Create: `server/api/doc-benchmark/chunk_adapter.go`
- Create: `server/api/doc-benchmark/metric_adapter.go`
- Test: `server/api/doc-benchmark/chunk_adapter_test.go`
- Test: `server/api/doc-benchmark/metric_adapter_test.go`

- [ ] **Step 1: Define the adapter contract in tests.** Require processor name, allowed overrides, fully initialized `ResolvedConfig`, applicability/gold load, seed, capture, `Reconcile`, score, upstream attribution, and explicit DB/file cleanup.
- [ ] **Step 2: Write chunk-adapter sqlmock/tempdir tests.** Require exactly one row from the exact section-11 query ending `ORDER BY id ASC`; parse ordered arrays by chunk sequence; independently parse `.chunks`; reconcile both representations; capture both plus deterministic diff; missing/disagreement returns `invalid_output`; verify hashes before success; delete only attempt-input rows.
- [ ] **Step 3: Write metric-adapter tests.** Require the exact `SELECT to_jsonb(m)` query ordered by `metric_id COLLATE "C" ASC NULLS LAST, id ASC`; preserve that as prediction input index; reconcile `.metrics` against the seven stable core fields; capture other fields, both representations, deterministic diff, telemetry/logs, and exact upstream chunk artifact/hash/config; missing/disagreement is `invalid_output`.
- [ ] **Step 4: Write `kb.inputs` seeding tests.** Require generated synthetic tenant/store identifiers, parser/staging/file/status values compatible with `DocMetadataSQLStore`, `RETURNING id`, persistent workspace ownership immediately after allocation, and rollback/cleanup on partial failure.
- [ ] **Step 5: Run `go test ./server/api/doc-benchmark -run 'Test(ChunkAdapter|MetricAdapter|SeedInput)' -count=1`.** Expected: FAIL.
- [ ] **Step 6: Implement the adapters using existing production schemas/parsers.** Keep the two exact SQL constants directly testable, do not call scorer substitutes for execution, do not reuse unordered metric store reads, and never silently prefer DB or file output during reconciliation.
- [ ] **Step 7: Re-run adapter tests.** Expected: PASS with byte-stable canonical artifacts.
- [ ] **Step 8: Commit with `jj describe -m 'feat: adapt production processors for benchmarks' && jj new`.**

## Chunk 3: Execution, CLI, fixtures, and operations

### Task 11: Attempt runner, retries, rescore, and cancellation

**Files:**
- Create: `server/api/doc-benchmark/runner.go`
- Test: `server/api/doc-benchmark/runner_test.go`

- [ ] **Step 1: Write execution-path tests.** Cover processor success, processor failure with capturable output, failure before capture, timeout, caller cancellation, heartbeat failure, and not-applicable processors; assert processor success is independent of scorer/capture success and every terminal state has a reason-coded class.
- [ ] **Step 2: Implement execution state transitions.** Use timeout context, heartbeat at least once/minute and no slower than one-third lease, immutable input snapshot, one terminalization path, and no work for filtered/non-applicable processors.
- [ ] **Step 3: Write verified-capture/rescore tests.** Scorer/post-capture infrastructure/stale-rescore failures must reverify source hashes and append a `rescore` referencing the execution attempt, creating no `kb.inputs`, workspace, runtime, or LLM call; tampered/unverified capture must route to fresh execution.
- [ ] **Step 4: Implement retry routing and budget.** Count execution and rescore together, never exceed max, leave non-stale running attempts untouched, do not auto-retry cancellation, and select the latest terminal result when exhausted.
- [ ] **Step 5: Write cleanup-order tests.** Verified artifact/state precedes scoring; terminal DB commit precedes disposable cleanup; evidence stays immutable; cleanup failures persist exact `db_pending`/`files_pending` state and are retryable.
- [ ] **Step 6: Implement cleanup sequencing and run `go test -race ./server/api/doc-benchmark -run TestRunner -count=1`.** Expected: PASS, including zero runtime/LLM calls for rescore.
- [ ] **Step 7: Commit with `jj describe -m 'feat: execute resumable benchmark attempts' && jj new`.**

### Task 12: Worker protocol and isolated orchestrator

**Files:**
- Create: `server/api/doc-benchmark/worker.go`
- Create: `server/api/doc-benchmark/orchestrator.go`
- Test: `server/api/doc-benchmark/worker_test.go`
- Test: `server/api/doc-benchmark/orchestrator_test.go`

- [ ] **Step 1: Write JSON-line protocol tests.** Freeze schema version and require `initialize` request, resolved redacted snapshot/config hash/executable hash in `ready`, run authorization, heartbeat/result messages, and structured failure; reject unknown fields, mismatched hashes, and extra output on stdout.
- [ ] **Step 2: Write orchestrator permit tests with a fake launcher.** Assert lexical variant order, peak initialized/executing variants never exceeds `max_parallel_variants`, peak in-worker case executions never exceeds `max_parallel_cases`, initialization permit releases after ready, execution permits cannot deadlock waiting workers, and cancellation terminates children.
- [ ] **Step 3: Write provenance/resume tests.** Capture Git commit, jj change/commit IDs when available, dirty flag, executable SHA-256, concurrency, and resolved snapshot. Default run/comparison rejects dirty/changed provenance; `--allow-dirty` permits creation but persists `reproducible=false` and a prominent warning. Expire only stale leases, preserve snapshots, and aggregate only after workers finish.
- [ ] **Step 4: Run `go test ./server/api/doc-benchmark -run 'Test(WorkerProtocol|Orchestrator|Resume)' -count=1`.** Expected: FAIL.
- [ ] **Step 5: Implement worker initialization and bounded case scheduler.** Build one `ProductionRuntime` per variant, validate secrets from environment, calculate authoritative config/code/jj/Git/dirty/executable hashes, send ready, wait for authorization, then reuse the closure while admitting no more than `max_parallel_cases` cases.
- [ ] **Step 6: Implement the `os/exec` launcher and scheduler.** Pass configuration through stdin JSON (not command-line secrets), reserve stdout for protocol and stderr for logs, use bounded semaphores, terminate children on cancellation, and persist every crash/failure through the store.
- [ ] **Step 7: Re-run focused tests and `go test -race ./server/api/doc-benchmark -count=1`.** Expected: PASS.
- [ ] **Step 8: Commit with `jj describe -m 'feat: orchestrate isolated benchmark workers' && jj new`.**

### Task 13: CLI commands and exit behavior

**Files:**
- Create: `server/cmd/doc-benchmark/main.go`
- Create: `server/cmd/doc-benchmark/main_test.go`

- [ ] **Step 1: Write command tests for `validate`, `run`, `compare`, `report`, `clean`, and hidden `worker`.** Inject bootstrap/orchestrator functions; assert required flags/mutual exclusions and exact exits: `0` success, `2` usage/validation/incompatibility, `3` harness/infrastructure, `4` completed command with incomplete benchmark cases, and `130` cancellation. Errors are one stderr JSON object `{"error":{"code":"<stable_code>","message":"<escaped message>"}}`; successful machine output is stdout only.
- [ ] **Step 2: Assert safety/provenance flags.** `clean --discard-unverified` requires one attempt ID; normal clean never deletes evidence; compare/report require stored IDs; compare rejects incompatible unless `--allow-incompatible`; run rejects dirty unless `--allow-dirty`, which marks results non-reproducible.
- [ ] **Step 3: Run `go test ./server/cmd/doc-benchmark -count=1`.** Expected: FAIL because command is absent.
- [ ] **Step 4: Implement the thin command.** Reuse `config.LoadConfig`, DB initialization, `config.RunMigrations`, logging, and graceful signal handling conventions; keep business logic in `docbenchmark`; expose only the exact v1 commands/options from the spec.
- [ ] **Step 5: Run `go test ./server/cmd/doc-benchmark -count=1 && go build ./server/cmd/doc-benchmark`.** Expected: PASS.
- [ ] **Step 6: Commit with `jj describe -m 'feat: add doc benchmark CLI' && jj new`.**

### Task 14: Synthetic v1 fixture corpus and end-to-end verification

**Files:**
- Create: `benchmark/doc-processors/generator/main.go`
- Create: `benchmark/doc-processors/generator/cases.go`
- Create: `benchmark/doc-processors/generator/main_test.go`
- Create: `benchmark/doc-processors/datasets/doc-processors-synthetic-core/1.0.0/manifest.json`
- Create: `benchmark/doc-processors/datasets/doc-processors-synthetic-core/1.0.0/cases/**`
- Create: `benchmark/doc-processors/experiments/example.toml`
- Test: `server/api/doc-benchmark/integration_test.go`
- Test: `server/api/doc-benchmark/failure_integration_test.go`
- Test: `server/api/doc-benchmark/isolation_integration_test.go`

- [ ] **Step 1: Write generator tests before generator code.** Structured parameterized cases must emit both canonical `.lines.txt` and `expected.json`; two runs with seed `20260713` are byte-identical; generation into a temp directory exactly matches committed files with no diff.
- [ ] **Step 2: Implement the generator/templates.** Cover every chunk rule and core metric behavior with positive, near-miss, and interaction cases/tags: TOC, boundaries/final-small, long protected lists, overlap, reorder, no/negative/duplicate/multiple/implicit/multilingual metrics, Unicode, decimals, percentages, and units. Text is synthetic and uncopyrighted.
- [ ] **Step 3: Commit versioned generated output.** Emit `manifest.json`, canonical lines, and expected JSON under dataset ID/version; gold processor sections exactly equal applicability; include ordered chunks/protected policies and metric IDs/fields/evidence spans.
- [ ] **Step 4: Write corpus validation/regeneration tests.** Assert fixed ID/version/hash, raw file hashes, case-set hashes/counts, tag/rule coverage, and byte-for-byte regeneration. Any change requires a new SemVer dataset directory.
- [ ] **Step 5: Make `example.toml` an acceptance matrix.** It selects the one dataset and defines at least two chunk configurations and at least two metric prompt/model/config configurations over identical filtered cases/repetitions, using real checked-in prompt/model references.
- [ ] **Step 6: Write happy-path DB integration.** With `TEST_DATABASE_URL`, exercise validate→two-plus variants→run/case/attempt→canonical reconciliation→score→compare/report→cleanup→resume; assert equal case-set hashes, deterministic identical chunk hashes for repeated identical config, no duplicate attempts/claims, unrelated `kb.chunks`/`kb.metrics` rows excluded, and snapshots/logs/reports contain no seeded secret.
- [ ] **Step 7: Write isolation/failure integration.** Assert per-process prompt/config isolation, `max_parallel_cases`, timeout, crash, partial capture interruption, cancellation, stale execution retry, stale verified rescore with zero new runtime/LLM calls, concurrent duplicate-claim prevention, adversarial symlink/root/marker cleanup rejection, and exact terminal/failure/cleanup states.
- [ ] **Step 8: Add opt-in live integration.** `BENCHMARK_LIVE_INTEGRATION=1` initializes the real production runtime and calls `ControlService.RunEvent` for one case; otherwise skip with prerequisites.
- [ ] **Step 9: Run generator and integration tests.** `go test ./benchmark/doc-processors/generator -count=1 && go test ./server/api/doc-benchmark -run 'Test(SyntheticV1|BenchmarkEndToEnd|BenchmarkFailures|BenchmarkIsolation)' -count=1`; expected generator/pure tests PASS and DB/live portions PASS or explicit prerequisite SKIP.
- [ ] **Step 10: Run the exact validator contract.** `go run ./server/cmd/doc-benchmark validate --experiment benchmark/doc-processors/experiments/example.toml`; expected exit 0 and one canonical JSON object with exact keys `dataset_id`, `dataset_version`, `dataset_hash`, `request_hash`, and sorted `processor_case_set_hashes`, whose values match the corpus golden test.
- [ ] **Step 11: Commit with `jj describe -m 'test: add synthetic doc benchmark corpus' && jj new`.**

### Task 15: Operator docs, capsule update, and final verification

**Files:**
- Create: `docs/doc-processor-benchmark-operations.md`
- Create: `docs/database/doc-processor-benchmark-schema.md`
- Modify: `docs/superpowers/specs/2026-07-13-doc-processor-benchmark-design.md` only if implementation clarifications are required
- Modify in separate repository: `/Users/cding/Workspace/KnowledgeStore/Capsules/coding-capsules/doc-processor/+CAPSULE.md`

- [ ] **Step 1: Write the operator guide.** Document prerequisites/environment, dataset/experiment layout, all CLI examples, lifecycle/failure meanings, resume/retry/rescore, evidence/work roots, cleanup recovery, SQL tables, reproducibility hashes, live integration opt-in, and troubleshooting.
- [ ] **Step 2: Write the schema/retention guide.** Document all seven tables, ownership/immutability/lease constraints, canonical queries, evidence versus disposable retention, safe cleanup states, and migration rollback boundaries.
- [ ] **Step 3: Include a literal `Out of scope for v1` section.** List MLflow, LangSmith, Promptfoo, web UI/public API, CI/release gating, PDF parsing/OCR, human-annotated production documents, LLM-as-judge/open semantic grading/arbitrary unit conversion, combined pipeline-wide score, automatic prompt optimization, automatic retention/purge of verified evidence, processors beyond `chunking`/`extract_metrics`, and multi-host distributed scheduling.
- [ ] **Step 4: Update the capsule only after `jj status` in KnowledgeStore is clean.** Link the ChenWeb spec/operator/schema guide/CLI and production runtime seam. If dirty due to user work, leave it untouched and report rather than overwrite.
- [ ] **Step 5: Format and run acceptance checks.** `files=$(find server/api/doc-benchmark server/cmd/doc-benchmark benchmark/doc-processors/generator -name '*.go' -type f); gofmt -w $files server/api/doc-processing/runtime.go server/api/doc-processing/runtime_test.go server/api/doc-processing/control.go server/api/doc-processing/control_test.go server/api/doc-processing/fix-size-chunking.go server/api/doc-processing/fix-size-chunking_test.go server/cmd/doc-processor/main.go`; then `go vet ./server/api/doc-benchmark ./server/cmd/doc-benchmark ./server/cmd/doc-processor ./benchmark/doc-processors/generator` and `go test -race ./server/api/doc-benchmark ./server/cmd/doc-benchmark ./server/cmd/config ./benchmark/doc-processors/generator -count=1`. Expected PASS (DB/live tests may explicitly SKIP prerequisites).
- [ ] **Step 6: Run build/migration checks.** `go build ./server/cmd/doc-benchmark ./server/cmd/doc-processor`; with `TEST_DATABASE_URL`, run `go test ./server/api/doc-benchmark -run 'Test(BenchmarkMigration|ConcurrentClaim|BenchmarkEndToEnd|BenchmarkFailures|BenchmarkIsolation)' -count=1`. Expected PASS or explicit prerequisite SKIP only.
- [ ] **Step 7: Run the full production suite separately as baseline evidence.** `go test ./server/api/doc-processing -count=1`; benchmark-focused tests must pass, while the already observed unrelated summary-ID failures (`93_1_0012` expected versus `93_sum_1_0012` actual) may remain and are reported as pre-existing—not an acceptance failure for this benchmark.
- [ ] **Step 8: Review documentation impact.** Record changed knowledge, affected specs/tests, updated/stale docs, and intentionally undocumented details; ensure no v1 external-eval integration is implied.
- [ ] **Step 9: Run final independent code review before final commits.** Use requesting-code-review, address findings test-first, and repeat Steps 5–7.
- [ ] **Step 10: Commit ChenWeb fixes/docs with `jj describe -m 'docs: document doc processor benchmark operations'`.** Inspect `jj diff --stat` and recent `jj log`; never rewrite unrelated work.
- [ ] **Step 11: Commit the capsule separately in KnowledgeStore with `jj describe -m 'docs: link doc processor benchmark'`.** Do not combine repositories.
