## MODIFIED Requirements

### Requirement: One value and one condition per row
The extraction and enrich prompts SHALL emit one independently interpretable assertion per row. Values for distinct source-supported conditions and independently stated properties SHALL split with their respective conditions and frame. Alternatives for one property and mandatory point collections SHALL remain one structured metric. Ambiguous relationships SHALL retain the full value as unresolved rather than invent independent assertions. Every filled table property column (value, resolution, permitted error) SHALL remain independently accounted for.

#### Scenario: Values by sex
- **WHEN** a table cell says 台阶高度 男300 mm、女250 mm
- **THEN** Pass 2 returns two rows, 300 with condition 男 and 250 with condition 女

#### Scenario: Property columns of one item
- **WHEN** a table row says 座板高度:335 mm | 1 mm | ±5 mm under 规格要求 | 分度值 | 允许误差
- **THEN** three rows are returned for height, resolution and permitted error

#### Scenario: Alternatives are not independent equalities
- **WHEN** a clause says Available diameters: 10, 20 or 30 mm
- **THEN** both passes SHALL preserve one alternatives assertion
