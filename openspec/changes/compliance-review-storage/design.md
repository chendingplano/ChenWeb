## Context

The approved design is KnowledgeStore design 2026112301 ("Compliance review storage").
This document records how ChenWeb implements it. The running reviewer is described in
devdoc 2026111201 and in the unarchived change `compliance-reviewer-redesign`.

Today `unitFinding` (`server/api/doc-reviews/review-compliance-judgement.go`) writes one
`ReviewFinding` per text activity or activity group. Its `ArtifactFields` (stored as
`metadata.artifact_fields`, version 3) hold activities, retrieval traces, relevance
decisions, candidate verdicts, verification checks, prompts, models and errors. Its
description lists the verdicts, the activities and the judged candidates; the compliance
scorer parses the judged list. `SaveFindings` inserts rows one by one, outside any
transaction, after all reviewers finish. `GET /doc-review/requests/:id` returns every
finding with its metadata; the report built from it copies descriptions and artifact
fields into `report_json` (10.5 MB for run 116), and the report view loads both.

Dev data (2026-11-23): runs 115 and 116 hold version-3 findings plus one unchecked
notice each; runs 98–109 hold an older shape (`compliance_grounding_candidates`, no
artifact fields). Search ranks always equal the result's position in its search. No
kept candidate lacks a verdict.

## Goals / Non-Goals

**Goals:**
- Store compliance findings as a small summary plus separately loaded records.
- Give every activity a global ID from the shared ID manager.
- Load per screen only what that screen shows; keep a complete export path.
- Migrate existing findings without losing decisions, ranks, quotes or attribution, and
  prove the scorer gives the same result before and after.

**Non-Goals:**
- Review accuracy, grouping, prompts or model calls (none change).
- Other reviewer aspects' storage (they may adopt the generic tables later).
- Saving results incrementally during a review, and cross-run activity matching.

## Decisions

### D1. Summary columns on `kb.doc_review_findings`
Add `line_start`, `line_end` (integers parsed from the unit's lines), `storage_version`
(smallint, default 1; 2 = normalized compliance), and nullable counters
`activity_count`, `search_result_count`, `selected_candidate_count`,
`relevant_candidate_count`, `judged_candidate_count`, plus `verdict_counts` (a JSONB
object with at most the five verdict keys). Counters are written with the records they
count, never computed by list queries. Five more integer columns for verdicts were
rejected: the set of verdicts is a closed vocabulary that the UI reads as one object.
Metadata of a normalized finding is `{"schema_version":1,"result_kind":…,"storage_version":2}`.

### D2. New tables (schema `kb`)
- `doc_review_finding_details` — `finding_id` PK/FK (cascade); description, evidence,
  suggestion, canonical language and origin, source language and confidence, error
  message, `stage_models` JSONB (relevance, judgement, verify: the model of each stage's
  last call), `extra` JSONB (the unchecked notice's limits and counts).
- `doc_review_compliance_activities` — `id` BIGSERIAL PK, `activity_id` BIGINT UNIQUE,
  `finding_id` FK (cascade), `local_label`, `display_order`, `kind`, `label`, line range,
  `source_artifact_type`, `source_artifact_id`, `source_row_id`, `source_text`, `details`
  JSONB (text-activity fields without `source_text`, context facts, cited documents,
  duplicate locations, document-level flag). UNIQUE (`finding_id`, `local_label`).
- `doc_review_compliance_candidates` — `id` BIGSERIAL PK, `activity_id` FK →
  activities(`activity_id`) (cascade), `candidate_type`, `candidate_id`,
  `candidate_record_id`, `document_title` (snapshot), `cosine` (the search cosine the
  reviewer held for the candidate), `relevance_answer`, `relevance_score`, `kept`,
  `status`, `verdict`, `binding`, `summary`, `note`, `error_message`, `evidence` JSONB,
  `display_order` and `judgement_order`. UNIQUE (`activity_id`, `candidate_type`,
  `candidate_id`). `display_order` numbers a finding's candidates for display: judged
  ones in verdict order (the order the description used), then the others by activity
  and relevance order. `judgement_order` is the position of the pair in the judgement's
  answers (null when not judged); the judged-candidate list the scorer reads follows it.
- `doc_review_compliance_searches` — `id` BIGSERIAL PK, `activity_id` FK (cascade),
  `ordinal`, `purpose`, `query`, `reference`, `cited_document`, `keywords` JSONB,
  `filters` JSONB (artifact types, record IDs, excluded record IDs), `result_limit`,
  `potentially_truncated`. UNIQUE (`activity_id`, `ordinal`).
- `doc_review_compliance_search_results` — `id` BIGSERIAL PK, `search_id` FK (cascade),
  `rank`, candidate type/ID/record, `document_title`, `score`, `diagnostics` JSONB.
  UNIQUE (`search_id`, `rank`): ranks equal positions in every stored trace.
- `doc_review_compliance_run_context` — `run_id` PK, FK → `kb.doc_review_runs` (cascade);
  `assessment_date`, `schema_version`, `prompts`, `models` (primary, correction,
  fallback), `settings`, `review_flags` JSONB (differences observed during migration).
- `doc_review_finding_translations` — `id` BIGSERIAL PK, `finding_id` FK (cascade),
  `language`, title, description, suggestion, provenance. UNIQUE (`finding_id`, `language`).

The findings' run reference keeps having no foreign key, so deleting a run's findings
by `run_id` still removes exactly those rows; children go with them by cascade.
`kb.doc_review_activities` (correction actions) gets no foreign key to findings.

### D3. Candidate status
`rejected` (relevance kept = false), `relevance_failed` (the activity's relevance call
failed; the activity's found candidates are recorded with the error), `pending` (kept,
no judgement and no recorded failure; only possible in migrated data), `judgement_failed`
(its judgement batch failed; the error is stored on the row), `judged` (accepted
judgement: verdict and binding set). Only `judged` rows have a verdict.

### D4. Activity IDs
Sequence `compliance_activity_id` in `shared.id_mgr`, provisioned with
`idmanager.Manager.Provision` on first use per process (`ApiTypes.SharedDBHandle`, as
`llmrun` and `docbuilder` do). A finding's activities take one `Generate` range before its
transaction starts; IDs abandoned by a rolled-back transaction are gaps, never reused.
Rows are inserted with `ON CONFLICT (finding_id, local_label) DO UPDATE … RETURNING
activity_id`, so a retried backfill keeps the activity ID it already has.

### D5. Write path
`ReviewFinding` gains `LineStart`, `LineEnd` and `Compliance *complianceRecord`
(`json:"-"`). `unitFinding`, the unchecked notice and the detect-failure finding fill the
record instead of `ArtifactFields`, `RelatedArtifacts`, `RelatedArtifactFields` and
`CitedDocuments`. The record carries the activities with their searches, results and
candidates, the stage models, the notice details, the counters and a pointer to the
run context. `ReviewFindingsSQLStore.SaveFindings` upserts the run context once, then
writes each normalized finding (summary, details, activities, searches, results,
candidates) in its own transaction; other findings keep their existing insert. The
description of a normalized finding is only finding-level text (D6); the old
`description` column holds `''` because it is `NOT NULL`.

The reviewer now also keeps, per activity, the candidates of a failed relevance call and
the pairs of a failed judgement batch, so their rows carry the failure.

### D6. Description content
Assessment findings: empty. No-grounding, incomplete, unchecked and detect-failure
findings: their existing explanatory sentence. Verdict rows, activity lines and the
judged-candidate list are never stored. Reports render a judgement section from
candidate rows (D8).

### D7. Read API
- `GET /doc-review/requests/:id`: compliance findings become compact entries — `id`,
  pass, aspect, severity, finding type, title, location, `line_start`, `line_end`, review
  status, processing status, `storage_version` — with no description, evidence,
  suggestion or metadata. Other aspects are unchanged.
- `GET /doc-review/runs/:run_id/findings?aspect=compliance&limit=25&cursor=…`: summary
  page (compact fields plus confidence, artifact ID and counters), ordered by severity
  rank then ID, cursor `"<rank>:<id>"`. Limit 1–100.
- `GET /doc-review/findings/:id/details?activity_limit=&candidate_limit=&filter=`:
  summary, content (from details), run-context models, stage models, extra, the first
  activity page (default 50) and the first candidate page (default 25, filter `judged`
  or `all`). Legacy findings return `legacy: {description, evidence, suggestion}` instead.
- `GET /doc-review/findings/:id/activities?cursor=&limit=` and
  `GET /doc-review/findings/:id/candidates?filter=&cursor=&limit=`: further pages.
  Candidates are ordered by `display_order`, then ID; cursor `"<display_order>:<id>"`.
- `GET /doc-review/compliance/candidates/:id`: one candidate's evidence and error.
- `GET /doc-review/findings/:id/searches`: search summaries (with result counts) for
  every activity of the finding; `GET /doc-review/compliance/searches/:id/results?cursor=&limit=`
  pages results by rank.
- `GET /doc-review/findings/:id/export`: the complete audit record, assembled on the
  server (normalized: summary, details, run context, activities with searches, results and
  candidates, translations; legacy: the stored columns and metadata).
- `GET /doc-review/reports/:id?view=summary`: report row and `report_json` without
  `findings` and `findings_by_pass`.

None of these endpoints calls a model. List queries name their columns and join no
child table.

### D8. Reports and other consumers
A shared loader hydrates normalized compliance findings for report building (review
completion, regeneration, Typst variants): description from details plus a judgement
section rendered from judged candidate rows and the activity list, in the same text form
the description had, without the judged-candidate list. `report_json` therefore keeps
the verdicts but no longer carries artifact fields. Auto-fix and the delete log read
description and evidence through `LEFT JOIN` details. Translation leaves compliance
untouched (as today) but returns details content. The KB Findings page returns marker
metadata for compliance rows and the details description, and its artifact filter also
matches activity source artifacts.

### D9. Scorer
`LoadReviewRows` reads, for findings with `storage_version = 2`, the judged candidate
rows (type, ID, record, cosine) deduplicated per finding by `judgement_order`; legacy
findings keep the description parser. Title and location still give the document level
and the span.

### D10. Backfill command `server/cmd/compliance-storage-migrate`
`--run`/`--all`, `--batch`, `--dry-run`, `--strip`, `--recovery-dir`. Readers:
artifact-fields version 3, unchecked notices, and detect-failure findings without
artifact fields; other shapes are reported and left as legacy. Per finding, in one
transaction: build records from the legacy payload, validate (labels resolve, every
verdict has a kept relevance row, counts and verdicts match, description verdict rows
and judged list match judged rows including record and cosine, language copies equal
canonical content or become non-canonical translations), insert, re-read counts and
compare, then set `storage_version = 2`. Cosine of other candidates is recomputed by
replaying the reviewer's pool order over the traces, and checked against the judged
list. Run context comes from the findings' recorded prompts, assessment date and
configured models; differences are kept in `review_flags`. Nothing is taken from
today's settings. `--strip` (step 8) runs only on validated findings: it writes a gzip
JSON-lines recovery copy of each original row, then reduces metadata to markers, sets
description `''` and clears evidence, suggestion and cited documents.

### D11. Report view and Finding Details panel
The view loads `?view=summary` and the request's compact entries. The Compliance folder
fetches summary pages of 25 with a "Load more" button and shows counters in each row.
Clicking the folder highlights all compliance findings from the compact entries, not
from the loaded pages. The Finding Details panel loads details for a compliance finding,
pages activities and candidates, expands a candidate on click, opens search history and
a search's results on request, and offers the audit export. Details are cached per
finding and language and dropped when a review action changes the finding.

### D12. Measurements
On run 116, before and after: request bytes for report, request and details calls; server
and database time from the request logs; browser rendering and PDF-highlighting time
measured separately in the browser. Results go into the devdoc; there is no target.

## Reused modules

- Shared ID Manager (20261108-01): allocates activity IDs (D4), used as documented.
- Uncatalogued code reused in place: Finding Details panel, `JsonTreeDialog`, the report
  view's highlighting through `findingShelf`, and the reviewer's grouping, search,
  relevance and judgement code. No catalogued module covers paginated lists, lazy
  details or audit export; the cursor code in `server/api/datasync/` belongs to that
  feature's sync protocol. The new store and endpoints are specific to this reviewer and
  are not registered as reusable modules.

## Risks / Trade-offs

- [Consumers outside the repo read compliance metadata or descriptions] → the export
  endpoint returns the complete record; legacy findings keep their payload.
- [Strip removes the only copy of migrated payload] → strip runs only on validated
  findings and writes a recovery file first; it is a separate, explicit step.
- [Report text changes] → the judgement section is rendered from rows in the old text
  form; only the judged-candidate list leaves the report.
- [One transaction per finding is slower than today's inserts] → findings already wait
  until the review ends; the ID range is reserved before the transaction opens.
- [Activity-ID gaps after rollbacks] → accepted; IDs only need to be unique.

## Migration Plan

1. Apply the goose migration (tables, columns, indexes).
2. Deploy the code: new compliance findings are written normalized; all readers handle
   both storage versions.
3. Run `compliance-storage-migrate --all --dry-run`, then without `--dry-run`.
4. Score run 116 before and after; compare rendered reports.
5. Run `--strip --recovery-dir …` once validation and comparisons pass.
Rollback: before step 5, legacy payloads are intact and the old code can read them; after
step 5, restore from the recovery file.

## Open Questions

None blocking. Saving results incrementally and other aspects' adoption are separate work.
