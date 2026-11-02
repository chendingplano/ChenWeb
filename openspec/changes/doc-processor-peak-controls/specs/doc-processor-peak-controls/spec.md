## ADDED Requirements

### Requirement: Task engine peak-hour configuration
The doc-processor SHALL read its peak-hour settings from the merged application config block
`[[task-engines.doc-processor]]`, which holds at most one entry with `default_mode` (`none`,
`auto_peak` or `auto_offpeak`; default `none`) and `cross_peak_mode` (`ignore`, `pause-llm` or
`abort`; default `ignore`). Any other value, including `pause-exec`, or more than one entry SHALL
fail doc-processor startup with an error naming the setting.

#### Scenario: No block configured
- **WHEN** neither `config.toml` nor `config.local.toml` contains `task-engines.doc-processor`
- **THEN** the engine runs with `default_mode = none` and `cross_peak_mode = ignore`

#### Scenario: Local override
- **WHEN** `config.local.toml` contains one `[[task-engines.doc-processor]]` entry with
  `default_mode = "auto_offpeak"` and `cross_peak_mode = "ignore"`
- **THEN** the doc-processor uses those values

#### Scenario: Unsupported value
- **WHEN** `cross_peak_mode = "pause-exec"`, or `default_mode` is not one of the three modes, or the
  block has two entries
- **THEN** the doc-processor refuses to start and logs the configuration error

### Requirement: Task peak-hour mode precedence
A doc-processor task's peak-hour mode SHALL be the task's own mode when it has one and the
engine's `default_mode` otherwise. A task whose input has `processing_mode = auto_offpeak` SHALL
have its own mode `auto_offpeak`; any other processing mode SHALL leave the task without its own
mode. If the input's processing mode cannot be read, the task SHALL use the engine default and a
WARN SHALL be logged.

#### Scenario: Task mode wins
- **WHEN** the engine's `default_mode` is `none` and the input's `processing_mode` is `auto_offpeak`
- **THEN** the task's mode is `auto_offpeak`

#### Scenario: Engine default applies
- **WHEN** the engine's `default_mode` is `auto_offpeak` and the input's `processing_mode` is `auto`
- **THEN** the task's mode is `auto_offpeak`

#### Scenario: Peak control disabled
- **WHEN** the task's mode is `none`
- **THEN** the task starts immediately and no boundary rule applies to it

### Requirement: Permitted span and admission
A task with mode `auto_offpeak` SHALL be permitted only while the configured peak-hours record is
inactive at the current instant; a task with mode `auto_peak` only while it is active. The record
is named by `DOC_PROCESS_OFFPEAK_PEAK_HOURS_NAME` (default `deepseek peak hours`), re-evaluated at
most once per `DOC_PROCESS_OFFPEAK_POLL_SEC` (default 30) and shared by all tasks. If the record is
missing or cannot be evaluated, every mode SHALL be treated as permitted and a WARN SHALL be
logged. A task that is not permitted when it arrives SHALL wait before any work, including
routing, and SHALL start on its own once its span is permitted.

#### Scenario: Off-peak task arrives during peak
- **WHEN** a task with mode `auto_offpeak` arrives at 10:00 Beijing time on a CN workday
- **THEN** it does no routing or processing until the `deepseek peak hours` window ends at 12:00,
  then starts

#### Scenario: Peak task arrives off-peak
- **WHEN** a task with mode `auto_peak` arrives at 20:00 on a CN workday
- **THEN** it waits until 09:00 on the next CN workday, then starts

#### Scenario: Missing peak-hours record
- **WHEN** the configured peak-hours record does not exist
- **THEN** tasks of every mode start immediately and a WARN is logged

### Requirement: Waiting tasks are visible, stoppable and do not hold a slot
While a task waits for admission it SHALL release its pipeline slot, and its `doc_processing`
status entry SHALL show `proc_status = running` with `progress` "waiting for off-peak hours
(<name>)" or "waiting for peak hours (<name>)". It SHALL re-acquire a slot before starting and
SHALL wait again if its span is no longer permitted once the slot is acquired. A user stop while
waiting SHALL end the task with `doc_processing` status `stopped` without routing or running any
processor.

#### Scenario: Plain task not blocked by waiting tasks
- **WHEN** as many tasks wait as there are pipeline slots and a task with mode `none` arrives
- **THEN** that task starts without waiting

#### Scenario: Stop while waiting
- **WHEN** a user stops a task that is waiting for admission
- **THEN** the task ends with status `stopped` and no processor runs

### Requirement: Cross-peak boundary rules
The doc-processor SHALL apply `cross_peak_mode` to a task with mode other than `none` that is
active when its span stops being permitted:
- `ignore`: the task continues to completion.
- `pause-llm`: the task continues, but each provider request it issues (chat, structured output,
  embedding, and every retry attempt, from routing through Phase C) SHALL be held before it is
  sent until the span is permitted again, then sent. A request already sent SHALL complete. A
  held request SHALL NOT keep a model permit.
- `abort`: the task SHALL be cancelled within one poll interval, end with status `stopped` and the
  error "aborted at peak-hour boundary" on its status entry and run row, and SHALL NOT be resumed.

#### Scenario: Ignore crossing into peak
- **WHEN** an `auto_offpeak` task is running at 08:59 and `cross_peak_mode = ignore`
- **THEN** it keeps running and sending LLM requests after 09:00

#### Scenario: Pause LLM crossing into peak
- **WHEN** an `auto_offpeak` task issues an LLM request at 09:05 with `cross_peak_mode = pause-llm`
- **THEN** no provider request is sent until 12:00, after which the request is sent and the task
  continues

#### Scenario: Pause LLM crossing out of peak
- **WHEN** an `auto_peak` task issues an embedding request at 18:01 with
  `cross_peak_mode = pause-llm`
- **THEN** the request is held until the next peak span

#### Scenario: Stop while an LLM call is held
- **WHEN** a user stops a task whose LLM call is held
- **THEN** the call returns the stop cause without being sent and the task ends `stopped`

#### Scenario: Abort crossing into peak
- **WHEN** an `auto_offpeak` task is running at 09:00 with `cross_peak_mode = abort`
- **THEN** it is cancelled, its status and run row are `stopped` with "aborted at peak-hour
  boundary", and it does not restart at 12:00

### Requirement: Peak-control activity logging
The doc-processor SHALL write to its own log when a task starts waiting for admission, when a
waiting task starts, when a waiting task is stopped, when a task is aborted at a boundary, when an
LLM call is held, and when a held call resumes or is cancelled. Each entry SHALL carry the
`record_id`, the task's peak mode and `cross_peak_mode`, and, for calls, the `run_id`, a `hold_id`,
the model and the call location.

#### Scenario: Task wait logged
- **WHEN** a task waits for admission and later starts
- **THEN** the doc-processor log has a waiting entry and a start entry for its `record_id`

### Requirement: LLM hold lifecycle events
Each held LLM call SHALL produce one `llm_usage_event` row with `event_kind = llm_hold` when it
starts waiting and one row with `event_kind = llm_resume` when it is released, or
`llm_hold_cancelled` when the task ends while it is held. Both rows SHALL carry the call's
account, profile, provider, model, prompt name, call reason and call location, the task's
`user_id`, `record_id` and `run_id`, zero tokens, and the same `metadata_json.hold_id`; the closing
row SHALL carry `metadata_json.held_ms`. Ordinary usage rows SHALL have `event_kind = request`.
A failure to write either row SHALL be logged at WARN and SHALL NOT block or fail the call.

#### Scenario: Paired rows
- **WHEN** a call is held from 09:05 to 12:00 and then sent
- **THEN** `llm_usage_event` has an `llm_hold` row, an `llm_resume` row with the same `hold_id`,
  and a `request` row for the provider request

#### Scenario: Event write fails
- **WHEN** the database rejects the `llm_hold` row
- **THEN** a WARN is logged and the call is still held and later sent

### Requirement: Usage reports exclude lifecycle rows
Usage aggregates SHALL count only rows with `event_kind = request`: the model activity report,
the today summary and the generated daily account report. LLM usage event lists SHALL return each
row's `event_kind`.

#### Scenario: Request counts unchanged by holds
- **WHEN** a day has 100 request rows and 10 hold/resume rows
- **THEN** that day's request count in each aggregate is 100
