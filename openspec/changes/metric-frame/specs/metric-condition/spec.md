## MODIFIED Requirements

### Requirement: Metric rows carry their condition

`extract_metrics` SHALL ask Pass 2 for `condition` and `condition_en` on every row and store them
in `kb.metrics.condition` and `kb.metrics.condition_en`. A condition is everything the source
states that limits when, where, for whom or in what state the value holds: applicability,
population, trigger, category, size class or variant; environment and operating state;
arrangement; location or body site; load, input level or time point. Test methods, apparatus,
agents, reagents, media and procedure steps are not conditions; they belong to the row's `frame`
(capability `metric-frame`). `condition` SHALL be empty when the source states none.
`condition_en` SHALL be empty for English documents.

#### Scenario: Population in a table column becomes the condition
- **WHEN** a table row gives 有效量程 600 mm~1 600 mm for 身高测试仪(计) in the 适用人群 column 幼儿
- **THEN** the stored row has `metric_subject` 身高测试仪(计) and `condition` containing 幼儿

#### Scenario: Two test methods with the same settings are two rows
- **WHEN** clause 6.5.14.1 sets 运动频率 1 次/min~60 次/min using a 运动模拟装置 and clause 6.5.14.2
  sets the same value using a 模拟物
- **THEN** two rows are stored, each with its own apparatus in `frame.uses` and neither in `condition`

#### Scenario: Condition reaches the database
- **WHEN** Pass 2 returns a row with `condition` "适用人群：幼儿" and `condition_en`
  "Population: preschool children"
- **THEN** `kb.metrics.condition` and `kb.metrics.condition_en` hold those strings after
  `SaveMetrics` and after `UpsertMetrics`
