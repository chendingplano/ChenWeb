## Why

Checking the quality of `extract_metrics` output is currently a manual job: someone
reads the source document line by line against the `kb.metrics` rows, as was done by
hand for record 416 (devdoc `2026092903-devdoc-extract-metrics-review-record-416`).
That review found missed metrics, non-metrics, re-run duplicates and systematic
attribute faults that are invisible from the Metrics page. A repeatable, LLM-driven
review that anyone on the admin side can run per document — and whose result is
kept — turns that one-off exercise into a routine quality check.

## What Changes

- Add a new page **System Admin → LLM → Review Metrics**.
- The page lets the user search `kb.inputs` (by record ID, title, doc number, file
  name — reusing the existing `GET /kb/inputs` search), select a record, and view a
  review of the metrics extracted from that document.
- A review is an LLM judgement of the record's `kb.metrics` rows against the
  document's line text, covering: (1) metrics that were missed, (2) rows that should
  not be metrics (not a metric / duplicate / formula input), (3) attribute
  correctness of the rows that are kept, plus a short summary and recommendations.
- Reviews are stored in a new table. Selecting a record shows its latest stored
  review without calling the LLM. A **Force to Review** flag runs a fresh review
  even when one exists; every review is kept (history), the newest is shown.
- The review runs in the background; the page polls until it finishes, so long
  reviews are not cut off by proxy timeouts.
- The review prompt lives in `prompts/prompt-review-metric-extraction-v1.md`, located via the
  `REVIEW_METRICS_PROMPT` env var. The model is chosen by a new
  `REVIEW_METRICS_MODEL_NAME` env var (a `.models.toml` reference, same mechanism
  as `WIKIPAGE_CREATION_MODEL_NAME`).
- The report is rendered as a readable page: summary, tally, then sections for
  missed metrics, non-metrics, and attribute issues, each linking back to the
  metric rows and source lines it cites.

## Capabilities

### New Capabilities
- `metric-extraction-review`: LLM review of one document's extracted metrics,
  persisted per record with history, with a force-rerun option, and presented on a
  System Admin → LLM page.

### Modified Capabilities
(none)

## Impact

- **Frontend**: new `metric-review-view.svelte` + `metric-review-client.ts` under
  `web/src/lib/components/home3/`; one nav child under System Admin → LLM
  (`nav-rail.svelte`); one dispatch branch in `content-panel.svelte`.
- **Backend**: new handler file `server/api/kbhandler/metric_review_handler.go` with
  two endpoints on the existing `/api/v1` group; LLM call through the existing
  `llmclients.ExtractJSON` path so usage is logged in LLM Usage Logs.
- **Database**: one new table `kb.metric_reviews` via goose migration.
- **Config**: `REVIEW_METRICS_PROMPT` and `REVIEW_METRICS_MODEL_NAME` added to
  `mise.local.toml`; production config must add the same two vars.
- **Prompts**: new `prompts/prompt-review-metric-extraction-v1.md`.
