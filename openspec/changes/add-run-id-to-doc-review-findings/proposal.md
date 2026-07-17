## Why

`kb.doc_review_findings.metadata` already carries `model_name` (ADR 2026070201 change log) so a finding is self-describing when read outside the context of its enclosing SQL query. `run_id` is a real, indexed, `NOT NULL` column on the same table, but it is never copied into `metadata`, so any consumer that only has the metadata JSON blob in hand (exports, corrections, future report/audit tooling) cannot tell which review run produced a finding without also carrying the column value alongside it. Mirroring `run_id` into `metadata`, the same way `model_name` was mirrored, closes that gap and keeps `FindingItem` self-contained.

## What Changes

- Add `run_id` to `FindingMetadataEnvelope` (`server/api/doc-reviews/models.go`) and to the reserved-keys list, so it is persisted into the existing `metadata JSONB` column
- Add `RunID` to `FindingItem` so it is available to API consumers/GUI without a separate join, read back from `metadata.run_id` by `applyFindingMetadata`
- Set `Metadata.RunID` in `ReviewFindingsSQLStore.SaveFindings` (`server/api/doc-reviews/review-document.go`) from the `runID` parameter already passed into that function — this is the single insert path for `kb.doc_review_findings`, so no threading through individual reviewer files or `ReviewFinding` construction sites is needed (unlike `model_name`, which is resolved per-reviewer at LLM-call time and therefore had to be threaded through every `normalizeFindingsJSON` call site and literal `ReviewFinding{}` construction)
- No schema migration required — `metadata` is already `JSONB` and `run_id` already exists as a column
- Add a change-log entry to ADR `2026070201-adr-document-review-changes.md` noting the addition

## Capabilities

### New Capabilities
- `doc-review-findings-run-attribution`: every persisted finding records the review run ID that produced it, in `kb.doc_review_findings.metadata.run_id` and on the API `FindingItem`

### Modified Capabilities

<!-- none -->

## Impact

- **Modified files**: `server/api/doc-reviews/models.go`, `server/api/doc-reviews/review-document.go`
- **Documentation**: `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md` gets a change-log entry
- No database migration (existing `JSONB` column, existing `run_id` column)
- No breaking changes — purely additive field on an existing JSONB blob and on the API response struct
