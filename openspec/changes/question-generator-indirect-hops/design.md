## Context

Questions are generated per sampled metric, three per metric (one per hop type), from the
metric's own source context (`question_generator_worker.go`, `question_generator_metrics.go`).
Validation already checks hop types, languages, citations and duplicates, and one correction round
sends `validation_error` back to the model.

## Decisions

**Hop = indirectness, not number of facts.** one-hop names the metric (direct lookup); two-hop
describes a situation, purpose or condition whose answer is the metric (one inference to know
what to search for); multi-hop needs a chain of at least two inferences (e.g. scenario → method or
table → metric). The answer is always the one target metric's value.

**Mechanical check for names only.** A verbatim, case-insensitive check of `metric_name` and
`metric_name_en` catches the common leak and costs nothing. Close paraphrases are forbidden by the
prompt but not checked; an LLM judge was rejected as extra cost and latency per question.
Subjects are not checked: they are often generic ("肥料", "fertilizer") and a situation may need
them. Names under two characters are skipped as too generic.

**English names are best effort.** They come from `kb.metrics.metric_name_en` in one extra query
(`readQuestionMetricEnglishNames`); if it fails, generation continues and the check uses the
Chinese name only.

**Old questions stay.** They keep their prompt name (`prompt_name` column), so benchmarks can
select by generation run or prompt version.

## Risks / Trade-offs

- [Paraphrased names pass the check] → prompt rule; review samples per prompt version.
- [Stricter questions fail validation more often] → the existing correction round; a run that
  still fails reports the validation error.
