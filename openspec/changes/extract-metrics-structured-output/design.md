## Context

The SemOS review (devdoc `2026080106`, finding 2b) found `MetricNormalizer.Normalize`
(`server/api/ontology/assertions/metric_normalizer.go`) re-parses `kb.metrics.threshold_or_target`
free text through `parseThresholdOrTarget`, where the fabrication/mis-parse bugs live. Meanwhile
the schema and extraction side of the ADR's "changed" `extract_metrics` row are **already built**:

- `kb.metrics` has `metric_value`, `value_data_type`, `value_range_type`, `value_class`,
  `value_class_en` (migration `20260507100002_add_kb_metrics_value_fields.sql`).
- The extraction handler persists them
  (`server/api/kbhandler/extract-metric-handler.go`, v4 prompt's closed enums).
- `kb.semantic_assertions` already has `unit_term_id`/`quantity_kind_term_id` columns
  (`assertions_store.go:47-48`) that are currently left NULL (P3 log §8 item 13).
- The QUDT catalog is in `kb.ontology_terms` under `module_id='quantity'` (via `qudt-import`).

So this change is not a schema build for the value axis — it is **re-pointing the normalizer** to
consume what already exists, adding the genuinely-missing columns (`value_min`/`value_max` for
`range` without parsing, `condition` for applicability), and resolving the QUDT term IDs.

Current `kb.metrics` value axis (all `TEXT`):
`metric_value` (single or "500:1 至 2000:1"), `value_range_type`
(lower_bound|upper_bound|exact|range|qualitative|limit_absent), `value_class`
(observation|requirement|target|reference|design_capability|definition), `metric_unit`,
`threshold_or_target` (verbatim original text — the evidence anchor).

## Goals / Non-Goals

**Goals:**
- `MetricNormalizer.Normalize` derives `value_form`/`comparator`/`assertion_kind`/`numeric_value`
  deterministically from `kb.metrics` structured columns, not from re-parsing `threshold_or_target`.
- `range` values need no free-text parsing: `value_min`/`value_max` numeric columns carry the two
  endpoints.
- Accepted metric assertions get `unit_term_id`/`quantity_kind_term_id` resolved against the QUDT
  `quantity` module (P3 log §8 item 13).
- `condition` column captured for applicability clauses (ADR §8.2 `extract_metrics` row).
- Existing rows without structured fields (legacy) still normalize via `parseThresholdOrTarget`
  as a fallback — never dropped, never fabricated.
- Gold-corpus record (`input_record_id=2`, 8 metric rows) re-verified; §C2/§D2 numbers reconciled.

**Non-Goals:**
- No change to `extract_metric_definitions` (DR23 metric-term harvesting) — separate processor,
  separate phase (P3–P4), feeds `ontology_terms`/comparison row identity, not `kb.metrics`.
- No LLM path added to the normalizer. Structured consumption is pure DB→payload mapping.
- No prompt rewrite from scratch — new prompt version emits two new fields, existing versions kept.
- No transactional rewrite of `AssociateSemantics.Run` (the un-transacted multi-write sequence
  from review finding 2a remains a separate, already-documented issue).
- No changes to the free-text `parseThresholdOrTarget` semantics for rows that use it — the parser
  stays exactly as fixed in the prior session.

## Decisions

### D1. Structured columns are the primary source; free-text parser is legacy fallback

`Normalize` selects the structured columns (`value_range_type`, `value_class`, `metric_value`,
`metric_unit`, plus `metric_unit_en`) in addition to `threshold_or_target`. For each row:

- If `value_range_type` is non-empty → map deterministically:
  - `lower_bound` → `value_form=single`, `comparator=>=`, `assertion_kind=lower_bound_requirement`
  - `upper_bound` → `<=`, `upper_bound_requirement`
  - `exact` → `=`, `exact_value` (assertion kind per gold corpus; mapped into
    `governedMetricAssertionKinds` if the kind term exists)
  - `range` → `value_form=range`, `comparator=between`, `interval_requirement`, endpoints from
    `value_min`/`value_max`
  - `qualitative` → `value_form=qualitative`, no numeric fields (kind from `value_class`-derived
    mapping or `qualitative_requirement` if governed)
  - `limit_absent` → `value_form=limit_absent`, no numeric fields
  - `value_class` refines the kind: `observation`→`observed_value`, `requirement`→requirement
    kinds, `target`/`reference`/`design_capability`→their names (all within the governed set).
- Else (structured empty → legacy row) → `parseThresholdOrTarget` on `threshold_or_target`.
- Numeric extraction from `metric_value` when structured:
  - single `metric_value` (e.g. `250`) + `value_range_type=lower_bound` → `numeric_value=250`.
  - `value_min`/`value_max` present → endpoints; else a single parseable number in `metric_value`
    with a single bound → that number; otherwise honest `unparsed` (fall back to the parser).

Rationale: removes the review's fabrication class entirely for new rows (deterministic enum→assertion
mapping), keeps legacy rows working, and honors the P3 log's "never fabricate" contract structurally
rather than by parser discipline.

Alternative considered: make `extract_metrics` stop emitting `threshold_or_target`. Rejected —
`threshold_or_target` is the verbatim evidence/grounding anchor used by `EvidenceStore.AddEvidence`
and the reviewer panel; it must be preserved.

### D2. Add `value_min` / `value_max` NUMERIC columns to `kb.metrics`

`value_range_type=range` needs two endpoints, but `metric_value` is a single `TEXT` and a range like
`500:1 至 2000:1` can't be reliably split. Add:

```sql
ADD COLUMN IF NOT EXISTS value_min DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS value_max DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS condition TEXT;
```

`value_min`/`value_max` are the two numeric endpoints for `range`; for single-bound rows they are
NULL and `metric_value` carries the value. The extraction handler persists them from the prompt's
structured output (see D3). `condition` holds the applicability/scope clause (ADR §8.2 "condition").

Rationale: a `DOUBLE PRECISION` pair is the smallest shape that makes `range` fully deterministic
in the normalizer. Keeping `metric_value` as the human-visible string preserves the reviewer
panel's display.

Alternative considered: store range endpoints in `metric_value` as a canonical JSON `[min,max]`.
Rejected — collides with the panel/benchmark's string expectations and adds parse risk back.

### D3. New prompt version emits `value_min`, `value_max`, `condition`

New `prompt-enrich-metrics-v5.md` (following the repo's v1→v4 progression): the output schema adds
`value_min`/`value_max` (numbers, present only for `range`), and `condition` (string). Existing
prompts untouched; the handler reads the new keys via `firstNonEmptyVal`-style fallbacks so v4
output still imports. `threshold_or_target` remains in the schema (evidence anchor).

Rationale: keeps the established "new prompt version per schema change" convention; backward
compatible with in-flight v4 extraction.

### D4. QUDT unit/quantity-kind resolution in `associate_semantics.processMetric`

Resolve `unit_term_id`/`quantity_kind_term_id` at accept time, not in the normalizer — the
normalizer proposes; the resolver validates. Lookup against `kb.ontology_terms` where
`module_id='quantity'`, matching the raw unit string by `label`/symbol/alias (the string is
preserved in the candidate payload's `unit`). Mapped onto the `Assertion` fields already present
(`assertions_store.go:47-48`). Unresolved unit → leave term IDs NULL (current behavior), do not
defer the whole assertion — unit resolution is enrichment, not a gate. Quantity kind for
temperature-style units needing offset handling is deliberately out of scope (comparison's
`unitDef` placeholder remains authoritative for conversion).

Rationale: closes P3 log §8 item 13 with the smallest touch, reuses the already-imported catalog,
and avoids turning a currently-passing accept path into a new deferral class.

Alternative considered: resolve in the normalizer and put term IDs in the candidate payload.
Rejected — the normalizer's payload is the *proposal*; the resolver owns validation, and unit
catalog drift is exactly a validate-time concern.

### D5. Enrichment is best-effort; no new deferral class

Every new structured path must either produce a valid candidate or an honest `unparsed` that flows
to the existing `no_governed_assertion_kind_term` deferral — never an invented value. This is the
P3 log's "never fabricate" contract made structural.

## Risks / Trade-offs

- **[LLM enum instability on legacy rows]** → For rows where `value_range_type` is set, the
  mapping is deterministic; the remaining LLM-influenced surface is `metric_value` transcription.
  Gold-corpus re-verification (`input_record_id=2`) checks the 8 real rows agree across runs.
- **[`exact`/`qualitative`/`limit_absent` assertion-kind terms may not be governed yet]**
  → `governedMetricAssertionKinds` (associate_semantics.go:168) currently lists
  lower/upper/interval/observed/target/reference/capability. If a kind isn't governed, the existing
  `no_governed_assertion_kind_term` deferral fires — no invented term. Extend the map only when the
  ontology-seed actually installs the term.
- **[QUDT lookup by raw unit string is fuzzy]** → `cd/m2` vs `cd/m²`, `ms` vs `毫秒`. Matching by
  label/symbol/alias with normalization; unresolved units leave term IDs NULL (no regression to
  today's behavior). The comparison layer's `unitDef` registry remains the conversion authority.
- **[Value min/max could contradict `metric_value`]** → extraction emits all three; the normalizer
  trusts `value_min`/`value_max` for the endpoints and keeps `metric_value` as display text.
  Discrepancies are a reviewer-panel concern, not a normalizer concern (non-goal).
- **[Backward compat for v4 output]** → D3's fallback keys mean v4 JSON still imports; legacy
  rows (no enum) take the free-text parser path unchanged.

## Migration Plan

1. New goose migration: `ADD COLUMN IF NOT EXISTS value_min/value_max/condition` on `kb.metrics`.
   Backward compatible (`IF NOT EXISTS`); no data backfill needed — NULL structured fields just
   route rows to the legacy parser until re-extraction populates them.
2. Code: normalizer structured-first path + legacy fallback; handler new-key persistence.
3. New prompt v5; wire into the enrichment config.
4. QUDT resolution in `processMetric`.
5. Tests (per spec `metric-structured-values`): enum→comparator bridge table, `range` from
   value_min/max, legacy fallback preserved, gold-corpus re-verify. Run
   `go test ./server/api/ontology/assertions/...`.
6. Docs: ADR §8.2 status annotation, P3 log §12 addendum, review devdoc "Separately" note → done.
7. Rollback: revert the migration (goose down drops the columns; structured fields NULL → legacy
   parser, no data loss); code revert is a simple diff.

## Open Questions

- Does the gold corpus contain an `exact` metric row, and is an `exact_value` assertion-kind term
  installed by ontology-seed? (Determines whether D1's `exact` mapping lands on a governed kind.)
- Should `condition` also be carried onto the assertion's `qualifiers`? (Currently only
  `metric_name` is.) Minor; can be added with the assertion payload in the same change.
