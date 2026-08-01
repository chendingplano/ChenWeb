## 1. Schema Migration

- [x] 1.1 Add goose migration `project_migrations/2026NNNNNNNNNN_extract_metrics_structured_value_fields.sql`: `ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS value_min DOUBLE PRECISION, ADD COLUMN IF NOT EXISTS value_max DOUBLE PRECISION, ADD COLUMN IF NOT EXISTS condition TEXT;` with the matching `-- +goose Down` dropping the three columns
- [x] 1.2 Verify idempotency: run the migration twice against the dev DB (via `mise dev`/goose) and confirm no error on the second run (re-ran the migration 14 SQL against `chenweb_test`: `IF NOT EXISTS` skips, no error)

## 2. Normalizer Structured-First Consumption

- [x] 2.1 Extend `metricRow` and the `Normalize` SELECT to read `value_range_type`, `value_class`, `metric_value`, `metric_unit_en`, `value_min`, `value_max` alongside `threshold_or_target` (update the Scan to match)
- [x] 2.2 Add deterministic enum→assertion mapping (D1): `lower_bound`→`>=`/`lower_bound_requirement`, `upper_bound`→`<=`/`upper_bound_requirement`, `exact`→`=`/`exact_value`, `range`→`between`/`interval_requirement` from `value_min`/`value_max`, `qualitative`→no numeric fields, `limit_absent`→no numeric fields; `value_class` refines kind (observation→observed_value, etc.)
- [x] 2.3 For structured rows, extract the numeric value from `metric_value`/`value_min`/`value_max` without invoking `parseThresholdOrTarget`; produce honest `unparsed` when the enum is set but no parseable number exists
- [x] 2.4 Keep the free-text fallback: when `value_range_type` is NULL/empty, route through `parseThresholdOrTarget` unchanged (existing parser semantics preserved)
- [x] 2.5 Populate the candidate payload with the structured-derived fields (value_form/comparator/assertion_kind/numeric_value/lower_value/upper_value) and keep `raw_text=threshold_or_target` as the evidence anchor

## 3. Normalizer Tests

- [x] 3.1 Add table-driven test: enum→assertion mapping for all six `value_range_type` values (lower_bound/upper_bound/exact/range/qualitative/limit_absent) with `sqlmock`-provided rows
- [x] 3.2 Add test: `range` row uses `value_min`/`value_max` endpoints and does NOT invoke the free-text parser (assert `parseThresholdOrTarget` not called for that row — e.g. structured row with a range-keyword-looking text that must not be regexed)
- [x] 3.3 Add test: legacy row (NULL `value_range_type`) still falls through to the parser with correct kind/numeric_value
- [x] 3.4 Add test: structured `lower_bound` with non-numeric `metric_value` produces `unparsed` (never fabricates)
- [x] 3.5 Add test: `qualitative`/`limit_absent` produce no numeric fields in the candidate payload
- [x] 3.6 Re-run existing `metric_normalizer_test.go` suite to confirm no parser regressions on the fallback path

## 4. Extraction Handler and Prompt

- [x] 4.1 Create `prompt-enrich-metrics-v5.md` adding `value_min` (number, range only), `value_max` (number, range only), `condition` (string) to the output schema; keep `threshold_or_target` as the verbatim evidence text
- [x] 4.2 Update `server/api/kbhandler/extract-metric-handler.go` map-building to read `value_min`/`value_max`/`condition` and the INSERT statement to persist them to `kb.metrics`
- [x] 4.3 Add handler fallback reads so v4 output (no new keys) still imports with NULL columns and no error
- [x] 4.4 Wire the v5 prompt into the enrichment prompt selection (check where v4 is referenced — handler default or config)

## 5. QUDT Unit / Quantity-Kind Resolution

- [x] 5.1 In `associate_semantics.processMetric`, resolve the raw `unit` string against `kb.ontology_terms` where `module_id='quantity'` (label/symbol/alias match with normalization); set `unit_term_id`/`quantity_kind_term_id` on the `Assertion` when found
- [x] 5.2 Ensure unresolved units leave term IDs NULL and the assertion is still accepted (no new deferral class)
- [x] 5.3 Add `sqlmock` tests: known unit (`ms`→`quantity:unit_MilliSEC`/`quantity:qk_Time`) resolves; unit absent from catalog (`cd/m²` — no candela term imported) leaves term IDs empty and the assertion is accepted

## 6. Gold-Corpus Re-Verification

- [x] 6.1 Run the normalizer against `input_record_id=2`'s 8 real metric rows and confirm the structured-first path reproduces §C2's correct assertions for rows 1-4 (lower_bound=250, lower_bound=1000, upper_bound=120 ×2) while rows 5-8 (v2-era `minimum`/`other` enums outside the v5 closed vocabulary) become honest `unparsed`. §C2's "6 parsed + 2 unparsed, never a fabricated value" is contradicted by the stored revision-1 candidates, which parsed all 8 — including the fabricated `observed_value=1` for "1 m 距离处清晰辨识" (the very row §C2 cited as an unparsed example) — so the outcome is 4 correct + 4 honest `unparsed`, not a regression
- [x] 6.2 Reconcile the §C2/§D2 drift (6+2 → 4+4) in the P3 log's §12 addendum: rows 1-4 parse identically; rows 5-8 carry non-contract enums pending re-extraction with `prompt-enrich-metrics-v5.md`, and honest `unparsed` (raw_text preserved) is the design-correct interim state

## 7. Docs

- [x] 7.1 Update ADR `2026072901` §8.2 `extract_metrics` status annotation: structured output now consumed; `value_min`/`value_max`/`condition` added; QUDT resolution closed
- [x] 7.2 Update P3 log `2026080103` §12 addendum: mark §8 item 10 (structured output) and §8 item 13 (unit-term resolution) closed
- [x] 7.3 Update review devdoc `2026080106` "Separately" note: `extract_metrics` decision made and implemented (link to this change)
