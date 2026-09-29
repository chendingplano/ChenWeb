## 1. Database

- [x] 1.1 Add goose migration `project_migrations/<ts>_create_kb_metric_reviews.sql` creating
      `kb.metric_reviews` (columns per design D1) and index
      `(input_record_id, created_at DESC)` (use the `db-migration` skill; confirm it applied via
      `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5`).

## 2. Prompt and config

- [x] 2.1 Write `prompts/prompt-review-metric-extraction-v1.md`: role, the three review questions, category
      and severity definitions, precision-first rules (from the record-416 review), and the exact
      JSON output schema of design D5.
- [x] 2.2 Add `REVIEW_METRICS_PROMPT = "prompt-review-metric-extraction-v1.md"` and
      `REVIEW_METRICS_MODEL_NAME` (an existing `.models.toml` reference) to `mise.local.toml`.

## 3. Backend

- [x] 3.1 Create `server/api/kbhandler/metric_review_handler.go`: report/row types; load prompt from
      `REVIEW_METRICS_PROMPT` (error naming the var when missing); load model via
      `loadWikiModelDef("REVIEW_METRICS_MODEL_NAME")`.
- [x] 3.2 Input builder: fetch record header + raw lines (`rawLinePathFor` / `readRawLinesFile`)
      + `kb.metrics` rows (design D4 columns); render `L<n>\t<type>\t<text>` lines + metrics
      JSON; fail with "document too large" above the 600 000-char limit.
- [x] 3.3 Report post-processing (design D5): parse LLM payload, drop unknown metric IDs and empty
      entries, normalise category/severity, compute tally, attach metrics snapshot.
- [x] 3.4 `GetMetricReview` (`GET /kb/metric-reviews/:record_id`): newest row for the record, with a
      `running` row older than the 10-min run timeout reported as `failed` ("interrupted");
      `{review: null}` when none.
- [x] 3.5 `StartMetricReview` (`POST /kb/metric-reviews/:record_id`, body `{force}`): cache rules of
      design D3; no metrics → 400 without LLM call; else insert `running` row, launch goroutine
      (own context, 10-min timeout) that runs the LLM via `ExtractJSON` and updates the row to
      `done`/`failed`; return the row.
- [x] 3.6 Register both routes in `server/api/routes.go` next to the other `/kb/metrics` routes.
- [x] 3.7 Unit tests: input builder (line format, size limit), post-processing (unknown IDs dropped,
      tally, normalisation), stale-running detection, POST cache decision logic.

## 4. Frontend

- [x] 4.1 Create `web/src/lib/components/home3/metric-review-client.ts`: types for the review row
      and report; `searchInputs(query)` (numeric → `record_id`, else `title`, via `GET /kb/inputs`),
      `getMetricReview(id)`, `startMetricReview(id, force)`; pure helpers (query building, severity
      ordering, grouping non-metrics by category) with unit tests.
- [x] 4.2 Create `metric-review-view.svelte` (design D7): search + results (left), record header,
      Force to Review checkbox, Review button, status line, polling while `running` (right).
- [x] 4.3 Report rendering: summary, tally tiles, Missed Metrics, Not Metrics (grouped by category),
      Attribute Issues (by severity), Recommendations; metric-ID chips showing snapshot name/value.
- [x] 4.4 Add `{ id: 'sysadmin-llm-review-metrics', label: 'Review Metrics' }` under System Admin → LLM
      in `nav-rail.svelte`; add the dispatch branch and the app-shell (no footer) exclusion in
      `content-panel.svelte`.

## 5. Verification

- [x] 5.1 `go build ./...` and `go test ./server/api/kbhandler/...`; frontend `bun run check` and
      client unit tests.
- [x] 5.2 End-to-end on record 416 in the running dev app: first review runs and is stored; reselecting
      shows stored review with no new LLM call; Force creates a second row; compare findings with
      devdoc `2026092903`.
      Verified 2026-09-29 by driving the GET/POST handlers against the dev DB + real model
      (temporary harness, since deleted): record 416 (45 metrics after the 13:19 re-extraction)
      reviewed in ~70 s, stored as kb.metric_reviews id=1 (done; tally 45 stored / 34 kept /
      5 not_metric / 6 formula_input / 9 missed); a POST during the run reused it; a non-forced
      POST afterwards returned it without a new LLM call; record with no metrics -> 400.
      Findings match the manual review (scope boundary, applicability descriptors, formula
      operands, conditions, comparators, value_min/max). NOT verified: routing/auth through the
      live server and the page in a browser (needs a logged-in session), and a forced re-review
      (covered only by the shouldStartMetricReview unit test).
- [x] 5.3 Devdoc for the feature (create-devdoc skill, OpenSpec template) and commit via `jj`
      (only this change's files).
