## Why

Hybrid search covers twelve artifact types (`kb.search_artifacts` partitions: chunk, summary,
topic, scene_block, metric, provision, product, inventory_item, semantic_projection, knowledge,
entity, relation), but the Question Generator (System Admin → Benchmark → Question Generator) can
only write questions whose answer is a metric (purpose `search_metrics`). Search over every other
type therefore has no generated, source-grounded test questions.

## What Changes

- Add one purpose per non-metric artifact type: `search_chunks`, `search_summaries`,
  `search_topics`, `search_scene_blocks`, `search_provisions`, `search_products`,
  `search_inventory_items`, `search_semantic_projections`, `search_knowledge`, `search_entities`
  and `search_relations`. `search_metrics` keeps its current behaviour, prompt and model input.
- Add purpose `search_mixed` (shown as Mix): each target of a run picks a random artifact type
  among those the document has eligible targets for, so one run mixes questions of different
  types. Each question still records its own target type.
- New purposes draw their targets from the search registry (`kb.search_artifacts` rows of that
  type for the document), so every target is exactly what hybrid search can return. A target is
  eligible when its source line spans exist in the document's canonical lines and its source
  context fits the per-target size limit.
- New purposes share one prompt file, `prompt-question-generator-search-artifacts-v1.md`. It gives
  type-specific guidance on what a question about each type asks for. The same one-, two- and
  multi-hop triplets, language assignment, citation rules, one-correction-attempt flow and
  name-leak check (now against the target's label) apply.
- `testbed.generated_questions` (and its archive) gain `artifact_type`, `artifact_id` and
  `artifact_snapshot`. Existing metric questions are backfilled (`metric`, `metric_id`). Metric
  questions continue to fill `metric_id` and `metric_snapshot`.
- Random batches sample documents that have indexed rows of the selected type. Single-document
  requests check capacity against that type's eligible targets.
- **BREAKING (admin API only):** the HTTP 409 capacity response of
  `POST /kb/question-generator/runs` returns `available_targets` instead of `available_metrics`.
  The page is the only client and changes in the same change.
- The page offers all thirteen purposes, names counts and messages by target rather than metric,
  and shows each question's target as type and artifact ID.

Non-goals: the Hybrid Search Benchmark still runs only `search_metrics` questions; running the new
purposes there (and a non-metric detail panel on its Analyze page) is a follow-up change. Metric
question generation is not moved onto the registry.

## Capabilities

### New Capabilities
- `question-generator-artifact-types`: generating search benchmark questions whose targets are
  search registry artifacts of any indexed type. Covers purposes, target eligibility, model input,
  validation, storage and page display.

### Modified Capabilities
<!-- None: the question generator's existing requirements live in the unarchived change
     question-generator-indirect-hops, not in openspec/specs/. -->

## Impact

- **Backend** (`server/api/kbhandler`): `question_generator.go` (purpose table, request
  normalisation), `question_generator_worker.go` (dispatch by purpose, document sampling, capacity),
  new `question_generator_artifacts.go` (registry target loading, contexts, validation),
  `question_generator_metrics.go` (context and validation core shared with the new path),
  `question_generator_store.go` / `_archive.go` (new columns), `question_generator_handler.go` (409
  field). Reads `searchArtifactPartitionTypes` from `search_registry.go` without changing it.
- **Database**: one goose migration adding three columns to `testbed.generated_questions` and
  `testbed.generated_questions_archive`, with a backfill for metric questions.
- **Prompt**: new `prompts/prompt-question-generator-search-artifacts-v1.md`. Optional env override
  `PROMPT_GENERATE_ARTIFACT_QUESTIONS`, like the existing `PROMPT_GENERATE_METRIC_QUESTIONS`. Model
  stays `QUESTION_GEN_MODEL_NAME`.
- **Frontend**: `question-generator-view.svelte`, `questionGeneratorService.ts`, new `qg_*` keys in
  `web/messages/en.json` and `zh-cn.json`.
- **Docs**: devdoc `2026102401-devdoc-question-generator.md`.
- **Reusable modules**: searched the catalogue (`search artifact registry`, `source line context`,
  `llm json extraction`, `question generator benchmark`, `document lines load`, capability
  `llm-input`). The only candidate, 20261110-01 Compact Document Lines, renders lines without the
  per-line JSON that the generator's citation validation relies on, so it is not used. The change
  reuses package code that is not catalogued: the generator's own source builder and structural
  validator, the metric-review span parser, and the registry's partition type list.
- **IDs**: no new IDs. Runs keep their own `BIGSERIAL` primary key, and LLM run IDs keep coming from
  the existing `taskLLMExecution` allocation.
- **Data**: no chunk or knowledge registry rows exist in the dev database today (chunk indexing is
  being built in `chunk-hybrid-search` and lands soon). Those purposes work once rows exist and
  report no eligible documents until then; Mix picks them up automatically.
