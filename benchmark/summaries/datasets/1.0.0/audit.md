# Dataset and harness audit — 2026-10-21

Scope: all 16 cases, 43 information units, the evaluator, candidate-input export,
README and semantic-review rubric. This is an independent agent audit by
`/root/benchmark_review`, separate from the author agent. It is not independent
human gold certification. No candidate summarizer outputs were used to derive gold.

## Findings and corrections

The design review requested exact evidence-conjunction semantics, complete output
accounting and explicit gates. The implementation requires intact spans, rejects
unsupported text/fields and malformed coordinates, and reports raw coverage apart
from the budget/citation/order/duplication/critical-coverage gates. Regression tests
include altered limits, missing exceptions, reordered fragments, extra text,
duplicate spans, whitespace-only output, missing cases and dataset edits.

The annotation review verified every real-source excerpt against the included
canonical snapshot and its SHA-256, and checked all 43 units' values, comparators,
units and offsets. It found that eight fertilizer table units did not include the
paragraph defining which treatment methods the tables applied to. Each now
requires line 128 in conjunction with the table evidence. Removing that context
now yields zero evidence coverage for the case; the complete gold control still
fits its budget and covers every unit. The regression is automated.

The reviewer rechecked the correction and candidate-input export and reported no
remaining freeze blockers. The export contains only ID, language, budget and
source lines for all cases; gold stays with the evaluator.

## Source and scope limits

The three real excerpts come from one document, record 416, DB33/T 2030—2018,
using the existing MinerU canonical extract. Original page images were not checked
in this benchmark; OCR/parser defects may be present. This benchmark makes no
claim about the standard's current legal applicability. The source snapshot hash
is `5acb3df6b320fdd4d3e293cc8170e4b68511ee274b2ee3f930268b1a4a590fcd`.

Annotations are a declared subset of important summary information, not a complete
requirements inventory. The collection case omits operational requirements; the
fertilizer case covers treatment applicability and quantitative indicators but
does not cover all management/facility provisions in the source excerpt. The
synthetic cases expose behaviors rather than production prevalence. Shared evidence
can make unit outcomes correlated; group/document-level reporting is required.

No human semantic review or model comparison has been performed. Exact quotation
coverage cannot establish readability, causal understanding or semantic fidelity of
paraphrases. These are covered by the separate unfilled human-review protocol.

## Knowledge impact

New knowledge is the frozen evaluation contract, source-backed information units,
source mapping and scaling protocol. `README.md`, `REVIEW.md`, the design/plan and
tests document them. No production API/schema or summary-generation behavior changed.
The original exploratory devdoc remains unchanged and should be read alongside
this benchmark; its recommendations are not validated candidate results. Production
adapter/integration design and algorithm comparisons are intentionally deferred
until after this benchmark-first phase.
