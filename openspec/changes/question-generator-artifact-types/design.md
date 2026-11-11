## Context

The Question Generator (devdoc `2026102401-devdoc-question-generator.md`) samples metrics of a
document from `kb.metrics`, sends each with a ±10-line source context to `QUESTION_GEN_MODEL_NAME`,
and saves one one-hop, one two-hop and one multi-hop question per metric in
`testbed.generated_questions`. Purpose is fixed to `search_metrics`. Its code lives in
`server/api/kbhandler/question_generator*.go`:

- `question_generator.go`: request normalisation, purpose → prompt, structural validation
  (`collectGeneratedQuestions`), source formatting (`buildQuestionGenerationSource`).
- `question_generator_metrics.go`: metric sampling, context building
  (`buildMetricQuestionContexts`), per-metric validation (`collectMetricQuestions`), name-leak check.
- `question_generator_worker.go`: run loop over documents, document sampling, metric loading, the
  model call with one correction attempt (`generateMetricQuestionDocument`).

Hybrid search indexes twelve artifact types in `kb.search_artifacts`. Every populated type has
`source_line_spans` (dev database, 2026-11-11):

| type | rows | records | span lines p50 / p95 / max |
|---|---|---|---|
| entity | 42,888 | 61 | 1 / 3 / 227 |
| relation | 23,933 | 61 | 1 / 2 / 17 |
| inventory_item | 8,770 | 54 | 1 / 2 / 30 |
| topic | 3,299 | 26 | 3 / 10 / 43 |
| product | 2,648 | 11 | 1 / 5 / 31 |
| metric | 2,400 | 15 | 1 / 2 / 7 |
| scene_block | 2,370 | 26 | 2 / 8 / 37 |
| semantic_projection | 2,076 | 60 | 11 / 25 / 53 |
| summary | 789 | 24 | 16 / 131 / 1,465 |
| provision | 205 | 2 | 1 / 2 / 12 |
| chunk, knowledge | 0 | – | – |

Chunk rows are being added by the unarchived change `chunk-hybrid-search`, whose uncommitted files
share this working copy.

## Goals / Non-Goals

**Goals:**
- Generate source-grounded one/two/multi-hop questions for every search artifact type.
- One generic path for all non-metric types, driven by the search registry.
- Store each question's target as `(artifact_type, artifact_id)`, ready for a later benchmark change.
- Leave metric questions byte-for-byte the same at the model boundary.

**Non-Goals:**
- Running the new purposes in the Hybrid Search Benchmark or showing non-metric details on its
  Analyze page.
- Moving metrics onto the registry path.
- Per-type prompt files or per-type model settings.

## Decisions

### D1. One purpose per artifact type, in one Go table
A table `questionPurposes` maps purpose → artifact type (spec lists the twelve). Request validation,
prompt choice, document sampling, target loading and LLM call reason all read it. A unit test checks
that the table and `searchArtifactPartitionTypes` contain the same types.

`search_mixed` (Mix) is bound to every registry type; see D12.

*Alternative:* a single `search_artifacts` purpose plus an `artifact_type` request field. Rejected:
purpose is already the unit that the question browser filters, the benchmark selects and the prompt
is chosen by. Per-type purposes fit all of these without new filters or columns in those features.

### D2. Non-metric targets come from `kb.search_artifacts`
Targets are registry rows of the type for the document (`WHERE artifact_type = $1 AND
input_record_id = $2`, pruned to one partition), deduplicated by `artifact_id`. The registry has
the same shape for every type (labels, snippet, spans, payload), and its rows are exactly what
hybrid search returns, so a later benchmark can rank `(artifact_type, artifact_id)` directly.

*Alternative:* each type's source table (`kb.provisions`, `kb.summaries`, …). Rejected: eleven
loaders with different columns and span conventions, and targets that might not be in the index.

### D3. Metrics keep their own path
`search_metrics` keeps `kb.metrics`, prompt v3 and the `sets[].metric` input. Prompt v3 relies on
metric fields (value, unit, condition, formula) that the registry flattens into text, and the 30
saved metric questions and their benchmark runs stay comparable. Metric questions additionally
write `artifact_type = 'metric'` and `artifact_id = metric_id`.

### D4. Shared core instead of a parallel copy
Three pieces become shared between both paths, with the metric path calling them unchanged in
behaviour:

1. **Context lines.** The span → context-ID logic inside `buildMetricQuestionContexts` (span lines,
   ten actual lines before and after, disjoint-span gaps excluded, one shared source map and hash)
   moves into a function that takes each target's spans. Both paths call it.
2. **Per-target validation.** `collectMetricQuestions` becomes a generic collector over target
   sets. Each set supplies its slots (hop type and language), anchor lines, allowed context lines,
   leak-check names and a function that stamps an accepted question (metric fields or artifact
   fields). Metric error messages keep their current wording; the artifact path says "target".
3. **Model call loop.** The attempt / save-partial / one-correction loop of
   `generateMetricQuestionDocument` becomes one function taking the request and the collector. The
   request struct holds `sets` as either set type. For metric runs it marshals to the same JSON as
   today.

The existing context, partial, correction and name-leak tests guard the metric behaviour through the
refactor.

### D5. Eligibility includes a 20,000-character context cap
A registry row is eligible when its spans parse, both endpoints of every span exist among the
non-empty canonical lines, and its formatted context (the same `LLMLineContent` text sent to the
model) is at most 20,000 characters. The cap is checked during eligibility, not after sampling, so
the capacity count and the 409 response are exact. It mainly removes level-0 summaries that cover a
whole document. Thirty targets at the cap fit inside the existing 600,000-character input limit. A
larger request of a large-context type fails with the existing "too large after formatting" error,
as an oversized metric request does today. Splitting a document's sets across several model calls
is left out.

*Alternative:* truncate long contexts. Rejected: cited span lines could be cut away, and anchoring
then rejects otherwise valid questions.

### D6. Model input for an artifact set
```json
{"set_no": 1,
 "artifact": {"artifact_type": "provision", "artifact_id": "416_prv_40",
              "primary_label": "…", "secondary_label": "mandatory",
              "snippet": "…", "semantic_payload": {"prov_desc": "…", "provision_type": "mandatory"},
              "source_line_spans": ["116"], "context": "…", "context_line_ranges": ["106:126"]},
 "questions": [{"hop_type": "one_hop", "lang": "en"}, {"hop_type": "two_hop", "lang": "zh-cn"},
               {"hop_type": "multi_hop", "lang": "en"}]}
```
`snippet` is `snippet_basis` cut to 1,000 characters (for chunks it repeats the passage already in
the context). `semantic_payload` is pruned of empty values with the existing metadata pruner.
`search_document` is left out: it repeats weighted fields and adds nothing the model needs.
The type sits in each set, not at the top level, so single-type and Mix runs have the same shape.

### D7. Leak-check names
Two- and multi-hop questions may not contain the target's `primary_label`, and for
`inventory_item` also `secondary_label` (its English name). Other types' secondary labels are
codes such as `Level 0`, `mandatory`, `software` or a predicate, which would reject good questions.
For chunk, summary and topic the primary label is a file name or a sentence, so the check rarely
fires there; it is applied uniformly anyway.

### D8. Three new columns, metric columns kept
`artifact_type TEXT NOT NULL DEFAULT ''`, `artifact_id TEXT NOT NULL DEFAULT ''` and
`artifact_snapshot JSONB NOT NULL DEFAULT '{}'` go on `testbed.generated_questions` and on
`testbed.generated_questions_archive` (created with `LIKE`, so it does not follow the live table).
The migration backfills `('metric', metric_id)` in both tables where `metric_id <> ''`.
`generatedQuestionColumns` gains the three names, so listing, the run view and archiving carry
them.

*Alternative:* store non-metric targets in `metric_id` / `metric_snapshot`. Rejected: the names
would be wrong, and the benchmark's `metric_id <> ''` question filter would silently include
non-metric questions.

### D9. One prompt with per-type guidance
`prompt-question-generator-search-artifacts-v1.md` copies v3's hop, language, single-target,
citation, correction and output rules, written for a "target", and adds one paragraph per type
on what a question asks for and what the answer is:

- chunk: a fact or statement in the passage;
- summary: what the summarised section covers;
- topic: the information the topic describes;
- scene_block: the block's content (definition, example, procedure, …);
- provision: what is required, allowed or forbidden, and under which condition;
- product: which product is meant, or what the document says about it;
- inventory_item: which instrument, equipment or material is used for a task, or its specification;
- semantic_projection: which document covers a subject;
- knowledge: the knowledge statement;
- entity: what the entity is, or which entity matches a description;
- relation: the missing end of a subject–predicate–object fact.

*Alternative:* eleven prompt files. Rejected: the shared rules would be copied eleven times and
drift apart.

### D10. Sampling and dispatch by purpose
`questionGenerationCandidates` takes the artifact type. Metrics keep the existing `kb.metrics`
query. Other types use `SELECT DISTINCT input_record_id FROM kb.search_artifacts WHERE
artifact_type = $1` joined to `kb.inputs`, in random order. `generateQuestionsForRun` calls the
metric or artifact document generator by purpose. Log and error messages name the artifact type.
The call reason is `question_generation_<purpose>` with a new call location for the artifact path.
The capacity check in `StartQuestionGeneration` counts the purpose's eligible targets and returns
`available_targets`.

### D12. Mix: type first, then target
`search_mixed` loads the eligible registry targets of every type for the document (metric rows
included; they go through the artifact prompt, not the `kb.metrics` path) and keeps one pool per
type. For each slot it draws a type uniformly among types with remaining targets, then a target of
that type without replacement. Pools refill only when all are empty, matching metric sampling. Batch
runs sample documents with any registry row (`SELECT DISTINCT input_record_id FROM
kb.search_artifacts`, joined to `kb.inputs`). Capacity is the total of eligible targets.

*Alternative:* one pool of all targets. Rejected: entities (42,888 rows) and relations (23,933)
would make almost every question an entity or relation question.

Loading every type for Mix means reading up to twelve partitions per document. Entity and relation
rows dominate (about 700 per document on average). Eligibility is linear in rows and lines, so this
stays cheap.

### D11. Page
The dialog and filter list the thirteen purposes from a static list in
`question-generator-view.svelte`, with `qg_purpose_*` and `qg_type_*` messages. Count labels, hints,
the plan line and the capacity message use target wording with the type as a parameter. Cards and
list rows replace "Metric ID" with "Target", shown as `<type label> · <artifact_id>`.

## Reused modules

- **Catalogue:** nothing used. The searches and why 20261110-01 Compact Document Lines does not fit
  are in the proposal's Impact. Its compact line format would change the citation format that the
  generator's validation depends on.
- **Uncatalogued package code reused:** `buildQuestionGenerationSource`, `collectGeneratedQuestions`,
  `pruneQuestionMetadata`, `readQuestionDocumentMetadata` (question generator);
  `metricReviewSpansFromJSON` / `metricReviewLineSpanPattern` (metric review, already used by the
  generator); `searchArtifactPartitionTypes` (search registry). All are package-internal helpers of
  `kbhandler` with no use outside it, so they are not registered as modules.
- **New code:** the artifact target loader and the shared context/validation core serve only the
  Question Generator. They are not meant for reuse and are not registered.

## Risks / Trade-offs

- [Stale registry rows become targets, for example rows whose source artifact was deleted] → They
  are still returned by search, so they remain valid search targets. The span check drops rows whose
  lines no longer exist, and the snapshot records what was targeted.
- [Broad types (summary, semantic_projection) give vague answers] → Live check of each populated type
  before the questions are used as ground truth. Prompt v2 if needed.
- [Entities and relations have no embeddings (`EMBED_ENTITY_RELATION` off)] → Does not affect
  generation. The benchmark follow-up must report it, since semantic search cannot find them.
- [Refactoring the metric path] → The metric context, correction, partial-save and name-leak tests
  run unchanged, plus a test that the metric model input JSON is identical before and after.
- [Shared working copy with `chunk-hybrid-search`] → Do not edit its files (`search_registry.go`,
  `routes.go`, `content-panel.svelte`, …). Commit by explicit path and check every committed diff.
- [Migration number collision with the parallel change] → Pick the next free number when writing the
  file, and recheck before committing.

## Migration Plan

1. Goose migration `2026111100000N_generated_questions_artifact_target.sql`: add the three columns to
   both tables, then backfill metric rows. The dev server applies it on restart; see
   `reference_chenweb_migrations_apply` for running it without air.
2. Deploy backend, prompt and page together. The 409 field rename needs the new page.
3. Rollback: the down migration refuses while any live or archived question has `artifact_type`
   other than `metric` or `''` (dropping the columns would orphan those questions). Otherwise it drops
   the three columns from both tables.

## Open Questions

- None blocking. Benchmark support for the new purposes is the next change.
