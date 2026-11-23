## Context

Compliance Review (`server/api/doc-reviews/review-compliance*.go`) runs per 200-line chunk under the
prompt-cache scheduler. Today, for each model-found activity it searches three artifact kinds, reads up
to eight source documents in full to extract requirements, screens up to 32 chunks of the reviewed
document for evidence, assesses, verifies and gives candidate verdicts. Any error aborts the aspect
and the scheduler drops all of its findings. The agreed redesign is described for readers in devdoc
2026111201 (sections 3–7); this design records how it is built.

Facts the design relies on (checked 2026-11-22 on the development database):

- `kb.doc_process_runs.processors` (JSONB array) and `status` record which processors completed.
- Every metric and provision has `line_range` (int8multirange) and, when indexed, a
  `kb.search_artifacts` row with `source_row_id` = the metric or provision id.
- `kb.metrics.provision_id` and `ref_mention_id` were empty for all metrics of records 416 and 825, so
  links between metrics, provisions and citations come from overlapping lines.
- `kb.metrics.value_min` / `value_max` are mostly empty and `metric_value` is free text, so numbers
  cannot be compared straight from the table.
- `searchengine.Search` accepts `Query.Ref` (an indexed artifact): it uses the artifact's stored
  embedding, text and keywords, so metric and provision searches need no new embedding.
- A decision-model request can carry many yes-or-no (`noul`) questions (`categorytree.JevDecider`).
- `ExtractStructuredJSON` returns a `StructuredOutputError` whose kind tells a provider failure from a
  wrong answer, with the raw answer attached.
- `finishReviewAspects` marks every aspect failed for an error that is not a
  `reviewerExecutionError`, so a per-aspect failure must be wrapped in one.

## Goals / Non-Goals

**Goals:**
- Reuse extracted metrics and provisions as activities; judge each activity in one call.
- Remove unrelated candidates before judgement with one relevance call per activity.
- Never abort the reviewer for one activity's error; record it and mark the run failed.
- Three tries per structured model call through two configured models.

**Non-Goals:**
- Starting extraction automatically (the reviewer refuses instead).
- Saving findings incrementally (a crash still loses the run).
- Applying the three tries to other reviewers (later, once proven here).
- A new gold-rules version (after the reviewer works).

## Decisions

**D1. One per-record plan, cached.** The first chunk task of a record loads a plan once (guarded per
record): the prerequisite check, every metric and provision with its line range, search-index
artifact id and duplicate key, exact-copy merging, the `max_metrics` / `max_provisions` limits (first
in line order), and the record's reference mentions. Every chunk reads its activities from the plan:
those whose first line falls in the chunk. Alternative: load per chunk — rejected, the limits and
copy merging are document-wide.

**D2. Prerequisite from processing runs.** A record qualifies when a `kb.doc_process_runs` row with
`status = 'success'` lists `extract_metrics_simple`, and one lists `extract_provisions`. A failure is a
plain error from every chunk task, so the scheduler aborts the aspect. Row counts are not used.

**D3. Activity groups by overlapping lines, inside a chunk.** Union-find over the chunk's metric and
provision activities whose half-open line ranges overlap. A group is one judgement unit and one
finding; each member keeps its own search, relevance decisions and verdicts. Exact copies (same type
and normalized duplicate key: metric name, value, unit and condition; provision text) merge before
grouping and keep every location. Activities without lines are counted as unchecked.

**D4. Text activities skip covered lines.** The detect input gains `existing_activities` (the chunk's
metric and provision activities, label and lines), appended inside the existing JSON envelope so the
chunk prefix stays byte-identical for prompt caching. Code then drops a text activity whose every line
is covered by a metric or provision of the chunk.

**D5. Search per kind.** Metric and provision members search with `Query.Ref` and
`ArtifactTypes = [own kind]`, limit `top_metrics` / `top_provisions`, taking the fused ranking. Text
activities run each model query over `[provision, metric, chunk]` with limit `top_artifacts` and merge
the queries' results round-robin by rank until the limit. Cited-document searches (first
`max_cited_documents`, default 0) add the citation text as one more query of the activity's kinds.
Every search excludes the record and its same-md5 copies. The trace keeps query text or reference,
keywords, kinds, limit, excluded records, possibly-cut-off flag and results. Candidate text comes from
`searchengine.Load`, cut to 2,000 characters. The old interleaving of three rankings and the
list-only mode with `min_cosine` are removed.

**D6. Relevance check: one structured call per activity** (revised 2026-11-23). The input holds the
activity and a list of candidates with keys `c1..cN`; `relevance_prompt` (v3) asks, for each one,
`relevant` yes or no and a `score` from 0 to 9. The call goes through `callWithTries` like the other
stages, so it runs at temperature 0, gets the three tries, and a wrong answer (a missing or unknown
key, an invalid value) gets a correction request naming every problem. `[reviewers.compliance].effort`
sets the cut-off: `normal` keeps score 3 or more, `high` keeps 2 or more. Decisions, with answer and
score, are stored in the trace.

The first version asked a decision model one yes-or-no question per candidate in one request. The
model was an ordinary chat profile run through the emulated decision provider, which sends every
question as its own one-token call carrying all the candidates: run 13882 (record 825) made 7,170
relevance calls for 239 activities. One call that answers all candidates writes the answers in
sequence, so each answer depends on the earlier ones. At temperature 1, which the emulation needs for
DeepSeek's probabilities, an early answer drawn at random sways the later ones. At temperature 0 the
answers are stable. Measured on 30 activities (900 candidates) against labels from a reasoning model
(62 relevant): the per-candidate calls kept 84% of the relevant candidates and passed 372 to the
judgement; v3 at `normal` kept 89% and passed about 345, and at `high` kept all and passed about 600.
Yes or no alone, without a score, kept only 69%.

**D7. Judgement call.** Structured calls per unit with `judgement_prompt`, at most 20
activity–candidate pairs each (the first live run on record 825 had a 48-metric group whose single
answer was cut off at the output limit); a failed batch keeps the other batches' verdicts. Input: assessment
date, the chunk lines, the unit's activities (`activity_id`, kind, label, text, lines, context facts,
relationship, candidate keys) and the candidate texts. Output `verdicts[]`: `activity_id`,
`artifact_key`, `verdict`, `binding` (`mandatory`, `recommendation`, `none`; optional except for
strong verdicts, missing means `none`), `summary`,
`candidate_quote`, `document_quote`, `document_line_start`, `document_line_end`, optional `numeric`
(the existing `complianceNumeric`). CheckParsed reports every problem at once: missing, repeated or
unknown pairs; bad enums; empty summary; compliant or not compliant without a mandatory binding or
without both quotes; quotes that do not match (candidate text by letters and digits; chunk lines by
the existing evidence check); a recommendation marked compliant or not compliant.

**D8. Numbers through the module.** When a verdict carries `numeric`, binding is mandatory, both
quotes are present and the verdict is compliant, compatible or not compliant, the existing
`checkComplianceNumeric` decides: supported → compliant, contradicted → not compliant, otherwise
undetermined with the reason as note.

**D9. Verification only for strong verdicts.** One structured call with `verifier_prompt` per unit
that has compliant or not compliant verdicts. Output `checks[]` (`activity_id`, `artifact_key`,
`valid`, `reason`), one per strong verdict. Invalid → undetermined with the reason as note. A finding
is medium when a valid not compliant verdict remains.

**D10. Three tries.** `callWithTries` runs a structured call on the reviewer's client and model, then
on clients built with `docprocessing.BuildReviewerLLMClientWithThinking` from
`DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME` and `DOC_REVIEWER_FALLBACK_MODEL_NAME` (profile thinking
setting). Each try uses `MaxRetries: 0`. A wrong answer is a `StructuredOutputError` with a raw answer
that is neither a provider failure nor a truncation. The correction request is the original input
plus the text of `correction_prompt` with `{{errors}}` (the CheckParsed message, unwrapped) and
`{{previous_answer}}` filled in; it is rebuilt from the latest wrong answer for the next try. Each retry records
`correcting call: <previous-event-id>` in its usage event's `error_message` and the same ID in
`metadata_json.correcting_call_id`. An accepted retry has only the note; a rejected retry keeps the
error before the note. The usage log and daily summary treat a note-only message as success. The
structured result or error returns the persisted event ID for that invocation, so concurrent chunk
calls never obtain it from shared last-usage state. A
try after a provider failure with no wrong answer yet sends the original input. Alerts of the chain
are deferred and dropped (`DeferCallAlerts`, finish with `true`). Each failed try becomes a
`kb.failed_llm_calls` row written after the chain ends, with `outcome` `recovered` or `failed`.
Activity detection, judgement and verification use it.

**D11. Errors stay with the activity.** Each unit runs inside a function that never returns an error:
a failure sets `ErrorMsg` and produces a `compliance_incomplete` finding (or keeps the assessment
finding when only verification failed). A detection failure gives one `compliance_incomplete` finding
for the chunk's lines. Configuration errors and the prerequisite still return errors.

**D12. Run status after all reviewers.** After the scheduler returns, the processor sets
`ProcStatus` on every finding: `failed` for an aspect with any `ErrorMsg`, else `success`, and appends a
`reviewerExecutionError` for each failed aspect so `finishReviewAspects` marks only that aspect failed.
Findings are saved first, as today.

**D13. Stored fields.** `kb.doc_review_findings` gains `cited_documents JSONB`, `error_msg TEXT`,
`proc_status TEXT`. `kb.failed_llm_calls` gains `attempt INT`, `model TEXT`, `outcome TEXT`,
`review_run_id BIGINT`, `aspect TEXT`, and `fallback_model` becomes nullable. The doc-review run id
reaches the reviewer through the context.

**D14. Unchecked activities.** The first chunk writes one `compliance_unchecked` finding when the
limits or missing lines left activities unchecked. The scorer skips this type and refuses runs whose
compliance findings have `proc_status = 'failed'`.

**D15. Concurrency.** Units of one chunk run with `runConcurrent`, at most four at a time; chunks
already run in parallel under `MAX_DOC_REVIEWER_TASKS`.

## Reused modules

- **20261111-01 Artifact Search** — `searchengine.Search` with reference and text queries, filters
  and `searchengine.Load` for candidate text. Used as is.
- **20261121-01 Structured LLM Output** — schema plus CheckParsed contracts; one attempt per model.
  The three-try chain sits in the reviewer, not in the shared module, because it is compliance-only
  for now; once proven it can move into the module for every reviewer.
- **20261119-01 Recovered LLM Calls** — `kb.failed_llm_calls` and deferred call alerts. Extended
  with attempt columns and a writer for every failed try; its row and doc are updated.
- **20261114-03 Quantity Constraint Comparison** — through the existing `checkComplianceNumeric`.
- **20261114-02 Document Identifier Keys** — through the existing cited-document match.
- **Decision model questions (20261122-01)** — used by the first relevance check and registered by
  this change; the revised relevance check (D6) no longer uses it.

## Risks / Trade-offs

- [Only the activity's chunk is evidence] → accepted; findings say so; omission reports removed.
- [Candidate scope outside its 2,000 characters is unseen] → accepted; verifier still checks strong
  verdicts.
- [Many decision questions] → one request per activity; limits cap metric and provision activities.
- [Correction model loses the prompt cache] → only on wrong answers, which are rare.
- [Partial runs look complete] → `proc_status` and the aspect status say failed; the scorer refuses.

## Migration Plan

Apply the two goose migrations (`doc-service --migrate-only`), set the three environment variables,
update `[reviewers.compliance]`, restart the API server. Rollback: revert the code; the new columns
are nullable and unused by old code.

## Open Questions

None.
