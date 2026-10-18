## ADDED Requirements

### Requirement: Non-scalar structures preserve relationships
Metric production and benchmark storage and API reads/writes SHALL preserve optional `value_structure` beside the complete `metric_value`. Supported kinds SHALL be alternatives, required_points, composite, expression and unresolved. Alternatives and required_points SHALL contain at least two distinct nonempty string members. Composite members SHALL retain order. Different relationships SHALL remain distinct during merging and scoring. Scalar and interval fields SHALL remain backward compatible.

#### Scenario: Permitted diameters
- **WHEN** a source permits diameters 10, 20 or 30 mm
- **THEN** one metric SHALL retain all three members with kind alternatives
- **AND** it SHALL NOT become three unrelated requirements or interval 10~30

#### Scenario: Mandatory inspection points
- **WHEN** a source requires inspection at 10, 30 and 50 mm
- **THEN** one metric SHALL retain those members with kind required_points
- **AND** its structure SHALL differ from permitted alternatives with those numbers

### Requirement: Unsupported values cannot become scalars
The normalizer SHALL interpret only complete supported numeric forms. Any non-null non-scalar structure, malformed structure, legacy list, composite or unsupported expression SHALL yield unparsed with no numeric fields. Existing scalar, interval and explicitly supported tolerance behavior SHALL be retained. Unsupported numeric comparisons SHALL defer, without losing the metric text.

#### Scenario: No first-number reduction
- **WHEN** a retained value is 10、20、30, 250×200, or 70+age×2
- **THEN** normalization SHALL NOT yield scalar 10, 250 or 70

### Requirement: Benchmark granularity and value checks follow meaning
New benchmark rules SHALL annotate the same semantic row boundaries and structure as extraction. Scoring SHALL compare collection kind and all members, ignoring order only for unordered collections. Frozen benchmark releases SHALL remain unchanged and runnable. Scoring changes SHALL carry a new version.

#### Scenario: Wrong relationship with matching numbers
- **WHEN** a predicted alternatives set matches every member of a gold required_points collection
- **THEN** the value check SHALL fail

#### Scenario: Partial prediction
- **WHEN** a gold alternatives set is predicted as its first member or bounding interval
- **THEN** the value check SHALL fail
