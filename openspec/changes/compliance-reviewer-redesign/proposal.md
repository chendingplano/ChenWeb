## Why

Compliance Review costs too much and its results are not good enough. For every activity it reads up to
eight whole source documents and screens up to 32 chunks of the reviewed document, and one bad model
answer aborts the whole reviewer and throws away everything it found. The user agreed a redesign on
2026-11-22 (devdoc 2026111201, sections 3–7): reuse the metrics and provisions that extraction has
already produced as activities, filter candidates with a relevance check, judge each activity in one
call, and never let a single error stop the reviewer.

## What Changes

- **Prerequisite check.** The reviewer refuses to start unless `extract_metrics_simple` and
  `extract_provisions` have each completed on the reviewed record (a successful
  `kb.doc_process_runs` row naming the processor). **BREAKING** for records without them.
- **Three kinds of activity.** Every metric and provision of the reviewed record becomes an activity
  (capped by `max_metrics` / `max_provisions`, default 100 each, first in document order after exact
  copies merge). The model finds only text activities that they do not already state
  (`prompt-review-compliance-v4.md`), and code drops a text activity whose lines lie inside them.
- **Duplicates.** Exact copies merge; metrics and provisions with overlapping lines form an activity
  group, judged in one call and written as one finding.
- **Search.** Metric activities search metrics (`top_metrics`), provision activities search provisions
  (`top_provisions`), both by indexed-artifact reference; text activities search provisions, metrics
  and document passages together (`top_artifacts`). Defaults 30. `max_cited_documents` (default 0)
  limits cited-document searches; cited document names are always saved.
- **Relevance check (new).** One structured call per activity answers, for each candidate, yes or no
  and a score from 0 to 9 (`prompt-review-compliance-relevance-v3.md`); `effort` sets the cut-off.
  Irrelevant candidates are removed but kept in the trace. (The first version used a decision model,
  `DOC_REVIEWER_RELEVANCE_MODEL_NAME`; replaced on 2026-11-23, see design D6.)
- **One judgement call** (`prompt-review-compliance-judgement-v1.md`) per text activity or activity
  group replaces requirement extraction, evidence screening and assessment. Only the activity's own
  chunk is evidence; missing-requirement reports are removed. Numbers keep going through the
  quantity-comparison module. **BREAKING**: `requirements_prompt`, `evidence_prompt` and
  `assessment_prompt` are no longer read.
- **Verification** (`prompt-review-compliance-verifier-v3.md`) checks only compliant and not compliant
  verdicts.
- **Three tries per model call:** the reviewer's model, then `DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME`
  with a correction request, then `DOC_REVIEWER_FALLBACK_MODEL_NAME` with the same request. No
  warnings; every failed try goes to `kb.failed_llm_calls`.
- **Errors do not stop the reviewer.** A failed activity gets `kb.doc_review_findings.error_msg` and a
  `compliance_incomplete` finding; the run's compliance findings get `proc_status` `failed` (else
  `success`) and the aspect status becomes failed. The scorer refuses failed runs.
- New columns: `kb.doc_review_findings.cited_documents` (JSONB), `error_msg` (TEXT), `proc_status`
  (TEXT); `kb.failed_llm_calls` gains `attempt`, `model`, `outcome`, `review_run_id`, `aspect`, and
  `fallback_model` becomes optional.
- The old list-only mode and `min_cosine` are removed.

## Capabilities

### New Capabilities
- `compliance-review-redesign`: the redesigned Compliance Review pipeline — prerequisites, activity
  kinds and limits, deduplication, per-kind search, relevance check, judgement, verification, three
  tries, error handling and the stored fields.

### Modified Capabilities
- None in `openspec/specs/`. The earlier changes `compliance-reviewer-phase2` and
  `compliance-candidate-verdicts` are not archived; where their specs describe requirement
  extraction, evidence screening, omission reporting or aborting on error, this change supersedes
  them.

## Impact

- Code: `ChenWeb/server/api/doc-reviews/review-compliance*.go` (largely rewritten),
  `review-config.go`, `review-document.go` (finding fields, run status), `models.go`;
  `ChenWeb/server/api/llmusage/recovered_call.go`; `ChenWeb/server/api/compliance-eval/scoreio.go`.
- Prompts: four new files in `ChenWeb/prompts/`; settings in `ChenWeb/doc-review.local.toml`.
- Database: two goose migrations in `ChenWeb/project_migrations/`.
- Environment: `DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME`, `DOC_REVIEWER_FALLBACK_MODEL_NAME`
  (profiles in `MODEL_DEF_FILE`).
- Reused catalogued modules: 20261111-01 Artifact Search (reference and text queries, Load),
  20261121-01 Structured LLM Output (schema plus CheckParsed, one attempt per model),
  20261119-01 Recovered LLM Calls (`kb.failed_llm_calls`, deferred call alerts; extended),
  20261114-03 Quantity Constraint Comparison, 20261114-02 Document Identifier Keys (cited-document
  matching). Searched for decision-model questions: not catalogued; the code in
  `ChenWeb/server/api/decisionmodel` and the Jev question pattern of `categorytree` is reused and will
  be registered.
- IDs: none new; rows keep their own primary keys.
