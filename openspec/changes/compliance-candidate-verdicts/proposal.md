## Why

Compliance Review shows each activity with a list of retrieved candidates. It leaves the reader to work out how each candidate relates to the activity and whether the activity meets it. The assessment stages reason about extracted *requirements*, not candidates. Candidates without an extracted requirement get no explanation at all. Users reviewing an activity need one plain conclusion per candidate: what the candidate requires, whether and how it applies, what the document states, and what is missing or in conflict. They also need a compliance class that can be acted on, and a way to recognise candidates that cannot be used to judge compliance. Those candidates may later be dropped.

Example: the activity says "live fish is stored in a container". A candidate requires that live sea fish be stored in a sealed container with 60% air and 40% water, at 5–10 °C. The reviewer should say that this applies if the fish are sea fish. It should note that the document states none of the container, air/water or temperature conditions. Nothing conflicts, so the candidate is *compatible*, but compliance is not shown.

## What Changes

- Every candidate of an assessed activity gets one **verdict** and a **summary**. This covers retrieved candidates and dependency artifacts, and applies even when no requirement was extracted. Verdicts:
  - `compliant`: the document shows the activity meets what the candidate requires of it.
  - `compatible`: nothing in the document conflicts, but it does not state enough to show compliance. Examples are missing conditions, values, or the fact that decides whether the candidate applies.
  - `not_compliant`: the document states something that conflicts with what the candidate requires.
  - `undetermined`: the candidate can be used, but the reviewer could not reach a verdict. The source text may be incomplete, a quantity may be unsupported, or verification may have rejected the draft.
  - `unqualified`: the candidate cannot be used to judge this activity's compliance. It may state no requirement (a definition, background, an example or a result), it may concern a different object or scope, or it may be informative only. These candidates are flagged for possible dropping.
- The summary is written in the activity's language. It explains how the candidate applies to the activity: what it requires and under which conditions, what the document states, and what is missing or in conflict. It is not a copy of the candidate text.
- `compliant` and `not_compliant` must rest on verified requirement results with reviewed-document evidence: `supported` or `contradicted`, respectively. When the requirement layer is downgraded, the verdict becomes `undetermined`. The verifier also checks each verdict and summary. A rejected verdict becomes `undetermined` and keeps the verifier's reason.
- The requirements stage links each requirement to **every** candidate in its source that expresses it. A sibling metric or provision for the same criterion is then judged on that criterion, not marked `unqualified`.
- Activities whose candidates yield no extracted requirement still get verdicts (normally `unqualified`) with reasons. Whole-document screening is skipped for them.
- New per-candidate output is checked inside the model call's retry loop (`CheckParsed`), with correction messages that name the candidate and field. Existing stage checks are unchanged.
- The Finding Details panel shows each candidate's verdict as a translated, coloured label beside its summary. It reuses the existing `related_artifacts` list. The row text leads with verdict counts and per-candidate verdict lines, and keeps the candidate lines the scorer reads.
- New prompt versions: `prompt-review-compliance-requirements-v2.md`, `-assessment-v2.md` and `-verifier-v2.md`. The v1 files stay for history.

## Capabilities

### New Capabilities
- `compliance-candidate-verdicts`: per-candidate compliance verdicts and summaries for each assessed activity, their constraints, verification, and display.

### Modified Capabilities
<!-- compliance-review lives in the unarchived change compliance-reviewer-phase2, not in openspec/specs/. Its requirements stay valid; this change adds a capability on top of them. -->

## Impact

- **Code:** `server/api/doc-reviews/review-compliance-assessment.go` (assessment and verifier stages, verdict rules, row text), `review-compliance.go` (candidate text loading), and their tests. `web/src/lib/components/home3/finding-details-panel.svelte` and `doc-review-json-dialog.ts` render verdict labels. `web/messages/en.json` and `zh-cn.json` add five verdict labels.
- **Prompts:** three new versioned prompt files in `prompts/`. The screening prompt is unchanged.
- **Data:** no schema change. Verdicts go into finding metadata: `related_artifacts` (relationship = verdict, summary) and `artifact_fields.candidate_verdicts`, with metadata `version` 2. Existing rows stay readable.
- **Cost:** no extra model calls for activities with requirements. Activities whose candidates yield no requirement gain one assessment call and one verifier call.
- **Scoring:** `compliance-score` is unchanged. Candidate lines keep their format, and verdict lines do not match its candidate pattern.
- **IDs:** none allocated.
- **Reused modules:** 20261111-01 Artifact Search (`searchengine.Load` reads candidate text for the assessment input). 20261114-03 Quantity Constraint Comparison is unchanged and still decides numeric support or contradiction. The structured-output retry path (`llmclients.StructuredOutputContract` with `CheckParsed`, in `shared/go/api/llm`) is reused but not catalogued. It is registered with `create-reusable-module`. The catalogue search covered: structured output, retry correction, finding details related artifact badge, compliance quantity comparison. The panel's related-artifact list is reused in place; it is not a separate component.
