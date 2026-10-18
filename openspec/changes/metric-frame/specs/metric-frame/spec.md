## ADDED Requirements

### Requirement: Metric rows carry a structured frame

`kb.metrics` and `testbed.metrics` SHALL have a nullable `frame jsonb` column. When it is set, it
SHALL be an object with any of these optional keys:
- `uses`: a list of resources, each with `role`, `name`, `name_en` and an optional `spec` list
  of `{property, property_en, value, unit}`;
- `method`: a list of `{label, value, label_en, value_en}`;
- `procedure`: an object with `step`, `stage`, `stage_en`, `after` and `after_en`.

`role` SHALL be one of `agent`, `material`, `apparatus`, `reagent`, `medium`, `sample` or
`device`. `extract_metrics` SHALL store `frame` from Pass 2 output in `SaveMetrics` and
`UpsertMetrics`. It SHALL store NULL when every facet is empty.

#### Scenario: Dose with agent, route and step
- **WHEN** clause A.2.4 says "如血糖＜2.8mmol/L…可予静推50％葡萄糖40ml"
- **THEN** the stored row has `metric_name` 葡萄糖静推剂量, `metric_value` 40, `metric_unit` ml,
  `frame.uses` `[{role: agent, name: 葡萄糖注射液, spec: [{property: 浓度, value: "50", unit: "%"}]}]`,
  `frame.method` containing 给药途径：静脉推注, and `frame.procedure.step` A.2.4

#### Scenario: Apparatus of a test value
- **WHEN** a clause sets 运动频率 1 次/min~60 次/min using a 运动模拟装置
- **THEN** the row has `frame.uses` `[{role: apparatus, name: 运动模拟装置}]` and no 试验装置 qualifier in `condition`

#### Scenario: No frame stated
- **WHEN** a row has no resource, method or procedure in its source
- **THEN** `frame` is NULL

#### Scenario: Invalid role rejected
- **WHEN** Pass 2 returns a `uses` entry with role `tool`
- **THEN** the normaliser maps it to no role, logs it, and keeps the entry as `role: ""` rather than
  dropping the row

### Requirement: Condition holds only the condition facet

`condition` / `condition_en` SHALL hold only qualifiers that say when, where, for whom or in what
state the value holds. Examples are population, trigger, clinical state, environment, operating
state, location, load, model or variant, and inspected item. Apparatus, materials, reagents,
media and agents SHALL be in `frame.uses`. Test methods, routes and modes SHALL be in
`frame.method`. Treatment stage, timing and operation step SHALL be in `frame.procedure`.

#### Scenario: Route moved out of condition
- **WHEN** Pass 2 returns `condition` "适用人群：成人；给药途径：静脉推注"
- **THEN** the stored `condition` is "适用人群：成人" and `frame.method` holds 给药途径：静脉推注

#### Scenario: Stage moved to procedure
- **WHEN** Pass 2 returns `condition` "触发：血糖＜2.8mmol/L；治疗阶段：意识转清后"
- **THEN** the stored `condition` is "触发：血糖＜2.8mmol/L" and `frame.procedure.stage` is 意识转清后

### Requirement: Legacy condition labels are mapped deterministically

A fixed label map SHALL assign each known legacy qualifier label to a facet and, for `uses`, a
role. The map SHALL be applied to Pass 2 output at save time and by a backfill command over
existing `kb.metrics` and `testbed.metrics` rows. A label not in the map SHALL stay in
`condition`. A `药物浓度` qualifier SHALL become the spec of the row's only agent. When the row has
zero or several agents, it SHALL stay in `condition` and be logged. The backfill SHALL support
`--dry-run`, SHALL write the before-image of every changed row to a backup file, SHALL restore such
a file with `--revert`, and SHALL report counts per label.

#### Scenario: Backfill of a test-equipment row
- **WHEN** an existing row has `condition` "适用人群：幼儿；试验装置：运动模拟装置"
- **THEN** after the backfill, `condition` is "适用人群：幼儿" and `frame.uses` is
  `[{role: apparatus, name: 运动模拟装置}]`

#### Scenario: Unknown label untouched
- **WHEN** a row has `condition` "取样方式：随机"
- **THEN** the backfill leaves `condition` unchanged and leaves `frame` unset

#### Scenario: Revert restores the text
- **WHEN** the backfill runs with `--revert <backup file>` after a run that changed a row with no frame
- **THEN** `condition` again contains the moved qualifiers and `frame` is NULL

### Requirement: Frame is part of metric identity

Two rows that differ in their normalised `uses` signature or `method` SHALL be different metrics
for the in-run dedupe, `metricsIdentical` (including the resolved-concept shortcut),
`metricStaticMatch` and merge-resolve. The uses signature is the sorted list of
(role, name, spec property, spec value, spec unit). `procedure` SHALL NOT be part of identity.

#### Scenario: Two strengths stay two rows
- **WHEN** one run produces 葡萄糖静推剂量 2~4 ml/kg for 儿童 twice, once with spec 浓度 25 % and once
  with spec 浓度 10 % and value 5~10
- **THEN** both rows survive `dedupeFinalMetricRows`

#### Scenario: Same metric restated at another step
- **WHEN** two rows are equal except for `frame.procedure.step`
- **THEN** they are treated as the same metric (occurrences)

### Requirement: Inheritance writes to the facet it serves

`inheritClauseConditions` SHALL copy a clause-wide apparatus, method or medium into `frame.uses`
or `frame.method`, not into `condition`. `inheritServedQuantityConditions` SHALL copy the served
quantity's `frame` (except `procedure.step`) together with its condition.

#### Scenario: Tolerance inherits apparatus
- **WHEN** a 允许误差 row serves a 量程 row whose `frame.uses` holds 握力测试仪(计)
- **THEN** the 允许误差 row's `frame.uses` holds 握力测试仪(计)

### Requirement: Metric read APIs expose the frame

Metric list, detail, dropped-metric and wiki responses that return `kb.metrics` columns SHALL
include `frame` as an object or null. The metric detail group SHALL show Uses (each resource with
its role and spec), Method and Procedure next to Condition, with all labels through Paraglide in
English and Chinese. Like the other attributes of the group, an empty one SHALL count as unfilled.

#### Scenario: Frame in detail response
- **WHEN** a client reads metric 364_mtc_29 after re-extraction
- **THEN** the response has `frame.uses[0].name` 葡萄糖注射液

#### Scenario: Chinese locale
- **WHEN** the detail panel is shown with locale `zh-cn`
- **THEN** the attribute labels are 条件, 使用, 方法 and 程序, and an agent shows as `药物：葡萄糖 (浓度 50%)`
