## Context

`kb.doc_review_findings` has a real `run_id BIGINT NOT NULL` column (added in `20260628000002_rebuild_doc_review_tables.sql`, replacing an earlier `review_run_id TEXT` soft-link) with its own index (`idx_doc_review_findings_run`). Every read path in `server/api/doc-reviews` (`controller.go:GetRequestWithFindings`, `typst_report.go`, `auto_fix.go:loadActiveFindings`) already filters `WHERE run_id = $N` for a single, known run, so the column itself is never ambiguous in context. The just-completed `add-model-name-to-doc-review-findings` change established the precedent of also mirroring a per-finding attribute into `metadata JSONB` (via `FindingMetadataEnvelope`) so a `FindingItem`/finding row is self-describing independent of the query that produced it — useful for exports, corrections, or any future consumer that only has the metadata blob.

`model_name` needed threading through every reviewer file because it is resolved per-reviewer at LLM-call time and varies per finding within a single `SaveFindings` call. `run_id`, by contrast, is uniform for an entire `SaveFindings` call: there is exactly one call site (`review-document.go:1299`, `p.FindingsStore.SaveFindings(ctx, recordID, p.RunID, allFindings)`), and `runID` is already a function parameter of `SaveFindings` itself.

## Goals / Non-Goals

**Goals:**
- `kb.doc_review_findings.metadata.run_id` is populated for every finding, mirroring the existing real `run_id` column.
- `FindingItem.RunID` is populated on read, following the same `applyFindingMetadata` read-back path used for `model_name`.

**Non-Goals:**
- Not adding `RunID` to the `ReviewFinding` struct or threading it through `normalizeFindingsJSON` / reviewer files — unnecessary since the value is uniform per `SaveFindings` call and already available there as a parameter.
- Not changing the real `run_id` column, its index, or any existing `WHERE run_id = $N` query.
- Not backfilling `metadata.run_id` on existing rows.

## Decisions

- **Set `Metadata.RunID` inside `SaveFindings`, not in `prepareFindingForStorage`/`prepareFindingForStorageWithoutTranslation`.** Those two functions take a single `ReviewFinding` with no run context. Rather than adding a `runID` parameter to both (and to `preparedFindingForStorage`), `SaveFindings` already loops over `preparedFindings` once after calling them (to bind SQL params); this loop sets `prepared.Metadata.RunID = runID` immediately before `json.Marshal(prepared.Metadata)`. Keeps the run-id concern local to the one function that owns it.
- **Reuse the existing flat-JSON envelope pattern** (`m["run_id"] = e.RunID` in `MarshalJSON`, reserved key + `UnmarshalJSON` case, `findingMetadataReservedKeys["run_id"] = true`) rather than inventing a new metadata shape, for consistency with `model_name`, `related_record_id`, etc.
- **Omit `run_id` from the marshaled JSON when zero** (`if e.RunID != 0 { m["run_id"] = e.RunID }`), consistent with how `RelatedRecordID` is handled, so old rows without the key don't need a migration and `FindingItem.RunID` naturally reads back as `0` (Go zero value) for them.

## Risks / Trade-offs

- [Duplication of `run_id` between the real column and `metadata`] → Same trade-off already accepted for `model_name`; the column remains the source of truth for querying/filtering, `metadata.run_id` is a denormalized convenience for consumers holding only the JSON blob.
- [Existing rows have no `metadata.run_id`] → Acceptable; no backfill requested, `FindingItem.RunID` will read as `0` for pre-existing findings until they are re-reviewed.
