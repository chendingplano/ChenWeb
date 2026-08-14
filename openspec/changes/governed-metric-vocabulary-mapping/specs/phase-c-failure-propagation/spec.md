## ADDED Requirements

### Requirement: A Phase C `PostProcessIndex` error is persisted as a processor failure
When any registered `PostProcessIndexer` returns a non-nil error from `PostProcessIndex` for a
given input record, the doc-processing harness SHALL persist that processor's runtime status for
that record as `"failed"` (with the error message) via the same status write path Phase A/B
processors already use, in addition to the existing log/span behavior.

#### Scenario: A Phase C processor's PostProcessIndex returns an error
- **WHEN** `runPostProcessIndexing` invokes a `PostProcessIndexer`'s `PostProcessIndex` for a record and it returns a non-nil error
- **THEN** `kb.inputs.status` gains a `proc_status="failed"` entry for that processor and record, matching the entry Phase A/B failures already produce

#### Scenario: A Phase C processor's PostProcessIndex succeeds
- **WHEN** `PostProcessIndex` returns nil
- **THEN** behavior is unchanged from today (span marked success, info log; no new failure status written)

### Requirement: A persisted Phase C failure reaches the retry and dashboard surfaces
A record with a persisted Phase C `"failed"` processor status SHALL be reachable through the
existing failed-processor retry mechanism and reflected in the existing admin dashboard
per-processor status column, without any change to either consumer.

#### Scenario: Failed-processor retry picks up a Phase C failure
- **WHEN** a record has a Phase C processor persisted as `"failed"` via the requirement above
- **THEN** `ListRecordsWithFailedDocProcessors` (`WHERE has_failed_proc`) includes that record, making it eligible for `--all=failed-procs` batch retry

### Requirement: A processor's own deliberate error-swallowing is unaffected
A `PostProcessIndexer` that intentionally does not propagate an internal failure as its own
return value (e.g. a telemetry/report-build step logged but not returned) SHALL continue to be
reported as successful — this harness fix only changes behavior for errors a processor already
chooses to return.

#### Scenario: A processor swallows an internal report-build error
- **WHEN** `ProjectSemanticsProcessor.PostProcessIndex` runs and its internal association-run report build fails, but the function itself returns nil as it does today
- **THEN** no failure status is persisted for `project_semantics` on that record — behavior identical to before this change
