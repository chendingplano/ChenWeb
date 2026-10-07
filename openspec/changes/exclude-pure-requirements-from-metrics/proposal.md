## Why

A metric is a measurable property. `extract_metrics` still stores clauses with nothing to measure
as metric rows. Examples are "the bin shall have a lid" (an inspection requirement) and "shall
comply with CJJ 52" (a delegated requirement). In record 416 these are 32 of the 69 gold rows.
Phase 1 (`requirements-metrics-phase1`) only relabels them when a page is shown. They still feed
metric search, the metric ontology (provisional classes with no unit) and every count of
"metrics". ADR 2026100603 DR4 waits for `extract_provisions` before removing them. The user has
decided (2026-10-07) not to wait: pure requirements leave `kb.metrics` now, and the
provisions benchmark is deferred.

## What Changes

- **BREAKING (data shape):** `extract_metrics` no longer stores rows whose statement kind is
  `inspection_requirement` or `delegated_requirement`. These are the same kinds the Phase 1
  classifier already computes. Metric counts per document drop. For record 416, 32 of the
  69 benchmark kinds are no longer expected.
- Rows that keep a measurable property are unchanged. These are numeric criteria
  (`requirement_with_criterion`), requirements that name a quantity and leave its value open
  (`requirement_value_open`), test parameters, metric definitions, observations and values.
- New prompt versions `prompt-extract-metric-candidates-v11.md` and `prompt-enrich-metrics-v8.md`
  stop asking for pure requirements. They remove "Requirements Delegated To A Cited Document"
  from the candidate prompt and replace "Do Not Drop Qualitative Requirements" in the enrich
  prompt. The code defaults move to these versions.
- A deterministic filter runs after enrichment in the server. It uses the same rules as the
  Phase 1 classifier, ported to Go, and drops any pure-requirement row the model still emits.
  This applies to both the sequential and chunk-batch paths.
- Each run that drops rows writes one `extract_metrics` entry in `kb.doc_proc_logs` (activity
  `exclude_pure_requirements`) listing every excluded row with its kind, name, subject, clause
  text and source line spans. Nothing disappears without a record.
- ADR 2026100603 is amended. The DR4 gate is relaxed for pure requirements only, and the
  lossless-across-families invariant is knowingly suspended for them until `extract_provisions`
  runs reliably.

## Capabilities

### New Capabilities
- `metric-pure-requirement-exclusion`: `extract_metrics` keeps only rows with a measurable
  property. It defines which statement kinds are excluded, where the filter applies, and how
  excluded rows are recorded.

### Modified Capabilities
<!-- None. metric-statement-kind's rules are reused unchanged; its display requirements still
     hold for rows already stored. -->

## Impact

- Code: `server/api/doc-processing/extract-metrics.go` (filter in `enrichMetricCandidates`,
  default prompt refs), new `server/api/doc-processing/metric_statement_kind.go` plus tests.
- Prompts: two new prompt files. The local `mise.local.toml` pins move to them. The production
  box's environment must be checked at the next deploy.
- Data: new extractions only. Existing rows stay until the record is re-extracted with
  `force_clear`. A merge-mode run (`force_clear=false`) does not delete old pure-requirement
  rows.
- Requirements excluded here have no customer-visible home until `extract_provisions` runs on
  the document. `kb.provisions` currently covers 2 of the 13 records that have metrics. A
  separate defect (provisions marked `success` with zero rows) is out of scope.
- Benchmark: there is no automated scorer for `testbed.metrics`. Anyone comparing a run against
  gold rules 2.1.0 must drop gold rows tagged `kind:inspection_requirement` or
  `kind:delegated_requirement`.
- Docs: ADR 2026100603 (change log, DR4), devdoc 2026100701 ("Known limitations").
