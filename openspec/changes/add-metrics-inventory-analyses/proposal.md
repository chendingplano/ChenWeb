## Why

The provisions reviewer (ADR 2026063003) now emits a mandatory per-match `analyses` array in
addition to reportable `findings`, and persists each analysis as a first-class
`kb.doc_review_findings` row (`finding_type="analysis"`). This closes a gap: previously "no
finding" was indistinguishable from "no review happened." The metrics reviewer (ADR 2026063002)
and inventory-items reviewer (ADR 2026063005) share the same artifact-based, cross-document
comparison architecture but still only emit `findings` — there is no record that a given matching
metric/item was actually compared when nothing was worth flagging. Bringing them to parity with
the provisions reviewer gives consumers (reports, coverage views) a complete picture of review
activity for all three artifact reviewers.

## What Changes

- Add a mandatory per-match `analyses` output to the metrics reviewer, mirroring the provisions
  reviewer's contract: one `analyses` entry per entry in `matching_metrics`, always present when
  matches exist, independent of whether any finding is raised.
- Add the same mandatory per-match `analyses` output to the inventory-items reviewer, one entry
  per entry in `matching_items`.
- New prompt versions `prompt-review-metrics-v3.md` and `prompt-review-inventory-items-v3.md`
  (new files; existing v1/v2 prompts are left untouched, following the provisions reviewer's
  v1→v4 progression).
- Parse `analyses` from each reviewer's LLM output, convert every entry into a `ReviewFinding` with
  `finding_type="analysis"`, `severity="info"`, `confidence=1.0`, `Pass="P5"`,
  `Aspect="metrics"|"inventory_items"`, `ArtifactID=<metric_id>|<inventory_item_id>`, and
  structured `RelatedArtifactID`/`RelatedRecordID`, and persist them through the existing
  `kb.doc_review_findings` store alongside reportable findings.
- Metadata: analysis rows carry `metadata.result_kind="metric_analysis"|"inventory_item_analysis"`
  and `metadata.analysis_relationship` (`same_subject | related_subject | unrelated`), matching the
  provisions reviewer's `ResultKind`/`AnalysisRelationship` fields already present on
  `ReviewFinding`.
- Wire the new prompt files into `doc-review.local.toml` for `reviewers.metrics` and
  `reviewers.inventory_items`.
- Update tests for both reviewers to cover analyses parsing/conversion/tagging, mirroring
  `review-provisions_test.go`.

No new database table is introduced: unlike provisions (which additionally dual-writes to a
reviewer-specific `kb.doc_review_provision_analyses` side table for historical reasons), metrics
and inventory-items analyses are written only to `kb.doc_review_findings` — there is no existing
`kb.doc_review_metric_analyses` / `kb.doc_review_inventory_item_analyses` side table to maintain,
and none will be created.

## Capabilities

### New Capabilities
- `metrics-reviewer-analyses`: mandatory per-match comparison analyses for the metrics reviewer,
  persisted as `finding_type="analysis"` rows in `kb.doc_review_findings`.
- `inventory-items-reviewer-analyses`: mandatory per-match comparison analyses for the
  inventory-items reviewer, persisted as `finding_type="analysis"` rows in
  `kb.doc_review_findings`.

### Modified Capabilities
(none — no existing `openspec/specs/` capabilities cover these reviewers yet)

## Impact

- `ChenWeb/server/api/doc-reviews/review-metrics.go` — parse/convert/tag analyses, mirroring
  `review-provisions.go`'s `parseProvisionAnalysesJSON` / `provisionAnalysesAsFindings`.
- `ChenWeb/server/api/doc-reviews/review-inventory-items.go` — same, for inventory items.
- `ChenWeb/prompts/prompt-review-metrics-v3.md` (new) and
  `ChenWeb/prompts/prompt-review-inventory-items-v3.md` (new) — require `analyses` output,
  following the provisions v3/v4 prompt diffs.
- `ChenWeb/doc-review.local.toml` — bump `reviewers.metrics.prompt` and
  `reviewers.inventory_items.prompt` to the new v3 files.
- `ChenWeb/server/api/doc-reviews/review-metrics_test.go`,
  `review-inventory-items_test.go` — new analyses test coverage.
- No migration/new table required; reuses `kb.doc_review_findings` exactly as the provisions
  reviewer does for its dual-write's primary (non-side-table) path.
