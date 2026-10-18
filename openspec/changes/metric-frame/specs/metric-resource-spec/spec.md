## ADDED Requirements

### Requirement: A resource's specification is not a metric

`extract_metrics` SHALL NOT emit a row for a number that only specifies a resource used by
another value of the same clause: a strength, concentration, dilution or ratio of an agent or
reagent, or a grade, class or model of an apparatus or material. The number SHALL be
stored as `frame.uses[].spec` of the row that uses the resource. This rule SHALL apply in every
document kind.

#### Scenario: Drug strength in a dose
- **WHEN** a clause says "静推50％葡萄糖40ml"
- **THEN** one row is stored (葡萄糖静推剂量 40 ml) with spec 浓度 50 %, and no 葡萄糖浓度 row

#### Scenario: Dilution
- **WHEN** a clause says "肾上腺素（1:1000）0.3mg～0.5mg 肌肉注射"
- **THEN** the dose row has spec 浓度 1:1000 and no 肾上腺素浓度 row exists

#### Scenario: Reagent concentration in a test method
- **WHEN** a test clause says "用 0.1 mol/L 盐酸溶液 50 mL 提取 30 min"
- **THEN** 提取液体积 50 mL and 提取时间 30 min are rows, each with `uses` 盐酸溶液 {浓度: 0.1 mol/L},
  and no 盐酸浓度 row exists

#### Scenario: Apparatus grade
- **WHEN** a clause says "采用 5 等量块进行检验，示值误差不大于 ±0.5 mm"
- **THEN** the 示值误差 row has `uses` `[{role: apparatus, name: 量块, spec: [{property: 等级, value: "5 等"}]}]`

### Requirement: The specified object keeps its own properties as metrics

When the resource is the object the document specifies, its specification SHALL remain an A1
metric. Examples are a product standard for an injection, a reagent specification and a
calibration standard for gauge blocks.

#### Scenario: Product standard
- **WHEN** a standard for 葡萄糖注射液 says "含量应为标示量的 95.0%～105.0%"
- **THEN** a row 葡萄糖含量 95.0~105.0 % with subject 葡萄糖注射液 is stored

### Requirement: An amount-less step is not a metric

`extract_metrics` SHALL NOT produce a metric row for a clause that names a resource and its spec
but gives no amount, dose, rate, duration, count or limit. It SHALL be handled as a pure requirement (X16).

#### Scenario: Maintenance drip without amount
- **WHEN** a clause says "若患者意识转清，之后以10％葡萄糖静点并建议转诊"
- **THEN** no metric row is stored for 10 %, and the candidate is recorded as a pure requirement

### Requirement: Resource-spec rows that slip through are dropped and logged

After Pass 2 and before the other set-aside stages, `dropResourceSpecRows` SHALL drop a row when all
of these hold:
- its value is a dilution (`1:1000`) or a number in %, mol/L, mmol/L, g/L, mg/mL or g/mL;
- it does not itself use a resource with that spec;
- another row with an overlapping `source_line_spans` uses a resource whose spec has the same
  value (and the same unit, when both give one).

A row with no such host row is kept. This is how the specified object's own property survives:
nothing in the clause uses it as a resource. Each dropped row SHALL be written to
`kb.metrics_dropped` with `drop_stage` `enrich` and `drop_reason` `resource_spec`. Its data
SHALL hold `resource_spec_host` with the host row's `candidate_id` and `metric_name`; metric ids
are assigned after this step.

#### Scenario: Stray strength row
- **WHEN** Pass 2 returns both 葡萄糖静推剂量 40 ml (uses 葡萄糖注射液 {浓度 50 %}) and 葡萄糖浓度 50 % on line 185
- **THEN** only the dose row is saved, and `kb.metrics_dropped` has the 50 % row with reason
  `resource_spec`, whose `resource_spec_host` names the dose row

#### Scenario: No host row
- **WHEN** a 浓度 50 % row has no other row on its lines with a matching spec
- **THEN** the row is kept
