## Why

`extract_provisions` has no answer key. Metrics, references and summaries can each be measured
against an independently reviewed gold set (System Admin › Benchmark › Metrics, References,
Summaries), but provision extraction cannot. Without a gold set, a prompt or model change to the
provisions extractor cannot be judged better or worse.

## What Changes

- New skill `.agents/skills/extract-provisions-benchmark`. It writes gold provisions for one
  `kb.inputs` record under frozen, versioned annotation rules (`rules/1.0.0.md`) and saves an
  immutable run to new `testbed` tables. It works the same way as `extract-metrics-benchmark` and
  `extract-references-benchmark`. Rules 1.0.0 contain the user's three rulings:
  - Imperative test-method steps are provisions (kind `instruction`, type `mandatory`).
  - One provision per normative statement: a sentence, list item or table cell. A sentence is
    split only where its provision type or grammatical subject changes.
  - The first gold run is record 416 (DB33/T 2030—2018).
- New project migration for the gold tables: `testbed.provision_benchmark_runs`,
  `testbed.provisions` and `testbed.provisions_dropped`.
- New skill `.agents/skills/score-extract-provisions`. It scores the record's `kb.provisions`
  rows against one gold run, from 1 to 100, using soft F1 over field checks. The agent or LLM
  pairs the rows and the script does the arithmetic.
- New project migration `kb.provision_score_runs` (score-run history) plus `kb.page_config`
  access rows (admin/root) for the two new menu entries.
- Backend:
  - `GET /kb/provisions/gold-options`.
  - `GET /kb/provisions?source=testbed&skill_version&model_name&run_id` returns the gold and
    dropped provisions of one run.
  - `gold_kind=provisions` in the kb inputs list.
  - `/kb/provision-scores` (models, gold-runs, list, start, get, artifacts) runs the scorer
    with LLM matching through the shared score runner (`scoreKind`).
  - Matching prompt `prompts/prompt-score-extract-provisions-v1.md`.
- Frontend (development page only): System Admin › Benchmark › **Provisions** gets two entries.
  - **Gold Provisions**: the existing Provisions view, read-only, over a chosen gold run, with
    gold and dropped lists.
  - **Benchmark**: a scoring page built on `BenchmarkReportViewer`, like Metrics › Benchmark.
  - All text goes through Paraglide (en + zh-cn).
- First gold run: record 416 under rules 1.0.0, saved and read back.

## Capabilities

### New Capabilities
- `provision-gold-benchmark`: annotating, validating and storing gold provisions per versioned rules (skill + testbed tables).
- `provision-benchmark-scoring`: scoring `kb.provisions` against a gold run (scorer skill, score-run API and table, Benchmark page, Gold Provisions page).

### Modified Capabilities
<!-- none: no existing openspec/specs capability covers benchmarks -->

## Impact

- `.agents` repo: two new skills (Python, tests).
- ChenWeb:
  - `project_migrations/` (two migrations).
  - `server/api/kbhandler/`: new `gold_provisions.go` and `provision_score_handler.go`; small
    edits to `gold_metrics_options.go` and `provision_handlers.go`.
  - `server/api/routes.go`.
  - `prompts/`.
  - `web/src/lib/components/home3/`: nav-rail, content-panel, provision-mgmt-view, new
    provision-score-view and client.
  - `web/src/lib/services/kbService.ts`.
  - `web/messages/*.json`.
- KnowledgeStore: devdoc for the benchmark, and a pointer from `extract-provisions-spec.md`.
- No change to `extract_provisions` itself or to `kb.provisions`.
