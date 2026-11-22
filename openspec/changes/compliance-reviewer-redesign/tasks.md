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
