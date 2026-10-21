# SemOS summary benchmark

Build and freeze the source-based benchmark **before** implementing or tuning
Sumy, PyTextRank, BERT or a replacement summary pipeline.

Version 1.0.0 contains 16 cases and 43 information units: 13 synthetic diagnostics
and three excerpts from one existing real Chinese standard (record 416). This is
a reviewed pilot benchmark, not a representative corpus sample or blind holdout.
The real excerpts share a document and are correlated. Never split them between
training and holdout, and never present them as three independent documents.

## Run offline

Python 3.10+ is the only dependency. From this directory:

```sh
python3 -m unittest discover -s tests -v
python3 benchmark.py validate
python3 benchmark.py inputs --output /tmp/summary-inputs.json
python3 benchmark.py baseline --method lead --output /tmp/summary-lead.json
python3 benchmark.py score /tmp/summary-lead.json --output /tmp/summary-lead-score.json
python3 benchmark.py baseline --method structural --output /tmp/summary-structural.json
python3 benchmark.py score /tmp/summary-structural.json --output /tmp/summary-structural-score.json
python3 benchmark.py baseline --method gold-control --output /tmp/summary-gold.json
python3 benchmark.py score /tmp/summary-gold.json --output /tmp/summary-gold-score.json
```

Commands emit JSON; logs go to stderr. Exit 0 means the command completed, including
a scored submission that fails quality gates. Exit 1 means invalid input or an I/O
error; argparse uses exit 2 for invalid command usage. No database rows, production
artifacts, model calls or network requests are involved.

`lead` greedily includes complete source lines in order while they fit, skipping
oversized lines. `structural` gives the first content line after each heading
priority, fills remaining space in source order, then renders in source order.
Both operate on canonical lines, not NLP sentences. `gold-control` reads the
annotations and emits their evidence: it is a scorer calibration control and must
never be reported as a real summarizer. None has measured runtime/cost fields.

## Dataset

`datasets/1.0.0/benchmark.json` contains the input and gold. Each case has a local
character budget, language, diagnostic tags and source provenance. Gold units
identify important *information*, not an allegedly ideal summary paragraph.
Critical units have weight 2; other units weight 1. Weights are initial benchmark
judgments, not validated estimates of real-world risk.

Each unit has evidence alternatives: outer list is OR, inner list is AND. Every
span in one alternative must be fully preserved in a single verified candidate
span. Partial quotation gets no credit for that unit. Conditions, exceptions,
negation, table headers and footnotes belong to the same unit when they change its
meaning. All selected gold evidence fits the declared budget. The budgets are
case-specific feasibility limits, not a production compression-ratio recommendation.

Source spans are `{ "line": 4, "start": 0, "end": 39 }`: original source line ID,
zero-based Unicode-code-point offset, inclusive start and exclusive end. They are
not UTF-8 byte offsets, UTF-16 positions, or token indexes. Synthetic line IDs are
local; record-416 excerpts preserve canonical line IDs and exact text. The real
canonical source is snapshotted and hashed. Some rows contain literal HTML table
markup: offsets address that exact string; individual cell text can be quoted
using separate spans without reproducing the markup.

The real annotations describe selected summary information within each excerpt,
not a complete requirements inventory. For example, the fertilizer case includes
all five heavy-metal limits and their shared units, while the collection excerpt
targets a declared subset of operational requirements. The full canonical source
is supplied for audit; each candidate receives only its case's lines.

## Candidate contract: exact extracts

Run your adapter separately and submit JSON in this shape. Replace the digest and
span offsets with values for the frozen dataset; the example is schematic:

```json
{
  "schema_version": 1,
  "dataset_sha256": "value printed by validate",
  "system": {
    "name": "candidate-name",
    "revision": "package version or commit",
    "configuration": {"segmentation": "describe actual tokenizer and chunking"}
  },
  "predictions": [
    {
      "case_id": "en-rare-limit",
      "segments": [
        {
          "text": "Storage temperature shall not exceed 8°C.",
          "spans": [{"line": 4, "start": 0, "end": 41}]
        }
      ]
    }
  ]
}
```

The rendered summary is **only** `segments[*].text`, joined with newlines. Each
segment must equal the space-joined cited substrings after collapsing whitespace.
Every non-whitespace output character counts toward the budget. Arbitrary headings,
uncited prefixes, numeric corrections, translated text, ellipses and unsupported
output fields are not accepted in this track. Empty, zero-length, non-integer and
out-of-range spans are invalid. Source order must be preserved across all spans;
duplicate/overlapping spans fail the no-duplication gate. Input normalization in an
adapter must retain an offset map back to the canonical source.

The scorer reports raw weighted evidence coverage, critical-unit recall, exact
citation validity (valid segments / all emitted segments), duplicate span rate
(repeated cited code points / all cited code points), output size, missing units,
and individual gates. **Citation validity establishes quotation accuracy, not
semantic coherence or completeness.** A fragmented list of cells still needs review.

Case acceptance requires a submitted, nonempty, within-budget output with all
citations valid, source order preserved, no repeated spans, and all critical units
covered. Raw coverage remains visible when a gate fails. This acceptance is a
mechanical diagnostic result, not a production approval. In particular, a case
with no critical units still needs its overview coverage and readability inspected.

Missing cases receive zero coverage/citation validity and fail acceptance. Cases
with no critical units report null critical recall and are excluded only from the
critical-recall denominator. Duplicated/unknown case IDs or a different dataset
digest invalidate the submission. Overall and group means are macro averages over
cases; groups overlap and must not be summed. Prioritize separate `diagnostic`
and `real-pilot` results, then language/tag breakdowns. Do not hide a language
failure behind the overall average.

## Generated or paraphrased summaries

Use `REVIEW.md` and `review-template.json`. Exact span coverage deliberately gives
no semantic credit for paraphrases: **do not use that score to rank generated
summaries against extracts**. Both methods can instead be assessed by the same
blinded human semantic rubric, using the same source and length budget. References
are the information units plus the source, never another model's output. An LLM
review can provide secondary feedback, but cannot establish independent gold.

Translation, keyword extraction, category generation, embeddings and summary-tree
integration are outside this v1 quality score. They are existing SemOS outputs:
removing generation costs for one field does not remove the cost of those others.

## Scaling and cost protocol

The LexRank sentence matrix is built per submitted input: one document or one
chunk. Corpus growth increases the number of inputs; it does not by itself create
a corpus-wide matrix. Test both input scopes explicitly:

```sh
python3 benchmark.py stress --sentences 1000 --output /tmp/summary-1000-document.json
python3 benchmark.py stress --sentences 10000 --output /tmp/summary-10000-document.json
python3 benchmark.py stress --sentences 10000 --chunk-sentences 100 --output /tmp/summary-10000-chunks.json
```

These deterministic inputs contain one constructed sentence per list entry. The
adapter must record its actual tokenizer sentence counts; generated repetition is
for resource scaling only, not semantic-quality evidence. Use 100, 1000, 5000 and
10000 sentences as a sweep, with per-input time/memory limits. Report timeouts and
OOMs as failures, never remove them from denominators. Record the number of chunks,
total sentences processed including overlap, configured concurrency, chunk output
budgets, and any parent-summary calls. Test multiple documents in a batch separately
from concatenating documents; only concatenate when explicitly evaluating that mode.

For each quality-case prediction, optional `measurement` contains `wall_ms`,
`peak_rss_bytes`, `cost_usd` (each numeric or null), plus required `scope`, `machine`
and positive integer `repetitions`. The scorer preserves these observations without
inventing missing values or treating missing model bills as zero total cost.
Use `scope` to state whether preprocessing, model loading and summary-tree work are
included. Record exact dependency/model versions, OS, CPU/RAM, accelerator, process
concurrency, cold/warm cache and failures in the experiment report.

Measure peak RSS in fresh isolated processes; convert platform-specific native
units to bytes. Run one cold trial and at least five warm trials, retaining raw
per-trial observations and reporting median/p95 with the small-sample limitation.
Measure summed chunk CPU time and end-to-end wall time separately. API charges,
local CPU/GPU charges, repeated runs and fallbacks must be accounted for separately.

Compare costs at the same quality/output budget. For corpus projections use
`new_or_changed_inputs × cost_per_input + requested_fallbacks × fallback_cost`,
including amortized infrastructure and maintenance. Cache by source content hash,
summarizer revision, tokenizer/model and configuration; unchanged inputs should
not be summarized again. Benchmark extraction/OCR and translation separately so
their cost is not attributed to the ranking algorithm.

## Freeze and next evaluation

`freeze.json` pins the canonical JSON digest. Validation also checks each real
excerpt against the hashed snapshot. Change data by publishing a new version;
never silently update frozen gold to favor a candidate. `audit.md` records the
independent review and limits. `validation.json` contains harness-control results,
not Sumy/PyTextRank results.

After freeze, implement candidate adapters without exposing `units` to them.
The `inputs` command exports only IDs, language, budgets and source lines, with
the full frozen dataset digest for the eventual submission. Feed this export to
the adapter; keep gold annotations with the evaluator.
Report all predefined cases, errors and raw outputs. Use the diagnostic set for
development; the pilot is already visible and cannot become a blind test later.
Before a production decision, sample previously unseen complete documents across
languages, lengths, document families and table density, annotate with independent
human reviewers, and preregister acceptable quality/cost thresholds. Split by
document family to prevent overlapping chunks or editions leaking across splits.
