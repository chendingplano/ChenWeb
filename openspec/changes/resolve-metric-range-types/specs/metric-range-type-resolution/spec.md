## ADDED Requirements

### Requirement: Admin Page Placement
The system SHALL provide a "Resolve Metric Range Types" page reachable only from
System Admin → Database Maintenance in the home3 admin nav, as a sibling of the
existing "Resolve Ambiguous Objects" entry. The page SHALL require no URL route of
its own (client-side nav selection, matching the existing admin pages) and SHALL
require no backend permission beyond the standard authenticated `/api/v1` session.

#### Scenario: Page reachable from admin nav
- **WHEN** an authenticated user expands System Admin → Database Maintenance in the
  home3 nav rail
- **THEN** a "Resolve Metric Range Types" entry is shown alongside "Resolve Ambiguous
  Objects", and selecting it renders the page without a URL change

### Requirement: Errored Metrics Search
The Left panel SHALL provide search controls to filter `kb.metrics` rows whose
`value_range_type_error` is non-empty by: exact input record ID, a created-at date
range, and an "error type" selected from the set of `raw_value` entries in
`kb.metric_value_range_type_map` whose `status` is not `approved`. Filters SHALL be
combinable (AND semantics) and the list SHALL update when a search is submitted.

#### Scenario: Filter by record ID
- **WHEN** a user enters an input record ID and searches
- **THEN** only errored metric rows with that `input_record_id` are shown

#### Scenario: Filter by time range
- **WHEN** a user sets a created-at start and/or end date and searches
- **THEN** only errored metric rows created within that range are shown

#### Scenario: Filter by error type
- **WHEN** a user selects a raw `value_range_type` value from the error-type dropdown
  and searches
- **THEN** only errored metric rows whose `value_range_type` normalizes to that raw
  value are shown

### Requirement: Errored Metrics Result List
The Left panel SHALL show search results as a list of `kb.metrics` rows, each
identifying at minimum the metric name and the offending `value_range_type`. Clicking
a list entry SHALL select it as the current record.

#### Scenario: Selecting a result
- **WHEN** a user clicks a row in the results list
- **THEN** that row becomes the selected record and the Information Block and PDF
  Display update to reflect it

### Requirement: Metric Information Block
When a record is selected, the Right panel SHALL show an Information Block containing
the metric's name, description, context, metric value, value data type, value range
type, and the full `value_range_type_error` message.

#### Scenario: Information Block reflects selection
- **WHEN** a user selects a metric row with `value_range_type_error = 'unmapped
  value_range_type: "threshold"'`
- **THEN** the Information Block shows that exact error text along with the metric's
  name, description, context, value, value data type, and value range type

### Requirement: PDF Source Highlight
When a record is selected, the Right panel's PDF Display SHALL render the source PDF
for the metric's `input_record_id` and highlight the page region(s) corresponding to
the metric's `source_line_spans`, using the same raw-line/bounding-box mechanism the
existing Metrics page uses (`GET /kb/raw-lines`).

#### Scenario: Highlight follows selection
- **WHEN** a user selects a metric row
- **THEN** the PDF Display jumps to the first page containing the metric's source
  lines and draws a highlight over those lines' bounding boxes

### Requirement: Value Range Type Map Listing
The page SHALL provide a Map Block listing every `kb.metric_value_range_type_map`
entry (`raw_value`, `canonical_bucket`, `status`), visually distinguishing entries
whose `status` is not `approved` as invalid, sorted with invalid entries first.

#### Scenario: Invalid entries are distinguishable
- **WHEN** the Map Block loads
- **THEN** every entry with `status != 'approved'` is visually flagged as invalid and
  listed before entries with `status = 'approved'`

### Requirement: Canonical Bucket Editor
Each Map Block entry SHALL expose its `canonical_bucket` as an editable combobox
pre-populated with the four known buckets (`lower_bound`, `upper_bound`, `exact`,
`range`) while still accepting arbitrary free-text input, since the underlying column
carries no database-level enum constraint.

#### Scenario: Choosing a known bucket
- **WHEN** a user opens the canonical bucket combobox on an entry
- **THEN** the four known buckets are offered as selectable options and the user may
  also type a value not in that list

### Requirement: Add New Map Entry
The Map Block SHALL let a user add a new `kb.metric_value_range_type_map` entry by
supplying a `raw_value` and a `canonical_bucket`. Submitting SHALL create the entry
with `status = 'approved'` if it does not already exist, or update it in place
(same behavior as applying a correction, per the Apply requirement below) if it does.

#### Scenario: Adding a brand-new raw value
- **WHEN** a user enters a `raw_value` that has no existing
  `kb.metric_value_range_type_map` row and a `canonical_bucket`, then submits
- **THEN** a new row is created with that `raw_value`, `canonical_bucket`, and
  `status = 'approved'`

### Requirement: Apply Correction with Cascade
Setting a `canonical_bucket` on a Map Block entry and applying it SHALL persist the
entry with `status = 'approved'`, SHALL invalidate the in-process governed-mapping
cache so subsequent lookups see the correction immediately, and SHALL clear
`value_range_type_error` on every `kb.metrics` row whose normalized `value_range_type`
equals that entry's `raw_value` and still carries a non-empty
`value_range_type_error`. The number of corrected rows SHALL be reported back to the
user.

#### Scenario: Applying a correction to an invalid entry
- **WHEN** a user sets `canonical_bucket = 'lower_bound'` on an entry with
  `raw_value = 'threshold_min'` and `status = 'proposed'`, and clicks Apply
- **THEN** the entry is saved with `canonical_bucket = 'lower_bound'` and
  `status = 'approved'`, every `kb.metrics` row with `value_range_type_error` set and
  `value_range_type` normalizing to `threshold_min` has its
  `value_range_type_error` cleared, and the user is shown how many rows were
  corrected

#### Scenario: Correction does not affect unrelated rows
- **WHEN** an entry with `raw_value = 'threshold_min'` is corrected
- **THEN** `kb.metrics` rows with a different (normalized) `value_range_type`, or with
  `value_range_type_error` already `NULL`, are left unchanged
