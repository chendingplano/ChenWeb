## ADDED Requirements

### Requirement: Symmetric tolerances are upper bounds

The system SHALL store a row whose `metric_value` is exactly `±x` as `upper_bound` when, after
range-type canonicalisation, its `value_range_type` is `exact` or `range`. The value keeps
its ± sign. A numeric row of the form `a±b` SHALL be `range`, retaining that value and deriving
endpoints `a-b` and `a+b` if its original comparator was `exact`, `lower_bound` or `upper_bound`.
Asymmetric tolerances SHALL NOT be changed.

#### Scenario: Tolerance stored as exact
- **WHEN** a row has `metric_value` `±2` and `value_range_type` `exact`
- **THEN** the row is stored with `metric_value` `±2` and `value_range_type` `upper_bound`

#### Scenario: Interval with tolerance is unchanged
- **WHEN** a row has `metric_value` `20±5` and `value_range_type` `range`
- **THEN** `value_range_type` stays `range`, and `value_min` 15 / `value_max` 25 are filled

#### Scenario: Centre with tolerance misclassified as an upper bound
- **WHEN** a row has `metric_value` `65±5` and `value_range_type` `upper_bound`
- **THEN** it becomes `range` with endpoints 60 and 70

#### Scenario: Non-range endpoints
- **WHEN** a numeric row has a comparator other than `range`
- **THEN** its `value_min` and `value_max` are NULL, even if the model supplied them

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

The system SHALL retry affected candidates once, in smaller evidence-preserving batches, when a
kept Pass 2 row's `metric_value` holds more than one number joined by a list separator
(、 ； ; ，; not `/`, which is part of units such as 次/min) or by sex labels (男…女).
The retry SHALL use the original evidence and return separate rows with their own units, properties
and conditions. A replacement SHALL contain multiple rows for the requested candidate, each with
a name, value and source lines, without combined values or drop tags. Unrequested candidates SHALL
be ignored. Retry source lines SHALL be contained in the original candidate or original row
evidence. Every numeric token from the combined rows SHALL survive, with its sign and
multiplicity, in the distinct replacement values; equivalent rewrites that cannot pass this
conservative check SHALL retain the original rows. Non-combined original siblings SHALL be
preserved for final deduplication.
An errored, missing or invalid replacement SHALL leave the original rows unchanged.
For any rows still holding combined values after recovery, the run SHALL write one
`kb.doc_proc_logs` warning that lists such rows.

#### Scenario: Merged cell values
- **WHEN** a row has `metric_value` `男300 mm、女250 mm`
- **AND** the corrective enrichment fails to return valid separate rows
- **THEN** the row is stored unchanged, and the run's log lists its metric name and value under
  `multi_value_rows`

#### Scenario: Percentage positions and a point count
- **WHEN** a row has `metric_value` `20%、80%及中间区段任意2~3个`
- **AND** corrective enrichment returns valid separate rows
- **THEN** the combined row is replaced with percentage-position rows `20` and `80` (unit `%`)
  and an intermediate-point count `2~3` (unit `个`, type `range`, endpoints 2 and 3)
- **AND** each row keeps the original candidate identity and supporting clause's source lines
