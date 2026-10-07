## 1. Server-side statement kind

- [x] 1.1 Add `server/api/doc-processing/metric_statement_kind.go` with `metricStatementKind(map[string]any) string` and `isPureRequirementKind(string) bool`, porting the rules of `web/src/lib/metric-statement-kind.ts` (tags as `[]string`, `[]any` or JSON string)
- [x] 1.2 Add `metric_statement_kind_test.go` covering every scenario of `web/src/lib/metric-statement-kind.test.ts`; add cross-reference comments in the TS file and `benchmark_io.py` naming the Go copy

## 2. Filter and audit log

- [x] 2.1 In `enrichMetricCandidates`, after dedupe: canonicalize range types, drop pure-requirement rows, return the kept rows
- [x] 2.2 Write one `exclude_pure_requirements` doc_proc_log entry (extra_info `num_excluded` + per-kind counts; artifact rows with kind, metric_name, subject, threshold_or_target, context, source_line_spans) only when something was excluded
- [x] 2.3 Tests: inspection and delegated rows excluded; criterion, value-open and canonicalized-synonym rows kept; contiguous `metric_id`s on a force_clear run; log written iff something excluded

## 3. Prompts and defaults

- [x] 3.1 Add `prompts/prompt-extract-metric-candidates-v11.md` (v10 without the delegated-requirement section and rule 8; "Do not extract" gains requirements with no measurable property; keep the test-method-citation exception)
- [x] 3.2 Add `prompts/prompt-enrich-metrics-v8.md` (v7 with "Do Not Emit Pure Requirements" replacing "Do Not Drop Qualitative Requirements"; delegated row shape and rule 7 removed; value-open requirements kept)
- [x] 3.3 Move code defaults in `NewMetricsProcessor` / `loadMetricsPromptFromEnv` to v11/v8 and update any test asserting the old defaults; repoint `mise.local.toml`
- [x] 3.4 `go build ./...` and `go test ./server/api/doc-processing/...` pass

## 4. Verify live

  - 2026-10-07 06:54 attempt did not test this change: the event was consumed by a stale
    `mise doc-processor-run` process (started 2026-10-06 03:35, old code, prompts v10/v7), not by
    the `mise dev` server. Result: 416 re-extracted the old way (45 → 34 rows, 18 delegated + 2
    inspection still stored). Needs a doc-processor restart, then re-run.
- [ ] 4.1 Re-extract record 416 with force_clear; confirm no `inspection_requirement`/`delegated_requirement` rows in `kb.metrics` and one `exclude_pure_requirements` log entry (or none, if the prompt already avoided them)
- [ ] 4.2 Compare remaining kinds with the gold 2.1.0 run for 416 (gold minus pure-requirement kinds) and record the result here

## 5. Docs

- [x] 5.1 Amend ADR 2026100603: change-log entry, DR4 gate relaxed for pure requirements, suspended lossless invariant stated
- [x] 5.2 Update devdoc 2026100701 "Known limitations" (requirements still in metrics table → pure requirements now excluded; point to this change)
- [x] 5.3 Update coding capsule `KnowledgeStore/Capsules/coding-capsules/doc-processor/extract-metrics-spec.md` (metric definition, Pass 1 rule, 3.3.1 rewritten, new 3.4.2 filter, logging, workflow, 5.1 note, references)
- [x] 5.4 Update coding capsule `extract-metrics-impl.md` (chunk input, chunk-batch path, disabled merge, grouped Pass 2, pure-requirement filter, prompt defaults v11/v8)
