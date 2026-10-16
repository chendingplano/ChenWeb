## ADDED Requirements

### Requirement: Symmetric tolerances are upper bounds

The system SHALL store a row whose `metric_value` is exactly `±x` as `upper_bound` when, after
range-type canonicalisation, its `value_range_type` is `exact` or `range`. The value keeps
its ± sign. Values of the form `a±b` and asymmetric tolerances SHALL NOT be changed.

#### Scenario: Tolerance stored as exact
- **WHEN** a row has `metric_value` `±2` and `value_range_type` `exact`
- **THEN** the row is stored with `metric_value` `±2` and `value_range_type` `upper_bound`

#### Scenario: Interval with tolerance is unchanged
- **WHEN** a row has `metric_value` `20±5` and `value_range_type` `range`
- **THEN** `value_range_type` stays `range`, and `value_min` 15 / `value_max` 25 are filled

### Requirement: metric_value holds no unit or comparator

A copy of the row's own `metric_unit` written next to a number inside `metric_value` SHALL be
removed when the remaining value still parses, and a single leading comparator SHALL be removed
from a single-number value. A strict comparator (> or <) SHALL add the tag `strict_bound`.

#### Scenario: Unit repeated in a range
- **WHEN** a row has `metric_value` `5 s~25 s` and `metric_unit` `s`
- **THEN** it is stored as `5~25`

#### Scenario: Composite unit is not stripped partially
- **WHEN** a row has `metric_value` `120次/min` and `metric_unit` `次`
- **THEN** `metric_value` is unchanged

#### Scenario: Leading comparator
- **WHEN** a row has `metric_value` `≥500` and `value_range_type` `lower_bound`
- **THEN** it is stored as `500`

### Requirement: Range endpoints are derived when missing

A `range` row without both `value_min` and `value_max` SHALL get them from a `metric_value` that is
a plain interval (`a~b`, `a～b`, `a-b`) or `a±b`. Other values SHALL leave the endpoints NULL.

#### Scenario: Plain interval
- **WHEN** a `range` row has `metric_value` `100~9999` and no endpoints
- **THEN** `value_min` is 100 and `value_max` is 9999

### Requirement: Rows holding several values are reported

The system SHALL keep unchanged a row whose `metric_value` still holds more than one number joined
by a list separator (、 ； ; ，; not `/`, which is part of units such as 次/min) or by sex labels (男…女). The run SHALL write one
`kb.doc_proc_logs` warning that lists such rows.

#### Scenario: Merged cell values
- **WHEN** a row has `metric_value` `男300 mm、女250 mm`
- **THEN** the row is stored unchanged, and the run's log lists its metric name and value under
  `multi_value_rows`
