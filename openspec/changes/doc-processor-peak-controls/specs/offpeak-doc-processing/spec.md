## MODIFIED Requirements

### Requirement: Off-peak-only processing mode
The system SHALL accept `auto_offpeak` as a `kb.inputs.processing_mode` value, and the Upload
Files and Pending Files "Auto Process" pulldowns SHALL offer it as "Auto - off-peak only" and
select it by default. An `auto_offpeak` input's doc-processor task SHALL have its own peak-hour
mode `auto_offpeak`, which takes precedence over the engine's `default_mode`
(capability `doc-processor-peak-controls`).

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

#### Scenario: Off-peak document under engine mode none
- **WHEN** the engine's `default_mode` is `none` and an `auto_offpeak` document reaches the
  doc-processor during peak hours
- **THEN** its task waits for off-peak hours before routing or processing

## REMOVED Requirements

### Requirement: Hold LLM processors around peak hours
**Reason**: Replaced by engine-level admission and `cross_peak_mode` boundary rules
(`doc-processor-peak-controls`). The per-processor hold with a look-ahead duplicated those
controls and made `ignore` not mean ignore.
**Migration**: An `auto_offpeak` task now waits once, at admission, and the engine's
`cross_peak_mode` decides what happens when it crosses into peak (`pause-llm` holds its LLM calls;
`abort` stops it). `DOC_PROCESS_OFFPEAK_LEAD_MINUTES` is no longer read.

### Requirement: Held pipelines release their slot
**Reason**: Slot release now happens while a task waits for admission
(`doc-processor-peak-controls`, "Waiting tasks are visible, stoppable and do not hold a slot").
**Migration**: None needed; waiting tasks still release their slot.
