## Context

Doc review findings are produced by two distinct call shapes:

1. **Chunk-based text reviewers** (`legal_compliance`, `regulatory_compliance`, grammar/clarity/correctness, etc.) — all funnel their LLM JSON response through the shared `normalizeFindingsJSON` (`review-document.go:1959-1993`), which builds `ReviewFinding` values from parsed JSON.
2. **Artifact reviewers** (`metrics`, `provisions`, `inventory_items`, `entities`, `metrics_completeness`) — each has its own loop that constructs `ReviewFinding{}` literals directly (`review-metrics.go:409`, `review-provisions.go:314`, `review-inventory-items.go:357`, `review-entities.go`, `review-metrics-completeness.go`).

Both shapes converge on `prepareFindingForStorage` / `prepareFindingForStorageWithoutTranslation` (`finding_translation.go:528`, `:703-757`), the single choke point that copies `ReviewFinding` fields into `FindingMetadataEnvelope` before `ReviewFindingsSQLStore.SaveFindings` marshals it into the `metadata JSONB` column (`review-document.go:271`).

Every call site already has access to the resolved model name via `ReviewerConfig.ModelName` (`review-document.go:177`), which is the same value passed to `newDocReviewLLMJSONInput(..., cfg.ModelName, ...)`.

## Goals / Non-Goals

**Goals:**
- Every finding row persists the model name that generated it, in `metadata.model_name`.
- Single shared assembly point (`prepareFindingForStorage*`) does the actual envelope-building, so per-reviewer changes are limited to setting one field on `ReviewFinding` before calling it.
- Field is additive/optional — historical findings without it remain valid (`model_name` absent/empty).

**Non-Goals:**
- Not backfilling `model_name` on existing rows.
- Not reconciling with `llm_usage_event` (that table remains the per-call cost/usage log; this is per-finding attribution, a different granularity — a finding may in principle draw on tool-use turns from more than one call, but we record the model of the primary generation call).
- Not exposing model name choice/config UI — this is pure data plumbing.

## Decisions

**D1 — Add `ModelName` to `ReviewFinding`, not a separate side-channel.**
`ReviewFinding` is the struct every reviewer already populates before storage; adding one field here means the existing `prepareFindingForStorage*` functions can read it directly, matching how `related_artifact_id`/`related_record_id` were added previously (ADR 2026070603). Alternative considered: pass `modelName` as an extra parameter to `prepareFindingForStorage*` instead of a struct field — rejected because it would require every call site to also change its call signature, no simpler than setting a field, and less consistent with the existing field-based pattern.

**D2 — Add `modelName` as a parameter to `normalizeFindingsJSON` itself, not a post-loop set at each call site.**
`normalizeFindingsJSON` has over 40 call sites across the chunk-based text reviewers (one per aspect file: `review-legal-compliance.go`, `review-clarity.go`, `review-grammar-spelling.go`, etc.), plus the two artifact reviewers that route through it (`review-entities.go`, `review-metrics-completeness.go`). Setting `finding.ModelName = cfg.ModelName` in a manual post-loop at each of 40+ call sites is exactly the "missed call site" risk called out below, with no safety net — a forgotten site compiles fine and silently persists an empty `model_name`.
Instead, change the signature to `normalizeFindingsJSON(payload map[string]any, modelName string) []ReviewFinding` and set `ModelName: modelName` inside the function's own construction loop (`review-document.go:1977`). Every call site must then pass `cfg.ModelName` (or equivalent) as the second argument — the compiler enforces this: an unmigrated call site is a build error, not a silent gap. Artifact reviewers with their own literal `ReviewFinding{}` constructions (`review-metrics.go:409`, `review-provisions.go:314`, `review-inventory-items.go:357`) still get `ModelName` set inline at the literal, since they don't go through `normalizeFindingsJSON`.

**D3 — `model_name` key in `FindingMetadataEnvelope`, always emitted when non-empty.**
Add `ModelName string `json:"model_name,omitempty"`` to `FindingMetadataEnvelope` and add `model_name` to the reserved-keys list (`models.go:123-135`) so it can't collide with a translation/analysis key. `omitempty` keeps old-shaped rows and any code path that doesn't yet set it from emitting a spurious empty string.

**D4 — Surface on `FindingItem` too.**
Add `ModelName string `json:"model_name,omitempty"`` to `FindingItem` (`models.go:70-96`) so API/GUI consumers don't have to parse `metadata` JSON to show which model produced a finding.

## Risks / Trade-offs

- **Missed call site** → a reviewer's findings silently persist `model_name: ""`. Mitigated for the `normalizeFindingsJSON` path by D2's signature change (compiler-enforced — a build failure, not a silent gap). For the three artifact reviewers with literal `ReviewFinding{}` constructions, mitigation is `grep -n "ReviewFinding{" server/api/doc-reviews/*.go` before considering the change done (excluding `_test.go`), plus a manual review-run smoke test confirming `model_name` is non-empty on at least one finding from each of: a chunk reviewer, an artifact reviewer with a literal construction, and an artifact reviewer routed through `normalizeFindingsJSON` (entities or metrics-completeness).
- **Model overrides mid-run** — if `ReviewerConfig.ModelName` reflects a per-reviewer override (not necessarily the literal model string sent to the provider after any internal fallback/retry-with-different-model logic), `model_name` records the *configured* model, not necessarily the exact model that ultimately serviced a retried call. This matches the granularity already used by `ReviewerConfig.ModelName` elsewhere and is treated as acceptable; exact-call attribution would require threading the LLM response's reported model back through the parsing path, out of scope here.

## Migration Plan

No database migration — `metadata` is already `JSONB`. Deploy is a single Go code change; no rollback concerns beyond a normal revert (new field is additive and `omitempty`, so removing the code is safe even against rows already written with `model_name` populated).
