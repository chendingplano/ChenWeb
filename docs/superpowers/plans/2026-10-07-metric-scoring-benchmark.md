# Metric Scoring Benchmark Implementation Plan

> For agentic workers: Use subagent-driven-development for frontend implementation and independent review; root handles server integration.

**Goal:** Run the approved metric scorer from ChenWeb and view persistent benchmark history.

**Architecture:** Existing Python score_io.py provides authoritative snapshots, validation and arithmetic. Go owns background jobs, database history and the configured LLM matching call. Svelte owns run controls, polling and result/history display.

**Tech Stack:** Go/Echo/PostgreSQL/goose, Python, Svelte 5/TypeScript/Paraglide.

## Chunk 1: Implementation
- [x] Add project migration for kb.metric_score_runs with immutable snapshots, status, provenance and one running job per document.
- [x] Add server/api/kbhandler/metric_score_handler.go and metric_score_runner.go: administrator gate, models/documents/gold choices, start/list/detail/download endpoints; safe bounded subprocess execution; validated LLM matches and scorer invocation.
- [x] Save matching prompt in prompts/prompt-score-extract-metrics-v1.md; notes follow requested language.
- [x] Add tests for request validation, artifact whitelist, subprocess failures and scoring parity before completing implementation.
- [x] Add web/src/lib/components/home3/metric-score-view.svelte and metric-score-client.ts; update nav-rail.svelte, content-panel.svelte, both messages files.
- [x] Add migration for development page_config visibility if needed.

## Chunk 2: Verification
- [x] Run go test ./server/api/kbhandler and scorer unittest; fix failures. Add and run the export → fixture LLM → real scorer integration and saved-artifact response tests.
- [x] Run go build ./server/... and bun run check/build in web. Production build passed with a 16 GB Node heap. Run web/scripts/test-metric-score-page.py against the dev server in both locales.
- [x] Verify startup applied the migration; validate its Down/Up inside a rolled-back transaction with environment-based credentials, without changing migration tracking or existing rows.
- [x] Independently review implementation and fix findings.
- [x] Update design with final API, limitations and validation results. Commit using jj and confirm linear jj log.
