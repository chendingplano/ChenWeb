## Why

The Question Generator is meant to produce questions of three difficulties: one-hop questions a
search can answer directly, and two-hop and multi-hop questions that do not name the metric, so a
searcher (or an LLM) must work out which metric to look for and compose its own query. Prompt v2
instead defined a hop as a reasoning step over the evidence. Of 188 stored questions, most two-
and multi-hop ones were compound lookups ("what are X, Y and Z?") that name the target and its
siblings; semantic search still found 98–100% of targets in the top 10 at every hop level. Such
questions cannot test query composition, and asking for several values gives one question several
correct answers while the benchmark records only one target.

## What Changes

- Prompt `prompt-question-generator-search-metrics-v3.md` (new default): hops measure how
  indirect the question is; two-hop and multi-hop questions must not name the metric; every
  question has exactly one target (no "X, Y and Z" lists).
- The model input carries the metric's English name (`metric_name_en`) next to `metric_name`.
- Server validation rejects a two-hop or multi-hop question that contains the target metric's
  name (Chinese or English, case-insensitive, names of two or more characters); the existing
  single correction round passes the error back to the model.
- Existing questions are kept; new runs produce the new kind.

## Capabilities

### New Capabilities
- `question-generator-hops`: what each hop type means for metric search questions and how the
  generator enforces it.

### Modified Capabilities
<!-- none (the generator has no spec yet) -->

## Impact

- `server/api/kbhandler/question_generator*.go`, `prompts/prompt-question-generator-search-metrics-v3.md`.
- Local config `mise.local.toml` `PROMPT_GENERATE_METRIC_QUESTIONS` must name v3 (it pinned v2).
