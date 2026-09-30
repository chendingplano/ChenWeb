## Context

`extract_metrics` writes one row per metric into `kb.metrics` (keyed by
`input_record_id`, with `metric_id`, `source_line_spans`, value fields, etc.). The
document's line-numbered text lives in the raw-line file next to
`kb.inputs.result_filename` (`rawLinePathFor` / `readRawLinesFile` in
`server/api/kbhandler`). A manual review of record 416 showed that the useful
review questions are: missed metrics, rows that are not metrics, and attribute
correctness — and that the answers need to cite metric IDs and source lines.

Existing pieces this change reuses:
- `GET /kb/inputs` (`kbhandler.ListInputs`) already searches `kb.inputs` by
  `record_id`, `title`, `doc_no`, `name`, `file_name` with paging.
- `loadWikiModelDef(envKey)` + `defaultNewExtractMetricsClient` +
  `client.ExtractJSON` (`metric_wiki_generate.go`) resolve a `.models.toml` entry
  from an env var and make a JSON-mode LLM call that is logged to LLM usage.
- System Admin pages are `home3` views dispatched by `activeMenu.childId` in
  `content-panel.svelte`, listed in `nav-rail.svelte`.

## Goals / Non-Goals

**Goals:**
- One-click LLM review of a single record's metrics, stored and re-shown without
  re-running the LLM.
- Force re-review, keeping earlier reviews.
- A report a person can read top to bottom, with every finding traceable to metric
  rows and source lines.

**Non-Goals:**
- Changing `kb.metrics` (no "apply fix" / delete actions) — the review is advisory.
- Reviewing documents too large for one LLM call (no chunking in v1).
- Batch review of many records, scheduling, or comparing reviews across runs.
- Showing older reviews in the UI (they are stored; UI shows the latest).

## Decisions

### D1. Storage: `kb.metric_reviews`, one row per review run

```
id               bigserial PK
input_record_id  bigint NOT NULL
status           text NOT NULL   -- 'running' | 'done' | 'failed'
report           jsonb           -- NULL until done
error_msg        text
model_name       text
prompt_name      text
metrics_count    int NOT NULL    -- kb.metrics rows reviewed
created_by       text
created_at       timestamptz NOT NULL DEFAULT now()
finished_at      timestamptz
INDEX (input_record_id, created_at DESC)
```
The "current review" of a record is its newest row. Forcing a review inserts a new
row rather than overwriting. *Alternative:* one row per record, overwritten on
force — simpler, but loses the before/after comparison that is the point of
re-reviewing after an extractor change. Rows are small (one JSON report each).

### D2. Asynchronous run with polling

`POST` inserts a `running` row, starts a goroutine that does the LLM call and
updates the row to `done`/`failed`, and returns the row immediately. The page polls
`GET` every few seconds while the status is `running`. *Why not synchronous:* a
full-document review takes tens of seconds to minutes; production traffic passes
through nginx (default 60 s proxy read timeout), so a synchronous request would be
cut off while the LLM call keeps running. The goroutine uses its own
`context.Background()` with a timeout (10 min) so it is not cancelled when the HTTP
request ends.

A server restart kills in-flight goroutines and leaves rows `running`. A `running`
row older than the run timeout is reported as `failed` ("interrupted") on read and
does not block a new review. No sweeper job.

### D3. Cache semantics of POST

`POST /kb/metric-reviews/:record_id` with body `{"force": bool}`:
- a `running` review within the timeout exists → return it (no second run, even
  with force);
- `force=false` and a `done` review exists → return it (no LLM call);
- otherwise → start a new run.
The page calls `GET` on record selection (never triggers the LLM) and `POST` only
from the Review button, sending the "Force to Review" checkbox value.

### D4. LLM input

Built server-side as one text block:
1. Document header: record ID, title, doc_no.
2. Source lines as `L<n>\t<type>\t<text>` — line number, block type
   (heading/paragraph/table…), text. Bounding boxes and fonts are dropped.
   A table line is sent as `L<n>\ttable` followed by its numbered rows
   (`<n>#<row id>: cell | cell`, the table-row-context rendering) so the model
   can cite rows (prompt v3).
3. Metrics as a JSON array with the review-relevant columns: `metric_id`,
   `source_line_spans`, `metric_name`, `metric_subject`, `metric_value`,
   `metric_unit`, `threshold_or_target`, `value_min`, `value_max`,
   `value_data_type`, `value_range_type`, `value_class`, `condition`,
   `is_explicit_metric`, `confidence`, `location_type`, `measurement_frequency`,
   `formula_or_definition`, `created_at` (date only — lets the model spot re-run
   duplicates).

If the assembled input exceeds a fixed limit (600 000 characters) the run fails
with a clear message rather than being silently truncated. Median raw-line files
are ~45 KB and p90 ~340 KB including bbox columns, so most documents fit.

### D5. Report shape (LLM output, validated server-side)

```json
{
  "summary": "string",
  "missed_metrics": [{"lines": "123", "table_rows": ["116#r3"], "name": "", "value": "",
                      "unit": "", "reason": "", "severity": "high|medium|low"}],
  "non_metrics": [{"metric_ids": ["416_mtc_2"], "category": "not_metric|duplicate|formula_input",
                   "duplicate_of": "", "reason": ""}],
  "attribute_issues": [{"metric_ids": [""], "field": "", "stored": "", "suggested": "",
                        "reason": "", "severity": "high|medium|low"}],
  "recommendations": ["string"]
}
```
Server-side post-processing before storing:
- drop metric IDs not in the reviewed set (log them); drop entries left with no IDs;
- normalise unknown `category`/`severity` values to `not_metric` / `medium`;
- compute the **tally** itself (stored, keep, not_metric, duplicate, formula_input,
  missed) from the lists — counts are not trusted from the model;
- turn a missed metric's `table_rows` (and any `116#r3` in `lines`) into
  `source_table_rows` (kb.metrics shape); GET adds a read-time `table_context`
  window (header + cited rows ± 1) so the export can bold the cited rows;
- attach a **metrics snapshot** (`metric_id`, name, value, unit, spans) of the rows
  reviewed, so the report stays readable after later re-extraction changes
  `kb.metrics`.

Prose is English; source quotes stay in the source language.

### D6. Prompt and model configuration

- `REVIEW_METRICS_PROMPT` = file name under `prompts/` (same resolution as
  `EXTRACT_METRICS_PROMPT`). Missing/empty → run fails with an explicit error. No
  in-code default prompt (ChenWeb rule: never hard-code prompts).
- `REVIEW_METRICS_MODEL_NAME` = `.models.toml` reference, loaded with the existing
  `loadWikiModelDef`. No fallback model (not requested).

### D7. Frontend

`metric-review-view.svelte`, two-column app-shell page (like LLM Usage Logs):
- **Left:** search box (record ID or text → `record_id` if numeric, else `title`)
  against `GET /kb/inputs`, result list (ID, title, doc_no).
- **Right:** selected record header, "Force to Review" checkbox + "Review" button,
  review status line (reviewed at / model / running spinner / failed message), then
  the report: summary, tally tiles, and three sections (Missed Metrics, Not Metrics
  — grouped by category, Attribute Issues — sorted by severity), then
  recommendations. Metric IDs render as chips showing the snapshot name/value on
  hover.

## Risks / Trade-offs

- [LLM verdicts are subjective and can be wrong] → the report is advisory only;
  nothing is changed in `kb.metrics`; model/prompt names are stored with each
  review so results can be compared after a prompt change.
- [Very large documents fail] → explicit error naming the size limit; chunked
  review is a follow-up if needed.
- [Server restart leaves a `running` row] → treated as failed after the timeout on
  read; user can re-run.
- [Duplicate concurrent clicks] → a recent `running` row is returned instead of
  starting another run.
- [Hallucinated metric IDs] → filtered against the reviewed set before storing.

## Migration Plan

Goose migration creates `kb.metric_reviews` (auto-applied by `mise dev`). Add the
two env vars to `mise.local.toml` and to the production config before deploying.
Rollback: drop the table and the nav entry; no other data is touched.

## Open Questions

None blocking.
