## Why

The metrics reviewer (ADR 2026063002) currently persists one `kb.doc_review_findings` row per
*(metric-under-review, candidate)* pair via `metricAnalysesAsFindings`. For a metric with 3
matching candidates, this writes 3 separate rows, each repeating the same "here is the metric
under review" framing inside its `summary` before getting to the candidate-specific reasoning
(see `KnowledgeStore/doc-repo/adrs/202606/2026063002-adr-doc-reviewer-metric.md`, e.g. metric
`244_mtc_2` produced 3 rows for candidates `306_mtc_29`, `306_mtc_73`, `306_mtc_93`, all
`related_distinct`). This is redundant storage and redundant reading: the metric-under-review
context is identical across all 3 rows, and there is no single place that shows "here is
everything found for this metric" without querying and re-grouping 3 rows.

## What Changes

- **BREAKING** (data shape, not schema): the metrics reviewer's comparison-analysis output
  changes from one `kb.doc_review_findings` row per candidate to exactly one row per
  metric-under-review, covering all of that metric's candidates.
- The row is only written when the metric has at least one candidate analysis; a metric with a
  match list that yields zero analyses (should not normally happen, but defensively handled)
  produces no row, same as a metric with no matches at all (unchanged: matching already gates the
  LLM call).
- `kb.doc_review_findings.metadata` for this row gains a new `related_artifacts` array (one entry
  per candidate: `related_artifact_id`, `related_record_id`, `relationship`, `summary`). The
  existing singular `metadata.related_artifact_id` / `related_record_id` / `analysis_relationship`
  keys are left as-is for every other finding type (conflict `issue`/`observation` findings,
  provisions/inventory-items analyses) — this change only touches the metrics comparison-analysis
  row shape.
- Prompt `prompt-review-metrics-v5.md` → new `prompt-review-metrics-v6.md`: the LLM now emits one
  `metric_summary` string (explaining the metric-under-review once) plus per-candidate
  `analyses[].summary` that covers only the candidate-specific relationship/evidence — the
  metric-under-review explanation is no longer repeated in every `analyses[].summary`.
- `ChenWeb/doc-review.local.toml` `reviewers.metrics.prompt` bumped to the new prompt file.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `metrics-reviewer-analyses`: changes the persisted row shape from one row per candidate to one
  row per metric-under-review with a `related_artifacts` array in `metadata`. (This capability was
  defined in the prior change `add-metrics-inventory-analyses` but never archived to
  `openspec/specs/`, so this change's spec file is written as the current full state of the
  capability, not a delta.)

## Impact

- `ChenWeb/server/api/doc-reviews/review-metrics.go` — `metricAnalysesAsFindings` becomes
  `metricAnalysesAsFinding` (singular), building one `ReviewFinding` from the full `analyses` list
  plus a new `metric_summary` string; `parseMetricAnalysesJSON` gains a sibling
  `parseMetricSummaryJSON`.
- `ChenWeb/server/api/doc-reviews/review-document.go` — `ReviewFinding` gains
  `RelatedArtifacts []RelatedArtifactAnalysis`.
- `ChenWeb/server/api/doc-reviews/models.go` — `FindingMetadataEnvelope` gains
  `RelatedArtifacts []RelatedArtifactAnalysis`, marshaled/unmarshaled under a new
  `related_artifacts` key, reserved in `findingMetadataReservedKeys`.
- `ChenWeb/server/api/doc-reviews/finding_translation.go` — `prepareFindingForStorage` and
  `prepareFindingForStorageWithoutTranslation` copy `RelatedArtifacts` through to both the
  canonical `ReviewFinding` and the stored `FindingMetadataEnvelope`, same pattern as the other
  passthrough metadata fields.
- `ChenWeb/prompts/prompt-review-metrics-v6.md` (new) — adds `metric_summary` to the output
  contract, trims `analyses[].summary` scope.
- `ChenWeb/doc-review.local.toml` — `reviewers.metrics.prompt` → `prompt-review-metrics-v6.md`.
- `ChenWeb/server/api/doc-reviews/review-metrics_test.go` — update
  `TestReviewMetric_ReturnsAnalysesAsFindings` and `TestParseMetricAnalysesJSON` for the new
  single-row/array shape; add a `TestParseMetricSummaryJSON` and a no-analyses-produces-no-row
  case.
- Not changed (explicitly out of scope, called out in design.md): `report.go`'s
  `buildRelatedSources` / `ReportFinding.Related` / `RelatedArtifactFields` snapshot, which are
  single-related-artifact based and will simply stay empty for this consolidated row (same as
  today for any finding with no single related artifact) — no PDF report template change.
