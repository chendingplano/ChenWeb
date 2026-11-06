## 1. Gold tables

- [x] 1.1 Project migration `create_testbed_provisions`: `testbed.provision_benchmark_runs`, `testbed.provisions`, `testbed.provisions_dropped` (checks for type/kind/location vocabularies, run-key FK, indexes, down migration)
- [x] 1.2 Apply it to the dev database and confirm the columns

## 2. Gold skill `extract-provisions-benchmark` (.agents)

- [x] 2.1 `rules/1.0.0.md` (unit, kinds/types, explicitness, location, subject, text; A/X/D rules; audits) + `rules/versions.json`
- [x] 2.2 `scripts/benchmark_io.py` prepare / validate / save (N / N-M spans, kind→type map, table locations, exact text parts, coverage, ledger, one-transaction save with run-key guard)
- [x] 2.3 `scripts/test_benchmark_io.py` (validation refusals, save SQL shape)
- [x] 2.4 `SKILL.md`, `references/benchmark-contract.md`, `USER_MANUAL.md`, `CHANGELOG.md`

## 3. Scorer skill `score-extract-provisions` (.agents)

- [x] 3.1 `scripts/score_io.py` export / score (gold run selection, predictions from kb.provisions, suggestions, match checks incl. `split`, weighted field checks, groups, report.md)
- [x] 3.2 `weights.json` + `scripts/test_score_io.py`
- [x] 3.3 `SKILL.md`

## 4. Backend (ChenWeb)

- [x] 4.1 Project migration `create_provision_score_runs` + page_config rows (admin/root) for `sysadmin-llm-provisions-gold` / `-benchmark`
- [x] 4.2 `goldRunTables["provisions"]`, `ListGoldProvisionOptions`, `GET /kb/provisions/gold-options`
- [x] 4.3 `listGoldProvisions` for `GET /kb/provisions?source=testbed…` (gold + dropped) + test
- [x] 4.4 `prompts/prompt-score-extract-provisions-v1.md`
- [x] 4.5 `provision_score_handler.go` (`provisionScoreKind`, gold-runs, start, list, get, artifacts), routes, runner repair loop and validation allowlist for provision messages + test

## 5. Frontend (ChenWeb)

- [x] 5.1 kbService: `GoldKind` adds `provisions`; `listKbProvisions` gold params; gold provision/dropped types
- [x] 5.2 Nav: Provisions folder (Gold Provisions, Benchmark) on the development page; content-panel routes
- [x] 5.3 `ProvisionMgmtView` `source="testbed"`: GoldRunFilter, Gold/Dropped switch, read-only
- [x] 5.4 `provision-score-client.ts` + `provision-score-view.svelte` on `BenchmarkReportViewer`
- [x] 5.5 Messages en + zh-cn; `bun run check`; `svelte-check`; text selectable

## 6. First gold run and verification

- [x] 6.1 Gold run of record 416 under rules 1.0.0 (prepare → annotate → audit → validate → save → read back)
  — run `20261106_143017` (claude-opus-5-5): 84 provisions, 35 dropped; read-back matched every field
- [x] 6.2 Gold Provisions page shows the 416 run (browser check)
  — handler SQL run against the dev DB (84 rows, 35 dropped, options, inputs filter); page checked with
  mocked APIs in en + zh-cn (`web/scripts/test-provision-benchmark-pages.py`); a logged-in live check was not done
- [x] 6.3 Benchmark page: score run on a record with gold and predictions (ask the user before running extract_provisions on 416), or the `NO_PREDICTIONS` path verified
  — `score_io.py export --record-id 416` exits 4 `NO_PREDICTIONS` (416 has no kb.provisions rows); page flow
  checked with mocked APIs; a live LLM score waits for an extract_provisions run on 416
- [x] 6.4 Go tests, Python tests, `bun run check`
  — new Go tests pass; 4 kbhandler tests (TestMetricScoreRunnerEndToEnd, TestMetricScoreScorerParityAndInvalidMatches,
  TestPlanInputDeletePaths×2) also fail without this change; Python 18 + 13 pass; `bun run check` clean

## 7. Docs and commits

- [x] 7.1 Devdoc for the provisions benchmark (KnowledgeStore) + pointer in `extract-provisions-spec.md`
- [x] 7.2 Commit `.agents`, ChenWeb, KnowledgeStore separately with explicit paths; `jj log` check
