## Context

The provisions reviewer (`review-provisions.go`, ADR 2026063003) already implements mandatory
per-match `analyses`: prompt v4 requires one `analyses` entry per `matching_provisions` entry,
`parseProvisionAnalysesJSON` extracts them from the raw LLM payload, `provisionAnalysesAsFindings`
converts each into a `ReviewFinding` with `FindingType="analysis"`, and the reviewer additionally
dual-writes to a legacy `kb.doc_review_provision_analyses` side table (a historical artifact of
provisions being the first reviewer built, before analyses were folded into
`kb.doc_review_findings`).

The metrics reviewer (`review-metrics.go`, ADR 2026063002) and inventory-items reviewer
(`review-inventory-items.go`, ADR 2026063005) are structurally identical to the provisions
reviewer: same `ReviewDocument` shape (load artifacts → build matches → hydrate context → load
windows → window-grouped execution), same one-LLM-call-per-artifact review unit, same two-path
call site (`cfg.MaxToolTurns > 0` tool-use loop vs. one-shot `ExtractJSON`), same finding-tagging
tail. Neither currently parses or persists `analyses` — both only convert the top-level `findings`
key. This change brings them to parity with the provisions reviewer's analyses contract, without
inheriting the side-table complexity, since no such side table exists for these two reviewers
today and none is being introduced.

## Goals / Non-Goals

**Goals:**
- One `analyses` entry per matching metric / matching item, always present when
  `matching_metrics` / `matching_items` is non-empty, regardless of whether a finding is raised.
- Analyses persisted as `kb.doc_review_findings` rows with `finding_type="analysis"`,
  `severity="info"`, `confidence=1.0`, correct `Pass`/`Aspect`/`ArtifactID`, and structured
  `RelatedArtifactID`/`RelatedRecordID` — using the `ResultKind`/`AnalysisRelationship` fields
  already on `ReviewFinding` (added for provisions; not artifact-type-specific).
- New prompt files (v3) for both reviewers; existing v1/v2 files untouched.
- Both call paths (one-shot JSON extraction and tool-use loop) support analyses parsing.
- Test coverage mirroring `review-provisions_test.go`'s analyses assertions.

**Non-Goals:**
- No new database table. Provisions' `kb.doc_review_provision_analyses` dual-write is a
  transitional/compatibility mechanism specific to that reviewer's migration history (see ADR
  2026063003 §3.4); metrics and inventory-items go straight to `kb.doc_review_findings` only.
- No change to Branch A/B/C match-building logic, windowing, scheduling, or tool registries for
  either reviewer — this change is scoped to the LLM contract (prompt + output parsing +
  persistence) for the existing review units.
- No change to the provisions reviewer itself.
- No report/Typst/UI changes to *display* analyses differently for metrics/inventory-items beyond
  what already exists generically for `finding_type="analysis"` rows (if any UI already filters by
  `finding_type`, it applies automatically since these are just more `doc_review_findings` rows).

## Decisions

**D1 — Prompt versioning: new v3 files, not edits to v1/v2.**
Follows the project's `prompt-<name>-v<n>.md` convention (ChenWeb/CLAUDE.md §2) and the
provisions reviewer's own incremental history (v1 → v2 → v3 → v4). `prompt-review-metrics-v3.md`
and `prompt-review-inventory-items-v3.md` are new files derived from the current v2 prompts plus
the `analyses` sections adapted from `prompt-review-provisions-v4.md` (the tool-use-aware
diff; metrics/inventory-items reviewers already support tool-use via `get_artifact_context`,
matching provisions' tool set). `doc-review.local.toml` is updated to point
`reviewers.metrics.prompt` / `reviewers.inventory_items.prompt` at the new v3 files.

**D2 — Reuse `ProvisionAnalysis`-shaped per-reviewer structs, not a shared generic type.**
Mirrors the existing pattern: each reviewer file defines its own small analysis struct
(`MetricAnalysis`, `InventoryItemAnalysis`) with the same four fields as `ProvisionAnalysis`
(`RelatedArtifactID`, `RelatedRecordID`, `Relationship`, `Summary`), plus its own
`parse<X>AnalysesJSON` and `<x>AnalysesAsFindings` functions. Alternative considered: extract a
single generic `parseAnalysesJSON([]byte) []Analysis` shared helper. Rejected for this change to
keep the diff minimal and consistent with the existing per-reviewer duplication (each reviewer
already has parallel-but-separate `docX`/`matchedX`/`resolvedX`/`assembleXMatches` types) —
the provisions reviewer never went through this generic-extraction step either, and doing it here
would touch code that isn't part of the request. Flagged as a candidate follow-up cleanup, not
performed now.

**D3 — Tool-use call sites switch from `runToolUseReview` to `runToolUseReviewWithPayload`.**
Both reviewers currently call `runToolUseReview(...)`, which discards the raw LLM payload and
returns only `[]ReviewFinding`. Analyses parsing needs the raw payload (the `analyses` key lives
alongside `findings` in the same JSON object), so both call sites switch to
`runToolUseReviewWithPayload(...)` — the same function the provisions reviewer already uses,
returning `(findings, payload, usage, err)`. This is a signature-compatible swap already proven by
the provisions reviewer; `runToolUseReview` remains unchanged and is still used by every other
caller.

**D4 — Metadata field values are artifact-type-specific but reuse the same `ReviewFinding`
fields.**
`ResultKind` becomes `"metric_analysis"` / `"inventory_item_analysis"` (vs. provisions'
`"provision_analysis"`); `AnalysisRelationship` uses the same three-value enum
(`same_subject | related_subject | unrelated`) already defined for provisions — no new enum
needed since the concept (does the matched artifact govern the same subject as the
artifact-under-review) is identical across artifact types.

**D5 — Title format mirrors provisions.**
`"Metric comparison: <metric_id> vs <related_artifact_id>"` and
`"Inventory item comparison: <inventory_item_id> vs <related_artifact_id>"`, matching
`"Provision comparison: <prov_id> vs <related_artifact_id>"`.

## Risks / Trade-offs

- **Row-count growth in `kb.doc_review_findings`** → same trade-off the provisions ADR already
  accepted (§6 Negative/cost); consumers must filter `finding_type='analysis'` for issue-only
  counts. No new mitigation needed beyond what's already in place for provisions.
- **Prompt regression risk**: rewriting v2 → v3 could silently change existing finding behavior
  if the analyses section isn't additive. Mitigation: base v3 on a minimal diff from v2 (same
  structure as the provisions v3→v4 diff), and keep existing "What to check" / "Reporting
  stance" / "Rules" sections behaviorally unchanged — only add the analyses section and update
  cross-references (section numbering, "Do NOT report" → "Do NOT report as findings", empty-result
  wording).
- **Tool-use payload swap (D3)**: `runToolUseReviewWithPayload` is already exercised in production
  by the provisions reviewer, so this is a low-risk mechanical substitution, not new code.

## Migration Plan

No data migration. Deploy is a code + prompt + config change:
1. Add new prompt files (dead until referenced).
2. Add analyses parsing/conversion/persistence code to both reviewers (inert until the prompt
   requires `analyses` in its output).
3. Flip `doc-review.local.toml` prompt refs to v3 for both reviewers — this is the activation
   point; a revert is just pointing the TOML back at v2.
4. No backfill: historical runs simply have no `finding_type="analysis"` rows for
   metrics/inventory_items, which is consistent with "analysis coverage started at run N."

## Open Questions

None — the provisions reviewer is a complete, shipped reference implementation for every decision
above.
