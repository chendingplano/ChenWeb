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

## Verification

Add a SQL-mock regression test for a tied candidate set with no LLM resolver.
The test will assert one `reconcile_object` insert and verify the serialized
extra information contains every candidate in the warning-equivalent list.
