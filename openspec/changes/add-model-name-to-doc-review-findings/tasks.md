## 1. Core struct and envelope changes

- [x] 1.1 Add `ModelName string` to `ReviewFinding` (`server/api/doc-reviews/review-document.go`, near the other cross-cutting fields like `RelatedArtifactID`)
- [x] 1.2 Add `ModelName string \`json:"model_name,omitempty"\`` to `FindingMetadataEnvelope` and add `model_name` to the reserved-keys list (`server/api/doc-reviews/models.go:123-135`)
- [x] 1.3 Add `ModelName string \`json:"model_name,omitempty"\`` to `FindingItem` (`server/api/doc-reviews/models.go:70-96`)

## 2. Shared assembly and parsing paths

- [ ] 2.1 In `prepareFindingForStorage` (`finding_translation.go:665-698`), copy `finding.ModelName` into both the `Canonical: ReviewFinding{...}` block and the `Metadata: FindingMetadataEnvelope{...}` block
- [ ] 2.2 In `prepareFindingForStorageWithoutTranslation` (`finding_translation.go:703-753`), copy `finding.ModelName` into both the `Canonical: ReviewFinding{...}` block and the `Metadata: FindingMetadataEnvelope{...}` block
- [x] 2.3 Change `normalizeFindingsJSON` signature to `normalizeFindingsJSON(payload map[string]any, modelName string) []ReviewFinding` (`review-document.go:1959`) and set `ModelName: modelName` in its construction loop (`review-document.go:1977`)
- [x] 2.4 In `applyFindingMetadata` (`models.go:253`), the shared read-path helper that unmarshals stored `metadata` JSON onto a `FindingItem` (called from the findings-list query in `controller.go:341-345`), copy the unmarshaled `model_name` into `FindingItem.ModelName`

## 3. Update every `normalizeFindingsJSON` call site to pass the model name

Update each call site below to pass its reviewer's model name (typically `cfg.ModelName`) as the new second argument. This is a mechanical signature migration; the compiler will fail the build on any missed site.

- [ ] 3.1 `review-assumptions.go:171`
- [ ] 3.2 `review-currency.go:162`
- [ ] 3.3 `review-evidence-rationale.go:203`
- [ ] 3.4 `review-diagrams.go:161`
- [ ] 3.5 `review-formatting-consistency.go:162`
- [ ] 3.6 `review-examples.go:162`
- [ ] 3.7 `review-internal-policy.go:153`
- [ ] 3.8 `review-clarity.go:161`
- [ ] 3.9 `review-limitations.go:167`
- [ ] 3.10 `review-completeness.go:172`
- [ ] 3.11 `review-modularity.go:147`
- [ ] 3.12 `review-metrics.go:272` (metrics conflict-detection LLM pass, distinct from the literal construction at line 409 — see task 4)
- [ ] 3.13 `review-cross-reference-correctness.go:172`
- [ ] 3.14 `review-navigability.go:145`
- [ ] 3.15 `review-conciseness.go:161`
- [ ] 3.16 `review-error-handling.go:167`
- [ ] 3.17 `review-performance.go:168`
- [ ] 3.18 `review-provisions.go:230` (distinct from the literal construction at line 314 — see task 4)
- [ ] 3.19 `review-readability.go:162`
- [ ] 3.20 `review-heading-hierarchy.go:141`
- [ ] 3.21 `review-regulatory-compliance.go:141`
- [ ] 3.22 `review-regulatory-compliance.go:223`
- [ ] 3.23 `review-correctness.go:173`
- [ ] 3.24 `review-security.go:167`
- [ ] 3.25 `review-section-balance.go:145`
- [ ] 3.26 `review-inventory-items.go:232` (distinct from the literal construction at line 357 — see task 4)
- [ ] 3.27 `review-testable-claims.go:162`
- [ ] 3.28 `review-technical-accuracy.go:168`
- [ ] 3.29 `review-localization.go:164`
- [ ] 3.30 `review-tool-loop.go:590`
- [ ] 3.31 `review-prerequisites.go:167`
- [ ] 3.32 `review-grammar-spelling.go:155`
- [ ] 3.33 `review-entities.go:170`
- [ ] 3.34 `review-internal-contradictions.go:172`
- [ ] 3.35 `review-requirement-traceability.go:174`
- [ ] 3.36 `review-logical-flow.go:140`
- [ ] 3.37 `review-standards-compliance.go:144`
- [ ] 3.38 `review-standards-compliance.go:226`
- [ ] 3.39 `review-legal-compliance.go:140`
- [ ] 3.40 `review-legal-compliance.go:222`
- [ ] 3.41 `review-tone-voice.go:159`
- [ ] 3.42 `review-metrics-completeness.go:196`
- [ ] 3.43 `review-terminology-consistency.go:173`
- [ ] 3.44 `review-relevance.go:159`
- [ ] 3.45 Run `go build ./...` from `ChenWeb/server` and fix any remaining call site the compiler flags (safety net for this task group)

## 4. Update artifact reviewers with literal `ReviewFinding{}` constructions

- [ ] 4.1 `review-metrics.go:409` — set `ModelName: cfg.ModelName` (or the in-scope equivalent) on the literal
- [ ] 4.2 `review-provisions.go:314` — set `ModelName: cfg.ModelName` on the literal
- [ ] 4.3 `review-inventory-items.go:357` — set `ModelName: cfg.ModelName` on the literal

## 5. Verification

- [ ] 5.1 `grep -n "ReviewFinding{" server/api/doc-reviews/*.go | grep -v _test.go` — confirm every non-test literal construction site sets `ModelName`
- [ ] 5.2 Run `go build ./...` and `go test ./...` in `ChenWeb/server/api/doc-reviews` (and workspace-wide `go vet ./...` per repo convention)
- [ ] 5.3 Run a live doc review (or the closest existing integration/smoke test) covering at least one chunk-based text reviewer, one artifact reviewer with a literal construction (metrics/provisions/inventory_items), and one artifact reviewer routed through `normalizeFindingsJSON` (entities or metrics_completeness); confirm `metadata.model_name` is populated on resulting `kb.doc_review_findings` rows and `model_name` appears on the API `FindingItem` response

## 6. Documentation

- [ ] 6.1 Add a change-log entry to `KnowledgeStore/doc-repo/adrs/202607/2026070201-adr-document-review-changes.md` noting that findings now carry `metadata.model_name`
