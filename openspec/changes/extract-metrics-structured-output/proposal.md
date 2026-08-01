## Why

The SemOS review (devdoc `2026080106`) confirmed the ADR's `extract_metrics` note is the
highest-leverage deferred change: `kb.metrics` already carries structured value columns
(`value_range_type`, `value_class`, `metric_value`, `metric_unit`) that the extraction prompt and
handler populate, yet `MetricNormalizer` ignores all of them and re-parses `threshold_or_target`
free text. That parser is where the review found fabrication and mis-parse bugs on corpus-shaped
Chinese text. The schema and extraction side are built; only the normalizer's consumption path lags.

## What Changes

- `MetricNormalizer.Normalize` consumes `kb.metrics` structured columns first:
  `value_range_type` (enum: lower_bound/upper_bound/exact/range/qualitative/limit_absent),
  `value_class`, `metric_value`, `metric_unit` → deterministic `value_form`/`comparator`/
  `assertion_kind`/`numeric_value`. No LLM in this path — pure DB→assertion mapping.
- Demote the free-text parser (`parseThresholdOrTarget`) to a **legacy fallback**: used only when
  a row has no structured value fields (pre-May rows, or LLM left the enum empty). Never fabricate.
- Handle `range` values without re-parsing free text: add numeric `value_min`/`value_max` columns
  to `kb.metrics`, populated by the extraction handler from `metric_value` when it encodes a range.
- Add the two genuinely-missing ADR fields: `condition` (applicability clause) and QUDT
  `unit_term`/`quantity_kind_term` resolution on accepted assertions (P3 log §8 item 13 was
  explicitly open).
- Keep `threshold_or_target` as the verbatim evidence/grounding anchor and fallback source.
- Re-verify against the gold-corpus record (`input_record_id=2`'s 8 metric rows) and reconcile
  with the P3 log §C2/§D2 numbers.

## Capabilities

### New Capabilities

- `metric-structured-values`: deterministic structured-value consumption in the metric normalizer,
  with `value_min`/`value_max`/`condition` schema fields and free-text parsing as legacy fallback.

### Modified Capabilities

(none — no existing `openspec/specs/` capabilities cover the SemOS assertion path yet)

## Impact

- `ChenWeb/project_migrations/*.sql` — new goose migration: add `value_min`/`value_max`/`condition`
  to `kb.metrics`.
- `ChenWeb/server/api/kbhandler/extract-metric-handler.go` — persist `value_min`/`value_max`/
  `condition` from extraction output.
- `ChenWeb/prompts/prompt-enrich-metrics-v*.md` — emit `value_min`/`value_max`/`condition` in the
  output schema (new prompt version; existing versions untouched).
- `ChenWeb/server/api/ontology/assertions/metric_normalizer.go` — structured-first consumption;
  `parseThresholdOrTarget` becomes fallback.
- `ChenWeb/server/api/ontology/assertions/metric_normalizer_test.go` — new tests: structured-row
  mapping, enum→comparator bridge, range from `value_min`/`value_max`, legacy fallback preserved.
- `ChenWeb/server/api/ontology/assertions/associate_semantics.go` — QUDT unit/quantity-kind
  resolution on accepted assertions (P3 log §8 item 13).
- Docs: ADR `2026072901` §8.2 status, P3 log §12 addendum, review devdoc "Separately" note.
