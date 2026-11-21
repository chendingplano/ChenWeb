## 1. Prompts

- [x] 1.1 Write `prompts/prompt-review-compliance-requirements-v2.md`: v1, but returning `artifact_keys` (every candidate of this source that expresses the requirement, at least one) instead of `artifact_key`
- [x] 1.2 Write `prompts/prompt-review-compliance-assessment-v2.md`: v1 plus the candidate input, the `candidates` output (verdict, summary, requirement_ids), the five verdict definitions, the consistency rules, the summary contents and the recommendation rule, with a domain-neutral pair of examples
- [x] 1.3 Write `prompts/prompt-review-compliance-verifier-v2.md`: v1 plus `candidate_checks` (verdict follows from the assessments, summary faithful, `unqualified` right given the candidate text, `compatible` gaps real)
- [x] 1.4 Point `complianceAssessmentPromptRefs` at the three v2 files (screening stays v1)

## 2. Requirement links

- [x] 2.1 Replace `complianceRequirement.ArtifactKey` with `ArtifactKeys` and validate every key as a candidate of the source being read (`discoverRequirements`)
- [x] 2.2 Update the existing compliance tests to the new requirement shape

## 3. Candidate verdicts

- [x] 3.1 Load candidate texts with `searchengine.Load` once per activity, capped at 2,000 characters with a `truncated` flag, and pass `candidates` to the assessment call
- [x] 3.2 Add the verdict types, the linked-requirement map and the draft checks (one per candidate, vocabulary, summary, linked requirement_ids, unlinked → unqualified/undetermined, not_compliant/compliant/unqualified predicates) with correction messages that name candidate and field
- [x] 3.3 Run the assessment and verifier calls through `ExtractStructuredJSON` with a schema and `CheckParsed` when the client supports it; otherwise `ExtractJSON` plus the same check, aborting on error
- [x] 3.4 Verifier `candidate_checks`: require one per candidate with a reason; a rejection gives `undetermined` with the reason as note
- [x] 3.5 After validation and verification, re-evaluate the `compliant`/`not_compliant` predicates on final requirement results and downgrade to `undetermined` with the cause
- [x] 3.6 Run assessment and verification with an empty requirement list (screening skipped) when no requirement was extracted; keep `compliance_grounding_candidates`

## 4. Storage and row text

- [x] 4.1 Add optional `Note` to `RelatedArtifactAnalysis`; fill `RelatedArtifacts` in verdict order and `artifact_fields.candidate_verdicts`; metadata version 2
- [x] 4.2 Start the row text with verdict counts and `<verdict> [type id] record N: summary` lines; keep the scorer's candidate lines unchanged

## 5. Panel

- [x] 5.1 Read `note` in `relatedArtifactsFromMetadata`; in `finding-details-panel.svelte` map the five verdict codes to Paraglide labels with a colour class and show the note under the summary
- [x] 5.2 Add the labels to `web/messages/en.json` and `web/messages/zh-cn.json`; `bun run check` and `bun test` for the dialog helpers

## 6. Tests

- [x] 6.1 Fake-model test: a full staged run yields verdicts and summaries for every candidate, `related_artifacts` in verdict order, unchanged scorer candidate lines, and verdict lines the scorer pattern ignores
- [x] 6.2 Draft-check tests: each rule rejects with a message naming candidate and field; a correction test over `httptest` with `OpenAIJSONClient` shows the next request carries the correction and a corrected answer is accepted
- [x] 6.3 Downgrade tests: incomplete source context, numeric disagreement and verifier rejection each give `undetermined` with a note; recommendation-only candidates cannot be `compliant`/`not_compliant`
- [x] 6.4 Zero-requirement activity: no screening call, assessment and verifier still run, every candidate is `unqualified` or `undetermined`
- [x] 6.5 `go test ./server/api/doc-reviews ./server/api/compliance-eval`, `go vet`, `go build ./server/...`

## 7. Docs and catalogue

- [x] 7.1 Update devdoc 2026111201 (stages, verdicts, configuration prompt names, limits) and the `compliance-reviewer-phase2` design note pointing to this change
- [x] 7.2 Register the structured-output retry path (`shared/go/api/llm` `StructuredOutputContract`/`ExtractStructuredJSON`/`CheckParsed`) with the `create-reusable-module` skill and run the catalogue check
- [x] 7.3 Record a release item (`improvement`, ChenWeb)
