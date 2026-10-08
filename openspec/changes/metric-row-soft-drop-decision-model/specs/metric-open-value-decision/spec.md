## ADDED Requirements

### Requirement: Only open-value requirements are judged
After pure requirements are set aside, `extract_metrics` SHALL send to the decision model every
row whose statement kind (spec `metric-statement-kind`) is `requirement_value_open`, and no other
row. Rows of any other kind SHALL be kept or dropped without a decision call.

#### Scenario: Numeric criterion skips the decision model
- **WHEN** a row has `value_class = requirement` and `value_range_type = exact`
- **THEN** no decision call SHALL be made for it

#### Scenario: Test parameter skips the decision model
- **WHEN** a row carries the tag `test_condition` and `value_range_type = limit_absent`
- **THEN** no decision call SHALL be made for it

#### Scenario: Open-value requirement is judged
- **WHEN** a row has `value_class = requirement` and `value_range_type = limit_absent`
- **THEN** one decision call SHALL be made for it, whether or not it has a unit

### Requirement: The decision question and policy
Each call SHALL use the `jev_emulated` or `jev_compatible` client for the `.models.toml` profile
named by `METRIC_DECISION_MODEL`, and the current version of decision policy
`metric_open_value_kind`. The state SHALL be a JSON object with `policy` (the policy text) and
`row` (`metric_name`, `subject`, `threshold_or_target`, `desc`, `context`). The call SHALL ask
three questions: `kind`, a `choice` with the options `object_quantity`, `activity_schedule` and
`not_a_quantity`; `named`, a `noul` asking whether the source clause itself names the row's
quantity; and `provision_only`, a `noul` asking whether the clause only requires providing
something without naming any quantity of it. The `named` and `provision_only` instructions SHALL
be loaded from prompt files (`prompt-metric-open-value-q-named-v1.md`,
`prompt-metric-open-value-q-provision-v1.md`). When the policy does not exist, it SHALL be
created from `prompts/prompt-metric-open-value-policy-v2.md`. No word list SHALL decide the drop.

#### Scenario: Policy missing on first run
- **WHEN** a run judges a row and no policy `metric_open_value_kind` exists
- **THEN** the policy SHALL be created with version 1 from the prompt file and used for the call

### Requirement: Only a confident activity schedule or an unnamed provision is dropped
A judged row SHALL be set aside (`drop_stage = decision_model`) only when either:
- `kind` answers `activity_schedule` with probability at least `METRIC_DECISION_DROP_MIN_P`
  (default `0.9`): `drop_reason = activity_schedule`; or
- `named` answers yes with probability at most `1 - METRIC_DECISION_DROP_MIN_P` and
  `provision_only` answers yes with probability at least `METRIC_DECISION_PROVISION_MIN_P`
  (default `0.1`): `drop_reason = no_named_quantity`.

Every other judged row SHALL be kept, including rows answered `not_a_quantity`, and rows whose
`named` answer is low but whose `provision_only` answer is below the veto threshold.

#### Scenario: Provision clause with an inferred quantity is dropped
- **WHEN** the row "收集、运输设备配备数量" from "应根据垃圾的类别、数量、作业时间等要求，配备相应的收集、运输设备和作业人员" gets P(named) = 0.00 and P(provision_only) = 0.68
- **THEN** it SHALL be set aside with `drop_reason = no_named_quantity`

#### Scenario: Unnamed in a table heading but not a provision is kept
- **WHEN** a row gets P(named) = 0.02 and P(provision_only) = 0.01
- **THEN** it SHALL be saved to `kb.metrics` with `reason = no_named_quantity_vetoed`

#### Scenario: Agreed collection schedule is dropped
- **WHEN** the row "餐厨垃圾收运时间和频次, 由收运单位与集中供餐单位约定" is answered `activity_schedule` with p = 1.0
- **THEN** it SHALL be set aside with `drop_stage = decision_model`

#### Scenario: Value to be declared is kept
- **WHEN** the row "比能耗, 由设备明确" is answered `object_quantity`
- **THEN** it SHALL be saved to `kb.metrics`

#### Scenario: Uncertain schedule is kept
- **WHEN** a row is answered `activity_schedule` with p = 0.57
- **THEN** it SHALL be saved to `kb.metrics`

#### Scenario: not_a_quantity is kept
- **WHEN** a row is answered `not_a_quantity` with p = 0.95
- **THEN** it SHALL be saved to `kb.metrics`

### Requirement: Every decision is recorded
For each judged row, `extract_metrics` SHALL record `{model, profile, policy_id, policy_version,
questions, choice, choice_meaning, probabilities, named, provision_only, examined, outcome, reason,
reason_text, threshold, provision_min_p, statement_kind, judged_at}` (plus `error` when the call failed): in `kb.metrics_dropped.decision`
when the row is dropped, and in `kb.metrics.ext_info.open_value_decision` when it is kept.
`outcome` SHALL be `kept` or `dropped`; `reason` SHALL be one of
`activity_schedule_confident`, `no_named_quantity_confident`, `object_quantity`,
`no_named_quantity_vetoed`, `not_a_quantity_not_droppable`, `activity_schedule_below_threshold`,
`decision_error`, `decision_model_not_configured`;
`examined` SHALL be false only when no decision model is configured. Each call SHALL be captured as an LLM
usage event with `PromptName = metric_open_value_kind`, `CallReason = extract_metrics` and the
record id.

#### Scenario: Kept row carries its decision
- **WHEN** a judged row is kept
- **THEN** its `ext_info.open_value_decision` SHALL hold the policy id and version, the choice and the probabilities
- **AND** `outcome = kept` with the `reason` and `reason_text` saying why it was kept

#### Scenario: Object quantity is recorded as examined and kept
- **WHEN** a judged row is answered `object_quantity` with p = 1.0
- **THEN** its `ext_info.open_value_decision` SHALL hold `examined = true`, `outcome = kept` and `reason = object_quantity`

### Requirement: Decision failures never drop and never fail the run
A decision failure SHALL NOT drop a row or fail the run. If `METRIC_DECISION_MODEL` is unset,
the policy cannot be loaded or created, or a call fails, the row SHALL be kept, its `ext_info.open_value_decision` SHALL record the error, a warning SHALL be
logged with the record and candidate ids, and the run SHALL continue.

#### Scenario: Model returns no logprobs
- **WHEN** the decision call fails with "response has no logprobs"
- **THEN** the row SHALL be saved to `kb.metrics` with the error in `ext_info.open_value_decision`
- **AND** the run SHALL finish successfully

#### Scenario: Decision model not configured
- **WHEN** `METRIC_DECISION_MODEL` is unset
- **THEN** no decision call SHALL be made and every open-value row SHALL be kept
