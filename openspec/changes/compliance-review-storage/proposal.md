## Why

A Compliance Review finding stores its activities, candidate decisions, search history,
language copies and execution details in one metadata object, and its description repeats
every judgement. In run 116 the 126 compliance findings carry about 5 MB of metadata (one
finding alone holds 1.1 MB), the saved report for the run is 10.5 MB of JSON because it
copies that metadata, and opening the report fetches both. Expanding Compliance renders
and highlights every finding at once. The approved design
(KnowledgeStore design 2026112301) splits this storage so screens load only what they show.

## What Changes

- The finding summary in `kb.doc_review_findings` gains a machine-readable source line
  range, a storage-version marker and small counters (activities, search results,
  selected, relevant and judged candidates, and counts by verdict).
- New tables in schema `kb`: finding details, compliance activities, candidate
  assessments, searches, search results, run context, and finding translations.
- Every activity gets an independent activity ID from the shared ID manager; the old
  `a1`, `a2` labels stay as local labels unique within their finding.
- Compliance Review writes new findings into this structure in one transaction per
  finding. The description holds only text about the finding as a whole: no verdict
  rows, activity lines or judged-candidate list. Metadata keeps only small markers.
- **BREAKING (API):** `GET /doc-review/requests/:id` returns compliance findings as
  compact entries (identity, title, severity, status and source range) without
  description, evidence or metadata. New endpoints return a cursor-paginated summary
  list, a finding's details with paged activities and candidates, one candidate's
  evidence, search history with paged results, and a complete audit export.
  `GET /doc-review/reports/:id?view=summary` returns a report without its finding arrays.
- Report building, the Typst PDF, auto-fix, finding translation, the correction log, the
  KB Findings page and the compliance scorer read the new tables. The scorer reads
  judged candidates from candidate rows instead of parsing the description.
- A backfill command migrates existing compliance findings (artifact-fields version 3,
  unchecked notices and detect-failure findings) one transaction per finding, validates
  each one before switching its storage marker, and in a separate step writes a recovery
  copy and removes the duplicated legacy payload. Older shapes stay readable through
  the legacy details path.
- The report view loads compact entries, renders compliance summaries 25 at a time,
  and the Finding Details panel loads details, candidates, evidence and search history
  only when asked.

## Capabilities

### New Capabilities
- `compliance-review-storage`: how Compliance Review findings, activities, candidate
  assessments, searches and run context are stored, identified, loaded, exported,
  migrated and deleted.

### Modified Capabilities
<!-- None: compliance-review-redesign is an unarchived change, not a spec in openspec/specs/. -->

## Impact

- Schema: one goose migration in `project_migrations/` (seven new tables, new
  finding columns and indexes).
- Backend: `server/api/doc-reviews/` (compliance reviewer records, finding store,
  controller and handlers, report and Typst loaders, auto-fix, translation),
  `server/api/doc-processing/doc_review_finding_store.go` (KB Findings page),
  `server/api/compliance-eval/` (scorer), new command `server/cmd/compliance-storage-migrate`.
- Frontend: `web/src/lib/services/docReviewService.ts`,
  `web/src/lib/components/home3/` report view, Finding Details panel and compliance
  helpers, with new messages in `web/messages/en.json` and `zh-cn.json`.
- IDs: activity IDs come from the shared ID manager, sequence `compliance_activity_id`,
  provisioned by the store on first use. Table primary keys stay `BIGSERIAL`.
- Reuse: catalogue searches for pagination, cursor lists, lazy loading, audit export,
  JSON trees, PDF highlighting and transaction stores found only the Shared ID Manager
  (20261108-01), which this change uses. It also reuses uncatalogued code in place: the
  existing Finding Details panel, `JsonTreeDialog`, the report view's PDF highlighting
  through `findingShelf`, and the reviewer's existing grouping, search, relevance and
  judgement code. Cursor pagination exists only inside `server/api/datasync/` and is
  specific to that feature. The new store and endpoints are project-specific.
- Docs: devdoc 2026111201 (Compliance reviewer) storage and display sections; design
  2026112301 status.
