## ADDED Requirements

### Requirement: Hop types measure indirectness
Generated `search_metrics` questions SHALL use hop types as follows: a one-hop question names the
target metric (or a close paraphrase) and asks for its value, limit, unit or condition; a two-hop
question does not name the metric and describes a situation, purpose or condition whose answer is
the metric, needing one inference to identify it; a multi-hop question does not name the metric
and needs a chain of at least two inferences to identify it. Every question SHALL have exactly one
target metric and SHALL NOT ask for several values.

#### Scenario: Indirect two-hop question
- **WHEN** a two-hop question is generated for the metric "Test duration"
- **THEN** it asks about the situation (e.g. how long recording must continue after steady state) without the words "test duration"

### Requirement: Name-leak validation
The server SHALL reject a two-hop or multi-hop question whose text contains its target metric's
`metric_name` or `metric_name_en` (case-insensitive; names of two or more characters), with an
error that the correction round passes back to the model.

#### Scenario: Leaked name
- **WHEN** a multi-hop question for "试验持续时间" contains "试验持续时间"
- **THEN** validation fails with an error naming the leaked name, and the model gets one correction attempt

#### Scenario: One-hop question
- **WHEN** a one-hop question contains its metric's name
- **THEN** validation accepts it

### Requirement: English name in the model input
The model input for each sampled metric SHALL include `metric_name_en` when `kb.metrics` has one.
If the English names cannot be loaded, generation SHALL continue and the check SHALL use the
stored name only.

#### Scenario: English names unavailable
- **WHEN** the English-name query fails
- **THEN** a warning is logged and the questions are generated and checked against `metric_name`
