## ADDED Requirements

### Requirement: Metric normalizer consumes structured value columns as primary source

The metric normalizer SHALL derive the assertion candidate's `value_form`, `comparator`,
`assertion_kind`, and numeric fields from the `kb.metrics` structured value columns
(`value_range_type`, `value_class`, `metric_value`, `metric_unit`, `value_min`, `value_max`) when
`value_range_type` is populated, rather than from re-parsing the `threshold_or_target` free text.
The mapping from `value_range_type` to assertion fields SHALL be deterministic and MUST NOT invoke
an LLM.

#### Scenario: Lower-bound row maps to lower_bound_requirement

- **WHEN** a `kb.metrics` row has `value_range_type='lower_bound'`, `metric_value='250'`, and
  `metric_unit='cd/m²'`
- **THEN** the normalizer produces a candidate with `value_form='single'`, `comparator='>='`,
  `assertion_kind='lower_bound_requirement'`, and `numeric_value=250`

#### Scenario: Upper-bound row maps to upper_bound_requirement

- **WHEN** a row has `value_range_type='upper_bound'`, `metric_value='120'`, `metric_unit='ms'`
- **THEN** the candidate has `comparator='<='` and `assertion_kind='upper_bound_requirement'`

#### Scenario: Exact row maps to exact value

- **WHEN** a row has `value_range_type='exact'` and a numeric `metric_value`
- **THEN** the candidate has `value_form='single'`, `comparator='='`, and `numeric_value` set to
  the row's `metric_value`

#### Scenario: Qualitative row produces no numeric fields

- **WHEN** a row has `value_range_type='qualitative'`
- **THEN** the candidate has `value_form='qualitative'` and no `numeric_value`/`lower_value`/
  `upper_value` fields, and `is_explicit_metric=false` semantics (no number invented)

#### Scenario: Limit-absent row produces no numeric fields

- **WHEN** a row has `value_range_type='limit_absent'`
- **THEN** the candidate has `value_form='limit_absent'` and no numeric fields

#### Scenario: Value class refines the assertion kind

- **WHEN** a row has `value_range_type='lower_bound'` and `value_class='observation'`
- **THEN** the candidate's `assertion_kind` reflects the observed-value classification rather than
  a requirement kind

### Requirement: Range values use value_min and value_max without free-text parsing

When `value_range_type='range'`, the normalizer SHALL take the two range endpoints from the
`value_min` and `value_max` numeric columns. It MUST NOT re-parse `threshold_or_target` to recover
range endpoints for such rows.

#### Scenario: Range row maps to interval_requirement from value_min and value_max

- **WHEN** a row has `value_range_type='range'`, `value_min=500`, `value_max=2000`,
  `metric_value='500:1 至 2000:1'`, and `metric_unit='ratio'`
- **THEN** the candidate has `value_form='range'`, `comparator='between'`,
  `assertion_kind='interval_requirement'`, `lower_value=500`, and `upper_value=2000`

#### Scenario: Range row missing value_max falls back honestly

- **WHEN** a row has `value_range_type='range'` but `value_min`/`value_max` are both NULL
- **THEN** the normalizer MAY fall back to the free-text parser, and if it cannot recover both
  endpoints it produces an honest `unparsed` candidate with no fabricated endpoints

### Requirement: Free-text parser remains the legacy fallback

For rows where `value_range_type` is NULL or empty (pre-structured-schema rows, or extraction that
left the enum unset), the normalizer SHALL fall back to the existing `parseThresholdOrTarget`
free-text parser on `threshold_or_target`. The fallback MUST preserve the parser's existing
never-fabricate contract: a string with no recognizable number produces `value_form='unparsed'`
with numeric fields left nil, and the original text is preserved in the candidate payload.

#### Scenario: Legacy row without value_range_type uses the free-text parser

- **WHEN** a row has `value_range_type` NULL and `threshold_or_target='不低于250 cd/m²'`
- **THEN** the candidate is produced by the free-text parser with `assertion_kind=
  'lower_bound_requirement'` and `numeric_value=250`

#### Scenario: Unparseable legacy row produces an unparsed candidate, not a fabricated value

- **WHEN** a row has `value_range_type` NULL and `threshold_or_target='1 m 距离处清晰辨识'`
- **THEN** the candidate has `value_form='unparsed'` and no numeric fields

### Requirement: kb.metrics exposes value_min, value_max, and condition columns

The `kb.metrics` table SHALL have `value_min` and `value_max` columns of numeric type for the two
endpoints of a `range` value, and a `condition` column of text type for the applicability/scope
clause (ADR §8.2 `extract_metrics` "condition"). The columns SHALL be added via a goose migration
with `IF NOT EXISTS` semantics so the migration is idempotent. Existing rows SHALL have these
columns NULL; NULL `value_min`/`value_max` on a `range` row routes the row to the fallback path.

#### Scenario: Migration adds the columns idempotently

- **WHEN** the goose migration runs against a database that already has `kb.metrics`
- **THEN** `value_min`, `value_max`, and `condition` exist on `kb.metrics` and running the
  migration again does not error

#### Scenario: New extraction populates value_min and value_max for a range

- **WHEN** a range metric is extracted through the new prompt version
- **THEN** the extraction handler persists numeric `value_min` and `value_max` alongside
  `metric_value='500:1 至 2000:1'`

### Requirement: Extraction prompt and handler emit value_min, value_max, and condition

A new version of the metric-enrichment prompt SHALL add `value_min` (number, for `range` values),
`value_max` (number, for `range` values), and `condition` (string) to the output schema. The
extraction handler SHALL persist these fields to `kb.metrics`. Existing prompt versions' output
SHALL still import without these keys (handler uses fallback reads).

#### Scenario: v5 prompt emits value_min and value_max only for range

- **WHEN** the new prompt version emits a metric with `value_range_type='range'`
- **THEN** `value_min` and `value_max` are present as numbers, and `threshold_or_target` remains
  present as the verbatim evidence text

#### Scenario: v4 output still imports after the schema change

- **WHEN** an extraction result from the v4 prompt (no `value_min`/`value_max`/`condition` keys)
  is passed to the handler
- **THEN** the handler imports it with NULL `value_min`/`value_max`/`condition` and no error

### Requirement: QUDT unit and quantity-kind terms resolve on accepted metric assertions

When a metric candidate is accepted, `associate_semantics` SHALL resolve the raw unit string
against the QUDT `quantity` module terms in `kb.ontology_terms` and set the resulting
`unit_term_id` and `quantity_kind_term_id` on the assertion. Resolution SHALL match the raw unit
string by label/symbol/alias with normalization. If the unit cannot be resolved, the term IDs
SHALL remain NULL and the assertion SHALL still be accepted (resolution is enrichment, not a gate).

#### Scenario: Known unit resolves to QUDT term IDs

- **WHEN** an accepted metric assertion has unit string `cd/m²` and a matching QUDT unit term
  exists under `module_id='quantity'`
- **THEN** `unit_term_id` and `quantity_kind_term_id` are set on the assertion, with
  `quantity_kind_term_id` reflecting the unit's quantity kind (e.g. luminance)

#### Scenario: Unknown unit leaves term IDs NULL without deferring

- **WHEN** an accepted metric assertion has a unit string with no matching QUDT term
- **THEN** the assertion is still accepted with `unit_term_id` and `quantity_kind_term_id` NULL,
  and no deferral reason is produced for the unit

### Requirement: Structured consumption never fabricates a value

For every structured `value_range_type` path, the normalizer MUST either produce a candidate whose
numeric fields are grounded in `metric_value`/`value_min`/`value_max`, or produce an honest
`unparsed` candidate. It MUST NOT invent a number, comparator, or assertion kind that is not
supported by the row's structured fields or the deterministic enum mapping.

#### Scenario: Structured lower-bound with a non-numeric metric_value produces unparsed

- **WHEN** a row has `value_range_type='lower_bound'` and `metric_value='clearly legible'` (no
  parseable number)
- **THEN** the normalizer produces `value_form='unparsed'` rather than extracting a number from
  an unrelated part of the row

#### Scenario: Structured path skips the free-text parser entirely

- **WHEN** a row has a populated `value_range_type` and consistent `metric_value`/`value_min`/
  `value_max`
- **THEN** `parseThresholdOrTarget` is not invoked for that row, and no range-keyword regex runs
  against its text
