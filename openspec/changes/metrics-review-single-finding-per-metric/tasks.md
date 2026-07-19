## 1. Prompt

- [x] 1.1 Copy `ChenWeb/prompts/prompt-review-metrics-v5.md` to
      `ChenWeb/prompts/prompt-review-metrics-v6.md` (leave v5 untouched, per prompt-file
      convention).
- [x] 1.2 In v6, add a top-level `metric_summary` output field (2-4 sentences, one-time
      explanation of the metric under review) to §6 Output Format and §1/§3 wherever the
      contract is described.
- [x] 1.3 In v6 §3/§6, change `analyses[].summary` guidance so it assumes the reader already has
      `metric_summary`: it must cover only the candidate's classification, decisive evidence,
      value/context comparison, and conclusion — not restate the metric-under-review.
- [x] 1.4 Update the JSON example in §6 to include `metric_summary` alongside `analyses` and
      `findings`.

## 2. Storage model

- [x] 2.1 In `server/api/doc-reviews/review-document.go`, add
      `RelatedArtifactAnalysis` struct (`RelatedArtifactID string`, `RelatedRecordID int64`,
      `Relationship string`, `Summary string`, JSON tags matching the metadata keys) and a
      `RelatedArtifacts []RelatedArtifactAnalysis` field on `ReviewFinding`.
- [x] 2.2 In `server/api/doc-reviews/models.go`, add `RelatedArtifacts []RelatedArtifactAnalysis`
      to `FindingMetadataEnvelope`; marshal/unmarshal under a new `related_artifacts` key; add
      `"related_artifacts"` to `findingMetadataReservedKeys`.
- [x] 2.3 In `server/api/doc-reviews/finding_translation.go`, thread `finding.RelatedArtifacts`
      through in both `prepareFindingForStorage` and `prepareFindingForStorageWithoutTranslation`
      (both the `Canonical` and `Metadata` construction), matching the existing
      `RelatedArtifactID`/`ResultKind` passthrough pattern.
- [x] 2.4 In `server/api/doc-reviews/models.go`, decide/implement whether `applyFindingMetadata`
      should also populate a `FindingItem.RelatedArtifacts` field for API consumers (per
      design.md D3/Non-Goals: not required — `FindingItem.Metadata` already carries the raw JSON;
      skip this unless it turns out to be needed).

## 3. Reviewer logic

- [x] 3.1 In `server/api/doc-reviews/review-metrics.go`, add `parseMetricSummaryJSON(payload
      map[string]any) string` alongside `parseMetricAnalysesJSON`.
- [x] 3.2 Replace `metricAnalysesAsFindings(dm docMetric, analyses []MetricAnalysis)
      []ReviewFinding` with `metricAnalysesAsFinding(dm docMetric, metricSummary string, analyses
      []MetricAnalysis) *ReviewFinding` (or a 0/1-length slice, matching call-site style): builds
      one `ReviewFinding` with `Description = metricSummary`, `RelatedArtifacts` populated from
      `analyses`, and the same `Pass/Aspect/Severity/FindingType/Confidence/ArtifactID/ResultKind`
      as before. Return nil/empty when `analyses` is empty.
- [x] 3.3 Update `reviewMetric` (around line 283) to call the new function and append at most one
      finding, instead of appending N.
- [x] 3.4 Review the per-finding post-processing loop (`review-metrics.go:295-314`, the
      `matchedByID` lookup that sets `RelatedArtifactFields`) — confirm it's a no-op for the
      consolidated finding now that `RelatedArtifactID` is empty (per design.md Non-Goals; no
      code change expected here, just verify).
- [x] 3.5 Update package doc-comment on `MetricAnalysis`/`metricAnalysesAsFindings` (now
      `metricAnalysesAsFinding`) to describe the new one-row shape.

## 4. Config

- [x] 4.1 In `ChenWeb/doc-review.local.toml`, bump `reviewers.metrics.prompt` to
      `prompt-review-metrics-v6.md`.

## 5. Tests

- [x] 5.1 Update `TestParseMetricAnalysesJSON` (unchanged expectations — parsing per-entry
      `analyses` is untouched) and add `TestParseMetricSummaryJSON` covering present/absent
      `metric_summary`.
- [x] 5.2 Update `TestReviewMetric_ReturnsAnalysesAsFindings` to assert exactly one finding is
      returned for a multi-candidate `analyses` payload, with `Description` equal to
      `metric_summary` and `RelatedArtifacts` containing one entry per candidate with the right
      `related_artifact_id`/`related_record_id`/`relationship`/`summary`.
- [x] 5.3 Add a test: `analyses` present but empty (or `metric_summary` present with zero
      analyses) → zero findings from `metricAnalysesAsFinding`.
- [x] 5.4 Add/extend a `finding_translation_test.go` or `models_test.go` case round-tripping a
      `FindingMetadataEnvelope` with `RelatedArtifacts` through `MarshalJSON`/`UnmarshalJSON`,
      confirming other reviewers' envelopes (no `RelatedArtifacts` set) are unaffected
      (`related_artifacts` key omitted).
- [x] 5.5 Run `cd ChenWeb/server && go test ./api/doc-reviews/...` and confirm the full package
      passes.

## 6. Verification

- [x] 6.1 Re-run (or simulate with the existing test fixtures) the `244_mtc_2` case from the bug
      report: confirm exactly one `kb.doc_review_findings` row is produced with
      `metadata.related_artifacts` length 3 and each candidate's `relationship` preserved as
      `related_distinct`.
- [ ] 6.2 Spot-check that `description` (from `metric_summary`) does not itself repeat inside each
      `related_artifacts[].summary` for a sample LLM response under the new prompt.
