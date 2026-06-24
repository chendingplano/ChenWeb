# Document Review — Activity Log & Correction Report

This feature records every reviewer correction action taken on a **Document
Review Report** and produces a **Document Review Correction Report** (Typst → PDF)
that summarizes those actions.

## 1. Activity Log (`kb.doc_review_activities`)

Every correction action is persisted to `kb.doc_review_activities`. Schema is
created by goose migration
`project_migrations/20260624000001_create_doc_review_activities.sql`.

| Column            | Notes                                                             |
| ----------------- | --------------------------------------------------------------- |
| `activity_type`   | one of the constants below                                       |
| `input_record_id` | document being reviewed (`kb.inputs.id`)                         |
| `review_run_id`   | review run; NULL for run-agnostic Document Structure edits       |
| `report_id`       | latest report id for the run, when resolvable                    |
| `finding_id`      | finding the action targeted (finding-based activities)           |
| `page_number` / `line_number` | Document Structure target line                      |
| `location`        | finding line range (e.g. `42`, `53-56`)                          |
| `old_content` / `new_content` | before/after text                                   |
| `detail`          | JSONB extras (title, aspect, severity, model, line_type, …)     |
| `actor`           | authenticated user name, when available                         |
| `create_time`     | timestamp                                                        |

### Activity types (`server/api/docactivity`)

| Constant              | `activity_type`    | Source                                                              |
| --------------------- | ------------------ | ------------------------------------------------------------------ |
| `TypeAutoFix`         | `auto_fix`         | LLM Auto Fix — `DocReviewController.AutoFixFinding`                 |
| `TypeEditTool`        | `edit_tool`        | Edit Tool save — `DocReviewController.ApplyFindingEdit`             |
| `TypeFindingDelete`   | `finding_delete`   | Delete button — `DocReviewController.UpdateFinding` (status=deleted)|
| `TypeStructureModify` | `structure_modify` | Document Structure edit — `kbhandler.UpdateDocStructureLine`        |
| `TypeStructureSplit`  | `structure_split`  | Document Structure split — `kbhandler.SplitDocStructureLine`        |
| `TypeStructureDelete` | `structure_delete` | Document Structure delete — `kbhandler.DeleteDocStructureLine`      |

`docactivity` is a **leaf package** (depends only on `database/sql` +
`loggerutil`) so both the doc-reviews controller and the kbhandler doc-structure
handlers can log without an import cycle. `docactivity.Log` is best-effort:
failures are logged and swallowed so a logging error never breaks the user's
action.

## 2. Correction Report (Typst → PDF)

`DocReviewController.GenerateCorrectionReport(ctx, reportID)`
(`server/api/doc-reviews/correction_report.go`) loads the activities for the
report's `(input_record_id, review_run_id)` via `docactivity.List`, renders a
Typst source from the template, and compiles it to PDF — mirroring
`GenerateTypstReport`.

- **Output dir:** `$DOC_REVIEW_REPORTS` (no-op if unset; same dir as the review report)
- **Template:** `$DOC_REVIEW_CORRECTION_TEMPLATE_FILENAME`
  (default `docs/doc-templates/template-correction-report.typ`)
- **Language:** `$DOC_REVIEW_REPORT_LANGUAGE` (default `en`)
- **File names:** `<yyyymmdd-hhmm>-<reportID>corrections.typ` / `.pdf`
  (parallels the review report's `<stamp>-<reportID>reports.{typ,pdf}`)

The template defines `#document-correction-report(...)` with a cover, basic
information, an "Actions by Type" summary table, and one `correction-entry(...)`
per action (before/after blocks, location, actor, time, context note).

### Endpoint & UI

- `POST /api/v1/doc-review/reports/:id/correction-report` → `docreviews.GenerateCorrectionReport`
  returns `{ status, pdf_path, pdf_file }`.
- Frontend: `generateCorrectionReport(reportId)` in
  `web/src/lib/services/docReviewService.ts`, triggered by the **Correction
  Report** button on the Document Review Report page
  (`web/src/routes/home3/doc-review-report/[id]/+page.svelte`).

## Knowledge changed / docs status

- **New:** this document; migration `20260624000001`; package `docactivity`;
  `correction_report.go`; `template-correction-report.typ`.
- **Updated:** `mise.local.toml` (added `DOC_REVIEW_CORRECTION_TEMPLATE_FILENAME`).
- **Stale:** none known.
- **Intentionally undocumented:** per-field detail JSON keys (self-describing in code).
