## Why

`kb.doc_review_findings` rows carry no record of which LLM model produced them. The model name is already resolved per reviewer (`ReviewerConfig.ModelName`) and threaded into every LLM call, but it is only logged in `llm_usage_event` (keyed by call, not by finding). Once a document has been reviewed with different model overrides across runs, or reviewers are configured with different models per aspect, there is no way to attribute an individual finding to the model that generated it — for QA, model-comparison analysis, or explaining an unexpected finding to a customer.

## What Changes

- Add a `ModelName` field to the `ReviewFinding` struct (`server/api/doc-reviews/review-document.go`)
- Set `ModelName` at every finding-construction site: the shared `normalizeFindingsJSON` path used by all chunk-based text reviewers, and the literal `ReviewFinding{}` constructions in the metrics, provisions, inventory-items, entities, and metrics-completeness artifact reviewers
- Add `model_name` to `FindingMetadataEnvelope` (`server/api/doc-reviews/models.go`) and to the reserved-keys list, so it is persisted into the existing `metadata JSONB` column via `prepareFindingForStorage` / `prepareFindingForStorageWithoutTranslation` (`server/api/doc-reviews/finding_translation.go`)
- Surface `model_name` on `FindingItem` so it is available to API consumers/GUI without parsing `metadata`
- No schema migration required — `metadata` is already `JSONB`
- Add a change-log entry to ADR `2026070201-adr-document-review-changes.md` noting the addition

## Capabilities

### New Capabilities

- `doc-review-findings-model-attribution`: every persisted finding records the LLM model name that generated it, in `kb.doc_review_findings.metadata.model_name` and on the API `FindingItem`

### Modified Capabilities

<!-- none -->

## Impact

- **Modified files**: `server/api/doc-reviews/review-document.go`, `server/api/doc-reviews/models.go`, `server/api/doc-reviews/finding_translation.go`, and each reviewer file that constructs a `ReviewFinding` directly (`review-metrics.go`, `review-provisions.go`, `review-inventory-items.go`, `review-entities.go`, `review-metrics-completeness.go`)
- **Documentation**: `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md` gets a change-log entry
- No database migration (existing `JSONB` column)
- No breaking changes — purely additive field on an existing JSONB blob and on the API response struct
