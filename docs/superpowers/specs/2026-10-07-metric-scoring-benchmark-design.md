# Metric scoring benchmark

Approved by the user on 2026-10-07. Add Development → System Admin → LLM → Metrics → Benchmark.

## Behavior
Select a document and configured LLM model; optionally select a specific gold version/model/run. Score current kb.metrics against testbed.metrics. Default gold selection and compatible-version requirements come from score-extract-metrics/scripts/score_io.py. The LLM matches the same assertions one-to-one, accounts for all rows, records false-positive causes and justified notation overrides, and never calculates the score. Python validates the matching and computes the existing weighted score. Missing gold or predictions fail explicitly. Runs do not alter extracted or gold metrics.

## Architecture
Use kbhandler background-run and configured OpenAI JSON client patterns. Persist running/done/failed jobs and immutable input, matches, score and report snapshots in kb.metric_score_runs through a project goose migration. Limit duplicate active jobs per document and time out interrupted work. Use the shared project database pool. The original Python scorer remains authoritative, configurable via METRIC_BENCHMARK_SCORER_PATH, with workspace sibling skill as the default. Requires python3, psql and the scorer's sibling extract-metrics-benchmark helper. Evidence lives under DATA_HOME_DIR/metric-score-runs/<run-id>/evidence; frozen scorer/helper scripts and the matching prompt are stored beside it; downloads use stored snapshots rather than arbitrary filesystem paths.

## Page and history
English/Chinese Paraglide page with selectable text, document search, model selection, optional explicit gold run, run action, status polling, paginated history (all documents or selected document), detailed score/precision/recall, field checks, missed/false-positive rows, warnings and evidence downloads. Keep extraction and gold provenance visible; compare only results against the same gold run. Store locale and ask the matcher for notes in that locale; Python report remains its canonical English artifact.

## Validation and documentation
Tests cover invalid requests, administrator access, scorer invocation/failure handling, snapshot matching validation and deterministic score parity. Run targeted Go tests, server build, scorer unit tests, frontend check and frontend build. Record deployment prerequisites and API contract in this document. Existing doc-processor Benchmark Setup and production extraction specifications retain their existing meaning.

## Implemented API and operations
All endpoints below are `/api/v1/kb/metric-scores`, protected by the existing authentication middleware plus an administrator/owner/root check:

- `GET /models`: configured LLM model profiles.
- `GET /gold-runs?record_id=N`: available version/model/run combinations for a document. Explicit selections use the full composite identity; the scorer chooses the compatible default when omitted.
- `POST /`: `{record_id, model, lang, gold_version?, gold_model?, gold_run_id?}`. Explicit gold fields must all be supplied. Returns HTTP 202 with the new or already-running job. Existing metrics/gold remain untouched.
- `GET /?record_id=N&limit=20&offset=0`: paginated history; record filter optional, maximum page size 100.
- `GET /:id`: job status and full immutable evidence.
- `GET /:id/artifacts/:kind`: downloads only `input`, `matches`, `score`, `report`, from database snapshots.

A project migration creates `kb.metric_score_runs` and bilingual development page-config visibility entries. Startup already runs project goose migrations. A partial unique index prevents multiple active jobs for a document. Workers have a ten-minute deadline; polling marks running rows older than twelve minutes interrupted. Scorer subprocesses use the pool's configured PostgreSQL credentials through environment variables, never command-line arguments; inherited libpq address/service overrides are stripped. Export failures map to explicit NO_GOLD/NO_PREDICTIONS codes. Invalid model matches fail instead of producing an estimated score.

Requires Python 3, psql, DATA_HOME_DIR, and the installed `score-extract-metrics/scripts/score_io.py` plus sibling `extract-metrics-benchmark/scripts/benchmark_io.py`. The default searches workspace ancestors for `.agents/skills`; deployments can set METRIC_BENCHMARK_SCORER_PATH. Each job freezes both scripts and the prompt, records SHA-256 digests, exact scoring model ID, source bytes and locale. Input is conservatively capped at 600,000 bytes; oversized input fails without truncation. The canonical Python report remains English; matching explanations follow the selected UI language. History records jobs launched through this page; legacy standalone CLI score folders are not automatically imported.

## Verification completed
- `go test ./server/api/kbhandler -count=1`: passed, including administrator middleware, no-gold/no-predictions, timeout, artifact response, pool settings, exact scorer arithmetic/validation, and export → local fixture LLM → authoritative scorer integration.
- `go build -buildvcs=false ./server/...` and `go vet ./server/api/kbhandler`: passed.
- Existing score skill unit suite: 14 tests passed.
- `bun test src/lib/components/home3/metric-score-client.test.ts`: two tests passed.
- `bun run check`: no errors, bilingual i18n checks passed; two pre-existing CSS warnings in unrelated pages.
- `NODE_OPTIONS=--max-old-space-size=16384 bun run build`: passed. Default and 8 GB heaps ran out of memory while bundling the existing application.
- `python3 web/scripts/test-metric-score-page.py`: browser smoke with mocked API responses passed in English and Chinese, including delayed responses longer than the polling interval. Requires the web server on localhost:5173; `--base-url` overrides it. This test does not call a paid LLM or write benchmark history.
- The applied migration's Down and Up were tested within a rolled-back PostgreSQL transaction, leaving existing data and migration tracking intact.
- Independent implementation review found no remaining blocking defects.

## Knowledge impact
The new scoring job lifecycle, API, deployment prerequisites, reproducibility evidence and page behavior are documented here; the implementation plan tracks completion. Tests capture the scorer contract and navigation workflow. No existing production extraction/gold rules changed, so their specifications and ADRs remain authoritative. The general doc-processor Benchmark Setup documents describe a separate workflow and remain unchanged. No new ADR or end-user manual was added for this administrative page.
