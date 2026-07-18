## Context

`metricsReviewer.reviewMetric` (`review-metrics.go`) already gates the LLM call correctly — it
only calls the LLM for a doc metric that has ≥1 cross-document match (`review-metrics.go:126-139`
in `ReviewDocument`). The LLM returns two arrays: `findings` (real conflicts, unchanged by this
proposal) and `analyses` (mandatory, one entry per candidate, added by the prior
`add-metrics-inventory-analyses` change). The bug is downstream:
`metricAnalysesAsFindings(dm, analyses)` (`review-metrics.go:396-441`) converts *each* `analyses`
entry into its *own* `ReviewFinding`, so N candidates → N stored rows. Each candidate's `summary`
text also independently re-explains the metric-under-review (per prompt §3, "each analyses entry
must state: relationship / value comparison / context comparison / conclusion" — nothing tells the
model the metric-under-review context is shared across entries).

`kb.doc_review_findings.metadata` is `JSONB` (no migration needed for a shape change).
`FindingMetadataEnvelope` (`models.go`) already round-trips a fixed set of scalar keys
(`related_artifact_id`, `related_record_id`, `result_kind`, `analysis_relationship`, ...) plus
arbitrary language codes for i18n. Adding one more reserved key (`related_artifacts`, an array) is
additive and does not disturb the i18n language-code detection (`findingMetadataReservedKeys`).

## Goals / Non-Goals

**Goals:**
- One `kb.doc_review_findings` row per metric-under-review for the comparison-analysis result,
  never one per candidate.
- No row at all when there are no candidate analyses (mirrors the existing no-match gate).
- `metadata.related_artifacts[]` carries each candidate's `related_artifact_id`,
  `related_record_id`, `relationship`, and a `summary` that is candidate-specific only.
- The metric-under-review explanation is written once (new `metric_summary` field → row
  `description`), not duplicated per candidate.
- Keep the change scoped to the `metrics` reviewer only. `provisions` and `inventory_items`
  reviewers use the same one-row-per-candidate pattern (`review-provisions.go`,
  `review-inventory-items.go`) but were not mentioned in the request and are not touched.

**Non-Goals:**
- Not changing the `findings` (real conflict) path — conflicts stay one row per conflict, as
  today; only the informational `analyses`-derived row is being consolidated.
- Not extending `report.go` (`buildRelatedSources`, `ReportFinding.Related`,
  `RelatedArtifactFields`) to render N related-artifact source blocks for the consolidated row.
  Those are single-related-artifact constructs (ADR 2026071603) and already tolerate an empty
  `RelatedArtifactID` (they just render nothing). Making the PDF report show per-candidate source
  context for this row is a real follow-up but out of scope here — not requested, and it would
  require a plural `Related` shape in the report template, which is a bigger surface than this
  storage-shape fix.
- Not adding a dedicated `RelatedArtifacts` field to `FindingItem` (the API-facing struct) beyond
  what's needed for round-tripping through storage. The frontend `finding-details-panel.svelte`
  already receives the full raw `metadata` JSONB via `FindingItem.Metadata` and renders it
  generically; it doesn't special-case `related_artifact_id` today either. If the panel later wants
  a dedicated per-candidate table, that's a frontend-only follow-up reading `metadata.related_artifacts`
  directly — no backend change needed for that.
- Not touching `provisions`' separate `kb.doc_review_provision_analyses` side table — unrelated
  reviewer, unrelated storage path.

## Decisions

**D1 — Consolidate in Go (`review-metrics.go`), not in the prompt.**
The LLM still emits one `analyses` entry per candidate (unchanged array shape — this is the
natural unit for the model to reason in, and keeps the "screen every candidate" contract from
ADR 2026063002 DR1 intact). Go-side `metricAnalysesAsFindings` becomes `metricAnalysesAsFinding`
(singular return, at most one `*ReviewFinding`) and folds the `analyses` slice into
`RelatedArtifacts` on a single finding, instead of emitting one finding per entry.
*Alternative considered:* have the LLM emit a single pre-aggregated JSON blob instead of an array.
Rejected — arrays keep the per-candidate output schema (and the existing hard-gate self-check in
prompt §4/§7, which cross-checks `findings[].related_artifact_id` against each `analyses` entry)
unchanged; only the Go-side conversion changes.

**D2 — Add `metric_summary` as a new top-level LLM output field, stored as the row's
`description`.**
Prompt v6 asks for one `metric_summary` (2-4 sentences: what the metric-under-review measures,
its role/threshold nature) written once, and instructs `analyses[].summary` to assume the reader
already has `metric_summary` — cover only the candidate's classification, decisive evidence, and
conclusion.
*Alternative considered:* synthesize `description` in Go by concatenating candidate summaries.
Rejected — that doesn't solve the actual duplication (each candidate summary still contains the
repeated metric-under-review framing); the fix has to happen in what the LLM writes into
`analyses[].summary`, which requires the prompt change regardless. Once the prompt separates the
two, Go can just take `metric_summary` verbatim.

**D3 — Metadata shape: new `related_artifacts` array key, existing singular keys untouched.**
```json
{
  "result_kind": "metric_analysis",
  "related_artifacts": [
    {"related_artifact_id": "306_mtc_29", "related_record_id": 306, "relationship": "related_distinct", "summary": "..."},
    {"related_artifact_id": "306_mtc_73", "related_record_id": 306, "relationship": "related_distinct", "summary": "..."}
  ]
}
```
The row's top-level `RelatedArtifactID`/`RelatedRecordID`/`AnalysisRelationship` (singular) are
left empty for this row — there is no single "the" related artifact anymore. `ResultKind` stays
`"metric_analysis"` (row-level tag, unchanged). This is additive to `FindingMetadataEnvelope`, so
`provisions`/`inventory_items`/conflict findings, which never populate `RelatedArtifacts`, are
byte-identical to before (`related_artifacts` key simply omitted, same as any other empty
optional field in that struct's `MarshalJSON`).
*Alternative considered:* keep the first candidate in the singular `RelatedArtifactID` fields
(for backward compat with `report.go`'s single-related-artifact rendering) and put the rest in the
array. Rejected — arbitrary "first wins" is misleading (it's not more important than the others)
and reintroduces exactly the kind of implicit precedence the user asked to remove.

**D4 — Empty-analyses guard.**
`metricAnalysesAsFinding` returns `nil` when the parsed `analyses` slice is empty (mirrors current
per-entry skip-if-empty-summary behavior, just at the whole-finding level now). Since
`ReviewDocument` already only calls the LLM for metrics with ≥1 match, and the prompt's hard rule
is "`analyses` must never be empty when `matching_metrics` is non-empty" (§8), this is a defensive
fallback (e.g. a malformed LLM response), not the primary gate — the primary "skip if no matches"
gate is unchanged, upstream, in `ReviewDocument`.

## Risks / Trade-offs

- **[Risk]** Any external consumer that queried `kb.doc_review_findings` expecting one row per
  `(metric, candidate)` pair (e.g. a saved dashboard query counting rows) will see row counts drop
  and will need to unnest `metadata.related_artifacts` instead. → **Mitigation**: called out as
  **BREAKING** in proposal.md; no code in this repo currently queries per-candidate row counts for
  metrics analyses (verified: `finding-details-panel.svelte` and `report.go` both already treat
  `RelatedArtifactID` as optional/single, nothing counts analysis rows by candidate).
- **[Risk]** `description` now holds `metric_summary`, a *summary of the metric*, not a
  *conclusion about the comparison* — readers scanning `description` alone (e.g. the Finding
  Details panel, which shows `description` verbatim) lose the "so what happened" framing that used
  to be in each per-candidate row. → **Mitigation**: this is inherent to consolidating N
  conclusions into 1 row; the per-candidate conclusions are still fully present in
  `metadata.related_artifacts[].summary`, just one level down. Not fixed further here — flagged
  as a UI follow-up if it turns out to matter in practice.
- **[Risk]** Prompt v6 is a new prompt file; behavior drift is possible until validated against
  real documents (same class of risk as every prior prompt version bump v1→v5). → **Mitigation**:
  same validation pattern used for v5 (ADR 2026063002 change log): run against the same
  `244_mtc_2` case from the bug report and confirm one row, `related_artifacts` length 3,
  `related_distinct` preserved per candidate, and no metric-under-review restatement inside any
  `analyses[].summary`.

## Migration Plan

No database migration (`metadata` is `JSONB`). Deploy is a normal code + prompt + config change:
1. Ship `prompt-review-metrics-v6.md`, leave `v5` in place (untouched, historical, per existing
   `ChenWeb/CLAUDE.md` prompt file convention — never edit a shipped prompt file in place).
2. Bump `doc-review.local.toml` `reviewers.metrics.prompt` to `v6`.
3. Go code change ships in the same deploy (the parser must match the prompt version in use).
4. No backfill of existing rows — old per-candidate rows already in `kb.doc_review_findings` stay
   as-is; only new review runs use the consolidated shape. (Consistent with how `run_id`-scoped
   findings already work: `PostProcessIndex` deletes-and-rewrites a run's findings, so a re-run of
   an existing document naturally replaces old-shape rows with new-shape ones for that run.)

## Open Questions

None — the request (File1 ADR excerpt) was concrete enough that no product ambiguity remains.
