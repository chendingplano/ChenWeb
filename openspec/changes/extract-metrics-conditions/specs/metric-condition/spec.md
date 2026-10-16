## ADDED Requirements

### Requirement: Metric rows carry their condition

`extract_metrics` SHALL ask Pass 2 for `condition` and `condition_en` on every row and store them
in `kb.metrics.condition` and `kb.metrics.condition_en`. A condition is everything the source
states that limits when, where, for whom or how the value holds: applicability, population,
category, size class or variant; environment and operating state; test method, apparatus or
arrangement; location or body site; load, input level or time point. `condition` SHALL be empty
when the source states none. `condition_en` SHALL be empty for English documents.

#### Scenario: Population in a table column becomes the condition
- **WHEN** a table row gives 有效量程 600 mm~1 600 mm for 身高测试仪(计) in the 适用人群 column 幼儿
- **THEN** the stored row has `metric_subject` 身高测试仪(计) and `condition` containing 幼儿

#### Scenario: Two test methods with the same settings are two rows
- **WHEN** clause 6.5.14.1 sets 运动频率 1 次/min~60 次/min using a 运动模拟装置 and clause 6.5.14.2
  sets the same value using a 模拟物
- **THEN** two rows are stored, each with its own method in `condition`

#### Scenario: Condition reaches the database
- **WHEN** Pass 2 returns a row with `condition` "适用人群：幼儿" and `condition_en`
  "Population: preschool children"
- **THEN** `kb.metrics.condition` and `kb.metrics.condition_en` hold those strings after
  `SaveMetrics` and after `UpsertMetrics`

### Requirement: Range rows store numeric endpoints

`extract_metrics` SHALL ask Pass 2 for `value_min` and `value_max` on `range` rows and store them in
`kb.metrics.value_min` / `kb.metrics.value_max`. Rows of other range types SHALL store NULL.

#### Scenario: Endpoints from the model are stored
- **WHEN** Pass 2 returns `value_range_type` `range`, `metric_value` `600~1600`, `value_min` 600 and
  `value_max` 1600
- **THEN** the stored row has `value_min` 600 and `value_max` 1600

### Requirement: Condition is part of metric identity

Two rows that differ only in their normalised condition SHALL be treated as different metrics by
the in-run dedupe, by `metricsIdentical` (including its resolved-concept shortcut), by
`metricStaticMatch`, and by the merge-resolve LLM step.

#### Scenario: In-run dedupe keeps both conditions
- **WHEN** one run produces 分度值 1 N for 握力测试仪(计) on line 116 twice, with conditions 幼儿 and
  成年人
- **THEN** both rows survive `dedupeFinalMetricRows`

#### Scenario: Same concept, different condition is not identical
- **WHEN** an existing row and a new row resolve to the same `keyword_concept_id` but have
  conditions 男 and 女
- **THEN** `metricsIdentical` returns false

#### Scenario: Empty and non-empty conditions do not static-match
- **WHEN** a new row with condition 幼儿 is compared with an existing row with the same name, subject
  and unit and an empty condition
- **THEN** `metricStaticMatch` returns false
