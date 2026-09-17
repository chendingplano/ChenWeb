## Context

`python/pdf-parser/shared.py::record_duplicated()` (lines 540-562) is the single write path invoked whenever `find_duplicate_processed_record()` (lines 394-421) finds an existing `kb.inputs` row with the same MD5 that already has `parse_state = 'parsed_success'`. Today it only upserts the `status` JSONB column with `{"proc_status": "duplicated", "dup_rcd_id": <id>, ...}`; every descriptive column (`title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, `parser_name`) is left as-is (typically unset, since a newly-staged record never reaches those writes).

`parse_state` is a trigger-maintained rollup (`20260609000002_add_kb_inputs_status_rollups.sql`) derived from the `status` JSON. A row whose own `status` has `proc_status: "duplicated"` rolls up to `parse_state = 'parsed_failed'`, never `'parsed_success'`. Because `find_duplicate_processed_record()` filters on `parse_state = 'parsed_success'`, it can **never** return the id of a row that is itself a duplicate — that invariant already holds structurally, today, with no extra code.

## Goals / Non-Goals

**Goals:**
- When a new `kb.inputs` row is marked `duplicated`, populate its `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, `parser_name` from the matched original row.
- Never copy from a row that is itself a duplicate (defense-in-depth, not just reliance on the caller's existing filter).

**Non-Goals:**
- Backfilling metadata onto rows already marked `duplicated` before this change ships.
- Changing what counts as a "duplicate" (still pure MD5 equality) or any Go/frontend code — the Import Inputs list already renders whatever is in these columns.
- Building a general merge/resolve-duplicate feature, or surfacing `dup_rcd_id` in the UI.

## Decisions

**Fetch metadata inside `record_duplicated()`, gated on the source row's own `parse_state`.**
Extend `record_duplicated()`'s signature with `conn` (already a param) to run one extra `SELECT ... WHERE id = %s AND parse_state = 'parsed_success'` against `dup_rcd_id` immediately before the existing UPDATE, in the same function/transaction. If the row is returned, its 9 columns are copied into the `UPDATE kb.inputs SET ...` alongside the existing `status`/`error_msg`/`modify_time` writes. If no row is returned (the source is no longer `parsed_success` — e.g., it was itself since re-flagged, or deleted, a narrow race window between the caller's check and this write), the UPDATE proceeds exactly as it does today: metadata columns untouched.

*Why re-check `parse_state = 'parsed_success'` here instead of trusting the caller's filter?* `find_duplicate_processed_record()` and `record_duplicated()` run as two separate statements/commits (`find_duplicate_processed_record` even does its own `conn.commit()` at line 420) with no lock held in between, so the source row's state could in principle change between the two calls in a concurrent run. Re-checking at the point of copy is one extra indexed lookup and keeps the guarantee ("never copy from a duplicate-marked row") true regardless of what the caller does, rather than being an implicit side effect of a filter defined 25 lines away in a different function. This directly implements the requirement's explicit precondition rather than leaving it as an unstated invariant.

*Why not a separate helper function (e.g. `copy_duplicate_metadata()`) called from `pdf_parser.py`?* `record_duplicated()` already owns the single UPDATE statement for this transition; splitting the fetch into the caller would mean two round-trips and two functions coordinating one write. Keeping it inside `record_duplicated()` matches the existing pattern where each `record_*` function in `shared.py` owns its full read-modify-write for one status transition (see `record_parse_active`, `record_parsed_success`).

**Alternatives considered:**
- *Parse the source row's `status` JSON in Python instead of checking `parse_state`.* Rejected: `parse_state` is already the canonical, trigger-maintained, indexed answer to "is this row a successfully-parsed non-duplicate"; re-deriving the same fact from JSON would duplicate logic that already exists and could drift from the trigger's CASE statement.
- *Backfill existing `duplicated` rows in this change.* Rejected as out of scope per proposal; can be a follow-up one-off script if needed.

## Risks / Trade-offs

- [One extra SELECT per duplicate-detection event] → Negligible: duplicate detection is already one SELECT (`find_duplicate_processed_record`) plus one UPDATE per staged file; this adds one more indexed SELECT (`id` is the PK) only on the already-rare duplicate path.
- [Source row mutates between `find_duplicate_processed_record` and `record_duplicated`] → Handled by the `parse_state = 'parsed_success'` re-check: worst case, the copy is silently skipped and the row keeps today's behavior (unset metadata), never copies stale/wrong data.
- [Copied `owner` (BIGINT) / `public_info` (JSONB) carry over the original uploader's identity/private-ish info onto a row created by a possibly different uploader] → Accepted per the proposal's explicit field list; flagged here for visibility, not a blocker.

## Migration Plan

No schema or deploy-order changes. Pure application-logic change in `python/pdf-parser/shared.py` (and a one-line call-site update if `record_duplicated`'s signature changes) picked up on next parser-service restart. No rollback beyond reverting the commit — no data migration involved.
