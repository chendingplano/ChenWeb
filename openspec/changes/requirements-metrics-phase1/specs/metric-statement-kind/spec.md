## ADDED Requirements

### Requirement: Metric rows are classified into statement kinds
The web client SHALL classify every metric row into exactly one statement kind and group, using
only `value_class`, `value_range_type`, `formula_or_definition` and `reasoning_tags`. The rules
SHALL apply in this order, after trimming and lower-casing the values. Numeric means
`lower_bound`, `upper_bound`, `exact` or `range`.

1. Tag `test_condition` → `test_parameter` (group `test`)
2. `value_class = definition` → `metric_definition` if `formula_or_definition` is non-empty, else `definition` (group `definition`)
3. `value_class = reference`, or a `cited_doc:` or `external_reference` tag → `metric_value` (group `metric`) if numeric, else `delegated_requirement` (group `requirement`)
4. `value_class` `requirement` or `target` → `requirement_with_criterion` if numeric, `requirement_value_open` if `limit_absent`, otherwise `inspection_requirement` (group `requirement`)
5. `value_class` `observation` or `design_capability` → `metric_value` if numeric, else `observation` (group `metric`)
6. Otherwise → `unclassified` (group `unclassified`)

#### Scenario: Qualitative requirement is not a metric
- **WHEN** a row has `value_class = requirement` and `value_range_type = qualitative`
- **THEN** it SHALL be classified `inspection_requirement` in group `requirement`

#### Scenario: Numeric requirement keeps its criterion
- **WHEN** a row has `value_class = requirement` and `value_range_type = upper_bound`
- **THEN** it SHALL be classified `requirement_with_criterion` in group `requirement`

#### Scenario: Delegated requirement
- **WHEN** a row has `value_class = reference`, `value_range_type = qualitative` and tag `cited_doc:CJJ 52`
- **THEN** it SHALL be classified `delegated_requirement`

#### Scenario: Test setting wins over requirement class
- **WHEN** a row has `value_class = requirement`, `value_range_type = exact` and tag `test_condition`
- **THEN** it SHALL be classified `test_parameter` in group `test`

#### Scenario: Formula definition
- **WHEN** a row has `value_class = definition` and a non-empty `formula_or_definition`
- **THEN** it SHALL be classified `metric_definition`

#### Scenario: Unknown class is never shown as a metric
- **WHEN** a row has a missing or unrecognised `value_class`
- **THEN** it SHALL be classified `unclassified`, not as a metric

#### Scenario: Tags arrive as a JSON string
- **WHEN** `reasoning_tags` is the string `["test_condition"]`
- **THEN** the classifier SHALL parse it and classify the row `test_parameter`

### Requirement: Customer-facing metric views show the statement kind
Customer-facing metric views SHALL show each row's statement group as a label, with the kind available as a tooltip. These views are the metric wiki view, the artifact category panel's item details, the Metric Ontology Explorer search results and the metric detail attribute groups. All label text SHALL come from Paraglide messages
present in both `en.json` and `zh-cn.json`. The Chinese labels SHALL use 要求 for requirement and
指标 for metric. The label text SHALL be selectable.

#### Scenario: Requirement row in Chinese
- **WHEN** a user with locale `zh-cn` views a row classified in group `requirement`
- **THEN** the label SHALL read 要求 and SHALL NOT read 指标

#### Scenario: i18n check passes
- **WHEN** `bun run check` runs in `web/`
- **THEN** it SHALL report no missing message keys and no new hard-coded text
