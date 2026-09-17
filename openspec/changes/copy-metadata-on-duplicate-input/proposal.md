## Why

When a newly uploaded file's content (MD5) matches an existing `kb.inputs` row, the parser only stamps the new row's `status` JSON with `proc_status: "duplicated"` — it copies none of the matched row's descriptive metadata. The duplicate row is left with empty `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, and `parser_name`, so it shows up in the Import Inputs list as an unidentifiable blank entry even though its content is already known and cataloged under the original record. Copying the original's metadata makes duplicate rows self-describing without requiring a human to cross-reference `dup_rcd_id` inside the status JSON.

## What Changes

- When `record_duplicated()` fires for a new `kb.inputs` row (MD5 match against an existing `parse_state = 'parsed_success'` row), copy `title`, `doc_no`, `result_filename`, `backup_filename`, `publish_date`, `authors`, `owner`, `public_info`, `parser_name` from the matched row onto the new duplicate row.
- Guard against chaining: only copy when the matched row is **not itself** a duplicate (its own `status` JSON has no `proc_status: "duplicated"` entry). If the matched row is itself a duplicate, skip the copy and leave the new row's fields unset — same as today's behavior.
  - This guard is largely redundant with the existing `find_duplicate_processed_record` query (which only matches rows with `parse_state = 'parsed_success'`, and a row that is itself duplicated does not carry that parse_state), but it is made explicit here as a safety net in case that invariant changes later.
- No schema change: all copied columns already exist on `kb.inputs`.

## Capabilities

### New Capabilities
- `kb-input-duplicate-metadata-copy`: Defines when and how descriptive metadata is copied from an original `kb.inputs` record onto a newly detected duplicate of it.

### Modified Capabilities
(none — no existing spec covers `kb.inputs` duplicate handling)

## Impact

- **Code:** `python/pdf-parser/shared.py` (`record_duplicated`, called from `python/pdf-parser/pdf_parser.py`). No Go server or frontend changes required — the Import Inputs list already renders whatever is in these columns.
- **Data:** `kb.inputs` rows written going forward will have populated metadata columns when flagged `duplicated`. Existing already-`duplicated` rows are not backfilled by this change.
- **Dependencies:** none.
