## ADDED Requirements

### Requirement: One value and one condition per row

The enrich prompt SHALL instruct the model to emit one row per value: a cell or clause that gives
one property several values for different conditions, or a list of settings, becomes one row per
value, each with its own `condition`; a cell that names several properties becomes one row per
property. Every filled cell of a table row whose columns give different properties of one item
(value, resolution, permitted error) SHALL be a candidate in Pass 1 and a row in Pass 2.

#### Scenario: Values by sex
- **WHEN** a table cell says 台阶高度 男300 mm、女250 mm
- **THEN** Pass 2 returns two rows, 300 with condition 男 and 250 with condition 女

#### Scenario: Property columns of one item
- **WHEN** a table row says 座板高度:335 mm | 1 mm | ±5 mm under the headers 规格要求 | 分度值 |
  允许误差
- **THEN** three rows are returned: 座板高度 335 mm, 座板高度分度值 1 mm and 座板高度允许误差 ±5 mm

### Requirement: Numeric settings are independently accounted for

Candidate extraction SHALL audit every normal clause for explicit numeric quantities, including
dimensions and accuracy embedded in apparatus descriptions. A quantity present only in another
candidate's condition SHALL NOT count as extracted. Enrichment SHALL preserve each candidate's
supported source-row population and SHALL NOT substitute a neighbouring row's population.

#### Scenario: Apparatus settings embedded in a dense clause
- **WHEN** a clause specifies loading width 80 mm, dynamometer accuracy 0.1%, three test points,
  and at least three readings per point
- **THEN** the prompts request four independently extracted metrics, with the apparatus inherited
  as a condition on each

#### Scenario: Identical errors for different populations
- **WHEN** adult and student table rows both specify beat error ±1 count/min
- **THEN** enrichment requests both assertions with their own populations

### Requirement: Adjustment actions do not create acceptance limits

Both prompts SHALL distinguish an independently stated acceptable range from an instruction to
adjust or repair equipment when a reading crosses an action trigger.

#### Scenario: Pressure adjustment trigger
- **WHEN** a clause specifies a permitted pointer reading 15–25 mm and instructs adjustment if
  it exceeds 25 mm
- **THEN** the acceptable range is a metric and the adjustment instruction does not create a
  second upper-bound metric

### Requirement: Data sets are not metrics

Both prompts SHALL instruct the model that a table of records, samples, example cases or
input/expected-output pairs (a test data set or parameter library) is data, not a requirement. Its
cells SHALL NOT become rows, and a column SHALL NOT be summarised into a min~max range. A candidate
from such a table SHALL be dropped with reason `dataset_record`, and a row the model tags
`dataset_record` SHALL be set aside like other drop-reason tags.

#### Scenario: Simulation parameter library
- **WHEN** an informative annex table lists simulated subjects (sex, age, height, impedances, expected
  body-fat %)
- **THEN** no metric rows are stored for it, and its candidates are listed with `dataset_record`

#### Scenario: Tagged row is set aside
- **WHEN** Pass 2 returns a row with reasoning tag `dataset_record`
- **THEN** the row is written to `kb.metrics_dropped` with stage `llm_tag`, not to `kb.metrics`

### Requirement: Pass criteria are requirements, not test settings

The enrich prompt SHALL instruct the model that a value which sets the pass criterion of a
requirement on the product (a load it must withstand, a tilt at which it must stay upright, a
duration it must hold) is a `requirement` with the matching bound and without the `test_condition`
tag. A value that only sets how a test is run (a test force range, number of readings) keeps
`test_condition`.

#### Scenario: Stability tilt
- **WHEN** a clause says 直立放置的测试器材在任意方向倾斜 10° 时不应翻倒
- **THEN** the row has `value_class` `requirement`, `value_range_type` `lower_bound`, `metric_value`
  `10`, and no `test_condition` tag
