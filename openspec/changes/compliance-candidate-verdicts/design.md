## Context

Compliance Review (openspec `compliance-reviewer-phase2`, devdoc 2026111201) works in stages for each activity:

1. Detect the activity.
2. Retrieve up to ten provisions, metrics and chunks per query.
3. Extract requirements from each source record. One requirement names one `artifact_key`.
4. Screen the whole reviewed document for each requirement.
5. Assess each requirement: applicability × evidence status.
6. Verify each requirement.

The row is one finding per activity. Its text lists the raw candidates, then the requirement assessments. A user reviewing an activity sees candidates with no conclusion about them. Candidates with no extracted requirement are never explained. Candidates that represent the same criterion as a sibling artifact get no link to it, because the requirements stage keeps one requirement and names only one key.

The doc-review pipeline already has a per-candidate result shape: `ReviewFinding.RelatedArtifacts`, where each entry holds a relationship and a summary. The metrics reviewer writes it, and the Finding Details panel renders it as a list. Each entry shows a relationship tag, the record and artifact, the summary, and a click-through to the candidate's source lines.

Constraints: prompts are versioned files in `prompts/`; the model is operator-configured; stored findings must stay readable; `compliance-score` reads candidate lines from the row text; user-visible UI text goes through Paraglide.

## Goals / Non-Goals

**Goals:**
- One verdict and one summary for every candidate of an assessed activity. Candidates are the retrieved artifacts and the dependency artifacts.
- Verdicts that are consistent with the verified requirement results. A strong verdict (`compliant`, `not_compliant`) never rests on an unverified or downgraded result.
- A clear `unqualified` class, so a later change can decide whether to drop those candidates.
- Display in the existing Finding Details panel, in English and Chinese.

**Non-Goals:**
- Dropping or hiding `unqualified` candidates, or removing them from scoring.
- Scoring verdicts against gold labels. The `governs` labels could later measure wrong `unqualified` verdicts.
- An activity-level conclusion, beyond verdict counts.
- Changes to detection, retrieval, screening, numeric comparison, severity rules, or the existing stage checks.
- Translating the fixed English scaffolding in the row text.

## Decisions

**D1. The candidate is the unit of the verdict; the requirement stays the unit of evidence.** Each candidate's verdict cites the requirement IDs it rests on. The existing requirement assessments keep their applicability, status, evidence, numeric and reference fields, with the existing guards. *Alternative:* replace per-requirement assessment with per-candidate assessment. Rejected: it would duplicate the assessment of a criterion shared by several candidates, and it would lose the per-requirement numeric checks. *Alternative:* derive the verdict purely in code from requirement statuses. Rejected: the existing statuses do not separate "the document does not show it" from "the reviewer could not check it" (`insufficient_evidence` covers both). The choice between `compatible`, `undetermined` and `unqualified` needs judgement, and the summary must match the verdict.

**D2. Five verdicts.** The user named `compliant`, `compatible`, not-compliant and `unqualified`. This design adds `undetermined`:
- `compliant`: rests on supported requirements.
- `compatible`: no conflict, but compliance is not shown. The summary names what is missing.
- `not_compliant`: rests on a contradicted requirement.
- `undetermined`: the candidate is usable, but the reviewer could not decide. Causes include incomplete source text, an unsupported quantity, a downgraded or rejected draft, or a requirement the extraction missed.
- `unqualified`: the candidate cannot judge this activity. It may state no requirement, concern another object or scope, or be informative only.

Without `undetermined`, a rejected draft would have to be called `compatible`, which claims there is no conflict, or `unqualified`, which marks a governing candidate for dropping. Both are wrong. Codes are snake_case; the user's "not-compliant" becomes `not_compliant`.

**D3. Requirements link to every candidate that expresses them.** `prompt-review-compliance-requirements-v2.md` returns `artifact_keys` (at least one) instead of `artifact_key`. Every key must be a candidate of the source being read. The server validates them as it validates the single key today. A candidate is *linked* to the requirements whose `artifact_keys` contain it. *Alternative:* let the assessment stage infer the links. Rejected: the requirements stage is the stage that reads the source in full.

**D4. The assessment stage proposes verdicts; code constrains them at two points.** `prompt-review-compliance-assessment-v2.md` receives the candidates (key, type, record, title and text) and returns `candidates: [{artifact_key, verdict, summary, requirement_ids}]` beside the unchanged `assessments`. "Required" below means `binding = required`.
- *Inside the call* (`CheckParsed`, corrected by the same model): each of the following draft checks names the candidate and field, and lists the allowed keys or values.
  - Every candidate appears exactly once, with no unknown keys.
  - The verdict is in the vocabulary, and the summary is not empty.
  - `requirement_ids` exist and are linked to that candidate.
  - A candidate with no linked requirement is `unqualified` or `undetermined`.
  - `not_compliant` needs a linked required requirement drafted `applicable` and `contradicted`.
  - Such a requirement also forces `not_compliant`.
  - `compliant` needs at least one linked required requirement drafted `applicable` and `supported`, and every other linked required requirement drafted `supported` or `not_applicable`.
  - `unqualified` is refused when a linked required requirement is drafted `applicable`.
- *After validation and verification* (deterministic): the `compliant` and `not_compliant` predicates are re-evaluated on the final requirement results. A verdict whose predicate no longer holds becomes `undetermined`, and the cause is recorded. For example, the source context was incomplete, the numeric check disagreed, or the verifier rejected the result.

Downgrades only move toward `undetermined`; code never invents `compliant` or `not_compliant`. Severity keeps its existing requirement-based rule, so a final `not_compliant` always comes with a medium-severity row.

**D5. Recommendations never decide compliance.** The existing guard sets non-required requirements to `not_assessed`. A candidate linked only to recommendations or unknown-binding requirements is therefore `compatible`, `undetermined` or `unqualified`. Its summary says it only recommends, and whether the document follows the recommendation. Trade-off: a followed recommendation shows as `compatible`, not `compliant`.

**D6. The verifier checks verdicts and summaries.** `prompt-review-compliance-verifier-v2.md` returns `candidate_checks: [{artifact_key, valid, reason}]` beside the unchanged `checks`. It checks four things:
- the verdict follows from the requirement assessments and evidence;
- the summary states the requirement, the conditions, the document's statements and the gaps without invented facts;
- `unqualified` is right, given the candidate text;
- `compatible` lists real gaps.

`CheckParsed` requires one check per candidate, each with a reason. A rejected verdict becomes `undetermined`. The draft summary is kept, and the verifier's reason is kept as the note. Wrong `unqualified` verdicts are the costly error, because those candidates may later be dropped.

**D7. Candidate text comes from the search index.** Before the assessment call, the reviewer loads the indexed text of every candidate with `searchengine.Load`, one call per activity. Each text is capped at 2,000 characters, and a cut text is marked `truncated`. The requirements stage still reads full sources; the text is for summarising and for recognising `unqualified` candidates. *Alternative:* carry search snippets in `related_artifact_fields`. Rejected: it duplicates the text in every stored row, and dependency artifacts arrive later.

**D8. Activities without extracted requirements are still judged.** When a requirements pass yields none, screening is skipped, and the assessment and verifier stages run with an empty requirement list. Every candidate then gets `unqualified` or `undetermined`, with a reason. The row keeps `compliance_grounding_candidates` and its existing sentence. The cost is two calls per such activity.

**D9. Storage and display reuse the related-artifacts list.**
- `ReviewFinding.RelatedArtifacts` gets one entry per candidate: artifact id, record, relationship = verdict, summary. A new optional `note` field holds the downgrade or verification reason; it is empty for metrics rows.
- `artifact_fields.candidate_verdicts` keeps each candidate's draft verdict, final verdict, summary, requirement IDs, verification and note. Metadata `version` becomes 2.
- Entries are ordered `not_compliant`, `compatible`, `compliant`, `undetermined`, `unqualified`, so the actionable verdicts come first.
- The row text starts with verdict counts and one line per candidate (`<verdict> [type id] record N: summary`). It keeps the existing `- [type id] … record N` candidate lines that the scorer reads; verdict lines start with the verdict, so the scorer's pattern ignores them.
- The panel maps the five codes to Paraglide labels with a colour class, and shows `note` under the summary with a translated label. Other relationship codes, such as metrics codes, still show as they are.

**D10. Correction retries only for the new output.** The assessment and verifier calls use `ExtractStructuredJSON` when the client supports it; the production reviewer client does. The contract has a schema and `CheckParsed`. A client without it falls back to `ExtractJSON` with one post-call check that aborts, which is today's convention. Existing post-call validation of requirement assessments is unchanged. *Alternative:* move all compliance checks into `CheckParsed`. Deferred: that is a robustness change outside these two issues.

## Reused modules

- **20261111-01 Artifact Search**: `searchengine.Load` reads candidate text (D7). Retrieval itself is unchanged.
- **20261114-03 Quantity Constraint Comparison**: unchanged. It still decides numeric support and contradiction, and its result feeds the final verdict predicates (D4).
- **Structured LLM output with correction retries** (`llmclients.StructuredOutputContract`, `ExtractStructuredJSON`, `CheckParsed` in `shared/go/api/llm`): reused, but not catalogued. It is registered with `create-reusable-module`.
- **Finding Details related-artifacts list** (`finding-details-panel.svelte`, `relatedArtifactsFromMetadata`): extended in place with verdict labels and `note`. It is part of the doc-review panel, not a separate component, so it is not registered.

No new package or component is created.

## Risks / Trade-offs

- [Assessment output grows by one entry per candidate, often 15–40] → Summaries are limited to four sentences, and `unqualified` reasons to one. Output is checked for completeness inside the retry loop.
- [The model marks governing candidates `unqualified`] → The verifier checks `unqualified` against the candidate text. Nothing is dropped in this change. Gold `governs` labels can measure the error later.
- [The 2,000-character cap hides context in long chunks] → The requirements stage reads full sources. The cap affects only the summaries of unlinked candidates, and cut texts are marked.
- [Draft-level checks reject answers that the model cannot fix] → The checks use only the model's own draft. Server-side downgrades happen after the call and never trigger retries.
- [Numeric checks or the verifier can flip a requirement after the verdict is drafted] → The verdict becomes `undetermined` with the cause recorded. It is never silently upgraded.

## Migration Plan

There is no schema change. New rows carry metadata `version` 2 and `related_artifacts`. Old rows render as before. Rollback is a code revert, because the v1 prompts stay in `prompts/`. Restart the API after deploying. Verifying the change on a real document needs a model-backed review run, which is paid, so the user decides when to run it.

## Open Questions

1. Confirm the vocabulary, in particular `undetermined` (D2) and the treatment of recommendations (D5).
2. When the verdicts are trusted, should `unqualified` candidates be hidden from the row, removed from candidate-precision scoring, or both?
