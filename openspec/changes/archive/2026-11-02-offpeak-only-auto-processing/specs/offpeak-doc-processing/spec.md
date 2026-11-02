## ADDED Requirements

### Requirement: Off-peak-only processing mode
The system SHALL accept `auto_offpeak` as a `kb.inputs.processing_mode` value, and the Upload
Files and Pending Files "Auto Process" pulldowns SHALL offer it as "Auto - off-peak only" and
select it by default.

#### Scenario: Default selection
- **WHEN** a user opens Knowledge → File Management → Upload Files
- **THEN** the Auto Process pulldown shows "Auto - off-peak only" selected

#### Scenario: Upload with off-peak mode
- **WHEN** a file is uploaded with `processing_mode = auto_offpeak`
- **THEN** the input row stores `auto_offpeak`, the PDF is parsed as in `auto` mode, and doc
  processing is started automatically after parsing

#### Scenario: Zip children inherit
- **WHEN** a zip is uploaded with `auto_offpeak`
- **THEN** every child input row gets `processing_mode = auto_offpeak`

### Requirement: Hold LLM processors around peak hours
For an input whose `processing_mode` is `auto_offpeak`, the doc-processor SHALL NOT start an
LLM-using processor (every processor except `blocking`, and Phase C post-processing) while the
configured peak-hours record is active at any minute from now through now + the lead time. It
SHALL instead hold and re-check every poll interval, and start the processor once the condition
clears.

#### Scenario: Upload during peak
- **WHEN** an `auto_offpeak` document reaches its first LLM processor at 10:00 Beijing time on a
  CN workday with the `deepseek peak hours` record
- **THEN** the processor is held with status `active` and progress
  `held: waiting for off-peak hours (deepseek peak hours)`, and it starts after 12:00

#### Scenario: Peak about to start
- **WHEN** the next processor would start at 08:55 with a 10-minute lead time
- **THEN** it is held until the 09:00–12:00 window ends

#### Scenario: Off-peak
- **WHEN** the next processor would start at 20:00, or on a weekend or a configured holiday
- **THEN** it starts immediately

#### Scenario: Plain auto is unaffected
- **WHEN** the input's `processing_mode` is `auto`
- **THEN** processors run immediately regardless of peak hours

#### Scenario: Missing peak-hours record
- **WHEN** the configured peak-hours record does not exist or cannot be evaluated
- **THEN** processors are not held and a WARN log is written

#### Scenario: Stop while held
- **WHEN** a user stops a pipeline that is held
- **THEN** the pipeline ends with status `stopped` without running the held processor

### Requirement: Held pipelines release their slot
A pipeline whose processors are all held SHALL release its pipeline slot while held, and SHALL
re-acquire a slot before resuming.

#### Scenario: Plain auto during peak with many held documents
- **WHEN** as many `auto_offpeak` pipelines are held as there are pipeline slots, and an `auto`
  document arrives
- **THEN** the `auto` document's pipeline starts without waiting for peak hours to end
