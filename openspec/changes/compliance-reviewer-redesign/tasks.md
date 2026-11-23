## 1. Database and stored fields

- [x] 1.1 Migration: `kb.doc_review_findings` gains `cited_documents JSONB`, `error_msg TEXT`, `proc_status TEXT`
- [x] 1.2 Migration: `kb.failed_llm_calls` gains `attempt`, `model`, `outcome`, `review_run_id`, `aspect`; `fallback_model` nullable
- [x] 1.3 `ReviewFinding` gains `CitedDocuments`, `ErrorMsg`, `ProcStatus`; storage preparation and `SaveFindings` write them
- [x] 1.4 `llmusage.RecordFailedCall` writes one row per failed try
- [x] 1.5 Apply both migrations to the development database

## 2. Settings and prompts

- [x] 2.1 `ReviewAspectConfig`: add `relevance_prompt`, `judgement_prompt`, `correction_prompt`, `top_metrics`, `top_provisions`, `top_artifacts`, `max_cited_documents`, `max_metrics`, `max_provisions`; remove the three retired prompt keys and `min_cosine`
- [x] 2.2 Prompt files: detect v4, relevance v1, judgement v1, verifier v3, correction v1
- [x] 2.3 `doc-review.local.toml` `[reviewers.compliance]` updated
- [x] 2.4 Reviewer construction: correction, fallback and relevance clients from the environment; configuration errors stop every chunk task with the name of the setting

## 3. Activities

- [x] 3.1 Prerequisite check from `kb.doc_process_runs`
- [x] 3.2 Per-record plan: load metrics and provisions with lines and index ids, merge exact copies, apply limits, load reference mentions
- [x] 3.3 Chunk activities and groups by overlapping lines
- [x] 3.4 Detect input with `existing_activities`; drop covered text activities; parse errors reported together

## 4. Search and relevance

- [x] 4.1 Per-kind search (reference queries, text queries merged round-robin, cited documents up to the limit), trace with keywords and excluded records
- [x] 4.2 Candidate text through `searchengine.Load`, cut to 2,000 characters
- [x] 4.3 Relevance check: one decision request per activity, removed candidates kept in the trace

## 5. Judgement, numbers and verification

- [x] 5.1 Judgement call with schema and CheckParsed reporting every problem
- [x] 5.2 Numeric comparison through `checkComplianceNumeric`
- [x] 5.3 Verification of strong verdicts; downgrade on rejection; severity
- [x] 5.4 Finding text, plain candidate list for the scorer, related-artifact rows, stored metadata
- [x] 5.5 Remove the requirement, evidence-screening and reference code paths and their tests

## 6. Three tries and errors

- [x] 6.1 `callWithTries`: three tries, correction request, provider-failure handling, alerts dropped, failed tries recorded
- [x] 6.2 Per-unit error capture; detection failure finding; `compliance_unchecked` finding
- [x] 6.3 Processor sets `proc_status` and marks only the failed aspect
- [x] 6.4 Doc-review run id passed to the reviewer through the context

## 7. Scorer

- [x] 7.1 `LoadReviewRows` refuses failed runs and skips `compliance_unchecked`

## 8. Tests and verification

- [x] 8.1 Unit tests: prerequisite, plan merging and limits, groups, covered text activities, per-kind search requests, relevance removal, judgement checks, numeric decision, verifier downgrade, three tries, error capture, proc status, scorer refusal
- [x] 8.2 `go build ./...`, `go vet`, and the doc-reviews, compliance-eval and llmusage test suites pass
- [x] 8.3 Live run on record 825 in development, with the result recorded in the devdoc

## 9. Documentation and catalogue

- [x] 9.1 Devdoc 2026111201 matches what was built (prompts, settings, tests, where to find it)
- [x] 9.2 Register the decision-model question code with `create-reusable-module`
- [x] 9.3 Update module 20261119-01 Recovered LLM Calls (row and doc) for the new columns and writer; run the catalogue check

## 10. Report language (2026-11-23)

- [x] 10.1 `output_language` (first `DOC_REVIEW_REPORT_LANGUAGE`) in judgement and verification input; judgement prompt v2 and verifier prompt v4 write in it; assessment findings declare it
- [x] 10.2 Compliance findings skip finding translation at save time, on page localization and on single-finding translation
- [x] 10.3 Unit tests for both; devdoc 2026111201 and ADR 2026063001 updated
- [ ] 10.4 Live review with the new prompts

## 11. Relevance in one call per activity (2026-11-23)

- [x] 11.1 Relevance check as one structured call per activity through `callWithTries`: yes or no and a 0–9 score per candidate, correction request naming every problem; `DOC_REVIEWER_RELEVANCE_MODEL_NAME` and the decision-model client removed
- [x] 11.2 `[reviewers.compliance].effort`: `normal` keeps score 3 or more (default), `high` keeps 2 or more; any other value refused when the configuration loads
- [x] 11.3 Prompt `prompt-review-compliance-relevance-v3.md`; offline replay on 30 activities of run 13882 recorded in design D6 and devdoc 2026111201
- [x] 11.4 Unit tests: one call per activity, score cut-off by effort, wrong answer corrected, three tries, configuration
- [ ] 11.5 Live review with the new relevance check

## 12. Metric candidate content (2026-11-23)

- [x] 12.1 Metric candidates carry `doc_no` and title (`kb.inputs`), `name` and `subject` (`kb.metrics`), and `metric_context` as their text, in the relevance, judgement and verification calls; a metric without `metric_context` keeps the search-index text
- [x] 12.2 Prompts `prompt-review-compliance-relevance-v4.md`, `prompt-review-compliance-judgement-v3.md`, `prompt-review-compliance-verifier-v5.md` describe the metric fields
- [x] 12.3 Unit test: a name-only metric reaches relevance and judgement with all five fields; a metric without source text keeps the index text
- [x] 12.4 Replay of one saved relevance call (record 825, lines 613–624) recorded in devdoc 2026111201
- [ ] 12.5 Replay the 30 cut-off activities with the new candidate content and re-check the `effort` cut-offs
