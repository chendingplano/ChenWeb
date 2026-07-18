## ADDED Requirements

<!--
This capability was previously specified in the (never-archived) change
add-metrics-inventory-analyses/specs/metrics-reviewer-analyses/spec.md as one-row-per-candidate.
Since openspec/specs/ has no archived copy to diff against, this file is written as ADDED
(the full current state of the capability after this change), not as a MODIFIED delta.
-->

### Requirement: One comparison-analysis row per reviewed metric
The metrics reviewer SHALL produce at most one comparison-analysis row per doc metric that has
one or more matching metrics, covering every candidate in that metric's match list, rather than
one row per candidate.

#### Scenario: Metric with multiple matches and no conflicts
- **WHEN** a doc metric has 3 matching metrics and the LLM classifies all 3 as `related_distinct`
  or `unrelated`, with no conflicts, outliers, or currency signals
- **THEN** the reviewer persists exactly one `kb.doc_review_findings` row for that metric, whose
  `metadata.related_artifacts` array has 3 entries (one per candidate)

#### Scenario: Metric with a conflict among its matches
- **WHEN** a doc metric has 3 matching metrics and one is classified `same_conflict`
- **THEN** the reviewer still persists exactly one comparison-analysis row covering all 3
  candidates, plus a separate `finding_type="issue"` row for the conflict (unchanged path)

#### Scenario: Metric with no matches
- **WHEN** a doc metric has zero matching metrics
- **THEN** no LLM call is made for that metric and no comparison-analysis row is produced

#### Scenario: LLM returns an empty analyses list despite matches
- **WHEN** a doc metric has ≥1 matching metric but the LLM's parsed `analyses` list is empty
  (e.g. malformed response)
- **THEN** no comparison-analysis row is produced for that metric (defensive fallback; this
  should not occur under the prompt's normal contract)

### Requirement: Per-candidate detail stored as a related-artifacts array
The comparison-analysis row's `metadata` SHALL carry a `related_artifacts` array with one entry
per candidate, each holding that candidate's `related_artifact_id`, `related_record_id`,
`relationship`, and a `summary` scoped to that candidate only (not repeating the
metric-under-review's own description).

#### Scenario: Related-artifacts entry shape
- **WHEN** the reviewer converts a parsed `analyses` entry for metric `M` matched against
  candidate `related_artifact_id=R` classified `related_distinct`
- **THEN** the row's `metadata.related_artifacts` contains an entry
  `{"related_artifact_id": "R", "related_record_id": <candidate's source record>, "relationship": "related_distinct", "summary": "<candidate-specific text>"}`

#### Scenario: Row-level fields describe the metric under review once
- **WHEN** the reviewer builds the comparison-analysis row for metric `M`
- **THEN** the row has `pass="P5"`, `aspect="metrics"`, `finding_type="analysis"`,
  `severity="info"`, `confidence=1.0`, `artifact_id=M`, `metadata.result_kind="metric_analysis"`,
  and `description` set to a one-time explanation of the metric under review (not repeated per
  candidate)

#### Scenario: Row-level related-artifact fields are not set
- **WHEN** the reviewer builds the comparison-analysis row for metric `M` with multiple candidates
- **THEN** the row's singular `related_artifact_id` / `related_record_id` / `analysis_relationship`
  fields are left empty, since no single candidate is "the" related artifact for a
  multi-candidate row

### Requirement: Analyses supported on both call paths
The reviewer SHALL parse `analyses` and `metric_summary` from the LLM output identically whether
the review unit used the one-shot JSON-extraction path or the tool-use loop path.

#### Scenario: One-shot path
- **WHEN** `cfg.MaxToolTurns == 0` (or no tool client is configured) and the LLM returns a JSON
  object containing `findings`, `analyses`, and `metric_summary`
- **THEN** the reviewer parses `analyses` and `metric_summary` from the same payload used to parse
  `findings`

#### Scenario: Tool-use path
- **WHEN** `cfg.MaxToolTurns > 0` and a tool client is configured, and the tool-use loop's final
  response contains `findings`, `analyses`, and `metric_summary`
- **THEN** the reviewer parses `analyses` and `metric_summary` from the raw payload returned by the
  tool-use loop
