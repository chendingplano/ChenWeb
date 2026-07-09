## 1. Prompts

- [x] 1.1 Create `ChenWeb/prompts/prompt-review-metrics-v3.md` by diffing
      `prompt-review-provisions-v3.md` → `prompt-review-provisions-v4.md` and applying the
      equivalent analysis-section additions to the current
      `prompt-review-metrics-v2.md` (analyses requirement, comparison-analysis section, updated
      "Do NOT report" wording, `analyses` block in Output Format with fields
      `related_artifact_id`/`related_record_id`/`relationship`/`summary`, updated empty-result
      wording). Leave existing metrics-specific rubric content (value/threshold/unit comparisons)
      unchanged.
- [x] 1.2 Create `ChenWeb/prompts/prompt-review-inventory-items-v3.md` the same way, based on the
      current `prompt-review-inventory-items-v2.md`.
- [x] 1.3 Update `ChenWeb/doc-review.local.toml`: set `reviewers.metrics.prompt` to
      `prompt-review-metrics-v3.md` and `reviewers.inventory_items.prompt` to
      `prompt-review-inventory-items-v3.md`.

## 2. Metrics reviewer code

- [x] 2.1 In `review-metrics.go`, add a `MetricAnalysis` struct (`RelatedArtifactID`,
      `RelatedRecordID`, `Relationship`, `Summary`) mirroring `ProvisionAnalysis`.
- [x] 2.2 Add `parseMetricAnalysesJSON(payload map[string]any) []MetricAnalysis`, mirroring
      `parseProvisionAnalysesJSON`.
- [x] 2.3 Add `metricAnalysesAsFindings(dm docMetric, analyses []MetricAnalysis) []ReviewFinding`,
      mirroring `provisionAnalysesAsFindings`: `Pass="P5"`, `Aspect="metrics"`,
      `Severity="info"`, `FindingType="analysis"`, `Confidence=1.0`, `ArtifactID=dm.view.MetricID`,
      `Title="Metric comparison: <metric_id> vs <related_artifact_id or \"unlinked match\">"`,
      `Description=analysis.Summary`, `Evidence` built the same way as provisions'
      (`metric_under_review=...; related_artifact_id=...; related_record_id=...;
      relationship=...`), `ResultKind="metric_analysis"`,
      `AnalysisRelationship=analysis.Relationship`.
- [x] 2.4 In `reviewMetric`, switch the tool-use branch from `runToolUseReview` to
      `runToolUseReviewWithPayload`, capturing the returned payload (see D3 in design.md;
      `review-provisions.go`'s `reviewProvision` is the reference for the call-site shape).
      Keep existing usage/error handling; only add the payload capture.
- [x] 2.5 After building `findings` in `reviewMetric` (both call paths converge to a `payload
      map[string]any` — the one-shot path already has `out`), call
      `parseMetricAnalysesJSON(payload)`, append `metricAnalysesAsFindings(dm, analyses)` to
      `findings`, matching the provisions reviewer's `reviewProvision` structure (parse →
      append; no side-table write, per D2/Non-Goals in design.md).
- [x] 2.6 Verify the existing tagging loop at the end of `reviewMetric` (Pass/Aspect/ArtifactID/
      FindingType/Severity/Location defaults) does not clobber the values already set on analysis
      rows — it must only fill *empty* fields, matching the provisions reviewer's equivalent loop.

## 3. Inventory-items reviewer code

- [x] 3.1 In `review-inventory-items.go`, add an `InventoryItemAnalysis` struct mirroring
      `ProvisionAnalysis`.
- [x] 3.2 Add `parseInventoryItemAnalysesJSON(payload map[string]any) []InventoryItemAnalysis`.
- [x] 3.3 Add `inventoryItemAnalysesAsFindings(di docInventoryItem, analyses
      []InventoryItemAnalysis) []ReviewFinding`: `Pass="P5"`, `Aspect="inventory_items"`,
      `Severity="info"`, `FindingType="analysis"`, `Confidence=1.0`, `ArtifactID=di.view.ItemID`,
      `Title="Inventory item comparison: <inventory_item_id> vs <related_artifact_id or
      \"unlinked match\">"`, `Description=analysis.Summary`, `Evidence` built analogously,
      `ResultKind="inventory_item_analysis"`, `AnalysisRelationship=analysis.Relationship`.
- [x] 3.4 In `reviewItem`, switch the tool-use branch from `runToolUseReview` to
      `runToolUseReviewWithPayload`, capturing the returned payload.
- [x] 3.5 After building `findings` in `reviewItem`, parse and append analyses the same way as
      task 2.5.
- [x] 3.6 Verify the tagging loop only fills empty fields, same as task 2.6.

## 4. Tests

- [x] 4.1 In `review-metrics_test.go`, add test coverage mirroring
      `review-provisions_test.go`'s analyses assertions: parsing `analyses` from a payload,
      converting to `ReviewFinding` with correct tag values, and the one-shot-path /
      tool-use-path payload plumbing (per spec scenarios in
      `specs/metrics-reviewer-analyses/spec.md`).
- [x] 4.2 In `review-inventory-items_test.go`, add the equivalent coverage per
      `specs/inventory-items-reviewer-analyses/spec.md`.

## 5. Verification

- [x] 5.1 `cd ChenWeb && go build ./...`
- [x] 5.2 `cd ChenWeb && go test ./server/api/doc-reviews/...` (186 passed, 0 failed)
- [x] 5.3 `go vet ./ChenWeb/... ./shared/go/...` — clean. Note: literal
      `cd /Users/cding/Workspace && go vet ./...` fails with a pre-existing, unrelated tooling
      error ("directory prefix . does not contain modules listed in go.work") because the
      workspace root itself is not a `go.work` `use` entry; this reproduces identically on
      `main` and is not caused by this change. Vetting the two affected modules directly
      (`ChenWeb`, `shared/go`) is the equivalent check and passes clean.
