# Summary scoring page

## Purpose

Add an admin page at Development → System Admin → LLM → Summaries → Benchmark. It scores one document's level-0 `kb.ex_summaries` rows against a selected run from `testbed.summary_benchmark_runs` and `testbed.summaries`, using the `score-extract-summaries` skill's export, judgment, and scoring contract.

## User flow

An admin searches for a document, chooses one extraction algorithm, chooses a released gold run (newest released run is the default), and selects a scoring model and output language. The page starts a background run, shows progress, and retains history. A run stores an immutable input snapshot, model-produced judgments, the helper-computed score, report, selected algorithm, gold identity, and provenance. Missing gold, missing candidates, ambiguous algorithm selection, stale gold, and invalid judgments are explicit failures.

## Implementation

- Add an admin-only API and `kb.summary_score_runs` history table, following the metric scorer's run lifecycle.
- Run the installed `score-extract-summaries/scripts/score_io.py export --candidate extract --algorithm ...` and freeze that helper, its weights, and `generate-summaries-benchmark/scripts/benchmark_io.py` with its versioned rules files for each run.
- Ask the selected model to produce skill-contract judgments; set reviewer identity and input digest server-side, validate and calculate only through the frozen skill helper, and retry once with validation feedback.
- Correct model span offsets only when an exact quote has one unique match in its target text; retain raw judgment attempts beside the normalized evidence and log validation reasons.
- Add an English/Chinese navigation entry and a page that shows candidate algorithm, selected gold run, run history, score components, diagnostics, report, and evidence artifacts.
- Do not alter extraction or gold data.

## Validation

Keep scoring semantics in the skill helper. The application only gathers judgments, supplies reproducibility metadata, and presents results. Verify access control, run failure codes, selected algorithm and gold identity, source/rules freshness enforcement from the helper, bilingual UI, and serialized run evidence.
