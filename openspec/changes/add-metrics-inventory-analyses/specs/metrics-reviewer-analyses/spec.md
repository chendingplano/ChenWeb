## ADDED Requirements

### Requirement: Mandatory per-match comparison analyses
The metrics reviewer SHALL produce exactly one comparison analysis entry per entry in
`matching_metrics` for every reviewed metric with one or more matching metrics, regardless of
whether the comparison rises to a reportable finding.

#### Scenario: Metric with matches and no issues
- **WHEN** a doc metric has 3 matching metrics and the LLM finds no conflicts, outliers, or
  currency signals
- **THEN** the reviewer returns 3 `analyses` entries (one per match) and `findings` may be empty

#### Scenario: Metric with matches and a conflict
- **WHEN** a doc metric has 3 matching metrics and one conflicts with the metric under review
- **THEN** the reviewer still returns 3 `analyses` entries, and at least one `findings` entry
  describing the conflict

#### Scenario: Metric with no matches
- **WHEN** a doc metric has zero matching metrics
- **THEN** no LLM call is made for that metric and no `analyses` entries are produced for it

### Requirement: Analyses persisted as findings-table rows
Each parsed comparison analysis SHALL be converted into a `ReviewFinding` and persisted through
the same `kb.doc_review_findings` store used for reportable findings, tagged so it is
distinguishable from `issue`/`observation` rows.

#### Scenario: Analysis row shape
- **WHEN** the reviewer converts a parsed `analyses` entry for metric `M` matched against
  `related_artifact_id=R`
- **THEN** the resulting row has `pass="P5"`, `aspect="metrics"`, `finding_type="analysis"`,
  `severity="info"`, `confidence=1.0`, `artifact_id=M`, `related_artifact_id=R`,
  `related_record_id` set from the analysis, `metadata.result_kind="metric_analysis"`, and
  `metadata.analysis_relationship` set to the analysis's relationship value

#### Scenario: Analysis persisted alongside findings in the same run
- **WHEN** a review run produces both `issue`/`observation` findings and `analysis` rows for the
  same metric
- **THEN** all rows are written under the same `run_id` via the existing findings store, with no
  separate side table

### Requirement: Analyses supported on both call paths
The reviewer SHALL parse `analyses` from the LLM output identically whether the review unit used
the one-shot JSON-extraction path or the tool-use loop path.

#### Scenario: One-shot path
- **WHEN** `cfg.MaxToolTurns == 0` (or no tool client is configured) and the LLM returns a JSON
  object containing both `findings` and `analyses`
- **THEN** the reviewer parses `analyses` from the same payload used to parse `findings`

#### Scenario: Tool-use path
- **WHEN** `cfg.MaxToolTurns > 0` and a tool client is configured, and the tool-use loop's final
  response contains both `findings` and `analyses`
- **THEN** the reviewer parses `analyses` from the raw payload returned by the tool-use loop
