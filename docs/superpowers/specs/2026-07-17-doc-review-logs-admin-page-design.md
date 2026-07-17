# Doc Review Logs Admin Page Design

## Goal

Add a read-only admin page for `kb.doc_review_logs` at **SYSTEM ADMIN → Logs → Doc Review Logs**. It lets administrators inspect individual document-review execution records without exposing large JSON values in the table.

## Scope

The change adds a paginated, filterable API and a matching Svelte view in the existing home3 content panel. No database migration or mutation actions are required.

## Navigation and page

- Add `Doc Review Logs` under the existing `SYSTEM ADMIN → Logs` menu.
- The page follows the existing Doc Processor Logs and LLM Usage Logs visual and interaction patterns.
- Keep refresh, filtering, pagination, loading, and error states.
- Columns, in order: Input Record ID, Run ID, Unit Key, Aspect, Unit Type, Unit Location, Outcome, Pass, # Findings, Detail, Create Time.

## API and data flow

- Register `GET /api/v1/kb/doc-review-logs` on the existing `/api/v1` group. The group already uses `authmiddleware.AuthMiddleware`, matching the protection of the adjacent System Admin log endpoints; no new authorization mechanism is introduced.
- Accepted query parameters are `page` (1-based, default `1`), `page_size` (default `50`, capped at `500`), `input_record_id`, `run_id`, `pass`, `aspect`, `unit_type`, `outcome`, `unit_key`, `create_start_time`, and `create_end_time`.
- `input_record_id` and `run_id` must be integers. `create_start_time` and `create_end_time` must be parseable RFC 3339 timestamps, and start must not be after end. Invalid supplied numeric/time values or inverted time ranges return HTTP 400 with the project-standard `{status:false,error_msg}` shape. Empty filter values mean no filter.
- `pass`, `aspect`, `unit_type`, and `outcome` use exact matches. `unit_key` uses case-insensitive literal-substring matching: `%`, `_`, and the escape character in the supplied value are escaped before it is used with `ILIKE`. The timestamp bounds are inclusive. Page values below one default to one; page sizes below one default to 50; page sizes above 500 are capped, consistent with the existing document-processing log API.
- A success response is `{status:true, results:[...], page, page_size, total}`. `total` is the number of matching records before pagination. Each result exposes `id`, `input_record_id`, `run_id`, `pass`, `aspect`, `unit_type`, `unit_key`, `unit_location`, `matched_units`, `findings`, `outcome`, `detail`, and `create_time`; `create_time` is an RFC 3339 timestamp.
- Order results by `create_time DESC, id DESC` to make paging stable.
- The handler validates query values then delegates SQL construction and scanning to a focused store function. The query uses bound parameters for values; ordering is fixed rather than request-selectable.

## Table behavior

- `# Findings` is the size of `findings` when it is a JSON array; null, non-array, or empty values render as `0`.
- `UNIT LOCATION` is a safe text rendering of the JSON value, truncated to 120 characters in the table. It must not render supplied content as HTML. The complete value remains available in Detail.
- `Detail` opens a modal containing `unit_location`, `matched_units`, `findings`, and `detail` rendered recursively as name-value entries. Arrays display indexed entries; scalar values display directly; null displays explicitly.

## Artifact interaction

- Treat a Unit Key as an artifact ID only if it exactly matches `^[1-9][0-9]*_(mtc|prv|inv)_[1-9][0-9]*$`. Both `record_id` and `seqno` are positive decimal integers.
- Only matching keys are interactive. Map `mtc` to artifact type `metric`, `prv` to `provision`, and `inv` to `inventory_item`; request the existing authenticated endpoint `GET /api/v1/kb/artifacts/wiki?artifact_type=<mapped>&artifact_id=<unit_key>&include_article=0`.
- Display the response `record` field in the same recursive name-value modal style. The page does not request or display wiki `article` or `source_document` content.
- A non-OK response or malformed artifact response shows an actionable error in the modal; the log row stays visible and usable. Nonmatching keys are plain text.

## Error handling

- Invalid filter or pagination values return a clear client error; database failures return the project-standard server error.
- The view shows API failures inline and clears stale rows.
- Dialog fetch failures are contained to the dialog and do not reload or discard the table.

## Tests and verification

- Add SQL-mock store tests for pagination, stable ordering, every filter (including escaped Unit Key wildcard characters), and JSON row scanning; add handler tests for the response contract, invalid numeric/time filters, and inverted date ranges.
- Add focused frontend checks or component-level tests for the artifact-ID expression/type mapping, `findings` null/non-array/empty/array counts, unit-location escaping/truncation, recursive JSON dialog preparation, and artifact-load failure states where the project tooling supports them.
- Run relevant Go tests and the web type/lint/build verification used by ChenWeb.

## Documentation impact

- This spec records the new admin capability and its API/UI contract.
- Existing user-facing documents are unaffected; no operational schema documentation changes because the table already exists.
