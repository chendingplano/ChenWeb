## ADDED Requirements

### Requirement: Requested questions select distinct metrics
For a specified document, the page SHALL accept a requested question count from 1 to 3,000
as `num_questions_to_generate`, rather than a metric count. The server SHALL retrieve all primary
document metrics, retain distinct metric IDs linked to usable canonical source lines, and randomly
sample `ceil(requested / 3)` metrics without replacement. Each selected metric SHALL produce one
question of each hop type. The page SHALL show the rounded question total before submission.
The question count SHALL take precedence over legacy metric/hop counts. Batch and legacy requests
without this field SHALL retain their existing behavior.

#### Scenario: Round up to complete triplets
- **WHEN** 10 questions are requested and at least 4 usable metrics exist
- **THEN** 4 distinct metrics are sampled and 12 questions are generated

### Requirement: Confirm an insufficient metric pool
When fewer than the required metrics are available, the server SHALL return HTTP 409 with
`available_metrics`, `available_questions`, `requested_questions`, and `confirmation_required`,
before creating a run or making a model call. The page SHALL show the reduced question count and
ask for confirmation in English or Chinese. On acceptance it SHALL resubmit with
`confirmed_available_questions` equal to that count. The server SHALL recheck capacity and require
another confirmation if the count changed. Once accepted, generation SHALL use the eligible metric
pool and canonical lines captured by that successful check.

#### Scenario: Confirm fewer questions
- **WHEN** 10 questions are requested but only 2 usable metrics exist and the user confirms
- **THEN** the run targets 6 questions using each of the 2 metrics once

#### Scenario: Cancel fewer questions
- **WHEN** the user cancels the shortage prompt
- **THEN** no run starts and the generation settings remain available for editing

#### Scenario: No usable metrics
- **WHEN** no usable metrics exist
- **THEN** `available_questions` is 0 and `confirmation_required` is false, and the page explains that no run started

#### Scenario: Source unavailable
- **WHEN** the document's canonical lines cannot be loaded
- **THEN** generation fails before creating a run rather than reporting a metric shortage

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
