## 1. Schema

- [x] 1.1 Goose migration (next free `20261111…` number): add `artifact_type`, `artifact_id`, `artifact_snapshot` to `testbed.generated_questions` and `testbed.generated_questions_archive`; backfill `('metric', metric_id)` where `metric_id <> ''`; down migration refuses while non-metric targets exist, else drops the columns. Verify up/down/up against the dev database.
- [x] 1.2 Add the three columns to `generatedQuestionColumns`, `scanGeneratedQuestion`, `generatedQuestion` and the insert in `saveGeneratedQuestionBatch`; test that archiving moves them.

## 2. Purposes and shared core (metric behaviour unchanged)

- [x] 2.1 Test first: capture the metric model input JSON for a fixed sample (fake LLM client) so the refactor can be checked byte-for-byte.
- [x] 2.2 `questionPurposes` table (purpose → artifact type → prompt env/default) replacing the switch in `questionPurposePrompt`; test that its types equal `searchArtifactPartitionTypes` and that unknown purposes are rejected.
- [x] 2.3 Extract the span → context-ID logic from `buildMetricQuestionContexts` into a shared function; metric context tests pass unchanged.
- [x] 2.4 Make `collectMetricQuestions` a generic target-set collector (slots, anchor lines, allowed lines, leak names, stamp function); metric wording of errors kept; metric questions also set `artifact_type`/`artifact_id`. Existing correction, partial and name-leak tests pass.
- [x] 2.5 Extract the attempt / save-partial / one-correction loop of `generateMetricQuestionDocument` into a shared function; task 2.1's test passes.

## 3. Artifact path

- [x] 3.1 Load registry targets for a record and type: distinct `artifact_id`, span parse, endpoints present, formatted context ≤ 20,000 characters; tests for stale span, oversized context, duplicate IDs and empty spans.
- [x] 3.2 Build artifact sets (D6 input shape: labels, 1,000-character snippet, pruned payload, spans, context, ranges, three language-assigned slots) and the artifact collector (context-only citations, span anchoring, leak check on primary label plus inventory-item secondary label, stamp type/ID/snapshot); tests per rule.
- [x] 3.3 `generateArtifactQuestionDocument` on the shared loop, with call reason `question_generation_<purpose>` and a new call location; test with a fake client including one correction.
- [x] 3.4 Worker dispatch by purpose; `questionGenerationCandidates(ctx, db, artifactType)` with the registry query for non-metric types; errors name the type. Update existing worker test fakes.
- [x] 3.5 Capacity check counts the purpose's eligible targets; HTTP 409 returns `available_targets`; update capacity tests and add a non-metric case.
- [x] 3.6 Mix (`search_mixed`): load all types' eligible targets, type-first sampling with per-type pools and refill when all are empty, batch document query over all types; tests for balanced type draw, refill and stored per-question type.

## 4. Prompt

- [x] 4.1 Write `prompts/prompt-question-generator-search-artifacts-v1.md` (v3 rules written for a target, plus per-type guidance from design D9); `PROMPT_GENERATE_ARTIFACT_QUESTIONS` override; add it to `mise.local.toml` only if the local setup pins prompt names.

## 5. Page

- [x] 5.1 `questionGeneratorService.ts`: purpose type widened to the thirteen purposes; `available_targets`; question `artifact_type`, `artifact_id`, `artifact_snapshot`.
- [x] 5.2 `question-generator-view.svelte`: thirteen purposes (Mix included) in the dialog and filter, target wording for counts, hints, plan and capacity message, Target field (`<type> · <id>`) in list and cards.
- [x] 5.3 Add `qg_purpose_*`, `qg_type_*` and the new target messages to `web/messages/en.json` and `zh-cn.json` with real Chinese; `bun run check` passes with no baseline increase.

## 6. Verification

- [x] 6.1 `go vet` and `go test ./server/api/kbhandler/...` (question generator, capacity, archive, worker tests).
- [x] 6.2 Live generation (record with rows of the type, 3–6 questions each) for metric (regression), Mix, and every populated non-metric type: summary, topic, scene_block, provision, product, inventory_item, semantic_projection, entity, relation. Record per type: accepted/requested, correction needed, and a read of question quality, especially two/multi-hop indirectness.
- [x] 6.3 Check `llm_usage_event` call reasons and run IDs for one artifact run.
- [x] 6.4 Page check in English and Chinese: generate one non-metric run and confirm the Target field and filters (browser login permitting; otherwise state that it was not browser-checked). — Not browser-checked (no login available to the agent); `bun run check` passes and the dev server compiles all four changed modules.

## 7. Docs and commit

- [x] 7.1 Update devdoc `2026102401-devdoc-question-generator.md`: purposes, registry targets, eligibility and cap, artifact prompt, new columns, 409 field rename, benchmark still metric-only.
- [x] 7.2 Answer the workspace "What knowledge changed?" checklist in the final report; no catalogue registration (design: Reused modules).
- [x] 7.3 Commit ChenWeb by explicit paths only (the `chunk-hybrid-search` files in the working copy are not ours), diff every committed file, then the KnowledgeStore devdoc separately; `jj log` shows only the expected commits.
