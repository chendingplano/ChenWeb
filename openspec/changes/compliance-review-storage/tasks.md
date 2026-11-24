## 1. Baseline

- [x] 1.1 Record the run-116 baseline: compliance-score output, rendered report markdown, request and report response bytes, and a JSON dump of every compliance finding (recovery reference)

## 2. Schema

- [x] 2.1 Goose migration: summary columns on `kb.doc_review_findings`, the seven new tables with foreign keys, unique keys and indexes (D1, D2); dry-run it in a rolled-back transaction, then apply to dev

## 3. Store and write path

- [x] 3.1 Record types (`complianceRecord`, activity, search, result, candidate, run context) and `ReviewFinding.LineStart/LineEnd/Compliance`
- [x] 3.2 Activity ID allocation from sequence `compliance_activity_id` (provision on first use)
- [x] 3.3 Transactional writer for one normalized finding and run-context upsert; `SaveFindings` routes normalized findings to it; tests for rollback and counters
- [x] 3.4 Reviewer builds records in `unitFinding`, the unchecked notice and the detect-failure finding; keeps relevance-failed and judgement-failed candidates; description without judgements; tests

## 4. Read API

- [x] 4.1 Compact compliance entries in `GET /doc-review/requests/:id`
- [x] 4.2 Paginated summaries `GET /doc-review/runs/:run_id/findings`
- [x] 4.3 Details, activity and candidate pages, candidate evidence, search history and results, audit export endpoints (normalized and legacy)
- [x] 4.4 `GET /doc-review/reports/:id?view=summary`
- [x] 4.5 Handler and query tests (pagination cursors, legacy path, no model calls)

## 5. Consumers

- [x] 5.1 Report loader hydrating normalized compliance findings (review completion, regeneration, Typst) with a judgement section rendered from candidate rows
- [x] 5.2 Auto-fix, delete log and translation read content through details
- [x] 5.3 KB Findings page: marker metadata and details description for compliance; artifact filter also matches activity source artifacts
- [x] 5.4 Compliance scorer reads judged candidate rows for normalized findings; tests

## 6. Backfill

- [x] 6.1 `compliance-storage-migrate`: legacy readers (version 3, unchecked notice, detect failure), validation, per-finding transaction, retry-safe activity IDs, run context with review flags, `--dry-run`
- [x] 6.2 `--strip` with recovery copy; usage errors
- [x] 6.3 Tests: retry, validation failures, unsupported shapes, language copies, missing historical fields
- [x] 6.4 Migrate runs 115 and 116 on dev; check run-116 counts (126 findings, 244 activities, finding 11869 with 48 activities, finding 11970 with 1/2/60/30/4), no translation rows, one run context without flags

## 7. Frontend

- [x] 7.1 Service functions and types for the new endpoints
- [x] 7.2 Report view: summary report, compact entries, Compliance folder pages of 25 with Load more and counters, highlighting from compact entries
- [x] 7.3 Finding Details panel: details, activity and candidate pages, candidate evidence, search history and results, audit export, legacy path, cache per finding and language
- [x] 7.4 Messages in `en.json` and `zh-cn.json`; `bun run check` and the web tests pass

## 8. Verification

- [x] 8.1 Score run 116 after migration and compare with the baseline; compare rendered reports
- [ ] 8.2 Measure request bytes, server and database time, list rendering and PDF highlighting before and after on run 116; confirm selecting a finding fetches no search results
  (Done: bytes, server time and browser JSON-parse time; details carry no search results. Open: list-rendering and PDF-highlighting time need a logged-in browser session on a server running this code.)
- [x] 8.3 Strip run 116 with a recovery copy and re-run the checks
- [x] 8.4 Go tests for doc-reviews, compliance-eval, doc-processing and kbhandler pass

## 9. Documentation and release

- [x] 9.1 Update devdoc 2026111201 (storage, loading, migration, measurements) and design 2026112301 status
- [x] 9.2 Release item; confirm no reusable module needs registering (the store is project-specific)
