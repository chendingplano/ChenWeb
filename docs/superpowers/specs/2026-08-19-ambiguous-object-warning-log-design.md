# Ambiguous Object Warning Log

## Goal

Persist every unresolved object-reconciliation ambiguity warning in
`kb.doc_proc_logs`, including the complete candidate list shown by the warning.

## Design

The existing `reconcile_object` log entry is extended to cover the no-LLM,
unresolved ambiguity path. The log uses the same `objectReconcileLogSink` and
`DocProcLogger.LogReconcileObject` used for LLM outcomes, with
`extra_info.outcome = "unresolved"`. Candidate IDs and candidate display
strings are both retained so the database record is as useful as the warning.

The application warning remains unchanged. Log insertion remains best-effort;
failure is reported through the application logger and does not change
reconciliation behavior.

## Batch refresh (2026-10-17)

LLM decisions are computed concurrently against an initial candidate snapshot,
then applied sequentially. One artifact's accepted decision may merge nodes
after another artifact's decision failed or fell below the confidence threshold.
Those objects are not persisted until the batch returns, so the database merge
cannot update them yet.

After any accepted node edits or merges, refresh candidates for objects still
ambiguous before returning the batch. Accept only a unique best candidate with
score at least 0.95; leave genuine ties and weaker matches ambiguous without
creating nodes. Emit ambiguity warnings after this refresh. Keep the original
LLM attempt logs and append a resolved outcome when the refresh finds a match.
The final log records the refreshed candidates and matching confidence.

The regression test covers a later merge resolving an earlier low-confidence
decision or failed model call. Existing ambiguity tests continue to cover ties
that remain unresolved. No schema or public API changes are involved. The
historical implementation plan remains a record of the original work.

## Verification

Add a SQL-mock regression test for a tied candidate set with no LLM resolver.
The test will assert one `reconcile_object` insert and verify the serialized
extra information contains every candidate in the warning-equivalent list.
