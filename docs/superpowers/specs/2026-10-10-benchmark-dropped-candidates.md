# Dropped benchmark candidates

The Benchmark page's dropped view reads the candidates discarded by
`extract-metrics-benchmark` for its selected gold run. Production drops continue
to be shown in the production Metrics page.

## Storage and saving

Migration `20261010000004_keep_only_run_id_in_testbed_metrics.sql` makes
`testbed.metrics.run_id` the single stored run ID column and removes the duplicate
`benchmark_run_id`. Renaming the original stored column preserves values and its
constraints. The helper writes `run_id` from the unchanged payload field
`benchmark_run_id`; the run audit table and scoring API/artifacts retain that name.
Scoring readers join the differently named columns using the full run identity.

Project Goose migration `20261010000003_create_testbed_metrics_dropped.sql` creates
`testbed.metrics_dropped` and backfills excluded and duplicate candidates from
`testbed.metric_benchmark_runs.candidates`. Each row retains the complete candidate
JSON, candidate ID, decision and rule ID (or legacy decision) as its drop reason.
The foreign key and uniqueness constraint include skill name, version, model and
`run_id`, which equals the gold metrics' run ID. A timestamp alone is not unique
across models or versions.

The skill helper saves the audit, gold metrics and drops in one transaction. The
run table prevents retries from appending to any saved run, including zero-gold
runs. Zero gold rows still persist the audit and any discarded candidates.

## API and page

`GET /api/v1/kb/metrics?source=testbed&include_dropped=true` applies the existing
version/model filters, resolves the latest run once, and queries both tables using
that exact composite identity. Rows and the response expose `run_id`; benchmark
rows also expose skill provenance. Empty gold runs can return their drops. No
matching run returns an empty response; failures do not fall back to production
drops. Existing document/version/model options still require gold metric rows.

The page requests both sets together. Its view toggle filters the loaded rows and
keeps the run fixed until the document or benchmark selection reloads. Counts and
empty states follow the visible set. Dropped candidates use the same source-line
and table-reference rendering as gold metrics. Older ledgers may have only an
excerpt and rationale; those are shown without inventing enriched metric fields.

## Knowledge and verification

The skill procedure, storage contract and changelog document persistence and
read-back of dropped candidates. PostgreSQL tests cover migration rollback/backfill,
atomic saves, duplicate-save rejection, zero-gold runs and identical timestamps
across models. API tests verify full-key selection and candidate rendering; client
tests verify that requesting drops keeps the benchmark source/version/model.
Frontend checks enforce English/Chinese labels and selectable text.

Verified: the benchmark helper's 37 tests (including disposable PostgreSQL), the
kbhandler Go suite, five focused client tests, and a server build pass. `bun run
check` passes with two existing CSS warnings in unrelated pages. The production
frontend build uses `NODE_OPTIONS=--max-old-space-size=16384` because its default
heap is too small for the existing app. `web/scripts/test-benchmark-dropped-page.py`
passes in English and Chinese with mocked APIs, checking source/version/model,
the cached view toggle, displayed run ID, and text selection.

The live project migration backfilled 1,975 candidates across 14 runs, with no
missing or mismatched run references. Document 753's rules 6.0.0 snapshots retain
232 drops for `claude-opus-5-5` / `20261010_055329` and 229 for `gpt-6-luna` /
`20261009_064809`; they remain separate.
Executing the API's actual SQL against the project database selects 115 gold rows
and 232 drops for the Claude run. Every backfilled candidate exactly matches its
original run audit entry.

Annotation rules, production extraction behavior and scoring contracts are unchanged.
Released rules remain frozen. No new ADR is required for storing already-reviewed
candidate decisions.
