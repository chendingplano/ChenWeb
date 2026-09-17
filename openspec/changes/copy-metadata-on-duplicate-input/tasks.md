## 1. Implement metadata copy in `record_duplicated`

- [x] 1.1 In `python/pdf-parser/shared.py`, extend `record_duplicated()` to `SELECT title, doc_no, result_filename, backup_filename, publish_date, authors, owner, public_info, parser_name FROM kb.inputs WHERE id = %s AND parse_state = 'parsed_success'` for `dup_rcd_id` before the existing UPDATE.
- [x] 1.2 When the SELECT returns a row, include those 9 columns in the existing `UPDATE kb.inputs SET ...` alongside `status`, `error_msg`, `modify_time` (single UPDATE, same transaction).
- [x] 1.3 When the SELECT returns no row, leave the UPDATE exactly as it is today (metadata columns untouched).
- [x] 1.4 Add a fresh `(MID_YYYYMMDDNN)` log line (module convention, see `docs/superpowers/plans/2026-07-01-deepseek-cache-processor-unification.md:18`) at the point metadata is copied vs. skipped, consistent with the existing `log.info(... MID_2026040709 ...)` call at `python/pdf-parser/pdf_parser.py:502`.

## 2. Verify

- [x] 2.1 Add/extend a unit or integration test in the `python/pdf-parser` test suite covering: (a) duplicate of a `parsed_success` original copies all 9 fields; (b) duplicate whose matched row is itself `duplicated`/not `parsed_success` copies nothing and behaves as before.
- [x] 2.2 Run the parser service's existing test suite to confirm no regressions.
- [ ] 2.3 Manually stage two identical PDF files through the running dev pipeline (`mise dev`) and confirm in the Import Inputs UI that the second (duplicate) row shows the same title/doc_no/etc. as the first.
