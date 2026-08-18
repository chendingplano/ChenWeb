# Phase 3 Writer Readiness — Tasks 6.9 / 6.10

Produced by `server/cmd/metric-writer-readiness`, run against the full `miner`
corpus on 2026-08-18, after tasks 6.1–6.8 landed (the DR5 atomic transaction,
gated behind `LOSSLESS_SEMANTIC_WRITES_METRIC`, default off).

## Result

```
conformance suite 1.0.0: passed=true

completeness projection: complete=false
  current artifacts:            7074
  missing stage outcomes:        21222
  artifacts missing any stage:   7074
  artifacts with neither path:   7020
  summary drift:                 0
  orphan active findings:        0
  assertions missing value state: 0
  artifacts with missing value:  0
  BLOCKING: 21222 (artifact, stage) pair(s) have no active outcome envelope
  BLOCKING: 7020 artifact(s) have neither a semantic instance nor an active unresolved occurrence

writer activation: WOULD BE AUTHORIZED (conformance + compliance registry checks pass)

LOSSLESS_SEMANTIC_WRITES_METRIC was not read or modified by this command.
```

## What this means

Two distinct gates are in play, and this run separates them cleanly for the
first time:

1. **Code-level authorization** (`AuthorizeWriterActivation`: gate + adapter
   conformance + compliance registry) — **now passes**. Before this command
   ran, `kb.semantic_adapter_compliance` had zero rows; the conformance
   suite had never been executed and recorded against real infrastructure,
   so activation would have been refused on that basis alone regardless of
   the gate.
2. **Data-level completeness** (`CompletenessChecker`, the corpus itself) —
   **does not pass, honestly and expectedly**. 21,222 = 7,074 metrics × 3
   required stages, all missing, because `writeMetricLossless` has only ever
   run inside this session's integration tests against scratch databases —
   it has never executed against `miner`'s real 7,074 metrics. Completeness
   cannot pass before the writer has actually processed the corpus at least
   once; that is a real chicken-and-egg step, not a bug in the checker.

## Why the gate was not flipped

Flipping `LOSSLESS_SEMANTIC_WRITES_METRIC=true` and then reprocessing the
corpus would write thousands of new `kb.semantic_assertions`,
`kb.assertion_evidence`, `kb.semantic_processing_outcomes`/`findings`, and
(per the approved design) potentially several thousand new provisional
`kb.ontology_term_headers` rows — append-only, non-deletable history. That
is exactly the class of consequential, hard-to-reverse production action
this session has been treating as requiring explicit sign-off, most
recently in the "Note on backfills" decision. Task 6.9's own wording —
"only after ... a passing completeness projection" — is not satisfied yet,
so the correct, honest state is: tooling ready, gate still off.

## What flipping the gate would actually take

**Update 2026-08-18 (ADR Appendix B):** the corpus's existing 7,074 metrics are
deliberately *not* backfilled to close this gap -- see the ADR's Appendix B
decision. Completeness is instead verified against artifacts the live
doc-processing pipeline produces during ordinary operation. The steps below
are updated to match; step 2 is no longer a special reprocessing pass, it is
just "let normal document processing run."

1. Set `LOSSLESS_SEMANTIC_WRITES_METRIC=true` in the target environment.
2. Let ordinary document processing run (new or reprocessed documents flowing
   through extract_metrics → normalize_assertions → associate_semantics in
   the normal course of operation) -- no dedicated backfill pass across the
   pre-existing 58 metric-bearing input records.
3. Re-run `metric-writer-readiness` periodically and observe the completeness
   projection move toward `complete=true` as live artifacts accumulate.
4. Re-run `metric-support-cleanup` (report-only) to confirm the duplicate
   current-support invariant still holds after real writes (it is
   database-enforced by `uq_assertion_evidence_current_metric_support`
   regardless, but worth confirming for the corpus report).
5. Only once completeness passes against those live artifacts is this a
   "true" Phase 3 cutover in the ADR's sense. There is no fixed timeline.

This is a deliberately separate, larger decision from everything else in
this session's Phase 3 work, consistent with how the prior session held all
of Phase 3 back as its own decision point before this one started.
