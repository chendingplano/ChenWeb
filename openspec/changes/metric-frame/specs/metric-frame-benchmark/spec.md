## ADDED Requirements

### Requirement: Gold rules 9.0.0 annotate the frame

The `extract-metrics-benchmark` skill SHALL add rules `9.0.0.md` and make it the default. It SHALL
state the facet definitions, the resource-spec rule with its specified-object exception, the
amount-less-step rule, and names without strength. It SHALL replace the §2.6 formulation sentence
of 8.0.0 with a reference to the general rule. Gold rows saved under 9.0.0 SHALL include `frame`
in `testbed.metrics`, and the gold tooling SHALL validate its shape.

#### Scenario: Gold row for a dose
- **WHEN** a 9.0.0 gold run annotates 364 A.2.4
- **THEN** it has a row 葡萄糖静推剂量 40 ml with `frame.uses` 葡萄糖注射液 {浓度 50 %} and no 葡萄糖浓度 row

#### Scenario: Invalid gold frame rejected
- **WHEN** a gold row's `frame.uses` entry lacks `name`
- **THEN** saving the run fails with a validation error naming the row

### Requirement: Scoring version 4 scores frame facets

`score-extract-metrics` SHALL add scoring version 4 with these relative weights: missing 30,
false_positive 30, value 10, range_type 6, kind 6, unit 6, lines 4, condition 5, uses 2, method 1. `uses` SHALL
pass when every resource is matched once (same role when both give one, names equal or one containing
the other, same spec values and units). `method` SHALL compare the method values, without their labels.
When neither the gold nor the prediction has a facet, that facet SHALL count as a match. Against gold
rules older than 9.0.0, `uses` and `method` SHALL NOT be checked, and the condition check SHALL add the
prediction's frame values to its condition. A score report SHALL record its scoring version, and the
skill SHALL tell the scorer never to compare scores of different scoring versions.

#### Scenario: Missing spec loses uses credit
- **WHEN** a prediction matches a gold row on every field but its `frame.uses` lacks the 50 % spec
- **THEN** the pair gets partial `uses` credit, below 1.0

#### Scenario: Legacy gold keeps apparatus in condition
- **WHEN** a gold row under rules 8.0.0 has `condition` "适用人群：幼儿；试验装置：5 等量块" and the prediction has
  `condition` "适用人群：幼儿" with `frame.uses` 量块 {等级 5 等}
- **THEN** the condition check passes and `uses` / `method` are not checked
