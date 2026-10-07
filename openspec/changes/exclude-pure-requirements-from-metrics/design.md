## Context

`extract_metrics` runs in two LLM passes. Pass 1 (`extract-metric-candidates`) finds
candidates. Pass 2 (`enrich-metrics`) turns each batch of candidates into final rows. Both save
paths go through `MetricsProcessor.enrichMetricCandidates`. The paths are the sequential
`HandleEvent` → `extractMetricsFromChunksWithLLM` and the chunk-batch `FinalizeChunkBatch`.
`metric_id`s are assigned after enrichment, by position.

Two prompt sections ask for pure requirements today:
- `prompt-extract-metric-candidates-v10.md`: "Requirements Delegated To A Cited Document"
  (emit a `ref:` candidate) and rule 8.
- `prompt-enrich-metrics-v7.md`: "Do Not Drop Qualitative Requirements" and the delegated row
  shape (`value_class = reference`, tags `external_reference`, `cited_doc:`).

Phase 1 classifies rows by statement kind in the browser (`web/src/lib/metric-statement-kind.ts`).
The benchmark helper has a Python copy (`statement_kind` in `benchmark_io.py`). The server has
none.

Code defaults are stale: `prompt-extract-metric-candidates-v5.md` and
`prompt-enrich-metrics-v6.md`. `mise.local.toml` pins v10 and v7.

## Goals / Non-Goals

**Goals:**
- No new `kb.metrics` row has kind `inspection_requirement` or `delegated_requirement`.
- Every excluded row is recorded where an operator can find it.
- The LLM stops spending tokens on rows that are then discarded.

**Non-Goals:**
- Fixing or benchmarking `extract_provisions`, or linking metrics to provisions (Phase 2).
- Deleting pure-requirement rows already in `kb.metrics`.
- Changing what counts as `requirement_value_open`. It stays a metric (user decision).
- UI changes. The Phase 1 badge still labels any old rows.

## Decisions

**D1 — Exclude by statement kind, using the Phase 1 rules.** The excluded set is exactly
`inspection_requirement` and `delegated_requirement` as defined in spec `metric-statement-kind`.
Reusing those rules means the browser label, the benchmark tag (rules 2.1.0 `kind:`) and the
server filter cannot disagree about what a pure requirement is. *Alternative:* a new,
server-only definition, for example "no unit and no number". Rejected because it would drift
from the labels customers already see.

**D2 — Port the rules to Go as a third copy, kept in step by tests.** The new file is
`server/api/doc-processing/metric_statement_kind.go`, exposing `metricStatementKind(row
map[string]any) string`. It handles `reasoning_tags` given as `[]string`, `[]any` or a JSON
string. The test reuses every scenario from `web/src/lib/metric-statement-kind.test.ts`. The
TS file's header comment and the Go file each name the other copies. *Alternative:* serve the
kind from the server and drop the TS copy. Rejected as scope creep, because Phase 1 deliberately
classifies at read time for rows that are already stored.

**D3 — Filter inside `enrichMetricCandidates`, after dedupe.** This is the one place both save
paths share, and it runs before `metric_id`s are assigned, so the ids stay contiguous.
`canonicalizeMetricValueRangeTypes` runs first. Raw model output can carry non-canonical range
types, and the classifier needs the canonical ones. Canonicalizing twice is harmless.
`uncertain_metrics` are not saved and are not filtered.

**D4 — Prompt change plus filter, not one or the other.** The prompt alone is not reliable:
models drift and old candidates still look like requirements. The filter alone wastes pass-2
tokens and contradicts the prompt's explicit instruction to emit these rows. With both, the
prompt avoids the cost and the filter guarantees the result.
- Candidates v11: remove the delegated-requirement section and rule 8. Add, under "Do not
  extract", clauses that impose a requirement with no measurable property (inspection features
  such as enclosed or lidded, and requirements whose criteria are only "per <standard>"). Keep
  the exception for a clause that states its own number and cites a document only for the test
  method.
- Enrich v8: replace "Do Not Drop Qualitative Requirements" with "Do Not Emit Pure
  Requirements". If a candidate has no measurable property, emit no row for it. A requirement
  that names a quantity but leaves the value open is still emitted, with `limit_absent`.
  Remove the delegated row shape and rule 7. Keep `reference` in the `value_class` vocabulary
  for numeric values quoted from a cited document. The classifier treats those as
  `metric_value`.

**D5 — Record exclusions in `kb.doc_proc_logs`.** One entry per run that excluded at least one
row: entry type `extract_metrics`, activity `exclude_pure_requirements`. `extra_info` holds
`num_excluded` and per-kind counts. The artifact lists each excluded row's `kind`,
`metric_name`, `subject`, `threshold_or_target`, `context` and `source_line_spans`. The same
counts go to the structured logger. *Alternative:* a new table, or writing the rows into
`kb.provisions`. Rejected because a second writer to provisions breaks ADR 2026072701 DR7's
single owner, and a table is more than an audit trail needs today.

**D6 — Code defaults move to v11/v8; no feature flag.** A fresh environment gets the new
behavior without local pins. Rollback means reverting the commit: pointing the prompts back at
v10/v7 would still be filtered. *Alternative:* an `EXTRACT_METRICS_EXCLUDE_PURE_REQUIREMENTS`
flag. Not added: nothing asks for a partial rollout, and the ADR only calls for a flag if one is
needed.

## Risks / Trade-offs

- [Requirements vanish from the product until provisions run] → They are logged (D5). The ADR
  amendment states the suspended invariant. Provisions remain the planned home.
- [Classifier misreads a measurable row as pure] → For example, a numeric value the model wrote
  as `qualitative`, or a test setting without the `test_condition` tag that has no number. Such
  rows are dropped but logged. Verification on record 416 compares the excluded list with the
  gold kinds.
- [Merge-mode runs keep old pure rows] → Documented. Re-extract with `force_clear` to heal.
- [Benchmark comparisons look like recall loss] → Filter gold by `kind:` tag (proposal Impact).
  The ADR already warns that counts will drop by definition, not by regression.
- [Three copies of the rules] → Shared scenarios in tests, plus cross-reference comments.

## Migration Plan

1. Ship the filter, the prompts and the defaults. Repoint the `mise.local.toml` pins.
2. Re-extract record 416 with `force_clear` and check: no excluded kinds in `kb.metrics`, one
   `exclude_pure_requirements` log entry, and the remaining kinds compared with gold.
3. Before deploying to production, check that the box's environment does not pin v10/v7.

Rollback: revert the commit. Excluded rows can be recovered from the logs if needed.

## Open Questions

- None blocking. When `extract_provisions` is revisited, decide whether the
  `exclude_pure_requirements` log should be checked against `kb.provisions` (the ADR's
  per-document lossless gate).
