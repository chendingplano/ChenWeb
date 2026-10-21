# Summary benchmark v1

Build the benchmark before implementing or tuning summarizers. The user approved
proceeding with independent evidence annotations and fixed output budgets on
2026-10-21. This phase adds an offline evaluation asset, not a production processor.

## Choice

Use a standalone Python standard-library harness in `benchmark/summaries/`, beside
the existing document-processor fixtures. Database integration would introduce
deployment dependencies before the scoring contract is stable; a prose-only rubric
would not catch evidence loss automatically. No production routes or tables change.

## Dataset and contract

Versioned JSON contains synthetic diagnostic cases and a separate real-document
pilot stratum. Each case contains source lines, language, provenance, a non-whitespace
Unicode-character output budget, and independently authored information units.
Units carry source spans, importance, criticality, and a rationale. A unit may require
multiple spans together, such as a table header, row and exception. All offsets use
Python Unicode code points, start inclusive and end exclusive. Cases come from source
content, not candidate summaries. Review annotations independently before freezing.

Synthetic diagnostics expose expected behaviors, not production representativeness.
The real pilot uses the existing canonical text for record 416, with original line
IDs preserved; it is not a blind holdout and does not establish domain-wide quality.
Development and pilot scores must remain separate, grouped by language and tags.

Candidate JSON identifies dataset digest, system/configuration and one prediction
per case. Extractive segments include exact text and source spans. The scorer
verifies text against cited spans before giving evidence credit. Missing cases
are included in all denominators except inapplicable critical recall. A unit gets
credit only when every required span in an alternative survives intact in a single
verified output span. Source order and non-overlap are mandatory. All rendered text
belongs in segments and is counted against the budget; unsupported output fields
and empty/malformed/non-integer spans are rejected. Citation validity, nonempty
output, budget, source order, no duplicate spans and full critical recall are the
mechanical acceptance gates; raw coverage remains visible even on failure. These
gates do not establish semantic quality or production acceptance. Missing cases
score zero; duplicate/unknown case IDs and mismatched dataset digests are errors.
Over-budget cases fail the acceptance gate rather than benefiting from more text.

Generated summaries use a separate manual rubric and review form; automatic span
coverage is not a semantic score for paraphrases. Annotators judge coverage, factual
support, contextual integrity and readability against the source, blinded to system
identity. An LLM output is a comparator, never the reference answer.

## Measures and limits

Report weighted unit coverage, critical-unit recall, exact citation validity,
duplicate span rate, length, missing units and gate failures. Macro means include
missing cases. Performance fields are optional observations, never inferred zeros:
elapsed time, peak RSS in bytes, monetary cost, scope, machine and repetitions.
Production cost needs end-to-end measurement including translation, keywords,
embedding and parent summaries; extracting source text cannot translate it.

Ship lead-line and structural-order baselines only as harness diagnostics, plus
an explicitly labelled gold-evidence control. No Sumy/PyTextRank/BERT implementation
or tuning happens before the benchmark is frozen. A hash lock detects edits.

## Validation

Tests must expose invalid citations, changed limits, dropped exceptions, duplicate
content, missing cases, Chinese budgets, oversized outputs, and edited fixtures.
Every case must have a feasible all-gold extract within budget and distractor text
outside that extract. Review real-source spans against the canonical source and
save the review result. Freeze only after correcting review findings.

## Admin UI integration (2026-10-21)

The benchmark harness and frozen JSON remain offline evaluation assets. The
separate Gold Summary admin page browses imported benchmark output from
`testbed.summary_benchmark_runs` and `testbed.summaries`: Search selects a document
and run, Summary lists that run's rows, and Summary Detail shows the selected row's
stored fields. This UI does not change the scorer or make the benchmark dataset a
production summarizer.
