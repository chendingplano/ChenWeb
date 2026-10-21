# Semantic review protocol

Use the same protocol for extracts and generated summaries. This complements the
automatic exact-evidence checks; it is not implemented by string overlap or an LLM
judge. No human judgments are claimed by the checked-in benchmark.

1. An experiment coordinator assigns anonymous output IDs and keeps the system
   mapping separate. Reviewers receive the case source, budget, gold units and
   candidate output, but not the algorithm/model identity or automatic scores.
2. Two human reviewers independently read the source and output. For each gold
   unit mark `covered`, `partial`, `missing` or `contradicted`, with quoted evidence
   and a rationale. Full coverage must preserve object, quantity/comparator/unit,
   applicability, negation, exceptions and uncertainty. Partial coverage gets no
   full-unit credit. Mark additional useful facts separately; these do not silently
   change the frozen gold or denominator.
3. Enumerate each unsupported or contradicted output claim with its location.
   A plausible claim lacking support in this source is unsupported. Unchanged
   source words can still mislead if reordered or stripped of their conditions.
4. Score contextual integrity and readability independently on the scales below.
   Review table fragments in their rendered output, not by assuming the reader
   can reconstruct missing headings. Check source citations if supplied.
5. Resolve disagreements through documented adjudication. Preserve both original
   forms, disagreement count and adjudicator rationale. Report raw agreement before
   adjudication (unit statuses and ordinal scores); a single consensus form does
   not demonstrate independent agreement. A candidate author must not adjudicate
   their own output. Use a third reviewer if needed.

## Scores

Semantic weighted coverage = sum of weights of fully covered units / all unit
weights. Critical recall = fully covered critical units / all critical units;
null when there are none. Contradicted units score zero and remain explicit errors.
These measures are comparable across generation and extraction only when scored
with the same human protocol. They are separate from automatic citation coverage.

Contextual integrity:

- 0: A critical scope, condition, negation, number, unit or exception is changed or
  detached so the output communicates a materially different claim.
- 1: Context is ambiguous or fragmented, requiring the source to interpret it.
- 2: Important claims retain their applicable context and uncertainty.

Readability:

- 0: Unusable fragments or incoherent output.
- 1: Meaning can be recovered with substantial effort.
- 2: Understandable with minor repetition or awkward joins.
- 3: Clear, concise and coherent as a standalone overview of the selected material.

Report unsupported/contradicted claim counts separately. Do not combine readability
and faithfulness into one score that allows pleasant wording to hide a critical
error. An LLM-generated summary is a candidate, not a gold reference. OCR ambiguity
must remain explicit unless a separately verified source correction is supplied.

## Reading the results

Report diagnostic and real-pilot strata separately, with language/tag breakdowns,
per-case budgets and the full failure list. With this small purposive pilot, avoid
statistical claims about production prevalence or generalization. If estimating
uncertainty on an expanded real corpus, resample independent document groups rather
than treating chunks from one document as independent observations.

Candidate evaluation and human scoring begin only after benchmark freeze. The
checked-in review template is blank intentionally.
